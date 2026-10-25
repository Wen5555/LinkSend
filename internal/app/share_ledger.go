package app

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/Wen5555/LinkSend/internal/protocol"
	"github.com/Wen5555/LinkSend/internal/sharegrant"
	"github.com/Wen5555/LinkSend/internal/signaling"
	"github.com/Wen5555/LinkSend/internal/transfer"
)

const temporarySharePrefix = "temporary-share/v1/"
const maxTemporaryShareRecords = 64
const temporaryShareRecordLimit = 2 * transfer.MaxMetadata

type temporaryShareIntent struct {
	ID        string                   `json:"id"`
	Operation signaling.ShareOperation `json:"operation"`
	Request   signaling.ShareRequest   `json:"request"`
}

// Private signed local metadata. Plain share codes never enter this type.
type temporaryShareRecord struct {
	Version            int                       `json:"version"`
	Purpose            string                    `json:"purpose"`
	Owner              string                    `json:"owner"`
	Origin             string                    `json:"server_origin"`
	ID                 string                    `json:"id"`
	Role               string                    `json:"role"`
	Revision           uint64                    `json:"revision"`
	Created            int64                     `json:"created"`
	TerminalObservedAt int64                     `json:"terminal_observed_at,omitempty"`
	State              string                    `json:"local_state"`
	Stopped            bool                      `json:"stopped"`
	CodeDigest         string                    `json:"code_digest"`
	ClaimID            string                    `json:"claim_id,omitempty"`
	ApprovalID         string                    `json:"approval_id,omitempty"`
	Status             signaling.ShareStatus     `json:"status"`
	Manifest           *transfer.Manifest        `json:"manifest,omitempty"`
	Paths              []string                  `json:"source_paths,omitempty"`
	Selection          *transfer.SourceSelection `json:"source_selection,omitempty"`
	Plan               *transfer.ReceivePlan     `json:"receive_plan,omitempty"`
	Directory          string                    `json:"target_directory,omitempty"`
	TaskRecordID       string                    `json:"task_record_id,omitempty"`
	Completion         *transfer.Result          `json:"engine_completion,omitempty"`
	Intent             *temporaryShareIntent     `json:"intent,omitempty"`
	Error              protocol.Code             `json:"error_code,omitempty"`
	Signature          []byte                    `json:"signature"`
}

type temporaryShareDecision struct {
	directory string
	request   transfer.PlanRequest
	result    chan error
}

type temporaryShareEntry struct {
	mu           sync.Mutex
	record       temporaryShareRecord
	raw          string
	active       bool
	cancel       context.CancelFunc
	decision     chan temporaryShareDecision
	decisionSent bool
	denied       bool
	controlBusy  int
	retired      bool
}

type temporaryShareOwner struct {
	once          sync.Once
	mu            sync.Mutex
	entries       map[string]*temporaryShareEntry
	err           error
	wg            sync.WaitGroup
	now           func() time.Time
	beforeRetire  func(string)          // Package-private deterministic CAS test barrier.
	beforePrepare func(context.Context) // Package-private cancellation test barrier; never stores a context.
}

func (s *Service) temporaryNow() time.Time {
	if s.temporaryShares.now != nil {
		return s.temporaryShares.now()
	}
	return time.Now()
}

func temporaryShareFailure(code protocol.Code) error {
	return protocol.Fail(code, "temporary share operation unavailable")
}

func temporaryRecordMessage(r temporaryShareRecord) []byte {
	r.Signature = nil
	raw, _ := json.Marshal(r)
	return append([]byte("LinkSend temporary share owner v1\x00"), raw...)
}

