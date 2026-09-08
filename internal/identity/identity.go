// Package identity owns device keys and explicit, out-of-band peer trust.
package identity

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/url"
	"os"
	"path/filepath"
	"time"
)

const ALPN = "linksend/1"
const maxTrustFileBytes = 1 << 20

var ErrAuthentication = errors.New("identity authentication failed")

func authenticationError(detail string) error {
	return fmt.Errorf("%w: %s", ErrAuthentication, detail)
}

type Identity struct{ private ed25519.PrivateKey }

func Generate() (*Identity, error) {
	_, k, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	return &Identity{private: k}, nil
}
func (i *Identity) PublicKey() ed25519.PublicKey {
	return bytes.Clone(i.private.Public().(ed25519.PublicKey))
}
func (i *Identity) ID() string                 { return DeviceID(i.PublicKey()) }
func (i *Identity) Sign(message []byte) []byte { return ed25519.Sign(i.private, message) }
func DeviceID(public ed25519.PublicKey) string {
	sum := sha256.Sum256(public)
	return hex.EncodeToString(sum[:])
}

// LoadOrCreate never overwrites an existing identity. Windows protects the seed
// with user-scoped DPAPI; other systems currently use a 0600 file in a 0700 dir.
func LoadOrCreate(dir string) (*Identity, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	p := filepath.Join(dir, "identity.key")
	if info, err := os.Lstat(p); err == nil {
		if !info.Mode().IsRegular() {
			return nil, errors.New("identity key is not a regular file")
		}
		if err := checkPrivatePermissions(info); err != nil {
			return nil, err
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		seed, err := unprotect(data)
		if err != nil {
			return nil, fmt.Errorf("unlock identity: %w", err)
		}
		if len(seed) != ed25519.SeedSize {
			return nil, errors.New("invalid identity seed length")
		}
		return &Identity{private: ed25519.NewKeyFromSeed(seed)}, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	i, err := Generate()
	if err != nil {
		return nil, err
	}
	data, err := protect(i.private.Seed())
	if err != nil {
		return nil, err
	}
	f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return nil, err
	}
	created := true
	defer func() {
		if created {
			_ = os.Remove(p)
		}
	}()
	if _, err = f.Write(data); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return nil, err
	}
	if closeErr != nil {
		return nil, closeErr
	}
	created = false
	return i, nil
}

func (i *Identity) certificate(now time.Time) (tls.Certificate, error) {
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return tls.Certificate{}, err
	}
	uri, _ := url.Parse("urn:linksend:device:" + i.ID())
	c := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: i.ID()}, NotBefore: now.Add(-5 * time.Minute), NotAfter: now.Add(24 * time.Hour), BasicConstraintsValid: true, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth}, URIs: []*url.URL{uri}, SignatureAlgorithm: x509.PureEd25519}
	der, err := x509.CreateCertificate(rand.Reader, c, c, i.PublicKey(), i.private)
	if err != nil {
		return tls.Certificate{}, err
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		return tls.Certificate{}, err
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: i.private, Leaf: leaf}, nil
}

// TLSConfig pins exactly one previously verified device key. The client disables
// CA/DNS verification ONLY because device certificates are self-signed; the
// mandatory verifier below replaces it with identity and certificate validation.
func (i *Identity) TLSConfig(expected ed25519.PublicKey, server bool) (*tls.Config, error) {
	if len(expected) != ed25519.PublicKeySize {
		return nil, authenticationError("expected trusted Ed25519 public key required")
	}
	cert, err := i.certificate(time.Now())
	if err != nil {
		return nil, err
	}
	key := bytes.Clone(expected)
	cfg := &tls.Config{MinVersion: tls.VersionTLS13, MaxVersion: tls.VersionTLS13, Certificates: []tls.Certificate{cert}, NextProtos: []string{ALPN}, SessionTicketsDisabled: true}
	if server {
		cfg.ClientAuth = tls.RequireAnyClientCert
	} else {
		cfg.InsecureSkipVerify = true
	}
	cfg.VerifyConnection = func(cs tls.ConnectionState) error {
		if cs.Version != tls.VersionTLS13 || cs.NegotiatedProtocol != ALPN {
			return authenticationError("TLS version or ALPN")
		}
		return VerifyPeer(cs.PeerCertificates, key, server, time.Now())
	}
	return cfg, nil
}

