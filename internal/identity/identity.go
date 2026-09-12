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
	"sync"
	"time"
)

const ALPN = "linksend/1"
const maxTrustFileBytes = 1 << 20

var ErrAuthentication = errors.New("identity authentication failed")

// ErrPeerDenied identifies an explicit local revocation. Discovery, pairing,
// or completing an already running transfer cannot remove this decision.
var ErrPeerDenied = errors.New("PEER_DENIED: device is locally blocked")
var errUnsupportedTrustSchema = errors.New("unsupported trust policy schema")

// The application owns the profile across processes. This mutex serializes
// read-modify-write operations and fallback replacement within that process.
var trustMu sync.Mutex

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
	SchemaVersion int           `json:"schema_version,omitempty"`
	Peers         []TrustedPeer `json:"peers"`
	DeniedPeers   []DeniedPeer  `json:"denied_peers,omitempty"`
}

type DeniedPeer struct {
	ID       string    `json:"id"`
	Name     string    `json:"name,omitempty"`
	DeniedAt time.Time `json:"denied_at"`
}

const trustSchemaVersion = 1
const trustMigrationBackup = "trust.json.pre-schema-1"

func LoadTrust(dir string) ([]TrustedPeer, error) {
	trustMu.Lock()
	defer trustMu.Unlock()
	f, err := loadTrustFile(dir)
	return f.Peers, err
}

// LoadDeniedPeers exposes local policy independently of live presence or pins.
func LoadDeniedPeers(dir string) ([]DeniedPeer, error) {
	trustMu.Lock()
	defer trustMu.Unlock()
	f, err := loadTrustFile(dir)
	return f.DeniedPeers, err
}

// CheckPeerAllowed checks the revocation barrier; nil does not establish trust.
// Unreadable, unsupported or corrupt policy is an error, never an empty denylist.
func CheckPeerAllowed(dir, peerID string) error {
	trustMu.Lock()
	defer trustMu.Unlock()
	if !validPeerID(peerID) {
		return authenticationError("invalid device identity")
	}
	f, err := loadTrustFile(dir)
	if err != nil {
		return err
	}
	return checkPeerAllowed(f, peerID)
}

func checkPeerAllowed(f TrustFile, peerID string) error {
	for _, denied := range f.DeniedPeers {
		if denied.ID == peerID {
			return fmt.Errorf("%w: %w", ErrPeerDenied, ErrAuthentication)
		}
	}
	return nil
}

func loadTrustFile(dir string) (TrustFile, error) {
	path := filepath.Join(dir, "trust.json")
	data, err := readTrustFile(path)
	if errors.Is(err, os.ErrNotExist) {
		// A migrated profile losing its policy file must not silently forget
		// revocations. Preserve the backup for explicit recovery, not auto-use.
		if _, backupErr := os.Lstat(filepath.Join(dir, trustMigrationBackup)); backupErr == nil {
			return TrustFile{}, errors.New("trust policy missing after migration; explicit recovery required")
		} else if !errors.Is(backupErr, os.ErrNotExist) {
			return TrustFile{}, backupErr
		}
		return TrustFile{}, nil
	}
	if err == nil {
		if f, parseErr := parseTrustFile(data); parseErr == nil {
			_ = os.Remove(path + ".previous")
			return f, nil
		} else {
			err = parseErr
		}
	}
	if errors.Is(err, errUnsupportedTrustSchema) {
		return TrustFile{}, err
	}
	// Legacy replacement recovery is retained only for an unmigrated legacy
	// file. Restoring an older policy can resurrect a subsequently revoked pin.
	if _, backupErr := os.Lstat(filepath.Join(dir, trustMigrationBackup)); !errors.Is(backupErr, os.ErrNotExist) {
		return TrustFile{}, errors.Join(err, backupErr)
	}
	backupPath := path + ".previous"
	backup, backupErr := readTrustFile(backupPath)
	if backupErr != nil {
		return TrustFile{}, err
	}
	f, backupErr := parseTrustFile(backup)
	if backupErr != nil {
		return TrustFile{}, errors.Join(err, backupErr)
	}
	if f.SchemaVersion != 0 {
		return TrustFile{}, errors.Join(err, errors.New("trust policy recovery requires explicit review"))
	}
	if restoreErr := overwriteFile(path, backup); restoreErr == nil {
		_ = os.Remove(backupPath)
	}
	return f, nil
}

func readTrustFile(path string) ([]byte, error) {
	fh, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer fh.Close()
	return io.ReadAll(io.LimitReader(fh, maxTrustFileBytes+1))
}

