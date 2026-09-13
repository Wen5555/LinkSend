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
	ID                 string    `json:"id"`
	Name               string    `json:"name"`
	PublicKey          []byte    `json:"public_key"`
	AutoAccept         bool      `json:"auto_accept,omitempty"`
	LastLANAddress     string    `json:"last_lan_address,omitempty"`
	LANAddressHistory  []string  `json:"lan_address_history,omitempty"`
	GroupID            string    `json:"group_id,omitempty"`
	PeerIncarnation    string    `json:"peer_incarnation,omitempty"`
	LocalIncarnation   string    `json:"local_incarnation,omitempty"`
	MembershipRevision uint64    `json:"membership_revision,omitempty"`
	GrantGeneration    uint64    `json:"grant_generation,omitempty"`
	GrantKind          string    `json:"grant_kind,omitempty"`
	GrantedAt          time.Time `json:"granted_at,omitempty"`
	LANRequestID       string    `json:"lan_request_id,omitempty"`
}
type TrustFile struct {
	SchemaVersion       int                   `json:"schema_version,omitempty"`
	Peers               []TrustedPeer         `json:"peers"`
	DeniedPeers         []DeniedPeer          `json:"denied_peers,omitempty"`
	MembershipSnapshots []MembershipSnapshot  `json:"membership_snapshots,omitempty"`
	ProvisionalLAN      []ProvisionalLANGrant `json:"provisional_lan,omitempty"`
	AuthorizationEpochs map[string]uint64     `json:"authorization_epochs,omitempty"`
}

type ProvisionalLANGrant struct {
	RequestID  string      `json:"request_id"`
	Nonce      string      `json:"nonce"`
	Peer       TrustedPeer `json:"peer"`
	Generation uint64      `json:"generation"`
	State      string      `json:"state"`
	ExpiresAt  time.Time   `json:"expires_at"`
	Credential string      `json:"credential,omitempty"`
}

type MembershipSnapshot struct {
	GroupID          string `json:"group_id"`
	LocalIncarnation string `json:"local_incarnation"`
	Revision         uint64 `json:"revision"`
}
type DeniedPeer struct {
	ID                      string    `json:"id"`
	Name                    string    `json:"name,omitempty"`
	DeniedAt                time.Time `json:"denied_at"`
	LocalBlock              bool      `json:"local_block,omitempty"`
	AuthorizationGeneration uint64    `json:"authorization_generation,omitempty"`
	TargetIncarnation       string    `json:"target_incarnation,omitempty"`
	SeenRevision            uint64    `json:"seen_revision,omitempty"`
	RequestID               string    `json:"request_id,omitempty"`
	PendingSync             bool      `json:"pending_sync,omitempty"`
}

const trustSchemaVersion = 2
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

func LANPairGeneration(dir, peerID string) (uint64, error) {
	trustMu.Lock()
	defer trustMu.Unlock()
	if !validPeerID(peerID) {
		return 0, authenticationError("invalid device identity")
	}
	f, err := loadTrustFile(dir)
	if err != nil {
		return 0, err
	}
	for _, denied := range f.DeniedPeers {
		if denied.ID == peerID {
			if denied.LocalBlock {
				return 0, ErrPeerDenied
			}
			return denied.AuthorizationGeneration, nil
		}
	}
	for _, peer := range f.Peers {
		if peer.ID == peerID {
			return max(uint64(1), peer.GrantGeneration), nil
		}
	}
	return max(uint64(1), f.AuthorizationEpochs[peerID]+1), nil
}

func AuthorizationGeneration(dir, peerID string) (uint64, error) {
	trustMu.Lock()
	defer trustMu.Unlock()
	f, err := loadTrustFile(dir)
	if err != nil {
		return 0, err
	}
	if err = checkPeerAllowed(f, peerID); err != nil {
		return 0, err
	}
	for _, peer := range f.Peers {
		if peer.ID == peerID {
			return max(uint64(1), peer.GrantGeneration), nil
		}
	}
	return 0, authenticationError("current peer grant missing")
}