func (s *Service) validTemporaryRecord(r temporaryShareRecord) bool {
	if r.Version != 1 || r.Purpose != "temporary-share" || r.Owner != s.identity.ID() || !validClipboardFileID(r.ID, 16) || r.Origin == "" || r.Revision == 0 || r.Created <= 0 || !validClipboardFileID(r.CodeDigest, 32) || (r.Role != "sender" && r.Role != "receiver") {
		return false
	}
	switch r.State {
	case "creating", "claiming", "ready", "active", "awaiting_save", "paused", "needs_attention", "stopped", "completed":
	default:
		return false
	}
	if r.Stopped && r.State != "stopped" {
		return false
	}
	if r.TerminalObservedAt < 0 || r.TerminalObservedAt != 0 && r.Status.State != sharegrant.Completed && r.Status.State != sharegrant.Revoked && r.Status.State != sharegrant.Expired {
		return false
	}
	if r.Role == "receiver" && (!validClipboardFileID(r.ClaimID, 16) || len(r.Paths) != 0 || r.Selection != nil) {
		return false
	}
	if len(r.Paths) > maxDraftPaths || len(r.Directory) > maxDesktopPathBytes {
		return false
	}
	for _, p := range r.Paths {
		if p == "" || len(p) > maxDesktopPathBytes {
			return false
		}
	}
	if r.Manifest != nil && (r.Manifest.Validate() != nil || r.Status.Manifest != "" && r.Status.Manifest != r.Manifest.Digest()) {
		return false
	}
	if r.Role == "sender" && (r.Manifest == nil || len(r.Paths) == 0) {
		return false
	}
	if r.Role == "sender" && r.Status.ID != "" && r.Status.ID != r.ID {
		return false
	}
	if r.Completion != nil && (r.Manifest == nil || r.Completion.TransferID != r.Manifest.TransferID || r.Completion.Digest != r.Manifest.Digest() || r.Completion.Bytes < 0 || r.Completion.Bytes > r.Manifest.TotalBytes() || r.Completion.State != "Completed" && r.Completion.State != "NoContent") {
		return false
	}
	if r.Plan != nil && (r.Manifest == nil || r.Plan.Validate(*r.Manifest) != nil || r.Directory == "" || r.Status.Plan != "" && r.Plan.Digest() != r.Status.Plan) {
		return false
	}
	if r.Status.ID != "" && !validClipboardFileID(r.Status.ID, 16) {
		return false
	}
	if r.Status.ID != "" && (!validClipboardFileID(r.Status.Manifest, 32) || r.Status.Generation == 0 || r.Status.SenderKey == "") {
		return false
	}
	if r.Intent != nil && (!validClipboardFileID(r.Intent.ID, 16) || r.Intent.Request.Code != "") {
		return false
	}
	if r.Intent != nil {
		switch r.Intent.Operation {
		case signaling.ShareCreate, signaling.ShareClaim, signaling.ShareApprove, signaling.ShareDeny, signaling.ShareBind, signaling.ShareRevoke, signaling.ShareComplete:
		default:
			return false
		}
	}
	return true
}

func (s *Service) encodeTemporaryRecord(r temporaryShareRecord) (string, error) {
	if !s.validTemporaryRecord(r) {
		return "", temporaryShareFailure(protocol.ShareInvalid)
	}
	r.Signature = s.identity.Sign(temporaryRecordMessage(r))
	raw, err := json.Marshal(r)
	if err != nil || len(raw) > temporaryShareRecordLimit {
		return "", temporaryShareFailure(protocol.RateLimited)
	}
	return string(raw), nil
}

func (s *Service) decodeTemporaryRecord(id, raw string) (temporaryShareRecord, error) {
	var r temporaryShareRecord
	if len(raw) > temporaryShareRecordLimit {
		return r, temporaryShareFailure(protocol.ShareInvalid)
	}
	d := json.NewDecoder(strings.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&r) != nil || d.Decode(new(any)) != io.EOF || id != r.ID || !s.validTemporaryRecord(r) || !ed25519.Verify(s.identity.PublicKey(), temporaryRecordMessage(r), r.Signature) {
		return temporaryShareRecord{}, temporaryShareFailure(protocol.ShareInvalid)
	}
	if r.Plan != nil {
		r.Plan.Directory = r.Directory
	}
	return r, nil
}

