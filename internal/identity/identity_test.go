package identity

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"math/big"
	"net"
	"os"
	"path/filepath"
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
	peers, err := LoadTrust(dir)
	if err != nil || len(peers) != 1 {
		t.Fatalf("trust %v", err)
	}
	if err = os.WriteFile(filepath.Join(dir, "identity.key"), []byte("corrupt"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = LoadOrCreate(dir); err == nil {
		t.Fatal("corrupt key replaced")
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
	}
}