func CommitLANPeer(dir string, peer TrustedPeer, expectedGeneration uint64) error {
	trustMu.Lock()
	defer trustMu.Unlock()
	f, err := loadTrustFile(dir)
	if err != nil {
		return err
	}
	if err = commitLANPeerFile(&f, peer, expectedGeneration); err != nil {
		return err
	}
	return saveTrust(dir, f)
}

func commitLANPeerFile(f *TrustFile, peer TrustedPeer, expectedGeneration uint64) error {
	if len(peer.PublicKey) != 32 || DeviceID(peer.PublicKey) != peer.ID || expectedGeneration == 0 || !validLANAddress(peer.LastLANAddress) {
		return authenticationError("invalid LAN pairing grant")
	}
	for index := 0; index < len(f.DeniedPeers); index++ {
		denied := f.DeniedPeers[index]
		if denied.ID != peer.ID {
			continue
		}
		if denied.LocalBlock || denied.AuthorizationGeneration != expectedGeneration {
			return ErrPeerDenied
		}
		retainAuthorizationEpoch(f, peer.ID, denied.AuthorizationGeneration)
		f.DeniedPeers = append(f.DeniedPeers[:index], f.DeniedPeers[index+1:]...)
		break
	}
	for index, existing := range f.Peers {
		if existing.ID != peer.ID {
			continue
		}
		if !bytes.Equal(existing.PublicKey, peer.PublicKey) {
			return authenticationError("peer key changed")
		}
		if expectedGeneration != existing.GrantGeneration {
			return authenticationError("stale LAN grant generation")
		}
		peer.GrantGeneration = existing.GrantGeneration
		peer.AutoAccept = existing.AutoAccept
		peer.GrantedAt = existing.GrantedAt
		if existing.GrantKind == "group" || existing.GrantKind == "group+lan" {
			peer.GroupID, peer.PeerIncarnation, peer.LocalIncarnation, peer.MembershipRevision = existing.GroupID, existing.PeerIncarnation, existing.LocalIncarnation, existing.MembershipRevision
			peer.GrantKind = "group+lan"
		} else {
			peer.GrantKind = "lan"
		}
		f.Peers[index] = peer
		return nil
	}
	peer.AutoAccept = false
	peer.GrantKind = "lan"
	peer.GrantGeneration = expectedGeneration
	peer.GrantedAt = time.Now().UTC()
	f.Peers = append(f.Peers, peer)
	return nil
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
	if len(f.AuthorizationEpochs) > 1024 {
		return TrustFile{}, errors.New("too many authorization epochs")
	}
	for peerID, generation := range f.AuthorizationEpochs {
		if !validPeerID(peerID) || generation == 0 {
			return TrustFile{}, errors.New("invalid authorization epoch")
		}
	}
	for _, p := range f.Peers {
		if len(p.PublicKey) != 32 || DeviceID(p.PublicKey) != p.ID || seen[p.ID] || !validLANAddress(p.LastLANAddress) || len(p.LANAddressHistory) > 8 || p.GrantKind != "" && p.GrantKind != "group" && p.GrantKind != "lan" && p.GrantKind != "group+lan" {
			return TrustFile{}, errors.New("invalid trust file")
		}
		for _, address := range p.LANAddressHistory {
			if address == "" || !validLANAddress(address) {
				return TrustFile{}, errors.New("invalid remembered LAN address history")
			}
		}
		seen[p.ID] = true
	}
	for _, denied := range f.DeniedPeers {
		if !validPeerID(denied.ID) || denied.DeniedAt.IsZero() || seen[denied.ID] || denied.AuthorizationGeneration == 0 && f.SchemaVersion >= 2 {
			return TrustFile{}, errors.New("invalid denied peer record")
		}
		seen[denied.ID] = true
	}
	for index := range f.MembershipSnapshots {
		snapshot := f.MembershipSnapshots[index]
		if snapshot.GroupID == "" || len(snapshot.LocalIncarnation) != 32 || snapshot.Revision == 0 {
			return TrustFile{}, errors.New("invalid membership snapshot")
		}
	}
	if len(f.ProvisionalLAN) > 64 {
		return TrustFile{}, errors.New("too many provisional LAN grants")
	}
	for _, grant := range f.ProvisionalLAN {
		if len(grant.RequestID) != 32 || len(grant.Nonce) != 32 || grant.Generation == 0 || grant.ExpiresAt.IsZero() || len(grant.Credential) > 512 || len(grant.Peer.PublicKey) != 32 || DeviceID(grant.Peer.PublicKey) != grant.Peer.ID {
			return TrustFile{}, errors.New("invalid provisional LAN grant")
		}
	}
	if f.SchemaVersion == 1 {
		for index := range f.DeniedPeers {
			f.DeniedPeers[index].LocalBlock = true
			f.DeniedPeers[index].AuthorizationGeneration = 1
		}
		for index := range f.Peers {
			if f.Peers[index].GrantGeneration == 0 {
				f.Peers[index].GrantGeneration = 1
			}
			if f.Peers[index].GrantKind == "" {
				f.Peers[index].GrantKind = "group"
			}
		}
	}
	return f, nil
}

