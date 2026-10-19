// Package sharegrant defines the one-task authorization state machine. It does
// not grant group membership, expose an HTTP handler, or transfer file bytes.
// Callers must authenticate actors before calling transitions and persist the
// returned record with a revision CAS in their existing database transaction.
package sharegrant

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"
)

const (
	Version        = 1
	OfferTTL       = 10 * time.Minute
	ClaimTTL       = 2 * time.Minute
	ResumeTTL      = 24 * time.Hour
	MaxRecordBytes = 8192
)

var (
	ErrInvalid  = errors.New("SHARE_INVALID")
	ErrRejected = errors.New("SHARE_REJECTED")
	ErrExpired  = errors.New("SHARE_EXPIRED")
	ErrConflict = errors.New("SHARE_CONFLICT")
	ErrTerminal = errors.New("SHARE_TERMINAL")
)

type Key [32]byte
type Digest [32]byte
type State string

const (
	Offered   State = "offered"
	Pending   State = "pending"
	Approved  State = "approved"
	Bound     State = "bound"
	Completed State = "completed"
	Revoked   State = "revoked"
	Expired   State = "expired"
)

// Record is private control-plane storage, not a frontend or diagnostic DTO.
// CodeDigest is a one-way credential verifier; the plaintext code is never stored.
type Record struct {
	Version       int    `json:"version"`
	ID            string `json:"id"`
	Sender        Key    `json:"sender"`
	Manifest      Digest `json:"manifest"`
	CodeDigest    Digest `json:"code_digest"`
	State         State  `json:"state"`
	Generation    uint64 `json:"generation"`
	Revision      uint64 `json:"revision"`
	CreatedAt     int64  `json:"created_at"`
	ExpiresAt     int64  `json:"expires_at"`
	Receiver      Key    `json:"receiver"`
	ClaimID       string `json:"claim_id,omitempty"`
	ClaimUntil    int64  `json:"claim_until,omitempty"`
	ApprovalID    string `json:"approval_id,omitempty"`
	TaskID        string `json:"task_id,omitempty"`
	Plan          Digest `json:"plan"`
	ResumeUntil   int64  `json:"resume_until,omitempty"`
	FinishedAt    int64  `json:"finished_at,omitempty"`
	DeniedClaimID string `json:"denied_claim_id,omitempty"`
}

func validID(id string) bool {
	if len(id) != 32 {
		return false
	}
	decoded, err := hex.DecodeString(id)
	return err == nil && len(decoded) == 16 && strings.ToLower(id) == id
}
func nonzero[T ~[32]byte](v T) bool { return v != T{} }

// GenerateCode is client-side. Only its digest is submitted when creating an
// offer; retrying creation can reuse the client's original ID and digest.
func GenerateCode() (string, Digest, error) {
	var raw [20]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", Digest{}, err
	}
	code := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw[:])
	digest, _ := CodeDigest(code)
	return code, digest, nil
}
func CodeDigest(code string) (Digest, error) {
	if len(code) > 39 {
		return Digest{}, ErrInvalid
	}
	var compact strings.Builder
	for i := 0; i < len(code); i++ {
		c := code[i]
		if len(code) == 39 && i%5 == 4 {
			if c != '-' && c != ' ' {
				return Digest{}, ErrInvalid
			}
			continue
		}
		if c >= 'a' && c <= 'z' {
			c -= 32
		}
		if !(c >= 'A' && c <= 'Z' || c >= '2' && c <= '7') {
			return Digest{}, ErrInvalid
		}
		compact.WriteByte(c)
	}
	if compact.Len() != 32 {
		return Digest{}, ErrInvalid
	}
	return sha256.Sum256([]byte("LinkSend/ShareCode/1\x00" + compact.String())), nil
}

