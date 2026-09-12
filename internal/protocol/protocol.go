// Package protocol defines versioned, bounded control messages. It never carries
// file bytes. Signature input is explicit length-prefixed binary, not JSON maps.
package protocol

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	Version        = 1
	ProductVersion = "0.4.0"
)
const MaxMessageBytes = 32 * 1024
const MaxPayloadBytes = 16 * 1024
const MaxCandidates = 32
const MaxSessionsPerDevice = 8

type Code string

const (
	SignalingUnreachable    Code = "SIGNALING_UNREACHABLE"
	SignalingTimeout        Code = "SIGNALING_TIMEOUT"
	CandidateTimeout        Code = "CANDIDATE_EXCHANGE_TIMEOUT"
	PeerOffline             Code = "PEER_OFFLINE"
	LANDiscoveryUnavailable Code = "LAN_DISCOVERY_UNAVAILABLE"
	LANControlUnreachable   Code = "LAN_CONTROL_UNREACHABLE"
	LANProbeInvalid         Code = "LAN_PROBE_INVALID"
	Unpaired                Code = "UNPAIRED"
	PairingCodeInvalid      Code = "PAIRING_CODE_INVALID"
	PairingCodeExpired      Code = "PAIRING_CODE_EXPIRED"
	PairingCodeUsed         Code = "PAIRING_CODE_USED"
	PairingIdentityConflict Code = "PAIRING_IDENTITY_CONFLICT"
	AuthenticationFailed    Code = "AUTHENTICATION_FAILED"
	VersionIncompatible     Code = "VERSION_INCOMPATIBLE"
	NoCandidates            Code = "NO_CANDIDATES"
	NoViableCandidate       Code = "NO_VIABLE_CANDIDATE"
	CheckTimeout            Code = "CHECK_TIMEOUT"
	ICEFailed               Code = "ICE_FAILED"
	QUICHandshakeTimeout    Code = "QUIC_HANDSHAKE_TIMEOUT"
	QUICHandshakeFailed     Code = "QUIC_HANDSHAKE_FAILED"
	DirectFailed            Code = "DIRECT_FAILED"
	RelayNotImplemented     Code = "RELAY_NOT_IMPLEMENTED"
	ReceiveRejected         Code = "RECEIVE_REJECTED"
	SourceChanged           Code = "SOURCE_CHANGED"
	IntegrityFailed         Code = "INTEGRITY_FAILED"
	DiskFull                Code = "DISK_FULL"
	PermissionDenied        Code = "PERMISSION_DENIED"
	FileConflict            Code = "FILE_CONFLICT"
	UnsafePath              Code = "UNSAFE_PATH"
	Cancelled               Code = "CANCELLED"
	InvalidMessage          Code = "INVALID_MESSAGE"
	Replay                  Code = "REPLAY"
	RateLimited             Code = "RATE_LIMITED"
	SessionConflict         Code = "SESSION_CONFLICT"
	ConnectionInterrupted   Code = "CONNECTION_INTERRUPTED"
	ResumeMismatch          Code = "RESUME_IDENTITY_MISMATCH"
)

type Error struct {
	Code   Code   `json:"code"`
	Detail string `json:"detail"`
	Cause  error  `json:"-"`
}

func (e *Error) Error() string         { return string(e.Code) + ": " + e.Detail }
func (e *Error) Unwrap() error         { return e.Cause }
func Fail(c Code, detail string) error { return &Error{Code: c, Detail: detail} }

// Wrap exposes a stable summary while retaining the private diagnostic cause.
func Wrap(c Code, detail string, cause error) error {
	return &Error{Code: c, Detail: detail, Cause: cause}
}

type Capabilities struct {
	ProtocolVersion          int    `json:"protocol_version"`
	ProductVersion           string `json:"product_version"`
	Relay                    bool   `json:"relay"`
	Transport                string `json:"transport"`
	HistoryPersisted         bool   `json:"history_persisted"`
	RestartRecoverySupported bool   `json:"restart_recovery_supported"`
	ByteResumeSupported      bool   `json:"byte_resume_supported"`
}

func Supported() Capabilities {
	return Capabilities{ProtocolVersion: Version, ProductVersion: ProductVersion, Relay: false, Transport: "quic"}
}