func BeginProvisionalLAN(dir string, grant ProvisionalLANGrant) error {
	trustMu.Lock()
	defer trustMu.Unlock()
	f, err := loadTrustFile(dir)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	active := f.ProvisionalLAN[:0]
	for _, existing := range f.ProvisionalLAN {
		if existing.ExpiresAt.After(now) {
			active = append(active, existing)
		}
	}
	f.ProvisionalLAN = active
	for _, existing := range f.ProvisionalLAN {
		if existing.RequestID == grant.RequestID {
			if existing.Peer.ID == grant.Peer.ID && existing.Nonce == grant.Nonce && existing.Generation == grant.Generation {
				return nil
			}
			return authenticationError("LAN pairing request conflict")
		}
	}
	if len(f.ProvisionalLAN) >= 64 || !grant.ExpiresAt.After(now) || grant.ExpiresAt.After(now.Add(60*time.Second)) {
		return errors.New("LAN_PAIR_RESOURCE_LIMIT")
	}
	current := uint64(1)
	for _, peer := range f.Peers {
		if peer.ID == grant.Peer.ID {
			current = peer.GrantGeneration
		}
	}
	for _, denied := range f.DeniedPeers {
		if denied.ID == grant.Peer.ID {
			if denied.LocalBlock {
				return ErrPeerDenied
			}
			current = denied.AuthorizationGeneration
		}
	}
	current = max(current, f.AuthorizationEpochs[grant.Peer.ID]+1)
	if current != grant.Generation {
		return authenticationError("stale LAN grant generation")
	}
	grant.Peer.AutoAccept = false
	grant.Peer.LANRequestID = grant.RequestID
	grant.Peer.GrantKind = "lan"
	grant.Peer.GrantGeneration = grant.Generation
	f.ProvisionalLAN = append(f.ProvisionalLAN, grant)
	return saveTrust(dir, f)
}

func LANPairStatus(dir, peerID, requestID, nonce string) (string, error) {
	trustMu.Lock()
	defer trustMu.Unlock()
	f, err := loadTrustFile(dir)
	if err != nil {
		return "", err
	}
	for _, peer := range f.Peers {
		if peer.ID == peerID && peer.LANRequestID == requestID {
			return "done", nil
		}
	}
	for _, grant := range f.ProvisionalLAN {
		if grant.Peer.ID == peerID && grant.RequestID == requestID && grant.Nonce == nonce && grant.ExpiresAt.After(time.Now().UTC()) {
			if grant.State == "done" {
				return "done", nil
			}
			return "ready", nil
		}
	}
	return "unknown", nil
}

func SetProvisionalLANCredential(dir, requestID, nonce, credential string) error {
	trustMu.Lock()
	defer trustMu.Unlock()
	if credential == "" || len(credential) > 512 {
		return errors.New("invalid LAN credential")
	}
	f, err := loadTrustFile(dir)
	if err != nil {
		return err
	}
	for index := range f.ProvisionalLAN {
		grant := &f.ProvisionalLAN[index]
		if grant.RequestID == requestID && grant.Nonce == nonce && grant.ExpiresAt.After(time.Now().UTC()) {
			grant.Credential = credential
			return saveTrust(dir, f)
		}
	}
	return errors.New("LAN_PAIR_REQUEST_NOT_FOUND")
}