func (s *Service) initializeTemporaryShares() error {
	s.temporaryShares.once.Do(func() {
		s.temporaryShares.entries = make(map[string]*temporaryShareEntry)
		if s.store == nil || s.storeErr != nil {
			s.temporaryShares.err = temporaryShareFailure(protocol.ShareUnavailable)
			return
		}
		rows, err := s.store.db.Query(`SELECT key,value FROM metadata WHERE key LIKE ? ORDER BY key LIMIT ?`, temporarySharePrefix+"%", maxTemporaryShareRecords+1)
		if err != nil {
			s.temporaryShares.err = temporaryShareFailure(protocol.ShareUnavailable)
			return
		}
		defer rows.Close()
		for rows.Next() {
			var key, raw string
			if rows.Scan(&key, &raw) != nil || len(s.temporaryShares.entries) >= maxTemporaryShareRecords {
				s.temporaryShares.err = temporaryShareFailure(protocol.ShareUnavailable)
				return
			}
			r, err := s.decodeTemporaryRecord(strings.TrimPrefix(key, temporarySharePrefix), raw)
			if err != nil {
				s.temporaryShares.err = err
				return
			}
			s.temporaryShares.entries[r.ID] = &temporaryShareEntry{record: r, raw: raw}
		}
		if rows.Err() != nil {
			s.temporaryShares.err = temporaryShareFailure(protocol.ShareUnavailable)
		}
	})
	return s.temporaryShares.err
}

func (s *Service) temporaryEntry(id string) (*temporaryShareEntry, error) {
	if err := s.initializeTemporaryShares(); err != nil {
		return nil, err
	}
	s.temporaryShares.mu.Lock()
	defer s.temporaryShares.mu.Unlock()
	e := s.temporaryShares.entries[id]
	if e == nil {
		return nil, temporaryShareFailure(protocol.ShareInvalid)
	}
	return e, nil
}

func cloneTemporaryRecord(r temporaryShareRecord) temporaryShareRecord {
	raw, _ := json.Marshal(r)
	var copy temporaryShareRecord
	_ = json.Unmarshal(raw, &copy)
	if copy.Plan != nil {
		copy.Plan.Directory = copy.Directory
	}
	return copy
}

// Entry lock is held. CAS binds every control intent and confirmation to the
// previous signed record, rather than an unsigned task snapshot.
func (s *Service) saveTemporaryRecord(e *temporaryShareEntry, next temporaryShareRecord) error {
	if next.ID != e.record.ID || next.Origin != e.record.Origin || next.Owner != e.record.Owner || next.Revision != e.record.Revision {
		return temporaryShareFailure(protocol.ShareConflict)
	}
	next.Revision++
	raw, err := s.encodeTemporaryRecord(next)
	if err != nil {
		return err
	}
	result, err := s.store.db.Exec(`UPDATE metadata SET value=? WHERE key=? AND value=?`, raw, temporarySharePrefix+next.ID, e.raw)
	if err != nil {
		return temporaryShareFailure(protocol.ShareUnavailable)
	}
	if count, err := result.RowsAffected(); err != nil || count != 1 {
		return temporaryShareFailure(protocol.ShareConflict)
	}
	var read string
	if s.store.db.QueryRow(`SELECT value FROM metadata WHERE key=?`, temporarySharePrefix+next.ID).Scan(&read) != nil || read != raw {
		return temporaryShareFailure(protocol.ShareUnavailable)
	}
	if _, err = s.decodeTemporaryRecord(next.ID, read); err != nil {
		return err
	}
	e.record, e.raw = next, raw
	return nil
}