type Envelope struct {
	ProtocolVersion int             `json:"protocol_version"`
	Type            string          `json:"type"`
	MessageID       string          `json:"message_id"`
	SessionID       string          `json:"session_id,omitempty"`
	Sender          string          `json:"sender"`
	Recipient       string          `json:"recipient,omitempty"`
	Generation      uint64          `json:"generation,omitempty"`
	IssuedAt        int64           `json:"issued_at"`
	ExpiresAt       int64           `json:"expires_at"`
	Payload         json.RawMessage `json:"payload"`
	Signature       []byte          `json:"signature"`
}

func RandomID() string {
	var b [16]byte
	_, err := rand.Read(b[:])
	if err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}
func NewEnvelope(kind, sender, recipient, session string, generation uint64, payload any) (Envelope, error) {
	b, err := json.Marshal(payload)
	if err != nil {
		return Envelope{}, err
	}
	now := time.Now()
	return Envelope{ProtocolVersion: Version, Type: kind, MessageID: RandomID(), SessionID: session, Sender: sender, Recipient: recipient, Generation: generation, IssuedAt: now.Unix(), ExpiresAt: now.Add(time.Minute).Unix(), Payload: b}, nil
}

func field(b *bytes.Buffer, v string) {
	_ = binary.Write(b, binary.BigEndian, uint32(len(v)))
	b.WriteString(v)
}
func (e Envelope) SigningBytes() []byte {
	var b bytes.Buffer
	b.WriteString("LinkSend/Signal/1\x00")
	_ = binary.Write(&b, binary.BigEndian, uint32(e.ProtocolVersion))
	field(&b, e.Type)
	field(&b, e.MessageID)
	field(&b, e.SessionID)
	field(&b, e.Sender)
	field(&b, e.Recipient)
	_ = binary.Write(&b, binary.BigEndian, e.Generation)
	_ = binary.Write(&b, binary.BigEndian, e.IssuedAt)
	_ = binary.Write(&b, binary.BigEndian, e.ExpiresAt)
	h := sha256.Sum256(e.Payload)
	b.Write(h[:])
	return b.Bytes()
}
func validHex(s string, size int) bool {
	if len(s) != size {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}
func (e Envelope) Validate(now time.Time) error {
	if e.ProtocolVersion != Version {
		return Fail(VersionIncompatible, "unsupported control protocol")
	}
	if len(e.Payload) > MaxPayloadBytes || !json.Valid(e.Payload) || !validHex(e.MessageID, 32) || !validHex(e.Sender, 64) || !validHex(e.Recipient, 64) || !validHex(e.SessionID, 32) || e.Generation == 0 {
		return Fail(InvalidMessage, "invalid envelope fields")
	}
	switch e.Type {
	case "connect_request", "connect_response", "candidate", "end_of_candidates", "status":
	default:
		return Fail(InvalidMessage, "unknown critical message type")
	}
	if e.IssuedAt > now.Add(15*time.Second).Unix() || e.ExpiresAt < now.Unix() || e.ExpiresAt < e.IssuedAt || e.ExpiresAt-e.IssuedAt > 120 {
		return Fail(Replay, "expired or invalid freshness window")
	}
	if len(e.Signature) != ed25519.SignatureSize {
		return Fail(AuthenticationFailed, "missing message signature")
	}
	return nil
}
func (e Envelope) Verify(public ed25519.PublicKey, now time.Time) error {
	if err := e.Validate(now); err != nil {
		return err
	}
	h := sha256.Sum256(public)
	if len(public) != 32 || hex.EncodeToString(h[:]) != e.Sender || !ed25519.Verify(public, e.SigningBytes(), e.Signature) {
		return Fail(AuthenticationFailed, "message identity or signature mismatch")
	}
	return nil
}

type Candidate struct {
	Candidate string `json:"candidate"`
}

// ValidateCandidate performs only the admission policy. Pion still parses and
// authenticates STUN/ICE packets and performs connectivity checks.
func ValidateCandidate(candidate string, allowLoopback bool) error {
	if len(candidate) > 2048 {
		return Fail(InvalidMessage, "candidate too large")
	}
	parts := strings.Fields(candidate)
	if len(parts) < 8 || parts[2] != "udp" || parts[6] != "typ" {
		return Fail(InvalidMessage, "invalid UDP candidate")
	}
	if parts[7] != "host" && parts[7] != "srflx" && parts[7] != "prflx" {
		return Fail(RelayNotImplemented, "only direct ICE candidates are supported")
	}
	a, err := netip.ParseAddr(parts[4])
	if err != nil || a.Zone() != "" || a.IsUnspecified() || a.IsMulticast() || a.IsLinkLocalUnicast() {
		return Fail(InvalidMessage, "invalid candidate address or scope")
	}
	if a.IsLoopback() && !allowLoopback {
		return Fail(InvalidMessage, "loopback candidates require explicit test configuration")
	}
	if a.Is4() {
		v := a.As4()
		if v[0] == 0 || v[0] >= 224 || v[3] == 255 {
			return Fail(InvalidMessage, "broadcast/reserved candidate address")
		}
	}
	port, err := strconv.Atoi(parts[5])
	if err != nil || port < 1 || port > 65535 {
		return Fail(InvalidMessage, "invalid candidate port")
	}
	return nil
}

func AuthBytes(method, path, device, nonce string, at int64, body []byte) []byte {
	var b bytes.Buffer
	b.WriteString("LinkSend/HTTP/1\x00")
	field(&b, method)
	field(&b, path)
	field(&b, device)
	field(&b, nonce)
	_ = binary.Write(&b, binary.BigEndian, at)
	sum := sha256.Sum256(body)
	b.Write(sum[:])
	return b.Bytes()
}
func ChallengeBytes(device, nonce string) []byte {
	var b bytes.Buffer
	b.WriteString("LinkSend/WebSocket/1\x00")
	field(&b, device)
	field(&b, nonce)
	return b.Bytes()
}

// ReplayWindow bounds both time and memory. A full window rejects admission;
// eviction never makes an unexpired nonce replayable.
type ReplayWindow struct {
	mu    sync.Mutex
	seen  map[string]time.Time
	limit int
}

func NewReplayWindow(limit int) *ReplayWindow {
	return &ReplayWindow{seen: map[string]time.Time{}, limit: limit}
}
func (w *ReplayWindow) Use(id string, expiry, now time.Time) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	for k, v := range w.seen {
		if !v.After(now) {
			delete(w.seen, k)
		}
	}
	if _, ok := w.seen[id]; ok {
		return Fail(Replay, "message already processed")
	}
	if len(w.seen) >= w.limit {
		return Fail(RateLimited, "replay window capacity reached")
	}
	w.seen[id] = expiry
	return nil
}