func LANPairCredential(dir, peerID, requestID, nonce string) (string, error) {
	trustMu.Lock()
	defer trustMu.Unlock()
	f, err := loadTrustFile(dir)
	if err != nil {
		return "", err
	}
	for _, grant := range f.ProvisionalLAN {
		if grant.Peer.ID == peerID && grant.RequestID == requestID && grant.Nonce == nonce && grant.State == "done" && grant.ExpiresAt.After(time.Now().UTC()) {
			return grant.Credential, nil
		}
	}
	return "", errors.New("LAN_PAIR_CREDENTIAL_NOT_FOUND")
}

// PendingProvisionalLAN returns the newest unexpired transaction for one peer.
// It enables an explicit RequestLANPair retry after process restart to resume
// the signed query instead of creating a second authorization transaction.
func PendingProvisionalLAN(dir, peerID string) (ProvisionalLANGrant, bool, error) {
	trustMu.Lock()
	defer trustMu.Unlock()
	f, err := loadTrustFile(dir)
	if err != nil {
		return ProvisionalLANGrant{}, false, err
	}
	now := time.Now().UTC()
	var newest ProvisionalLANGrant
	for _, grant := range f.ProvisionalLAN {
		if grant.Peer.ID == peerID && grant.ExpiresAt.After(now) && (newest.ExpiresAt.IsZero() || grant.ExpiresAt.After(newest.ExpiresAt)) {
			newest = grant
		}
	}
	return newest, !newest.ExpiresAt.IsZero(), nil
}

func CommitProvisionalLAN(dir, requestID, nonce string) error {
	trustMu.Lock()
	defer trustMu.Unlock()
	f, err := loadTrustFile(dir)
	if err != nil {
		return err
	}
	for index, grant := range f.ProvisionalLAN {
		if grant.RequestID != requestID || grant.Nonce != nonce {
			continue
		}
		if !grant.ExpiresAt.After(time.Now().UTC()) {
			f.ProvisionalLAN = append(f.ProvisionalLAN[:index], f.ProvisionalLAN[index+1:]...)
			_ = saveTrust(dir, f)
			return errors.New("LAN_PAIR_EXPIRED")
		}
		if grant.State == "done" {
			return nil
		}
		if err = commitLANPeerFile(&f, grant.Peer, grant.Generation); err != nil {
			return err
		}
		f.ProvisionalLAN[index].State = "done"
		return saveTrust(dir, f)
	}
	return errors.New("LAN_PAIR_REQUEST_NOT_FOUND")
}

func CancelProvisionalLAN(dir, requestID string) error {
	trustMu.Lock()
	defer trustMu.Unlock()
	f, err := loadTrustFile(dir)
	if err != nil {
		return err
	}
	for index, grant := range f.ProvisionalLAN {
		if grant.RequestID == requestID {
			f.ProvisionalLAN = append(f.ProvisionalLAN[:index], f.ProvisionalLAN[index+1:]...)
			return saveTrust(dir, f)
		}
	}
	return nil
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
				history := append([]string{}, existing.LANAddressHistory...)
				if existing.LastLANAddress != "" {
					history = appendRememberedAddress(history, existing.LastLANAddress)
				}
				history = appendRememberedAddress(history, peer.LastLANAddress)
				f.Peers[index].LastLANAddress = peer.LastLANAddress
				f.Peers[index].LANAddressHistory = history
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
	if peer.GrantGeneration == 0 {
		peer.GrantGeneration = max(uint64(1), f.AuthorizationEpochs[peer.ID]+1)
	}
	if peer.GrantKind == "" {
		peer.GrantKind = "lan"
	}
	if peer.GrantedAt.IsZero() {
		peer.GrantedAt = time.Now().UTC()
	}
	f.Peers = append(f.Peers, peer)
	return saveTrust(dir, f)
}

