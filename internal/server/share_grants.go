package server

import (
	"bytes"
	"crypto/ed25519"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/Wen5555/LinkSend/internal/identity"
	"github.com/Wen5555/LinkSend/internal/protocol"
	"github.com/Wen5555/LinkSend/internal/sharegrant"
)

// ShareRequest uses only bounded authorization/coordination metadata. File
// names, file bytes, disk paths and group selectors have no accepted field.
type ShareRequest struct {
	ID          string `json:"id,omitempty"`
	Code        string `json:"code,omitempty"`
	CodeDigest  string `json:"code_digest,omitempty"`
	Manifest    string `json:"manifest_digest,omitempty"`
	Plan        string `json:"plan_digest,omitempty"`
	ClaimID     string `json:"claim_id,omitempty"`
	ApprovalID  string `json:"approval_id,omitempty"`
	TaskID      string `json:"task_id,omitempty"`
	Generation  uint64 `json:"generation,omitempty"`
	ReceiverKey string `json:"receiver_public_key,omitempty"`
}

// ShareStatus is a scoped network DTO. It intentionally omits the credential
// verifier, private timestamps/metadata, membership and other grant actors.
type ShareStatus struct {
	ID          string           `json:"id"`
	State       sharegrant.State `json:"state"`
	Generation  uint64           `json:"generation"`
	Revision    uint64           `json:"revision"`
	SenderKey   string           `json:"sender_public_key"`
	ReceiverKey string           `json:"receiver_public_key,omitempty"`
	Manifest    string           `json:"manifest_digest"`
	Plan        string           `json:"plan_digest,omitempty"`
	ClaimID     string           `json:"claim_id,omitempty"`
	TaskID      string           `json:"task_id,omitempty"`
	ExpiresAt   int64            `json:"expires_at"`
	ClaimUntil  int64            `json:"claim_until,omitempty"`
	ResumeUntil int64            `json:"resume_until,omitempty"`
}

func shareStatus(r sharegrant.Record) ShareStatus {
	status := ShareStatus{ID: r.ID, State: r.State, Generation: r.Generation, Revision: r.Revision, SenderKey: base64.RawStdEncoding.EncodeToString(r.Sender[:]), Manifest: hex.EncodeToString(r.Manifest[:]), ClaimID: r.ClaimID, TaskID: r.TaskID, ExpiresAt: r.ExpiresAt, ClaimUntil: r.ClaimUntil, ResumeUntil: r.ResumeUntil}
	if r.Receiver != (sharegrant.Key{}) {
		status.ReceiverKey = base64.RawStdEncoding.EncodeToString(r.Receiver[:])
	}
	if r.Plan != (sharegrant.Digest{}) {
		status.Plan = hex.EncodeToString(r.Plan[:])
	}
	return status
}
func decodeShareDigest(value string) (sharegrant.Digest, error) {
	if len(value) != 64 {
		return sharegrant.Digest{}, sharegrant.ErrInvalid
	}
	raw, err := hex.DecodeString(value)
	if err != nil || len(raw) != 32 {
		return sharegrant.Digest{}, sharegrant.ErrInvalid
	}
	return sharegrant.Digest(raw), nil
}
func decodeShareKey(value string) (sharegrant.Key, error) {
	if len(value) != 43 {
		return sharegrant.Key{}, sharegrant.ErrInvalid
	}
	raw, err := base64.RawStdEncoding.DecodeString(value)
	if err != nil || len(raw) != 32 {
		return sharegrant.Key{}, sharegrant.ErrInvalid
	}
	key := sharegrant.Key(raw)
	if key == (sharegrant.Key{}) {
		return key, sharegrant.ErrInvalid
	}
	return key, nil
}

