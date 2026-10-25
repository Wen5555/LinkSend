package sharegrant

import (
	"bytes"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Wen5555/LinkSend/internal/identity"
	"github.com/Wen5555/LinkSend/internal/protocol"
)

const SignalSubprotocol = "linksend.share-signal.v1"

type SignalPhase string

const (
	SignalPreview  SignalPhase = "preview"
	SignalTransfer SignalPhase = "transfer"
)

// SignalScope is an immutable, one-task authorization binding. The grant
// generation is independent of the ICE generation inside Envelope.
type SignalScope struct {
	GrantID         string      `json:"grant_id"`
	ClaimID         string      `json:"claim_id"`
	TaskID          string      `json:"task_id"`
	GrantGeneration uint64      `json:"grant_generation"`
	Phase           SignalPhase `json:"phase"`
	Manifest        string      `json:"manifest_digest"`
	Plan            string      `json:"plan_digest,omitempty"`
}

func signalDigest(value string) bool {
	if len(value) != 64 || value != strings.ToLower(value) {
		return false
	}
	b, err := hex.DecodeString(value)
	return err == nil && len(b) == 32 && !bytes.Equal(b, make([]byte, 32))
}

func (s SignalScope) Validate() error {
	if !validID(s.GrantID) || !validID(s.ClaimID) || !validID(s.TaskID) || s.GrantGeneration == 0 || s.GrantGeneration > 1<<63-1 || !signalDigest(s.Manifest) {
		return ErrInvalid
	}
	switch s.Phase {
	case SignalPreview:
		if s.Plan != "" {
			return ErrInvalid
		}
	case SignalTransfer:
		if !signalDigest(s.Plan) {
			return ErrInvalid
		}
	default:
		return ErrInvalid
	}
	return nil
}

func (s SignalScope) Path() string { return "/v1/shares/" + s.GrantID + "/ws" }

func (s SignalScope) RequestURI() string {
	values := url.Values{"claim_id": {s.ClaimID}, "task_id": {s.TaskID}, "generation": {strconv.FormatUint(s.GrantGeneration, 10)}, "phase": {string(s.Phase)}, "manifest_digest": {s.Manifest}}
	if s.Plan != "" {
		values.Set("plan_digest", s.Plan)
	}
	return s.Path() + "?" + values.Encode()
}

func ParseSignalScope(grantID, rawQuery string) (SignalScope, error) {
	if len(rawQuery) > 512 {
		return SignalScope{}, ErrInvalid
	}
	values, err := url.ParseQuery(rawQuery)
	if err != nil {
		return SignalScope{}, ErrInvalid
	}
	for key, list := range values {
		if len(list) != 1 {
			return SignalScope{}, ErrInvalid
		}
		switch key {
		case "claim_id", "task_id", "generation", "phase", "manifest_digest", "plan_digest":
		default:
			return SignalScope{}, ErrInvalid
		}
	}
	generation, err := strconv.ParseUint(values.Get("generation"), 10, 63)
	if err != nil || strconv.FormatUint(generation, 10) != values.Get("generation") {
		return SignalScope{}, ErrInvalid
	}
	s := SignalScope{GrantID: grantID, ClaimID: values.Get("claim_id"), TaskID: values.Get("task_id"), GrantGeneration: generation, Phase: SignalPhase(values.Get("phase")), Manifest: values.Get("manifest_digest"), Plan: values.Get("plan_digest")}
	if err = s.Validate(); err != nil {
		return SignalScope{}, err
	}
	// One canonical spelling is signed and compared at both endpoints.
	if s.RequestURI() != s.Path()+"?"+rawQuery {
		return SignalScope{}, ErrInvalid
	}
	return s, nil
}