func parseTrustFile(data []byte) (TrustFile, error) {
	if len(data) > maxTrustFileBytes {
		return TrustFile{}, errors.New("trust file too large")
	}
	var f TrustFile
	if err := json.Unmarshal(data, &f); err != nil {
		return TrustFile{}, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil || fields["peers"] == nil {
		return TrustFile{}, errors.New("invalid trust policy object")
	}
	if f.SchemaVersion < 0 || f.SchemaVersion > trustSchemaVersion || (f.SchemaVersion == 0 && len(f.DeniedPeers) != 0) {
		return TrustFile{}, errUnsupportedTrustSchema
	}
	seen := map[string]bool{}
	for _, p := range f.Peers {
		if len(p.PublicKey) != 32 || DeviceID(p.PublicKey) != p.ID || seen[p.ID] || !validLANAddress(p.LastLANAddress) {
			return TrustFile{}, errors.New("invalid trust file")
		}
		seen[p.ID] = true
	}
	for _, denied := range f.DeniedPeers {
		if !validPeerID(denied.ID) || denied.DeniedAt.IsZero() || seen[denied.ID] {
			return TrustFile{}, errors.New("invalid denied peer record")
		}
		seen[denied.ID] = true
	}
	return f, nil
}

func validPeerID(peerID string) bool {
	decoded, err := hex.DecodeString(peerID)
	return err == nil && len(decoded) == sha256.Size && hex.EncodeToString(decoded) == peerID
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
	trustMu.Lock()
	defer trustMu.Unlock()
	if !validLANAddress(peer.LastLANAddress) {
		return errors.New("invalid remembered LAN address")
	}
	f, err := loadTrustFile(dir)
	if err != nil {
		return err
	}
	if err = checkPeerAllowed(f, peer.ID); err != nil {
		return err
	}
	for index, existing := range f.Peers {
		if existing.ID == peer.ID {
			if !bytes.Equal(existing.PublicKey, peer.PublicKey) {
				return authenticationError("peer key changed")
			}
			if peer.LastLANAddress != "" && existing.LastLANAddress != peer.LastLANAddress {
				f.Peers[index].LastLANAddress = peer.LastLANAddress
				return saveTrust(dir, f)
			}
			// The authenticated service remains the live source of display names.
			// A cosmetic rename must not rewrite the local key pin or make every
			// connection depend on filesystem replacement support.
			return nil
		}
	}
	// Auto-accept is a separate local command, never an enrollment field.
	peer.AutoAccept = false
	f.Peers = append(f.Peers, peer)
	return saveTrust(dir, f)
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
	trustMu.Lock()
	defer trustMu.Unlock()
	f, err := loadTrustFile(dir)
	if err != nil {
		return err
	}
	if err = checkPeerAllowed(f, peerID); err != nil {
		return err
	}
	for i := range f.Peers {
		if f.Peers[i].ID != peerID {
			continue
		}
		if f.Peers[i].AutoAccept == enabled {
			return nil
		}
		f.Peers[i].AutoAccept = enabled
		return saveTrust(dir, f)
	}
	return errors.New("UNPAIRED: device is not paired")
}

// RevokePeer durably removes all privileges and records a local denial, even
// when the peer has only been discovered and was never pinned. It is idempotent.
func RevokePeer(dir, peerID string) error {
	trustMu.Lock()
	defer trustMu.Unlock()
	if !validPeerID(peerID) {
		return authenticationError("invalid device identity")
	}
	f, err := loadTrustFile(dir)
	if err != nil {
		return err
	}
	if errors.Is(checkPeerAllowed(f, peerID), ErrPeerDenied) {
		return nil
	}
	denied := DeniedPeer{ID: peerID, DeniedAt: time.Now().UTC()}
	for index, peer := range f.Peers {
		if peer.ID == peerID {
			denied.Name = peer.Name
			f.Peers = append(f.Peers[:index], f.Peers[index+1:]...)
			break
		}
	}
	f.DeniedPeers = append(f.DeniedPeers, denied)
	return saveTrust(dir, f)
}

// AllowPeer is an explicit local unblock command. It restores neither the old
// pin nor auto-accept; subsequent pairing or confirmed LAN transfer must pin anew.
func AllowPeer(dir, peerID string) error {
	trustMu.Lock()
	defer trustMu.Unlock()
	if !validPeerID(peerID) {
		return authenticationError("invalid device identity")
	}
	f, err := loadTrustFile(dir)
	if err != nil {
		return err
	}
	for index, denied := range f.DeniedPeers {
		if denied.ID == peerID {
			f.DeniedPeers = append(f.DeniedPeers[:index], f.DeniedPeers[index+1:]...)
			return saveTrust(dir, f)
		}
	}
	return nil
}

func saveTrust(dir string, f TrustFile) error {
	legacy := f.SchemaVersion == 0
	f.SchemaVersion = trustSchemaVersion
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	if _, err = parseTrustFile(data); err != nil {
		return err
	}
	if err = os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	if legacy {
		oldData, readErr := readTrustFile(filepath.Join(dir, "trust.json"))
		if errors.Is(readErr, os.ErrNotExist) {
			// Preserve an explicit empty baseline for new profiles too, so loss
			// of the policy file never turns a known profile into first use.
			oldData = []byte(`{"peers":[]}`)
		} else if readErr != nil {
			return readErr
		}
		backupPath := filepath.Join(dir, trustMigrationBackup)
		if err = writeNewFile(backupPath, oldData); err != nil {
			if !errors.Is(err, os.ErrExist) {
				return fmt.Errorf("back up legacy trust policy: %w", err)
			}
			info, statErr := os.Lstat(backupPath)
			if statErr != nil {
				return statErr
			}
			if !info.Mode().IsRegular() {
				return errors.New("legacy trust backup is not a regular file")
			}
			backup, readErr := readTrustFile(backupPath)
			if readErr != nil {
				return readErr
			}
			if _, parseErr := parseTrustFile(backup); parseErr != nil {
				return fmt.Errorf("invalid legacy trust backup: %w", parseErr)
			}
		}
	}
	file, err := os.CreateTemp(dir, "trust-*.tmp")
	if err != nil {
		return err
	}
	name := file.Name()
	defer os.Remove(name)
	if err = file.Chmod(0600); err == nil {
		_, err = file.Write(data)
	}
	if err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
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