func New(id string, sender Key, manifest, code Digest, now time.Time) (Record, error) {
	if !validID(id) || !nonzero(sender) || !nonzero(manifest) || !nonzero(code) || now.Unix() <= 0 {
		return Record{}, ErrInvalid
	}
	r := Record{Version: Version, ID: id, Sender: sender, Manifest: manifest, CodeDigest: code, State: Offered, Generation: 1, Revision: 1, CreatedAt: now.Unix(), ExpiresAt: now.Add(OfferTTL).Unix()}
	return r, r.Validate()
}
func (r Record) Terminal() bool {
	return r.State == Completed || r.State == Revoked || r.State == Expired
}
func (r Record) Validate() error {
	if r.Version != Version || !validID(r.ID) || !nonzero(r.Sender) || !nonzero(r.Manifest) || !nonzero(r.CodeDigest) || r.Generation == 0 || r.Revision == 0 || r.Generation > 1<<63-1 || r.Revision > 1<<63-1 || r.CreatedAt <= 0 || r.CreatedAt > 7258118400 || r.ExpiresAt-r.CreatedAt != int64(OfferTTL/time.Second) {
		return ErrInvalid
	}
	switch r.State {
	case Offered, Pending, Approved, Bound, Completed, Revoked, Expired:
	default:
		return ErrInvalid
	}
	if r.ClaimID != "" && (!validID(r.ClaimID) || !nonzero(r.Receiver) || r.Receiver == r.Sender || r.ClaimUntil <= r.CreatedAt || r.ClaimUntil > r.ExpiresAt) {
		return ErrInvalid
	}
	if r.DeniedClaimID != "" && !validID(r.DeniedClaimID) {
		return ErrInvalid
	}
	if r.State == Pending || r.State == Approved || r.State == Bound || r.State == Completed {
		if r.ClaimID == "" {
			return ErrInvalid
		}
	}
	if r.State == Approved || r.State == Bound || r.State == Completed {
		if !validID(r.ApprovalID) || !validID(r.TaskID) {
			return ErrInvalid
		}
	}
	if r.State == Bound || r.State == Completed {
		if !nonzero(r.Plan) || r.ResumeUntil <= r.CreatedAt || r.ResumeUntil > r.ExpiresAt+int64(ResumeTTL/time.Second) {
			return ErrInvalid
		}
	}
	if r.State == Pending && (r.ApprovalID != "" || r.TaskID != "" || nonzero(r.Plan) || r.ResumeUntil != 0) {
		return ErrInvalid
	}
	if r.State == Approved && (nonzero(r.Plan) || r.ResumeUntil != 0) {
		return ErrInvalid
	}
	if (r.State == Bound || r.State == Completed) && r.ResumeUntil < r.CreatedAt+int64(ResumeTTL/time.Second) {
		return ErrInvalid
	}
	if r.Terminal() && r.FinishedAt < r.CreatedAt {
		return ErrInvalid
	}
	if !r.Terminal() && r.FinishedAt != 0 {
		return ErrInvalid
	}
	if r.State == Offered && (r.ClaimID != "" || nonzero(r.Receiver) || r.ClaimUntil != 0 || r.ApprovalID != "" || r.TaskID != "" || nonzero(r.Plan) || r.ResumeUntil != 0) {
		return ErrInvalid
	}
	return nil
}