func (s *Service) insertTemporaryRecord(r temporaryShareRecord) (*temporaryShareEntry, error) {
	if err := s.initializeTemporaryShares(); err != nil {
		return nil, err
	}
	s.temporaryShares.mu.Lock()
	defer s.temporaryShares.mu.Unlock()
	if err := s.retireTemporaryRecordsLocked(s.temporaryNow()); err != nil {
		return nil, err
	}
	if len(s.temporaryShares.entries) >= maxTemporaryShareRecords {
		return nil, temporaryShareFailure(protocol.RateLimited)
	}
	active := 0
	for _, e := range s.temporaryShares.entries {
		e.mu.Lock()
		old := e.record
		if old.Role == r.Role && !old.Stopped && old.State != "completed" && (old.Status.ResumeUntil > time.Now().Unix() || old.Status.ExpiresAt > time.Now().Unix() || old.Status.ID == "") {
			active++
		}
		e.mu.Unlock()
	}
	limit := 8
	if r.Role == "receiver" {
		limit = 4
	}
	if active >= limit {
		return nil, temporaryShareFailure(protocol.RateLimited)
	}
	raw, err := s.encodeTemporaryRecord(r)
	if err != nil {
		return nil, err
	}
	result, err := s.store.db.Exec(`INSERT INTO metadata(key,value) VALUES(?,?) ON CONFLICT(key) DO NOTHING`, temporarySharePrefix+r.ID, raw)
	if err != nil {
		return nil, temporaryShareFailure(protocol.ShareUnavailable)
	}
	if count, err := result.RowsAffected(); err != nil || count != 1 {
		return nil, temporaryShareFailure(protocol.ShareConflict)
	}
	var read string
	if s.store.db.QueryRow(`SELECT value FROM metadata WHERE key=?`, temporarySharePrefix+r.ID).Scan(&read) != nil || !bytes.Equal([]byte(read), []byte(raw)) {
		return nil, temporaryShareFailure(protocol.ShareUnavailable)
	}
	e := &temporaryShareEntry{record: r, raw: raw}
	s.temporaryShares.entries[r.ID] = e
	return e, nil
}

// Only confirmed, old terminal authorization metadata is retired. History,
// checkpoint and user files are not deletion targets. Old v1 rows without an
// observation timestamp stay protected; CreatedAt never substitutes for it.
func (s *Service) retireTemporaryRecordsLocked(now time.Time) error {
	for id, e := range s.temporaryShares.entries {
		e.mu.Lock()
		r, err := s.decodeTemporaryRecord(id, e.raw)
		if err != nil || e.active || e.controlBusy != 0 || r.Intent != nil || r.TerminalObservedAt <= 0 || now.Unix() < r.TerminalObservedAt || now.Sub(time.Unix(r.TerminalObservedAt, 0)) < 7*24*time.Hour || r.Status.State != sharegrant.Completed && r.Status.State != sharegrant.Revoked && r.Status.State != sharegrant.Expired {
			e.mu.Unlock()
			continue
		}
		if s.temporaryShares.beforeRetire != nil {
			s.temporaryShares.beforeRetire(id)
		}
		result, err := s.store.db.Exec(`DELETE FROM metadata WHERE key=? AND value=?`, temporarySharePrefix+id, e.raw)
		if err != nil {
			e.mu.Unlock()
			return temporaryShareFailure(protocol.ShareUnavailable)
		}
		if count, err := result.RowsAffected(); err != nil {
			e.mu.Unlock()
			return temporaryShareFailure(protocol.ShareUnavailable)
		} else if count == 1 {
			delete(s.temporaryShares.entries, id)
			e.denied = true
			e.retired = true
		}
		e.mu.Unlock()
	}
	return nil
}

func beginTemporaryControl(e *temporaryShareEntry) (func(), error) {
	e.mu.Lock()
	if e.retired {
		e.mu.Unlock()
		return nil, temporaryShareFailure(protocol.ShareTerminal)
	}
	e.controlBusy++
	e.mu.Unlock()
	return func() { e.mu.Lock(); e.controlBusy--; e.mu.Unlock() }, nil
}

func temporaryRecordCopy(e *temporaryShareEntry) temporaryShareRecord {
	e.mu.Lock()
	defer e.mu.Unlock()
	return cloneTemporaryRecord(e.record)
}

func temporaryIntent(op signaling.ShareOperation, request signaling.ShareRequest) *temporaryShareIntent {
	return &temporaryShareIntent{ID: protocol.RandomID(), Operation: op, Request: request}
}