// TrustMembershipPeer applies an authenticated membership_v2 snapshot. A
// newer verified incarnation may clear only a group-removal barrier; an
// explicit local block is never cleared by server state.
func TrustMembershipPeer(dir string, peer TrustedPeer) error {
	trustMu.Lock()
	defer trustMu.Unlock()
	f, err := loadTrustFile(dir)
	if err != nil {
		return err
	}
	if err = applyMembershipPeer(&f, peer); err != nil {
		return err
	}
	return saveTrust(dir, f)
}

func applyMembershipPeer(f *TrustFile, peer TrustedPeer) error {
	if len(peer.PublicKey) != 32 || DeviceID(peer.PublicKey) != peer.ID || peer.GroupID == "" || len(peer.PeerIncarnation) != 32 || len(peer.LocalIncarnation) != 32 || peer.MembershipRevision == 0 {
		return authenticationError("invalid membership_v2 grant")
	}
	generation := max(uint64(1), f.AuthorizationEpochs[peer.ID]+1)
	for index := 0; index < len(f.DeniedPeers); index++ {
		denied := f.DeniedPeers[index]
		if denied.ID != peer.ID {
			continue
		}
		if denied.LocalBlock || denied.TargetIncarnation == peer.PeerIncarnation || peer.MembershipRevision <= denied.SeenRevision {
			return fmt.Errorf("%w: %w", ErrPeerDenied, ErrAuthentication)
		}
		generation = denied.AuthorizationGeneration + 1
		retainAuthorizationEpoch(f, peer.ID, denied.AuthorizationGeneration)
		f.DeniedPeers = append(f.DeniedPeers[:index], f.DeniedPeers[index+1:]...)
		break
	}
	for index, existing := range f.Peers {
		if existing.ID != peer.ID {
			continue
		}
		if !bytes.Equal(existing.PublicKey, peer.PublicKey) {
			return authenticationError("peer key changed")
		}
		if existing.PeerIncarnation == peer.PeerIncarnation && existing.LocalIncarnation == peer.LocalIncarnation && existing.GroupID == peer.GroupID {
			if peer.MembershipRevision < existing.MembershipRevision {
				return authenticationError("stale membership revision")
			}
			peer.AutoAccept = existing.AutoAccept
			peer.LastLANAddress = existing.LastLANAddress
			peer.LANAddressHistory = existing.LANAddressHistory
			peer.GrantGeneration = existing.GrantGeneration
			peer.GrantedAt = existing.GrantedAt
		} else {
			peer.AutoAccept = false
			peer.LastLANAddress = ""
			peer.GrantGeneration = max(generation, existing.GrantGeneration+1)
		}
		if existing.GrantKind == "lan" || existing.GrantKind == "group+lan" {
			peer.GrantKind = "group+lan"
		} else {
			peer.GrantKind = "group"
		}
		f.Peers[index] = peer
		return nil
	}
	peer.AutoAccept = false
	peer.GrantKind = "group"
	peer.GrantGeneration = generation
	peer.GrantedAt = time.Now().UTC()
	f.Peers = append(f.Peers, peer)
	return nil
}