// A public unpredictable boot challenge binds otherwise normal AuthBytes
// signatures to this server run. After restart old HTTP proofs are rejected;
// durable grants can be resumed by fetching the new challenge and signing a
// fresh request. It is not a bearer token or a replacement for key verification.
func (s *Server) authenticateShare(w http.ResponseWriter, r *http.Request, body []byte, now time.Time) (sharegrant.Key, bool) {
	key, keyErr := decodeShareKey(r.Header.Get("X-LinkSend-Public-Key"))
	id, nonce, epoch := r.Header.Get("X-LinkSend-Device"), r.Header.Get("X-LinkSend-Nonce"), r.Header.Get("X-LinkSend-Share-Epoch")
	at, timeErr := strconv.ParseInt(r.Header.Get("X-LinkSend-Time"), 10, 64)
	signature, sigErr := base64.RawStdEncoding.DecodeString(r.Header.Get("X-LinkSend-Signature"))
	nonceBytes, nonceErr := hex.DecodeString(nonce)
	if keyErr != nil || timeErr != nil || sigErr != nil || len(signature) != ed25519.SignatureSize || nonceErr != nil || len(nonce) != 32 || len(nonceBytes) != 16 || len(epoch) != 32 || subtle.ConstantTimeCompare([]byte(epoch), []byte(s.shareEpoch)) != 1 || at < now.Add(-time.Minute).Unix() || at > now.Add(15*time.Second).Unix() || id != identity.DeviceID(key[:]) {
		reject(w, 401, protocol.AuthenticationFailed, "invalid share request authentication")
		return sharegrant.Key{}, false
	}
	if !ed25519.Verify(key[:], protocol.AuthBytes(r.Method, r.URL.RequestURI(), id, epoch+":"+nonce, at, body), signature) {
		reject(w, 401, protocol.AuthenticationFailed, "invalid share request authentication")
		return sharegrant.Key{}, false
	}
	if err := s.replays.Use("share-http:"+epoch+":"+id+":"+nonce, now.Add(2*time.Minute), now); err != nil {
		reject(w, 409, protocol.Replay, "share request replayed")
		return sharegrant.Key{}, false
	}
	return key, true
}

func (s *Server) admitShare(key string, burst, perSecond float64, now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, exists := s.shareBuckets[key]
	if !exists {
		if len(s.shareBuckets) >= 2048 {
			for k, v := range s.shareBuckets {
				if now.Sub(v.at) > 5*time.Minute {
					delete(s.shareBuckets, k)
				}
			}
			if len(s.shareBuckets) >= 2048 {
				return false
			}
		}
		b = bucket{tokens: burst, at: now}
	}
	if now.After(b.at) {
		b.tokens = min(burst, b.tokens+now.Sub(b.at).Seconds()*perSecond)
		b.at = now
	}
	allowed := b.tokens >= 1
	if allowed {
		b.tokens--
	}
	s.shareBuckets[key] = b
	return allowed
}

func shareFailure(w http.ResponseWriter, err error, claim bool) {
	if claim && (errors.Is(err, sharegrant.ErrRejected) || errors.Is(err, sharegrant.ErrConflict) || errors.Is(err, sharegrant.ErrExpired) || errors.Is(err, sharegrant.ErrTerminal)) {
		reject(w, 403, protocol.ShareRejected, "share unavailable for this claim")
		return
	}
	switch {
	case errors.Is(err, sharegrant.ErrInvalid):
		reject(w, 400, protocol.ShareInvalid, "invalid share request")
	case errors.Is(err, sharegrant.ErrRejected):
		reject(w, 403, protocol.ShareRejected, "share action not authorized")
	case errors.Is(err, sharegrant.ErrExpired):
		reject(w, 410, protocol.ShareExpired, "share authorization expired")
	case errors.Is(err, sharegrant.ErrConflict):
		reject(w, 409, protocol.ShareConflict, "share state changed; refresh before confirming")
	case errors.Is(err, sharegrant.ErrTerminal):
		reject(w, 409, protocol.ShareTerminal, "share has already ended")
	default:
		reject(w, 503, protocol.ShareUnavailable, "share store unavailable")
	}
}

