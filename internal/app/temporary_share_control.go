package app

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"path/filepath"
	"sort"
	"time"

	"github.com/Wen5555/LinkSend/internal/identity"
	"github.com/Wen5555/LinkSend/internal/protocol"
	"github.com/Wen5555/LinkSend/internal/sharegrant"
	"github.com/Wen5555/LinkSend/internal/signaling"
	"github.com/Wen5555/LinkSend/internal/transfer"
)

type TemporaryShareSnapshot struct {
	ID                string                   `json:"id"`
	GrantID           string                   `json:"grant_id,omitempty"`
	Role              string                   `json:"role"`
	State             string                   `json:"state"`
	GrantState        sharegrant.State         `json:"grant_state,omitempty"`
	PeerID            string                   `json:"peer_id,omitempty"`
	ReceiverPublicKey string                   `json:"receiver_public_key,omitempty"`
	ClaimID           string                   `json:"claim_id,omitempty"`
	Generation        uint64                   `json:"generation,omitempty"`
	TaskID            string                   `json:"task_id,omitempty"`
	TaskRecordID      string                   `json:"task_record_id,omitempty"`
	Manifest          *transfer.Manifest       `json:"manifest,omitempty"`
	TargetDirectory   string                   `json:"target_directory,omitempty"`
	ExpiresAt         int64                    `json:"expires_at,omitempty"`
	ResumeUntil       int64                    `json:"resume_until,omitempty"`
	PendingAction     signaling.ShareOperation `json:"pending_action,omitempty"`
	ErrorCode         protocol.Code            `json:"error_code,omitempty"`
	Revision          uint64                   `json:"revision"`
}

type TemporaryShareCreation struct {
	Share TemporaryShareSnapshot `json:"share"`
	Code  string                 `json:"code"` // Only returned by an explicit local create action.
}

func temporaryPeer(r temporaryShareRecord) (string, error) {
	key := r.Status.ReceiverKey
	if r.Role == "receiver" {
		key = r.Status.SenderKey
	}
	if key == "" {
		return "", nil
	}
	public, err := base64.RawStdEncoding.DecodeString(key)
	if err != nil || len(public) != 32 {
		return "", temporaryShareFailure(protocol.ShareInvalid)
	}
	return identity.DeviceID(public), nil
}

func temporarySnapshot(r temporaryShareRecord) TemporaryShareSnapshot {
	r = cloneTemporaryRecord(r)
	peer, _ := temporaryPeer(r)
	v := TemporaryShareSnapshot{ID: r.ID, GrantID: r.Status.ID, Role: r.Role, State: r.State, GrantState: r.Status.State, PeerID: peer, ReceiverPublicKey: r.Status.ReceiverKey, ClaimID: r.Status.ClaimID, Generation: r.Status.Generation, TaskID: r.Status.TaskID, TaskRecordID: r.TaskRecordID, Manifest: r.Manifest, TargetDirectory: r.Directory, ExpiresAt: r.Status.ExpiresAt, ResumeUntil: r.Status.ResumeUntil, ErrorCode: r.Error, Revision: r.Revision}
	if r.Intent != nil {
		v.PendingAction = r.Intent.Operation
	}
	return v
}

