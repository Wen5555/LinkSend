package identity

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/json"
	"errors"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestPersistentIdentityAndTrust(t *testing.T) {
	dir := t.TempDir()
	a, err := LoadOrCreate(dir)
	if err != nil {
		t.Fatal(err)
	}
	b, err := LoadOrCreate(dir)
	if err != nil || a.ID() != b.ID() {
		t.Fatalf("identity changed %v", err)
	}
	message := []byte("authenticated bytes")
	if !ed25519.Verify(a.PublicKey(), message, b.Sign(message)) {
		t.Fatal("signature")
	}
	p := TrustedPeer{ID: a.ID(), PublicKey: a.PublicKey(), Name: "A"}
	if err := TrustPeer(dir, p, "wrong"); err == nil {
		t.Fatal("unconfirmed pin accepted")
	}
	if err := TrustPeer(dir, p, a.ID()); err != nil {
		t.Fatal(err)
	}
	if err := SetAutoAccept(dir, a.ID(), true); err != nil {
		t.Fatal(err)
	}
	peers, err := LoadTrust(dir)
	if err != nil || len(peers) != 1 {
		t.Fatalf("trust %v", err)
	}
	if !peers[0].AutoAccept {
		t.Fatal("auto-accept preference was not persisted")
	}
	if err = os.WriteFile(filepath.Join(dir, "identity.key"), []byte("corrupt"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = LoadOrCreate(dir); err == nil {
		t.Fatal("corrupt key replaced")
	}
}

func TestRememberedLANAddressUpdatesWithoutReplacingTrust(t *testing.T) {
	dir := t.TempDir()
	peer, err := Generate()
	if err != nil {
		t.Fatal(err)
	}
	pinned := TrustedPeer{ID: peer.ID(), PublicKey: peer.PublicKey(), Name: "peer"}
	if err = TrustPairedPeer(dir, pinned); err != nil {
		t.Fatal(err)
	}
	if err = SetAutoAccept(dir, peer.ID(), true); err != nil {
		t.Fatal(err)
	}
	pinned.LastLANAddress = "10.234.171.192"
	if err = TrustPairedPeer(dir, pinned); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadTrust(dir)
	if err != nil || len(loaded) != 1 {
		t.Fatalf("load trust: %#v %v", loaded, err)
	}
	if loaded[0].LastLANAddress != pinned.LastLANAddress || !loaded[0].AutoAccept {
		t.Fatalf("remembered peer=%+v", loaded[0])
	}
	pinned.LastLANAddress = "203.0.113.7:443"
	if err = TrustPairedPeer(dir, pinned); err == nil {
		t.Fatal("accepted a non-address LAN hint")
	}
}

func TestReplaceFileFallsBackAndRestoresSafely(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "trust.json")
	temp := filepath.Join(dir, "trust-new.tmp")
	if err := os.WriteFile(target, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(temp, []byte("new"), 0600); err != nil {
		t.Fatal(err)
	}
	first := true
	rename := func(old, new string) error {
		if first && old == temp && new == target {
			first = false
			return syscall.EXDEV
		}
		return os.Rename(old, new)
	}
	noPlatformReplace := func(string, string) (bool, error) { return false, nil }
	noInPlace := func(_, _ string, cause error) error { return cause }
	if err := replaceFileWith(temp, target, rename, noPlatformReplace, noInPlace); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(target); err != nil || string(got) != "new" {
		t.Fatalf("fallback replacement = %q, %v", got, err)
	}

	broken := filepath.Join(dir, "trust-broken.tmp")
	if err := os.WriteFile(broken, []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	installAttempts := 0
	failingRename := func(old, new string) error {
		if old == broken && new == target {
			installAttempts++
			return syscall.EXDEV
		}
		return os.Rename(old, new)
	}
	if err := replaceFileWith(broken, target, failingRename, noPlatformReplace, noInPlace); err == nil || installAttempts != 2 {
		t.Fatalf("expected failed install and rollback, attempts=%d err=%v", installAttempts, err)
	}
	if got, err := os.ReadFile(target); err != nil || string(got) != "new" {
		t.Fatalf("rollback did not restore previous trust file = %q, %v", got, err)
	}
}

func TestInPlaceTrustReplacementAndCrashRecovery(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "trust.json")
	source := filepath.Join(dir, "trust-new.tmp")
	oldIdentity, _ := Generate()
	newIdentity, _ := Generate()
	oldData, _ := json.Marshal(TrustFile{Peers: []TrustedPeer{{ID: oldIdentity.ID(), PublicKey: oldIdentity.PublicKey(), Name: "old"}}})
	newData, _ := json.Marshal(TrustFile{Peers: []TrustedPeer{{ID: newIdentity.ID(), PublicKey: newIdentity.PublicKey(), Name: "new"}}})
	if err := os.WriteFile(target, oldData, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, newData, 0600); err != nil {
		t.Fatal(err)
	}
	if err := replaceFileInPlace(source, target, syscall.EXDEV); err != nil {
		t.Fatal(err)
	}
	peers, err := LoadTrust(dir)
	if err != nil || len(peers) != 1 || peers[0].ID != newIdentity.ID() {
		t.Fatalf("in-place replacement was not readable: %+v %v", peers, err)
	}

	if err = os.WriteFile(target, []byte("truncated"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(target+".previous", oldData, 0600); err != nil {
		t.Fatal(err)
	}
	peers, err = LoadTrust(dir)
	if err != nil || len(peers) != 1 || peers[0].ID != oldIdentity.ID() {
		t.Fatalf("previous trust file was not recovered: %+v %v", peers, err)
	}
	if _, err = os.Stat(target + ".previous"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("recovery journal was not removed: %v", err)
	}
}

func TestMutualTLS(t *testing.T) {
	a, _ := Generate()
	b, _ := Generate()
	bad, _ := Generate()
	for _, tt := range []struct {
		name                 string
		clientPin, serverPin ed25519.PublicKey
		pass                 bool
	}{{"trusted", b.PublicKey(), a.PublicKey(), true}, {"server-key-replaced", bad.PublicKey(), a.PublicKey(), false}, {"unauthorized-client", b.PublicKey(), bad.PublicKey(), false}} {
		t.Run(tt.name, func(t *testing.T) {
			ac, err := a.TLSConfig(tt.clientPin, false)
			if err != nil {
				t.Fatal(err)
			}
			bc, err := b.TLSConfig(tt.serverPin, true)
			if err != nil {
				t.Fatal(err)
			}
			x, y := net.Pipe()
			defer x.Close()
			defer y.Close()
			client, server := tls.Client(x, ac), tls.Server(y, bc)
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			ch := make(chan error, 1)
			go func() { ch <- server.HandshakeContext(ctx) }()
			ce := client.HandshakeContext(ctx)
			se := <-ch
			if tt.pass && (ce != nil || se != nil) {
				t.Fatalf("client %v server %v", ce, se)
			}
			if !tt.pass && ce == nil && se == nil {
				t.Fatal("bad peer accepted")
			}
		})
	}
}

func TestCertificatePolicy(t *testing.T) {
	i, _ := Generate()
	now := time.Now()
	cert, _ := i.certificate(now)
	if err := VerifyPeer([]*x509.Certificate{cert.Leaf}, i.PublicKey(), false, now); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name string
		edit func(*x509.Certificate)
	}{
		{"expired", func(c *x509.Certificate) { c.NotAfter = now.Add(-time.Second) }},
		{"future", func(c *x509.Certificate) { c.NotBefore = now.Add(time.Hour) }},
		{"long-lived", func(c *x509.Certificate) { c.NotAfter = now.Add(365 * 24 * time.Hour) }},
		{"ca", func(c *x509.Certificate) { c.IsCA = true }},
		{"wrong-id", func(c *x509.Certificate) { c.Subject.CommonName = "other" }},
		{"no-uri", func(c *x509.Certificate) { c.URIs = nil }},
		{"wrong-usage", func(c *x509.Certificate) { c.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageCodeSigning} }},
		{"unknown-critical", func(c *x509.Certificate) {
			c.ExtraExtensions = append(c.ExtraExtensions, pkix.Extension{
				Id: asn1.ObjectIdentifier{1, 2, 3, 4}, Critical: true, Value: []byte{5, 0},
			})
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c := *cert.Leaf
			c.SerialNumber = big.NewInt(5)
			tt.edit(&c)
			// Force x509.CreateCertificate to encode the modified Subject fields;
			// cert.Leaf carries the original DER subject in RawSubject.
			c.RawSubject = nil
			der, err := x509.CreateCertificate(rand.Reader, &c, &c, i.PublicKey(), i.private)
			if err != nil {
				t.Fatal(err)
			}
			parsed, err := x509.ParseCertificate(der)
			if err != nil {
				t.Fatal(err)
			}
			if err = VerifyPeer([]*x509.Certificate{parsed}, i.PublicKey(), false, now); err == nil {
				t.Fatal("invalid policy accepted")
			}
		})
	}
	if err := VerifyPeer(nil, i.PublicKey(), false, now); err == nil {
		t.Fatal("missing certificate accepted")
	}
	if _, err := i.TLSConfig(nil, false); err == nil {
		t.Fatal("missing pin accepted")
	} else if !errors.Is(err, ErrAuthentication) {
		t.Fatalf("missing identity sentinel: %v", err)
	}
}