// ApplyMembershipSnapshot atomically advances one complete authenticated group
// snapshot and revokes every formerly grouped peer missing from the new revision.
func ApplyMembershipSnapshot(dir, localID string, peers []TrustedPeer) error {
	trustMu.Lock()
	defer trustMu.Unlock()
	if len(peers) == 0 {
		return authenticationError("empty membership snapshot")
	}
	var groupID, localInc string
	var revision uint64
	present := map[string]bool{}
	for _, peer := range peers {
		if groupID == "" {
			groupID, localInc, revision = peer.GroupID, peer.LocalIncarnation, peer.MembershipRevision
		}
		if peer.GroupID != groupID || peer.LocalIncarnation != localInc || peer.MembershipRevision != revision {
			return authenticationError("inconsistent membership snapshot")
		}
		present[peer.ID] = true
	}
	f, err := loadTrustFile(dir)
	if err != nil {
		return err
	}
	for index := range f.MembershipSnapshots {
		snapshot := &f.MembershipSnapshots[index]
		if snapshot.GroupID == groupID {
			if revision < snapshot.Revision || localInc != snapshot.LocalIncarnation && revision <= snapshot.Revision {
				return authenticationError("stale membership snapshot")
			}
			snapshot.Revision, snapshot.LocalIncarnation = revision, localInc
			goto snapshotReady
		}
	}
	f.MembershipSnapshots = append(f.MembershipSnapshots, MembershipSnapshot{GroupID: groupID, LocalIncarnation: localInc, Revision: revision})
snapshotReady:
	for index := len(f.Peers) - 1; index >= 0; index-- {
		peer := f.Peers[index]
		if peer.GroupID == "" || peer.GroupID == groupID {
			continue
		}
		if peer.GrantKind == "group+lan" {
			f.Peers[index].GroupID, f.Peers[index].PeerIncarnation, f.Peers[index].LocalIncarnation, f.Peers[index].MembershipRevision, f.Peers[index].GrantKind, f.Peers[index].AutoAccept = "", "", "", 0, "lan", false
			continue
		}
		denied := DeniedPeer{ID: peer.ID, Name: peer.Name, DeniedAt: time.Now().UTC(), AuthorizationGeneration: max(uint64(1), peer.GrantGeneration+1), TargetIncarnation: peer.PeerIncarnation, SeenRevision: revision}
		f.Peers = append(f.Peers[:index], f.Peers[index+1:]...)
		already := false
		for _, existing := range f.DeniedPeers {
			if existing.ID == denied.ID {
				already = true
			}
		}
		if !already {
			f.DeniedPeers = append(f.DeniedPeers, denied)
		}
	}
	for index := len(f.Peers) - 1; index >= 0; index-- {
		peer := f.Peers[index]
		if peer.ID == localID || peer.GroupID != groupID || present[peer.ID] {
			continue
		}
		denied := DeniedPeer{ID: peer.ID, Name: peer.Name, DeniedAt: time.Now().UTC(), AuthorizationGeneration: max(uint64(1), peer.GrantGeneration+1), TargetIncarnation: peer.PeerIncarnation, SeenRevision: revision}
		f.Peers = append(f.Peers[:index], f.Peers[index+1:]...)
		already := false
		for _, existing := range f.DeniedPeers {
			if existing.ID == denied.ID {
				already = true
			}
		}
		if !already {
			f.DeniedPeers = append(f.DeniedPeers, denied)
		}
	}
	for _, peer := range peers {
		if peer.ID != localID {
			if err = applyMembershipPeer(&f, peer); err != nil && !errors.Is(err, ErrPeerDenied) {
				return err
			}
		}
	}
	return saveTrust(dir, f)
}

func appendRememberedAddress(history []string, address string) []string {
	for index, existing := range history {
		if existing == address {
			history = append(history[:index], history[index+1:]...)
			break
		}
	}
	history = append(history, address)
	if len(history) > 8 {
		history = history[len(history)-8:]
	}
	return history
}

func CheckTaskAuthorization(dir, peerID, startedAt string) error {
	when, err := time.Parse(time.RFC3339Nano, startedAt)
	if err != nil {
		return authenticationError("task authorization timestamp invalid")
	}
	trustMu.Lock()
	defer trustMu.Unlock()
	f, err := loadTrustFile(dir)
	if err != nil {
		return err
	}
	if err = checkPeerAllowed(f, peerID); err != nil {
		return err
	}
	for _, peer := range f.Peers {
		if peer.ID == peerID {
			if peer.GrantGeneration > 1 && !peer.GrantedAt.IsZero() && when.Before(peer.GrantedAt) {
				return authenticationError("task predates current peer grant")
			}
			return nil
		}
	}
	return authenticationError("current peer grant missing")
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
	denied := DeniedPeer{ID: peerID, DeniedAt: time.Now().UTC(), LocalBlock: true, AuthorizationGeneration: 1}
	for index, peer := range f.Peers {
		if peer.ID == peerID {
			denied.Name = peer.Name
			denied.AuthorizationGeneration = max(uint64(1), peer.GrantGeneration+1)
			f.Peers = append(f.Peers[:index], f.Peers[index+1:]...)
			break
		}
	}
	for index := len(f.ProvisionalLAN) - 1; index >= 0; index-- {
		grant := f.ProvisionalLAN[index]
		if grant.Peer.ID == peerID {
			denied.Name = grant.Peer.Name
			denied.AuthorizationGeneration = max(denied.AuthorizationGeneration, grant.Generation+1)
			f.ProvisionalLAN = append(f.ProvisionalLAN[:index], f.ProvisionalLAN[index+1:]...)
		}
	}
	retainAuthorizationEpoch(&f, peerID, denied.AuthorizationGeneration)
	f.DeniedPeers = append(f.DeniedPeers, denied)
	return saveTrust(dir, f)
}