type StateMachine struct {
	mu    sync.Mutex
	state string
	kind  string
}

func NewConnectionState() *StateMachine { return &StateMachine{state: "Idle", kind: "connection"} }
func NewTransferState() *StateMachine   { return &StateMachine{state: "Preparing", kind: "transfer"} }
func (s *StateMachine) State() string   { s.mu.Lock(); defer s.mu.Unlock(); return s.state }
func (s *StateMachine) Transition(next string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var allowed map[string][]string
	if s.kind == "connection" {
		allowed = map[string][]string{"Idle": {"Gathering", "Closed"}, "Gathering": {"Signaling", "Checking", "Failed", "Closed"}, "Signaling": {"Gathering", "Checking", "Failed", "Closed"}, "Checking": {"Nominating", "Failed", "Closed"}, "Nominating": {"Authenticating", "Failed", "Closed"}, "Authenticating": {"Connected", "Failed", "Closed"}, "Connected": {"Reconnecting", "Failed", "Closed"}, "Reconnecting": {"Gathering", "Failed", "Closed"}, "Failed": {"Reconnecting", "Closed"}}
	} else {
		allowed = map[string][]string{"Preparing": {"AwaitingAcceptance", "Failed", "Cancelled"}, "AwaitingAcceptance": {"Transferring", "Rejected", "Failed", "Cancelled", "Paused"}, "Transferring": {"Verifying", "Paused", "Recovering", "Cancelled", "Failed"}, "Verifying": {"Completed", "Failed", "Cancelled"}, "Paused": {"Recovering", "Cancelled"}, "Recovering": {"AwaitingAcceptance", "Transferring", "Paused", "Failed", "Cancelled"}, "Failed": {"Recovering", "Cancelled"}}
	}
	for _, n := range allowed[s.state] {
		if n == next {
			s.state = next
			return nil
		}
	}
	return fmt.Errorf("invalid %s transition %s -> %s", s.kind, s.state, next)
}

func ErrorCode(err error) Code {
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	return DirectFailed
}