func (s SignalScope) Authorize(r Record, actor Key, now time.Time) error {
	if err := s.Validate(); err != nil {
		return err
	}
	if err := r.Validate(); err != nil {
		return err
	}
	if s.GrantID != r.ID || s.ClaimID != r.ClaimID || s.TaskID != r.TaskID || s.GrantGeneration != r.Generation || s.Manifest != hex.EncodeToString(r.Manifest[:]) || actor == (Key{}) || (actor != r.Sender && actor != r.Receiver) {
		return ErrRejected
	}
	if now.Unix() < r.CreatedAt {
		return ErrExpired
	}
	if s.Phase == SignalPreview {
		if r.State != Approved {
			return ErrTerminal
		}
		if now.Unix() >= r.ExpiresAt {
			return ErrExpired
		}
		return nil
	}
	if s.Plan != hex.EncodeToString(r.Plan[:]) {
		return ErrRejected
	}
	return r.Authorize(actor, s.TaskID, r.Manifest, r.Plan, s.GrantGeneration, now)
}

// SignalFrame preserves the original signed Envelope. The outer proof binds
// that envelope to this grant, task, phase and server run using AuthBytes.
// ready/error are server-only TLS messages; clients may send signal/heartbeat.
type SignalFrame struct {
	Version   int                `json:"version"`
	Type      string             `json:"type"`
	Scope     SignalScope        `json:"scope"`
	ActorID   string             `json:"actor_id"`
	Epoch     string             `json:"epoch"`
	Nonce     string             `json:"nonce,omitempty"`
	IssuedAt  int64              `json:"issued_at,omitempty"`
	ExpiresAt int64              `json:"expires_at,omitempty"`
	Message   *protocol.Envelope `json:"message,omitempty"`
	Signature []byte             `json:"signature,omitempty"`
	PeerReady bool               `json:"peer_ready,omitempty"`
	Code      protocol.Code      `json:"code,omitempty"`
}

func NewSignalFrame(scope SignalScope, epoch, actorID, kind string, message *protocol.Envelope, now time.Time) (SignalFrame, error) {
	f := SignalFrame{Version: 1, Type: kind, Scope: scope, ActorID: actorID, Epoch: epoch, Nonce: protocol.RandomID(), IssuedAt: now.Unix(), ExpiresAt: now.Add(time.Minute).Unix(), Message: message}
	if scope.Validate() != nil || !validID(epoch) || len(actorID) != 64 || (kind != "signal" && kind != "heartbeat") || (kind == "signal") != (message != nil) {
		return SignalFrame{}, ErrInvalid
	}
	return f, nil
}

func (f SignalFrame) SigningBytes() []byte {
	type unsigned SignalFrame
	u := unsigned(f)
	u.Signature = nil
	body, _ := json.Marshal(u)
	return protocol.AuthBytes("LINKSEND-SHARE-SIGNAL", f.Scope.Path(), f.ActorID, f.Epoch+":"+f.Nonce, f.IssuedAt, body)
}

func DecodeSignalFrame(raw []byte) (SignalFrame, error) {
	var f SignalFrame
	if len(raw) == 0 || len(raw) > protocol.MaxMessageBytes || strictSignalJSON(raw, &f) != nil {
		return f, ErrInvalid
	}
	return f, nil
}

func (f SignalFrame) Verify(public ed25519.PublicKey, expectedScope SignalScope, epoch string, now time.Time, allowLoopback bool) error {
	if f.Version != 1 || f.Scope != expectedScope || f.Scope.Validate() != nil || !validID(f.Epoch) || f.Epoch != epoch || !validID(f.Nonce) || len(public) != ed25519.PublicKeySize || identity.DeviceID(public) != f.ActorID || f.PeerReady || f.Code != "" || f.IssuedAt > now.Add(15*time.Second).Unix() || f.ExpiresAt <= now.Unix() || f.ExpiresAt < f.IssuedAt || f.ExpiresAt-f.IssuedAt > 60 || len(f.Signature) != ed25519.SignatureSize {
		return ErrRejected
	}
	switch f.Type {
	case "signal":
		if f.Message == nil || f.Message.Sender != f.ActorID {
			return ErrRejected
		}
		if err := f.Message.Verify(public, now); err != nil {
			return ErrRejected
		}
		if err := ValidateSignalPayload(*f.Message, allowLoopback); err != nil {
			return err
		}
	case "heartbeat":
		if f.Message != nil {
			return ErrInvalid
		}
	default:
		return ErrInvalid
	}
	encoded, err := json.Marshal(f)
	if err != nil || len(encoded) > protocol.MaxMessageBytes || !ed25519.Verify(public, f.SigningBytes(), f.Signature) {
		return ErrRejected
	}
	return nil
}