// RevokeMembershipPeer persists the group-removal intent before the network
// request. It invalidates group and LAN grants and is idempotent by request ID.
func RevokeMembershipPeer(dir, peerID, targetIncarnation, requestID string, seenRevision uint64) error {
	trustMu.Lock()
	defer trustMu.Unlock()
	if !validPeerID(peerID) || len(targetIncarnation) != 32 || requestID == "" || seenRevision == 0 {
		return authenticationError("invalid membership revocation")
	}
	f, err := loadTrustFile(dir)
	if err != nil {
		return err
	}
	for _, denied := range f.DeniedPeers {
		if denied.ID == peerID {
			if denied.LocalBlock || denied.RequestID == requestID {
				return nil
			}
			return ErrPeerDenied
		}
	}
	denied := DeniedPeer{ID: peerID, DeniedAt: time.Now().UTC(), AuthorizationGeneration: 1, TargetIncarnation: targetIncarnation, SeenRevision: seenRevision, RequestID: requestID, PendingSync: true}
	for index, peer := range f.Peers {
		if peer.ID == peerID {
			denied.Name = peer.Name
			denied.AuthorizationGeneration = max(uint64(1), peer.GrantGeneration+1)
			f.Peers = append(f.Peers[:index], f.Peers[index+1:]...)
			break
		}
	}
	f.DeniedPeers = append(f.DeniedPeers, denied)
	return saveTrust(dir, f)
}

func MarkMembershipRevokeSynced(dir, peerID, requestID string, revision uint64) error {
	trustMu.Lock()
	defer trustMu.Unlock()
	f, err := loadTrustFile(dir)
	if err != nil {
		return err
	}
	for index := range f.DeniedPeers {
		d := &f.DeniedPeers[index]
		if d.ID == peerID && d.RequestID == requestID {
			if revision > d.SeenRevision {
				d.SeenRevision = revision
			}
			d.PendingSync = false
			return saveTrust(dir, f)
		}
	}
	return ErrPeerDenied
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
			retainAuthorizationEpoch(&f, peerID, denied.AuthorizationGeneration)
			f.DeniedPeers = append(f.DeniedPeers[:index], f.DeniedPeers[index+1:]...)
			return saveTrust(dir, f)
		}
	}
	return nil
}

func retainAuthorizationEpoch(f *TrustFile, peerID string, generation uint64) {
	if f.AuthorizationEpochs == nil {
		f.AuthorizationEpochs = map[string]uint64{}
	}
	f.AuthorizationEpochs[peerID] = max(f.AuthorizationEpochs[peerID], generation)
}

func saveTrust(dir string, f TrustFile) error {
	legacy := f.SchemaVersion == 0
	for index := range f.Peers {
		if f.Peers[index].GrantGeneration == 0 {
			f.Peers[index].GrantGeneration = 1
		}
		if f.Peers[index].GrantKind == "" {
			f.Peers[index].GrantKind = "group"
		}
	}
	for index := range f.DeniedPeers {
		if f.DeniedPeers[index].AuthorizationGeneration == 0 {
			f.DeniedPeers[index].AuthorizationGeneration = 1
			f.DeniedPeers[index].LocalBlock = true
		}
	}
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