func Decode(raw []byte) (Record, error) {
	if len(raw) > MaxRecordBytes {
		return Record{}, ErrInvalid
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var r Record
	if err := decoder.Decode(&r); err != nil {
		return Record{}, ErrInvalid
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return Record{}, ErrInvalid
	}
	if err := r.Validate(); err != nil {
		return Record{}, err
	}
	return r, nil
}
func (r Record) check(now time.Time) error {
	if err := r.Validate(); err != nil {
		return err
	}
	// Rollback cannot extend authorization into a pre-creation clock era.
	if now.Unix() < r.CreatedAt {
		return ErrExpired
	}
	return nil
}
func (r *Record) bump() error {
	if r.Revision >= 1<<63-1 {
		return ErrInvalid
	}
	r.Revision++
	return nil
}
func (r Record) liveOffer(now time.Time) error {
	if err := r.check(now); err != nil {
		return err
	}
	if r.Terminal() {
		return ErrTerminal
	}
	if now.Unix() >= r.ExpiresAt {
		return ErrExpired
	}
	return nil
}

func (r Record) Claim(actor Key, code, claimID string, now time.Time) (Record, error) {
	digest, err := CodeDigest(code)
	if err != nil || subtle.ConstantTimeCompare(digest[:], r.CodeDigest[:]) != 1 || !nonzero(actor) || actor == r.Sender || !validID(claimID) {
		return r, ErrRejected
	}
	if err = r.check(now); err != nil {
		return r, err
	}
	// A retry can learn only this claimant's existing outcome. It never grants
	// another transfer, renews a deadline or reopens a terminal record.
	if r.ClaimID == claimID && r.Receiver == actor && (r.State == Approved || r.State == Bound || r.State == Completed) {
		return r, nil
	}
	if err = r.liveOffer(now); err != nil {
		return r, err
	}
	if r.State == Pending && now.Unix() < r.ClaimUntil {
		if r.ClaimID == claimID && r.Receiver == actor {
			return r, nil
		}
		return r, ErrConflict
	}
	if r.State != Offered && r.State != Pending {
		return r, ErrConflict
	}
	next := r
	if r.State == Pending {
		if next.Generation >= 1<<63-1 {
			return r, ErrInvalid
		}
		next.Generation++
	}
	next.State = Pending
	next.Receiver = actor
	next.ClaimID = claimID
	next.ClaimUntil = min(now.Add(ClaimTTL).Unix(), r.ExpiresAt)
	if err = next.bump(); err != nil {
		return r, err
	}
	return next, nil
}

func (r Record) Approve(actor Key, claimID, approvalID, taskID string, generation uint64, now time.Time) (Record, error) {
	if actor != r.Sender || claimID != r.ClaimID || generation != r.Generation || !validID(approvalID) || !validID(taskID) {
		return r, ErrRejected
	}
	if err := r.check(now); err != nil {
		return r, err
	}
	if (r.State == Approved || r.State == Bound || r.State == Completed) && r.ApprovalID == approvalID && r.TaskID == taskID {
		return r, nil
	}
	if err := r.liveOffer(now); err != nil {
		return r, err
	}
	if r.State != Pending || now.Unix() >= r.ClaimUntil {
		return r, ErrConflict
	}
	next := r
	next.State = Approved
	next.ApprovalID = approvalID
	next.TaskID = taskID
	if err := next.bump(); err != nil {
		return r, err
	}
	return next, nil
}

func (r Record) Deny(actor Key, claimID string, generation uint64, now time.Time) (Record, error) {
	if actor != r.Sender || !validID(claimID) {
		return r, ErrRejected
	}
	if err := r.check(now); err != nil {
		return r, err
	}
	if r.State == Offered && r.DeniedClaimID == claimID && generation+1 == r.Generation {
		return r, nil
	}
	if err := r.liveOffer(now); err != nil {
		return r, err
	}
	if r.State != Pending || r.ClaimID != claimID || r.Generation != generation || now.Unix() >= r.ClaimUntil {
		return r, ErrConflict
	}
	if r.Generation >= 1<<63-1 {
		return r, ErrInvalid
	}
	next := r
	next.State = Offered
	next.Generation++
	next.Receiver = Key{}
	next.DeniedClaimID = claimID
	next.ClaimID = ""
	next.ClaimUntil = 0
	if err := next.bump(); err != nil {
		return r, err
	}
	return next, nil
}

func (r Record) Bind(actor Key, taskID string, manifest, plan Digest, generation uint64, now time.Time) (Record, error) {
	if actor != r.Receiver || generation != r.Generation || taskID != r.TaskID || manifest != r.Manifest || !nonzero(plan) {
		return r, ErrRejected
	}
	if err := r.check(now); err != nil {
		return r, err
	}
	if (r.State == Bound || r.State == Completed) && r.Plan == plan {
		return r, nil
	}
	if err := r.liveOffer(now); err != nil {
		return r, err
	}
	if r.State != Approved {
		return r, ErrConflict
	}
	next := r
	next.State = Bound
	next.Plan = plan
	next.ResumeUntil = now.Add(ResumeTTL).Unix()
	if err := next.bump(); err != nil {
		return r, err
	}
	return next, nil
}

func (r Record) Authorize(actor Key, taskID string, manifest, plan Digest, generation uint64, now time.Time) error {
	if err := r.check(now); err != nil {
		return err
	}
	if r.State != Bound {
		return ErrTerminal
	}
	if now.Unix() >= r.ResumeUntil {
		return ErrExpired
	}
	if (actor != r.Sender && actor != r.Receiver) || taskID != r.TaskID || manifest != r.Manifest || plan != r.Plan || generation != r.Generation {
		return ErrRejected
	}
	return nil
}
func (r Record) Complete(actor Key, taskID string, manifest, plan Digest, generation uint64, now time.Time) (Record, error) {
	if err := r.check(now); err != nil {
		return r, err
	}
	if r.State == Completed && (actor == r.Sender || actor == r.Receiver) && taskID == r.TaskID && manifest == r.Manifest && plan == r.Plan && generation == r.Generation {
		return r, nil
	}
	if err := r.Authorize(actor, taskID, manifest, plan, generation, now); err != nil {
		return r, err
	}
	next := r
	next.State = Completed
	next.FinishedAt = now.Unix()
	if err := next.bump(); err != nil {
		return r, err
	}
	return next, nil
}
func (r Record) Revoke(actor Key, now time.Time) (Record, error) {
	if actor != r.Sender {
		return r, ErrRejected
	}
	if err := r.check(now); err != nil {
		return r, err
	}
	if r.State == Revoked {
		return r, nil
	}
	if r.Terminal() {
		return r, ErrTerminal
	}
	if r.Generation >= 1<<63-1 {
		return r, ErrInvalid
	}
	next := r
	next.State = Revoked
	next.Generation++
	next.FinishedAt = now.Unix()
	if err := next.bump(); err != nil {
		return r, err
	}
	return next, nil
}

// Reconcile is a bounded caller's per-record expiration operation, not a timer
// owner. The database owner decides the scan budget and persists through CAS.
func (r Record) Reconcile(now time.Time) (Record, error) {
	if err := r.check(now); err != nil {
		return r, err
	}
	if r.Terminal() {
		return r, nil
	}
	deadline := r.ExpiresAt
	if r.State == Bound {
		deadline = r.ResumeUntil
	}
	if now.Unix() >= deadline {
		next := r
		next.State = Expired
		next.FinishedAt = now.Unix()
		if err := next.bump(); err != nil {
			return r, err
		}
		return next, nil
	}
	if r.State == Pending && now.Unix() >= r.ClaimUntil {
		if r.Generation >= 1<<63-1 {
			return r, ErrInvalid
		}
		next := r
		next.State = Offered
		next.Generation++
		next.DeniedClaimID = r.ClaimID
		next.ClaimID = ""
		next.Receiver = Key{}
		next.ClaimUntil = 0
		if err := next.bump(); err != nil {
			return r, err
		}
		return next, nil
	}
	return r, nil
}