// VerifyPeer is shared by TLS and negative policy tests. A pinned key alone is
// insufficient: all certificate policy checks still apply.
func VerifyPeer(chain []*x509.Certificate, expected ed25519.PublicKey, peerIsClient bool, now time.Time) error {
	if len(chain) != 1 || len(expected) != ed25519.PublicKeySize {
		return authenticationError("invalid certificate chain")
	}
	c := chain[0]
	pub, ok := c.PublicKey.(ed25519.PublicKey)
	if !ok || !bytes.Equal(pub, expected) {
		return authenticationError("peer public key changed")
	}
	if c.SignatureAlgorithm != x509.PureEd25519 || !bytes.Equal(c.RawIssuer, c.RawSubject) {
		return authenticationError("certificate must be Ed25519 self-signed")
	}
	if err := c.CheckSignature(c.SignatureAlgorithm, c.RawTBSCertificate, c.Signature); err != nil {
		return errors.Join(ErrAuthentication, fmt.Errorf("certificate signature: %w", err))
	}
	if c.IsCA || !c.BasicConstraintsValid || c.KeyUsage != x509.KeyUsageDigitalSignature || len(c.UnhandledCriticalExtensions) != 0 {
		return authenticationError("invalid certificate policy")
	}
	if c.NotAfter.Sub(c.NotBefore) > 25*time.Hour || now.Before(c.NotBefore) || !now.Before(c.NotAfter) {
		return authenticationError("certificate validity")
	}
	if len(c.URIs) != 1 || c.URIs[0].String() != "urn:linksend:device:"+DeviceID(expected) || c.Subject.CommonName != DeviceID(expected) || len(c.DNSNames) != 0 || len(c.IPAddresses) != 0 || len(c.EmailAddresses) != 0 {
		return authenticationError("target device identity")
	}
	usage := x509.ExtKeyUsageServerAuth
	if peerIsClient {
		usage = x509.ExtKeyUsageClientAuth
	}
	found := false
	for _, u := range c.ExtKeyUsage {
		if u == usage {
			found = true
		}
	}
	if !found || len(c.UnknownExtKeyUsage) != 0 {
		return authenticationError("certificate extended usage")
	}
	roots := x509.NewCertPool()
	roots.AddCert(c)
	if _, err := c.Verify(x509.VerifyOptions{Roots: roots, CurrentTime: now, KeyUsages: []x509.ExtKeyUsage{usage}}); err != nil {
		return errors.Join(ErrAuthentication, fmt.Errorf("certificate validation: %w", err))
	}
	return nil
}

type TrustedPeer struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	PublicKey []byte `json:"public_key"`
}
type TrustFile struct {
	Peers []TrustedPeer `json:"peers"`
}

func LoadTrust(dir string) ([]TrustedPeer, error) {
	fh, err := os.Open(filepath.Join(dir, "trust.json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer fh.Close()
	data, err := io.ReadAll(io.LimitReader(fh, maxTrustFileBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxTrustFileBytes {
		return nil, errors.New("trust file too large")
	}
	var f TrustFile
	if err = json.Unmarshal(data, &f); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, p := range f.Peers {
		if len(p.PublicKey) != 32 || DeviceID(p.PublicKey) != p.ID || seen[p.ID] {
			return nil, errors.New("invalid trust file")
		}
		seen[p.ID] = true
	}
	return f.Peers, nil
}

// TrustPeer requires the full fingerprint confirmed through a trusted channel;
// registration in the signaling group never implicitly establishes this trust.
func TrustPeer(dir string, peer TrustedPeer, confirmedFingerprint string) error {
	if len(peer.PublicKey) != 32 || DeviceID(peer.PublicKey) != peer.ID || confirmedFingerprint != peer.ID {
		return errors.New("UNPAIRED: fingerprint confirmation does not match")
	}
	peers, err := LoadTrust(dir)
	if err != nil {
		return err
	}
	for _, p := range peers {
		if p.ID == peer.ID {
			if !bytes.Equal(p.PublicKey, peer.PublicKey) {
				return authenticationError("peer key changed")
			}
			return nil
		}
	}
	peers = append(peers, peer)
	data, err := json.MarshalIndent(TrustFile{Peers: peers}, "", "  ")
	if err != nil {
		return err
	}
	if err = os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, "trust-*.tmp")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(name, filepath.Join(dir, "trust.json"))
}