func strictSignalJSON(body []byte, out any) error {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) < 2 || trimmed[0] != '{' || trimmed[len(trimmed)-1] != '}' {
		return ErrInvalid
	}
	keys := json.NewDecoder(bytes.NewReader(body))
	if uniqueSignalJSON(keys, 0) != nil {
		return ErrInvalid
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out); err != nil {
		return ErrInvalid
	}
	if decoder.Decode(new(any)) != io.EOF {
		return ErrInvalid
	}
	return nil
}

func uniqueSignalJSON(decoder *json.Decoder, depth int) error {
	if depth > 8 {
		return ErrInvalid
	}
	token, err := decoder.Token()
	if err != nil {
		return ErrInvalid
	}
	if delimiter, ok := token.(json.Delim); ok {
		if delimiter != '{' { // No accepted signal structure contains an array.
			return ErrInvalid
		}
		var seen []string
		for decoder.More() {
			token, err = decoder.Token()
			key, ok := token.(string)
			if err != nil || !ok || len(key) > 128 || len(seen) >= 32 {
				return ErrInvalid
			}
			for _, previous := range seen {
				if strings.EqualFold(previous, key) {
					return ErrInvalid
				}
			}
			seen = append(seen, key)
			if uniqueSignalJSON(decoder, depth+1) != nil {
				return ErrInvalid
			}
		}
		if token, err = decoder.Token(); err != nil || token != json.Delim('}') {
			return ErrInvalid
		}
	}
	return nil
}

// ValidateSignalPayload is a metadata allowlist, not a file forwarding API.
func ValidateSignalPayload(e protocol.Envelope, allowLoopback bool) error {
	if len(e.Payload) > protocol.MaxPayloadBytes {
		return ErrInvalid
	}
	switch e.Type {
	case "connect_request", "connect_response":
		var description struct {
			Ufrag                   string `json:"ufrag"`
			Password                string `json:"password"`
			SessionReuse            bool   `json:"session_reuse,omitempty"`
			ClipboardSync           bool   `json:"clipboard_sync,omitempty"`
			ClipboardFiles          bool   `json:"clipboard_files_v1,omitempty"`
			AuthorizationGeneration uint64 `json:"authorization_generation,omitempty"`
		}
		if strictSignalJSON(e.Payload, &description) != nil || description.SessionReuse || description.ClipboardSync || description.ClipboardFiles || description.AuthorizationGeneration != 0 {
			return ErrInvalid
		}
		for _, value := range []string{description.Ufrag, description.Password} {
			if value == "" || len(value) > 256 {
				return ErrInvalid
			}
			for i := range value {
				if value[i] < 33 || value[i] > 126 {
					return ErrInvalid
				}
			}
		}
	case "candidate":
		var candidate protocol.Candidate
		if strictSignalJSON(e.Payload, &candidate) != nil || protocol.ValidateCandidate(candidate.Candidate, allowLoopback) != nil {
			return ErrInvalid
		}
	case "end_of_candidates":
		if strictSignalJSON(e.Payload, &struct{}{}) != nil {
			return ErrInvalid
		}
	case "status":
		var status struct {
			State string        `json:"state"`
			Code  protocol.Code `json:"code"`
		}
		if strictSignalJSON(e.Payload, &status) != nil || status.State != "failed" || !SignalFailureCode(status.Code) {
			return ErrInvalid
		}
	default:
		return ErrInvalid
	}
	return nil
}

func SignalFailureCode(code protocol.Code) bool {
	switch code {
	case protocol.CandidateTimeout, protocol.NoCandidates, protocol.NoViableCandidate, protocol.CheckTimeout, protocol.ICEFailed, protocol.QUICHandshakeTimeout, protocol.QUICHandshakeFailed, protocol.AuthenticationFailed, protocol.DirectFailed:
		return true
	default:
		return false
	}
}