func (s *Server) shareControl(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.ExperimentalShareControl || r.Header.Get("X-LinkSend-Share-Grants") != "1" {
		reject(w, http.StatusUpgradeRequired, protocol.ShareUnsupported, "temporary share control not enabled")
		return
	}
	now := time.Now()
	operation := r.PathValue("operation")
	if operation == "claim" {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil || !s.admitShare("ip:"+host, 20, 1.0/6, now) {
			reject(w, 429, protocol.RateLimited, "share claim rate limit")
			return
		}
	}
	body, err := readBody(w, r)
	if err != nil {
		reject(w, 400, protocol.ShareInvalid, "share body limit exceeded")
		return
	}
	actor, ok := s.authenticateShare(w, r, body, now)
	if !ok {
		return
	}
	if operation == "claim" && !s.admitShare("actor:"+identity.DeviceID(actor[:]), 10, 1.0/6, now) {
		reject(w, 429, protocol.RateLimited, "share claim rate limit")
		return
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var request ShareRequest
	if err = decoder.Decode(&request); err != nil {
		shareFailure(w, sharegrant.ErrInvalid, false)
		return
	}
	if err = decoder.Decode(new(any)); err != io.EOF {
		shareFailure(w, sharegrant.ErrInvalid, false)
		return
	}
	if len(request.ID) > 32 || len(request.Code) > 39 || len(request.CodeDigest) > 64 || len(request.Manifest) > 64 || len(request.Plan) > 64 || len(request.ClaimID) > 32 || len(request.ApprovalID) > 32 || len(request.TaskID) > 32 || len(request.ReceiverKey) > 43 {
		shareFailure(w, sharegrant.ErrInvalid, false)
		return
	}
	ctx := r.Context()
	var record sharegrant.Record
	switch operation {
	case "create":
		var manifest, digest sharegrant.Digest
		manifest, err = decodeShareDigest(request.Manifest)
		if err != nil {
			break
		}
		digest, err = decodeShareDigest(request.CodeDigest)
		if err != nil {
			break
		}
		record, err = sharegrant.New(request.ID, actor, manifest, digest, now)
		if err != nil {
			break
		}
		record, err = s.store.CreateShare(ctx, record, now)
	case "claim":
		record, err = s.store.ClaimShare(ctx, actor, request.Code, request.ClaimID, now)
	case "status":
		record, err = s.store.ReadShare(ctx, request.ID, actor)
	case "approve":
		var expected sharegrant.Key
		expected, err = decodeShareKey(request.ReceiverKey)
		if err != nil {
			break
		}
		record, err = s.store.ReadShare(ctx, request.ID, actor)
		if err != nil {
			break
		}
		var manifest sharegrant.Digest
		manifest, err = decodeShareDigest(request.Manifest)
		if err != nil {
			break
		}
		if record.Receiver != expected || record.Manifest != manifest {
			err = sharegrant.ErrRejected
			break
		}
		record, err = s.store.ApproveShare(ctx, request.ID, actor, request.ClaimID, request.ApprovalID, request.TaskID, request.Generation, now)
	case "deny":
		record, err = s.store.DenyShare(ctx, request.ID, actor, request.ClaimID, request.Generation, now)
	case "bind", "authorize", "complete":
		var manifest, plan sharegrant.Digest
		manifest, err = decodeShareDigest(request.Manifest)
		if err != nil {
			break
		}
		plan, err = decodeShareDigest(request.Plan)
		if err != nil {
			break
		}
		if operation == "bind" {
			record, err = s.store.BindShare(ctx, request.ID, actor, request.TaskID, manifest, plan, request.Generation, now)
		} else if operation == "authorize" {
			record, err = s.store.AuthorizeShare(ctx, request.ID, actor, request.TaskID, manifest, plan, request.Generation, now)
		} else {
			record, err = s.store.CompleteShare(ctx, request.ID, actor, request.TaskID, manifest, plan, request.Generation, now)
		}
	case "revoke":
		record, err = s.store.RevokeShare(ctx, request.ID, actor, now)
	default:
		shareFailure(w, sharegrant.ErrInvalid, false)
		return
	}
	if err != nil {
		shareFailure(w, err, operation == "claim")
		return
	}
	respond(w, http.StatusOK, shareStatus(record))
}
