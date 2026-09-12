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
	"net/netip"
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

// TLSServerConfig verifies a self-signed LinkSend client certificate against a
// caller-maintained allowlist. It is used by LAN discovery where the expected
// peer becomes known from a recent signed announcement rather than before the
// listener accepts the TCP connection.
func (i *Identity) TLSServerConfig(allowed func(string, ed25519.PublicKey) bool) (*tls.Config, error) {
	if allowed == nil {
		return nil, authenticationError("LAN peer verifier required")
	}
	cert, err := i.certificate(time.Now())
	if err != nil {
		return nil, err
	}
	cfg := &tls.Config{
		MinVersion:             tls.VersionTLS13,
		MaxVersion:             tls.VersionTLS13,
		Certificates:           []tls.Certificate{cert},
		NextProtos:             []string{ALPN},
		SessionTicketsDisabled: true,
		ClientAuth:             tls.RequireAnyClientCert,
	}
	cfg.VerifyConnection = func(cs tls.ConnectionState) error {
		if cs.Version != tls.VersionTLS13 || cs.NegotiatedProtocol != ALPN || len(cs.PeerCertificates) != 1 {
			return authenticationError("TLS version, ALPN, or client certificate")
		}
		key, ok := cs.PeerCertificates[0].PublicKey.(ed25519.PublicKey)
		if !ok || !allowed(DeviceID(key), key) {
			return authenticationError("client is not a recently discovered LAN peer")
		}
		return VerifyPeer(cs.PeerCertificates, key, true, time.Now())
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
	ID             string `json:"id"`
	Name           string `json:"name"`
	PublicKey      []byte `json:"public_key"`
	AutoAccept     bool   `json:"auto_accept,omitempty"`
	LastLANAddress string `json:"last_lan_address,omitempty"`
}
type TrustFile struct {
	Peers []TrustedPeer `json:"peers"`
}

func LoadTrust(dir string) ([]TrustedPeer, error) {
	path := filepath.Join(dir, "trust.json")
	data, err := readTrustFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err == nil {
		if peers, parseErr := parseTrustFile(data); parseErr == nil {
			_ = os.Remove(path + ".previous")
			return peers, nil
		} else {
			err = parseErr
		}
	}
	backupPath := path + ".previous"
	backup, backupErr := readTrustFile(backupPath)
	if backupErr != nil {
		return nil, err
	}
	peers, backupErr := parseTrustFile(backup)
	if backupErr != nil {
		return nil, errors.Join(err, backupErr)
	}
	if restoreErr := overwriteFile(path, backup); restoreErr == nil {
		_ = os.Remove(backupPath)
	}
	return peers, nil
}

func readTrustFile(path string) ([]byte, error) {
	fh, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer fh.Close()
	return io.ReadAll(io.LimitReader(fh, maxTrustFileBytes+1))
}

func parseTrustFile(data []byte) ([]TrustedPeer, error) {
	if len(data) > maxTrustFileBytes {
		return nil, errors.New("trust file too large")
	}
	var f TrustFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, p := range f.Peers {
		if len(p.PublicKey) != 32 || DeviceID(p.PublicKey) != p.ID || seen[p.ID] || !validLANAddress(p.LastLANAddress) {
			return nil, errors.New("invalid trust file")
		}
		seen[p.ID] = true
	}
	return f.Peers, nil
}

// TrustPeer is retained for CLI compatibility with the former manual
// fingerprint flow. The desktop now uses TrustPairedPeer after code pairing.
func TrustPeer(dir string, peer TrustedPeer, confirmedFingerprint string) error {
	if len(peer.PublicKey) != 32 || DeviceID(peer.PublicKey) != peer.ID || confirmedFingerprint != peer.ID {
		return errors.New("UNPAIRED: fingerprint confirmation does not match")
	}
	return trustPeer(dir, peer)
}

// TrustPairedPeer pins a peer returned by the authenticated pairing service.
// Possession of the short-lived pairing code replaces manual fingerprint
// comparison in the desktop product; a previously pinned key can never be
// replaced silently.
func TrustPairedPeer(dir string, peer TrustedPeer) error {
	if len(peer.PublicKey) != 32 || DeviceID(peer.PublicKey) != peer.ID {
		return errors.New("UNPAIRED: invalid paired device identity")
	}
	return trustPeer(dir, peer)
}

func trustPeer(dir string, peer TrustedPeer) error {
	if !validLANAddress(peer.LastLANAddress) {
		return errors.New("invalid remembered LAN address")
	}
	peers, err := LoadTrust(dir)
	if err != nil {
		return err
	}
	for index, existing := range peers {
		if existing.ID == peer.ID {
			if !bytes.Equal(existing.PublicKey, peer.PublicKey) {
				return authenticationError("peer key changed")
			}
			if peer.LastLANAddress != "" && existing.LastLANAddress != peer.LastLANAddress {
				peers[index].LastLANAddress = peer.LastLANAddress
				return saveTrust(dir, peers)
			}
			// The authenticated service remains the live source of display names.
			// A cosmetic rename must not rewrite the local key pin or make every
			// connection depend on filesystem replacement support.
			return nil
		}
	}
	peers = append(peers, peer)
	return saveTrust(dir, peers)
}

func validLANAddress(value string) bool {
	if value == "" {
		return true
	}
	address, err := netip.ParseAddr(value)
	return err == nil && address.Is4() && !address.IsUnspecified() && !address.IsMulticast() && !address.IsLoopback()
}

// SetAutoAccept persists receiver consent for one already paired device.
func SetAutoAccept(dir, peerID string, enabled bool) error {
	peers, err := LoadTrust(dir)
	if err != nil {
		return err
	}
	for i := range peers {
		if peers[i].ID != peerID {
			continue
		}
		if peers[i].AutoAccept == enabled {
			return nil
		}
		peers[i].AutoAccept = enabled
		return saveTrust(dir, peers)
	}
	return errors.New("UNPAIRED: device is not paired")
}

func saveTrust(dir string, peers []TrustedPeer) error {
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
	return replaceFile(name, filepath.Join(dir, "trust.json"), os.Rename)
}

// replaceFile keeps the common atomic rename path. Windows EFS rejects direct
// replacement when an older target and the new temp file have different
// encryption states (ERROR_NOT_SAME_DEVICE), even inside one directory. The
// fallback first gives the verified old file a unique backup name, installs
// the new file, and restores the old name if installation fails.
func replaceFile(name, target string, rename func(string, string) error) error {
	return replaceFileWith(name, target, rename, replaceAtomicFile, replaceFileInPlace)
}

func replaceFileWith(name, target string, rename func(string, string) error, platformReplace func(string, string) (bool, error), inPlace func(string, string, error) error) error {
	directErr := rename(name, target)
	if directErr == nil {
		return nil
	}
	if attempted, replaceErr := platformReplace(name, target); attempted {
		if replaceErr == nil {
			return nil
		}
		directErr = errors.Join(directErr, replaceErr)
	}
	if aligned, alignErr := alignAtomicReplaceSource(name, target); aligned {
		if alignErr == nil {
			if attempted, replaceErr := platformReplace(name, target); attempted {
				if replaceErr == nil {
					return nil
				}
				directErr = errors.Join(directErr, replaceErr)
			}
			if retryErr := rename(name, target); retryErr == nil {
				return nil
			} else {
				directErr = errors.Join(directErr, retryErr)
			}
		} else {
			directErr = errors.Join(directErr, alignErr)
		}
	}
	if _, err := os.Stat(target); err != nil {
		return directErr
	}
	placeholder, err := os.CreateTemp(filepath.Dir(target), "trust-previous-*.tmp")
	if err != nil {
		return errors.Join(directErr, err)
	}
	backup := placeholder.Name()
	if closeErr := placeholder.Close(); closeErr != nil {
		_ = os.Remove(backup)
		return errors.Join(directErr, closeErr)
	}
	if err = os.Remove(backup); err != nil {
		return errors.Join(directErr, err)
	}
	if err = rename(target, backup); err != nil {
		return inPlace(name, target, errors.Join(directErr, err))
	}
	if err = rename(name, target); err != nil {
		restoreErr := rename(backup, target)
		return errors.Join(directErr, err, restoreErr)
	}
	_ = os.Remove(backup)
	return nil
}

func replaceFileInPlace(source, target string, cause error) error {
	newData, err := readTrustFile(source)
	if err != nil {
		return errors.Join(cause, err)
	}
	oldData, err := readTrustFile(target)
	if err != nil {
		return errors.Join(cause, err)
	}
	backup := target + ".previous"
	_ = os.Remove(backup)
	if err = writeNewFile(backup, oldData); err != nil {
		return errors.Join(cause, err)
	}
	if err = overwriteFile(target, newData); err != nil {
		restoreErr := overwriteFile(target, oldData)
		return errors.Join(cause, err, restoreErr)
	}
	verified, err := readTrustFile(target)
	if err != nil || !bytes.Equal(verified, newData) {
		restoreErr := overwriteFile(target, oldData)
		return errors.Join(cause, err, errors.New("trust replacement verification failed"), restoreErr)
	}
	_ = os.Remove(backup)
	return nil
}

func writeNewFile(path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	return writeAndSync(f, data)
}

func overwriteFile(path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	return writeAndSync(f, data)
}

func writeAndSync(f *os.File, data []byte) error {
	_, writeErr := f.Write(data)
	if writeErr == nil {
		writeErr = f.Sync()
	}
	closeErr := f.Close()
	return errors.Join(writeErr, closeErr)
}