func (s *Service) TemporaryShares() ([]TemporaryShareSnapshot, error) {
	done, err := s.beginProfileWork()
	if err != nil {
		return nil, err
	}
	defer done()
	if err = s.initializeTemporaryShares(); err != nil {
		return nil, err
	}
	s.temporaryShares.mu.Lock()
	entries := make([]*temporaryShareEntry, 0, len(s.temporaryShares.entries))
	for _, e := range s.temporaryShares.entries {
		entries = append(entries, e)
	}
	s.temporaryShares.mu.Unlock()
	out := make([]TemporaryShareSnapshot, 0, len(entries))
	for _, e := range entries {
		r := temporaryRecordCopy(e)
		v := temporarySnapshot(r)
		e.mu.Lock()
		active := e.active
		e.mu.Unlock()
		if !active && (v.State == "active" || v.State == "awaiting_save") {
			v.State = "needs_attention"
		}
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (s *Service) temporaryClient(r temporaryShareRecord) (*signaling.Client, error) {
	c, err := s.client()
	if err != nil {
		return nil, temporaryShareFailure(protocol.SignalingUnreachable)
	}
	if c.ServerURL() != r.Origin {
		return nil, temporaryShareFailure(protocol.ShareRejected)
	}
	return c, nil
}

// Synchronous callers can outlive a profile (for example the desktop runtime
// context). Join their cancellation with the existing profile owner without
// retaining contexts or running cancellation callbacks under owner locks.
func (s *Service) temporaryOperationContext(parent context.Context) (context.Context, func()) {
	ctx, cancel := context.WithCancel(parent)
	if s.workCtx == nil {
		cancel()
		return ctx, cancel
	}
	stop := context.AfterFunc(s.workCtx, cancel)
	if s.workCtx.Err() != nil {
		cancel()
	}
	return ctx, func() { stop(); cancel() }
}

func (s *Service) CreateTemporaryShare(ctx context.Context, paths []string, selection *transfer.SourceSelection) (TemporaryShareCreation, error) {
	done, err := s.beginProfileWork()
	if err != nil {
		return TemporaryShareCreation{}, err
	}
	defer done()
	ctx, release := s.temporaryOperationContext(ctx)
	defer release()
	c, err := s.client()
	if err != nil {
		return TemporaryShareCreation{}, temporaryShareFailure(protocol.SignalingUnreachable)
	}
	if _, err = c.ShareControlInfo(ctx); err != nil {
		return TemporaryShareCreation{}, err
	}
	selection, err = transfer.NormalizeSourceSelection(selection)
	if err != nil {
		return TemporaryShareCreation{}, temporaryShareFailure(protocol.ShareInvalid)
	}
	paths = append([]string(nil), paths...)
	for i, path := range paths {
		absolute, pathErr := filepath.Abs(path)
		if pathErr != nil {
			return TemporaryShareCreation{}, temporaryShareFailure(protocol.ShareInvalid)
		}
		paths[i] = absolute
	}
	if hook := s.temporaryShares.beforePrepare; hook != nil {
		hook(ctx)
	}
	prepared, err := s.prepareUserSources(ctx, paths, transfer.DefaultChunkSize, selection)
	if err != nil {
		return TemporaryShareCreation{}, classifyTaskError(err)
	}
	defer prepared.Close()
	code, digest, err := sharegrant.GenerateCode()
	if err != nil {
		return TemporaryShareCreation{}, temporaryShareFailure(protocol.ShareUnavailable)
	}
	id := protocol.RandomID()
	manifest := prepared.Manifest
	r := temporaryShareRecord{Version: 1, Purpose: "temporary-share", Owner: s.identity.ID(), Origin: c.ServerURL(), ID: id, Role: "sender", Revision: 1, Created: time.Now().Unix(), State: "creating", CodeDigest: hex.EncodeToString(digest[:]), Manifest: &manifest, Paths: append([]string(nil), paths...), Selection: cloneSourceSelection(selection)}
	r.Intent = temporaryIntent(signaling.ShareCreate, signaling.ShareRequest{ID: id, CodeDigest: r.CodeDigest, Manifest: manifest.Digest()})
	e, err := s.insertTemporaryRecord(r)
	if err != nil {
		return TemporaryShareCreation{}, err
	}
	_, err = s.executeTemporaryIntent(ctx, e, "")
	return TemporaryShareCreation{Share: temporarySnapshot(temporaryRecordCopy(e)), Code: code}, err
}

func (s *Service) ClaimTemporaryShare(ctx context.Context, code string) (TemporaryShareSnapshot, error) {
	done, err := s.beginProfileWork()
	if err != nil {
		return TemporaryShareSnapshot{}, err
	}
	defer done()
	ctx, release := s.temporaryOperationContext(ctx)
	defer release()
	digest, err := sharegrant.CodeDigest(code)
	if err != nil {
		return TemporaryShareSnapshot{}, temporaryShareFailure(protocol.ShareInvalid)
	}
	c, err := s.client()
	if err != nil {
		return TemporaryShareSnapshot{}, temporaryShareFailure(protocol.SignalingUnreachable)
	}
	if _, err = c.ShareControlInfo(ctx); err != nil {
		return TemporaryShareSnapshot{}, err
	}
	r := temporaryShareRecord{Version: 1, Purpose: "temporary-share", Owner: s.identity.ID(), Origin: c.ServerURL(), ID: protocol.RandomID(), Role: "receiver", Revision: 1, Created: time.Now().Unix(), State: "claiming", CodeDigest: hex.EncodeToString(digest[:]), ClaimID: protocol.RandomID()}
	r.Intent = temporaryIntent(signaling.ShareClaim, signaling.ShareRequest{ClaimID: r.ClaimID})
	e, err := s.insertTemporaryRecord(r)
	if err != nil {
		return TemporaryShareSnapshot{}, err
	}
	return s.executeTemporaryIntent(ctx, e, code)
}

func temporaryStatusRequest(r temporaryShareRecord) signaling.ShareRequest {
	return signaling.ShareRequest{ID: r.Status.ID, ClaimID: r.Status.ClaimID, TaskID: r.Status.TaskID, Manifest: r.Status.Manifest, Plan: r.Status.Plan, Generation: r.Status.Generation, ReceiverKey: r.Status.ReceiverKey}
}

func (s *Service) mergeTemporaryStatus(r *temporaryShareRecord, status signaling.ShareStatus) error {
	old := r.Status
	if (old.State == sharegrant.Completed || old.State == sharegrant.Revoked || old.State == sharegrant.Expired) && old.State != status.State {
		return temporaryShareFailure(protocol.ShareTerminal)
	}
	if status.ID == "" || old.ID != "" && old.ID != status.ID || old.Manifest != "" && old.Manifest != status.Manifest || old.SenderKey != "" && old.SenderKey != status.SenderKey || r.Manifest != nil && r.Manifest.Digest() != status.Manifest || old.TaskID != "" && old.TaskID != status.TaskID || old.Plan != "" && old.Plan != status.Plan || old.Generation > status.Generation {
		return temporaryShareFailure(protocol.ShareRejected)
	}
	local := base64.RawStdEncoding.EncodeToString(s.identity.PublicKey())
	if r.Role == "sender" && status.SenderKey != local || r.Role == "receiver" && (status.ReceiverKey != local || status.ClaimID != r.ClaimID) {
		return temporaryShareFailure(protocol.ShareRejected)
	}
	if r.ApprovalID != "" && old.ReceiverKey != "" && (old.ReceiverKey != status.ReceiverKey || old.ClaimID != status.ClaimID) {
		return temporaryShareFailure(protocol.ShareRejected)
	}
	if peer, err := temporaryPeer(temporaryShareRecord{Role: r.Role, Status: status}); err != nil {
		return err
	} else if peer != "" {
		if err = identity.CheckPeerAllowed(s.cfg.DataDir, peer); err != nil {
			return protocol.Wrap(protocol.ShareRejected, "temporary share peer is blocked", err)
		}
	}
	r.Status = status
	if r.TerminalObservedAt == 0 && (status.State == sharegrant.Completed || status.State == sharegrant.Revoked || status.State == sharegrant.Expired) {
		r.TerminalObservedAt = s.temporaryNow().Unix()
	}
	return nil
}

func (s *Service) executeTemporaryIntent(ctx context.Context, e *temporaryShareEntry, code string) (TemporaryShareSnapshot, error) {
	ctx, release := s.temporaryOperationContext(ctx)
	defer release()
	done, workErr := beginTemporaryControl(e)
	if workErr != nil {
		return TemporaryShareSnapshot{}, workErr
	}
	defer done()
	r := temporaryRecordCopy(e)
	if r.Intent == nil {
		return temporarySnapshot(r), nil
	}
	intent := *r.Intent
	if r.Stopped && intent.Operation != signaling.ShareRevoke {
		return temporarySnapshot(r), temporaryShareFailure(protocol.Cancelled)
	}
	request := intent.Request
	if intent.Operation == signaling.ShareClaim {
		digest, err := sharegrant.CodeDigest(code)
		if err != nil || hex.EncodeToString(digest[:]) != r.CodeDigest {
			return temporarySnapshot(r), temporaryShareFailure(protocol.ShareInvalid)
		}
		request.Code = code
	}
	c, err := s.temporaryClient(r)
	if err != nil {
		return temporarySnapshot(r), err
	}
	status, callErr := c.ShareControl(ctx, intent.Operation, request)
	e.mu.Lock()
	defer e.mu.Unlock()
	next := cloneTemporaryRecord(e.record)
	if next.Intent == nil || next.Intent.ID != intent.ID {
		return temporarySnapshot(next), temporaryShareFailure(protocol.ShareConflict)
	}
	if callErr != nil {
		next.Error = protocol.ErrorCode(callErr)
		if intent.Operation == signaling.ShareComplete && next.Completion != nil {
			next.State = "needs_attention"
		}
		if err = s.saveTemporaryRecord(e, next); err != nil {
			return temporarySnapshot(e.record), err
		}
		return temporarySnapshot(e.record), callErr
	}
	if err = s.mergeTemporaryStatus(&next, status); err != nil {
		return temporarySnapshot(e.record), err
	}
	next.Intent = nil
	next.Error = ""
	if next.Stopped {
		next.State = "stopped"
	} else if status.State == sharegrant.Completed {
		// A peer can terminate its grant without proving this local process
		// completed the file engine. Preserve that terminal authority boundary
		// while requiring our persisted engine receipt for local completion.
		next.State = "needs_attention"
		if next.Completion != nil {
			next.State = "completed"
		}
	} else {
		next.State = "ready"
	}
	if err = s.saveTemporaryRecord(e, next); err != nil {
		return temporarySnapshot(e.record), err
	}
	if status.State == sharegrant.Completed && next.Completion != nil && next.TaskRecordID != "" {
		s.tasks.mu.RLock()
		task := s.tasks.tasks[next.TaskRecordID]
		s.tasks.mu.RUnlock()
		if task != nil && task.recoverySnapshot().TemporaryShareID == next.ID {
			task.completeTransfer(task.snapshot().AttemptID, *next.Completion, "")
		}
	}
	return temporarySnapshot(e.record), nil
}

func (s *Service) RetryTemporaryShareAction(ctx context.Context, id string) (TemporaryShareSnapshot, error) {
	done, err := s.beginProfileWork()
	if err != nil {
		return TemporaryShareSnapshot{}, err
	}
	defer done()
	e, err := s.temporaryEntry(id)
	if err != nil {
		return TemporaryShareSnapshot{}, err
	}
	return s.executeTemporaryIntent(ctx, e, "")
}

func (s *Service) RetryTemporaryShareClaim(ctx context.Context, id, code string) (TemporaryShareSnapshot, error) {
	done, err := s.beginProfileWork()
	if err != nil {
		return TemporaryShareSnapshot{}, err
	}
	defer done()
	e, err := s.temporaryEntry(id)
	if err != nil {
		return TemporaryShareSnapshot{}, err
	}
	r := temporaryRecordCopy(e)
	if r.Role != "receiver" || r.Intent == nil || r.Intent.Operation != signaling.ShareClaim {
		return temporarySnapshot(r), temporaryShareFailure(protocol.ShareConflict)
	}
	return s.executeTemporaryIntent(ctx, e, code)
}

func (s *Service) RefreshTemporaryShare(ctx context.Context, id string) (TemporaryShareSnapshot, error) {
	done, err := s.beginProfileWork()
	if err != nil {
		return TemporaryShareSnapshot{}, err
	}
	defer done()
	e, err := s.temporaryEntry(id)
	if err != nil {
		return TemporaryShareSnapshot{}, err
	}
	return s.refreshTemporaryShare(ctx, e, false)
}

func (s *Service) refreshTemporaryShare(ctx context.Context, e *temporaryShareEntry, authorize bool) (TemporaryShareSnapshot, error) {
	ctx, release := s.temporaryOperationContext(ctx)
	defer release()
	done, workErr := beginTemporaryControl(e)
	if workErr != nil {
		return TemporaryShareSnapshot{}, workErr
	}
	defer done()
	r := temporaryRecordCopy(e)
	if r.Status.ID == "" {
		return temporarySnapshot(r), temporaryShareFailure(protocol.ShareConflict)
	}
	c, err := s.temporaryClient(r)
	if err != nil {
		return temporarySnapshot(r), err
	}
	op := signaling.ShareRead
	if authorize {
		op = signaling.ShareAuthorize
	}
	request := temporaryStatusRequest(r)
	if !authorize {
		request.Generation = 0
	}
	// Sender status must be able to observe a newly claimed actor or a denial's
	// newer generation. Authorization operations always bind every old field.
	if !authorize && r.Role == "sender" && r.ApprovalID == "" {
		request.ReceiverKey = ""
		request.ClaimID = ""
		request.Generation = 0
	}
	status, err := c.ShareControl(ctx, op, request)
	if err != nil {
		return temporarySnapshot(r), err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	next := cloneTemporaryRecord(e.record)
	if err = s.mergeTemporaryStatus(&next, status); err != nil {
		return temporarySnapshot(e.record), err
	}
	if !next.Stopped && status.State == sharegrant.Completed && next.Completion != nil {
		next.State = "completed"
	}
	if status.State == sharegrant.Revoked || status.State == sharegrant.Expired {
		next.Stopped = true
		next.State = "stopped"
		if e.cancel != nil {
			e.cancel()
		}
	}
	if err = s.saveTemporaryRecord(e, next); err != nil {
		return temporarySnapshot(e.record), err
	}
	return temporarySnapshot(e.record), nil
}

func (s *Service) ApproveTemporaryShare(ctx context.Context, id, claimID, receiverKey string, generation uint64) (TemporaryShareSnapshot, error) {
	done, err := s.beginProfileWork()
	if err != nil {
		return TemporaryShareSnapshot{}, err
	}
	defer done()
	e, err := s.temporaryEntry(id)
	if err != nil {
		return TemporaryShareSnapshot{}, err
	}
	e.mu.Lock()
	r := cloneTemporaryRecord(e.record)
	if r.Stopped || r.Role != "sender" || r.Intent != nil || r.Status.State != sharegrant.Pending || r.Status.ClaimID != claimID || r.Status.ReceiverKey != receiverKey || r.Status.Generation != generation || r.Manifest == nil {
		e.mu.Unlock()
		return temporarySnapshot(r), temporaryShareFailure(protocol.ShareConflict)
	}
	r.ApprovalID = protocol.RandomID()
	r.Intent = temporaryIntent(signaling.ShareApprove, signaling.ShareRequest{ID: r.Status.ID, ClaimID: claimID, ReceiverKey: receiverKey, Generation: generation, ApprovalID: r.ApprovalID, TaskID: r.Manifest.TransferID, Manifest: r.Manifest.Digest()})
	err = s.saveTemporaryRecord(e, r)
	e.mu.Unlock()
	if err != nil {
		return temporarySnapshot(r), err
	}
	return s.executeTemporaryIntent(ctx, e, "")
}

func (s *Service) DenyTemporaryShare(ctx context.Context, id, claimID string, generation uint64) (TemporaryShareSnapshot, error) {
	done, err := s.beginProfileWork()
	if err != nil {
		return TemporaryShareSnapshot{}, err
	}
	defer done()
	e, err := s.temporaryEntry(id)
	if err != nil {
		return TemporaryShareSnapshot{}, err
	}
	e.mu.Lock()
	r := cloneTemporaryRecord(e.record)
	if r.Stopped || r.Role != "sender" || r.Intent != nil || r.Status.State != sharegrant.Pending || r.Status.ClaimID != claimID || r.Status.Generation != generation {
		e.mu.Unlock()
		return temporarySnapshot(r), temporaryShareFailure(protocol.ShareConflict)
	}
	r.Intent = temporaryIntent(signaling.ShareDeny, signaling.ShareRequest{ID: r.Status.ID, ClaimID: claimID, Generation: generation})
	err = s.saveTemporaryRecord(e, r)
	e.mu.Unlock()
	if err != nil {
		return temporarySnapshot(r), err
	}
	return s.executeTemporaryIntent(ctx, e, "")
}

func (s *Service) stopTemporaryShare(e *temporaryShareEntry, revoke bool) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	r := cloneTemporaryRecord(e.record)
	if revoke && r.Role != "sender" {
		return temporaryShareFailure(protocol.ShareRejected)
	}
	if r.State == "completed" {
		return temporaryShareFailure(protocol.ShareTerminal)
	}
	r.Stopped = true
	r.State = "stopped"
	r.Error = protocol.Cancelled
	e.denied = true
	grantID := r.Status.ID
	if grantID == "" && r.Role == "sender" {
		grantID = r.ID
	}
	if revoke && grantID != "" {
		r.Intent = temporaryIntent(signaling.ShareRevoke, signaling.ShareRequest{ID: grantID})
	} else {
		r.Intent = nil
	}
	err := s.saveTemporaryRecord(e, r)
	// Cancellation wins in memory even if durable storage is unavailable.
	if e.cancel != nil {
		e.cancel()
	}
	return err
}

func (s *Service) CancelTemporaryShare(ctx context.Context, id string) (TemporaryShareSnapshot, error) {
	done, err := s.beginProfileWork()
	if err != nil {
		return TemporaryShareSnapshot{}, err
	}
	defer done()
	e, err := s.temporaryEntry(id)
	if err != nil {
		return TemporaryShareSnapshot{}, err
	}
	r := temporaryRecordCopy(e)
	if err = s.stopTemporaryShare(e, r.Role == "sender"); err != nil {
		return temporarySnapshot(temporaryRecordCopy(e)), err
	}
	if r.Role == "sender" {
		return s.executeTemporaryIntent(ctx, e, "")
	}
	return temporarySnapshot(temporaryRecordCopy(e)), nil
}

func (s *Service) RevokeTemporaryShare(ctx context.Context, id string) (TemporaryShareSnapshot, error) {
	done, workErr := s.beginProfileWork()
	if workErr != nil {
		return TemporaryShareSnapshot{}, workErr
	}
	defer done()
	e, err := s.temporaryEntry(id)
	if err != nil {
		return TemporaryShareSnapshot{}, err
	}
	if temporaryRecordCopy(e).Role != "sender" {
		return TemporaryShareSnapshot{}, temporaryShareFailure(protocol.ShareRejected)
	}
	return s.CancelTemporaryShare(ctx, id)
}
