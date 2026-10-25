package app

import (
	"context"
	"errors"
	"time"

	"github.com/Wen5555/LinkSend/internal/identity"
	"github.com/Wen5555/LinkSend/internal/protocol"
	"github.com/Wen5555/LinkSend/internal/sharegrant"
	"github.com/Wen5555/LinkSend/internal/signaling"
	"github.com/Wen5555/LinkSend/internal/transfer"
	"github.com/Wen5555/LinkSend/internal/transport"
)

type temporaryBodyBudget struct {
	service *Service
	entry   *temporaryShareEntry
	scope   sharegrant.SignalScope
}

func (s *Service) prepareTemporaryShareShutdown() {
	// Temporary workers inherit workCtx. Publish their control intent before
	// that parent is cancelled, so shutdown cannot turn resumable work into a
	// user-cancelled terminal task before the ordinary shutdown loop reaches it.
	s.tasks.mu.RLock()
	defer s.tasks.mu.RUnlock()
	for _, task := range s.tasks.tasks {
		task.mu.Lock()
		if task.recovery.TemporaryShareID != "" && task.cancel != nil && !isTerminal(task.snap.State) && task.snap.State != "cancel_requested" && task.snap.State != "pause_requested" {
			task.snap.State = "shutdown_requested"
			task.snap.Phase = "saving_before_exit"
			task.snap.CanPause = false
			task.snap.CanCancel = false
			task.snap.Revision++
			task.snap.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		}
		task.mu.Unlock()
	}
}

func (b temporaryBodyBudget) Acquire(ctx context.Context, size int) (int, error) {
	if err := b.service.temporaryLocalCheck(b.entry, b.scope); err != nil {
		return 0, err
	}
	n, err := processFileBudget.Acquire(ctx, size)
	if err != nil {
		return 0, err
	}
	if err = b.service.temporaryLocalCheck(b.entry, b.scope); err != nil {
		return 0, err
	}
	return n, nil
}

func temporaryScope(r temporaryShareRecord, phase sharegrant.SignalPhase) sharegrant.SignalScope {
	plan := ""
	if phase == sharegrant.SignalTransfer {
		plan = r.Status.Plan
	}
	return sharegrant.SignalScope{GrantID: r.Status.ID, ClaimID: r.Status.ClaimID, TaskID: r.Status.TaskID, GrantGeneration: r.Status.Generation, Phase: phase, Manifest: r.Status.Manifest, Plan: plan}
}

func (s *Service) temporaryLocalCheck(e *temporaryShareEntry, scope sharegrant.SignalScope) error {
	e.mu.Lock()
	denied := e.denied
	e.mu.Unlock()
	if denied {
		return temporaryShareFailure(protocol.Cancelled)
	}
	r := temporaryRecordCopy(e)
	if s.isClosing() || r.Stopped || r.Intent != nil && r.Intent.Operation == signaling.ShareRevoke {
		return temporaryShareFailure(protocol.Cancelled)
	}
	if temporaryScope(r, scope.Phase) != scope || scope.Validate() != nil {
		return temporaryShareFailure(protocol.ShareRejected)
	}
	deadline := r.Status.ExpiresAt
	if scope.Phase == sharegrant.SignalTransfer {
		if r.Status.State != sharegrant.Bound {
			return temporaryShareFailure(protocol.ShareTerminal)
		}
		deadline = r.Status.ResumeUntil
	} else if r.Status.State != sharegrant.Approved {
		return temporaryShareFailure(protocol.ShareTerminal)
	}
	if time.Now().Unix() >= deadline {
		return temporaryShareFailure(protocol.ShareExpired)
	}
	peer, err := temporaryPeer(r)
	if err != nil {
		return err
	}
	if peer == "" {
		return temporaryShareFailure(protocol.ShareRejected)
	}
	if err = identity.CheckPeerAllowed(s.cfg.DataDir, peer); err != nil {
		return temporaryShareFailure(protocol.ShareRejected)
	}
	_, err = s.temporaryClient(r)
	return err
}

func (s *Service) temporaryAuthorize(ctx context.Context, e *temporaryShareEntry, scope sharegrant.SignalScope) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := s.temporaryLocalCheck(e, scope); err != nil {
		return err
	}
	if _, err := s.refreshTemporaryShare(ctx, e, scope.Phase == sharegrant.SignalTransfer); err != nil {
		return err
	}
	return s.temporaryLocalCheck(e, scope)
}

func (s *Service) connectTemporaryScope(ctx context.Context, e *temporaryShareEntry, scope sharegrant.SignalScope, status signaling.ShareStatus, cfg DirectConfig) (*PeerSession, error) {
	// Each new scope connection gets a fresh control proof. The dedicated WS
	// then rechecks its grant on registration and every frame; intermediate
	// handshake callbacks enforce the local cancellation/identity barrier.
	if err := s.temporaryLocalCheck(e, scope); err != nil {
		return nil, err
	}
	if scope.Phase == sharegrant.SignalTransfer {
		if err := s.temporaryAuthorize(ctx, e, scope); err != nil {
			return nil, err
		}
	}
	return s.connectSharePeer(ctx, scope, status, cfg, func() error {
		if err := ctx.Err(); err != nil {
			return err
		}
		return s.temporaryLocalCheck(e, scope)
	})
}

func (s *Service) StartTemporaryShare(ctx context.Context, id string, cfg DirectConfig) (TaskSnapshot, error) {
	return s.startTemporaryShare(ctx, id, cfg, false)
}

func (s *Service) ResumeTemporaryShare(ctx context.Context, id string, cfg DirectConfig) (TaskSnapshot, error) {
	return s.startTemporaryShare(ctx, id, cfg, true)
}

func (s *Service) startTemporaryShare(ctx context.Context, id string, cfg DirectConfig, resume bool) (TaskSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return TaskSnapshot{}, err
	}
	done, err := s.beginProfileWork()
	if err != nil {
		return TaskSnapshot{}, err
	}
	defer done()
	e, err := s.temporaryEntry(id)
	if err != nil {
		return TaskSnapshot{}, err
	}
	r := temporaryRecordCopy(e)
	if r.Stopped || r.Intent != nil || r.Status.State != sharegrant.Approved && r.Status.State != sharegrant.Bound {
		return TaskSnapshot{}, temporaryShareFailure(protocol.ShareConflict)
	}
	if resume && r.Status.State != sharegrant.Bound {
		return TaskSnapshot{}, temporaryShareFailure(protocol.ShareConflict)
	}
	if r.Status.State != sharegrant.Approved && r.Status.State != sharegrant.Bound {
		return TaskSnapshot{}, temporaryShareFailure(protocol.ShareTerminal)
	}
	if r.Status.State == sharegrant.Bound && (r.Manifest == nil || r.Role == "receiver" && r.Plan == nil) {
		return TaskSnapshot{}, temporaryShareFailure(protocol.ShareRejected)
	}
	peer, err := temporaryPeer(r)
	if err != nil {
		return TaskSnapshot{}, err
	}
	s.operationMu.Lock()
	defer s.operationMu.Unlock()
	if s.isClosing() {
		return TaskSnapshot{}, temporaryShareFailure(protocol.Cancelled)
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.active || e.denied || e.record.Stopped || e.record.Intent != nil {
		return TaskSnapshot{}, temporaryShareFailure(protocol.ShareConflict)
	}
	r = cloneTemporaryRecord(e.record)
	deadline := r.Status.ExpiresAt
	if r.Status.State == sharegrant.Bound {
		deadline = r.Status.ResumeUntil
	}
	runCtx, cancel := context.WithDeadline(s.workCtx, time.Unix(deadline, 0))
	var task *taskRecord
	if r.TaskRecordID != "" {
		s.tasks.mu.RLock()
		task = s.tasks.tasks[r.TaskRecordID]
		for key, other := range s.tasks.tasks {
			if key != r.TaskRecordID {
				other.mu.RLock()
				busy := other.cancel != nil
				other.mu.RUnlock()
				if busy {
					s.tasks.mu.RUnlock()
					cancel()
					return TaskSnapshot{}, temporaryShareFailure(protocol.ShareConflict)
				}
			}
		}
		s.tasks.mu.RUnlock()
		if task == nil {
			cancel()
			return TaskSnapshot{}, temporaryShareFailure(protocol.ShareInvalid)
		}
		task.mu.Lock()
		if task.cancel != nil || task.recovery.TemporaryShareID != r.ID || task.snap.State == "completed" || task.snap.State == "cancelled" {
			task.mu.Unlock()
			cancel()
			return TaskSnapshot{}, temporaryShareFailure(protocol.ShareConflict)
		}
		task.snap.AttemptID = protocol.RandomID()
		task.snap.AttemptStartedAt = time.Now().UTC().Format(time.RFC3339Nano)
		task.snap.State = "recovering"
		task.snap.Phase = "recovering"
		task.snap.ErrorCode = ""
		task.snap.ErrorMessage = ""
		task.snap.FailureDiagnostic = nil
		task.snap.CanResume = false
		task.snap.CanRetry = false
		task.snap.CanCancel = true
		task.snap.CanPause = false
		task.snap.EndedAt = ""
		clearDirectEvidence(&task.snap)
		task.snap.Revision++
		task.snap.UpdatedAt = task.snap.AttemptStartedAt
		task.cancel = cancel
		task.cfg = cfg
		snap, recovery := task.snap, task.recovery
		task.mu.Unlock()
		task.save(snap, recovery)
	} else {
		direction := "send"
		if r.Role == "receiver" {
			direction = "receive"
		}
		task, err = s.tasks.create(TaskSnapshot{TemporaryShareID: r.ID, Direction: direction, PeerID: peer, SourceSummary: sourceSummary(r.Paths), TransferID: r.Status.TaskID, ManifestDigest: r.Status.Manifest}, cancel)
		if err != nil {
			cancel()
			return TaskSnapshot{}, err
		}
		r.TaskRecordID = task.snapshot().ID
	}
	r.State = "active"
	r.Error = ""
	if err = s.saveTemporaryRecord(e, r); err != nil {
		cancel()
		task.finish("failed", err)
		return TaskSnapshot{}, err
	}
	e.active = true
	e.cancel = cancel
	e.decision = make(chan temporaryShareDecision, 1)
	e.decisionSent = false
	task.mu.Lock()
	task.peerID = peer
	task.paths = append([]string(nil), r.Paths...)
	task.cfg = cfg
	task.mu.Unlock()
	attempt := task.snapshot().AttemptID
	task.updateRecovery(attempt, func(recovery *taskRecovery) {
		recovery.TemporaryShareID = r.ID
		recovery.SourcePaths = append([]string(nil), r.Paths...)
		recovery.SourceSelection = cloneSourceSelection(r.Selection)
		if r.Selection != nil {
			recovery.Version = 2
		}
		recovery.PeerID = peer
		recovery.PeerFingerprint = peer
	})
	if r.Manifest != nil {
		s.bindTemporaryTaskManifest(task, attempt, r)
	}
	cfg = taskAttemptConfig(task, attempt, cfg)
	s.tasks.workers.Add(1)
	s.temporaryShares.wg.Add(1)
	go func() {
		defer s.tasks.workers.Done()
		defer s.temporaryShares.wg.Done()
		s.runTemporaryShare(runCtx, e, task, attempt, cfg)
		cancel()
		e.mu.Lock()
		e.active = false
		e.cancel = nil
		e.decision = nil
		e.mu.Unlock()
	}()
	return task.snapshot(), nil
}

func (s *Service) bindTemporaryTaskManifest(t *taskRecord, attempt string, r temporaryShareRecord) {
	if r.Manifest == nil {
		return
	}
	m := *r.Manifest
	t.updateRecordAttempt(attempt, func(v *TaskSnapshot, recovery *taskRecovery) {
		v.TransferID = m.TransferID
		v.ManifestDigest = m.Digest()
		v.ManifestSummary = manifestSummary(m)
		v.ChunkSize = m.ChunkSize
		v.FileCount = len(m.Files)
		total := m.TotalBytes()
		v.TotalBytes = &total
		v.TargetDirectory = r.Directory
		v.ReceivePlanDigest = r.Status.Plan
		v.RestartRecoverySupported = v.HistoryPersisted
		v.ByteResumeSupported = true
		recovery.TemporaryShareID = r.ID
		recovery.TransferID = m.TransferID
		recovery.ManifestDigest = m.Digest()
		recovery.ChunkSize = m.ChunkSize
		recovery.TotalBytes = total
		recovery.FileCount = len(m.Files)
		recovery.TargetDirectory = r.Directory
		recovery.ReceivePlanDigest = r.Status.Plan
		if r.Role == "sender" && recovery.SentChunks == nil {
			recovery.SentChunks = make(map[uint32][]bool)
			for _, file := range m.Files {
				if file.Type == "file" {
					recovery.SentChunks[file.ID] = make([]bool, len(file.Chunks))
				}
			}
		}
	})
}

func (s *Service) ConfirmTemporaryShareSave(ctx context.Context, id, directory string, request transfer.PlanRequest) (TemporaryShareSnapshot, error) {
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
	if r.Role != "receiver" || r.Stopped || !e.active || r.State != "awaiting_save" || e.decision == nil || e.decisionSent || r.Manifest == nil {
		e.mu.Unlock()
		return temporarySnapshot(r), temporaryShareFailure(protocol.ShareConflict)
	}
	// Freeze caller-owned slices/maps before the asynchronous confirmation.
	if request.SelectedIDs != nil {
		ids := make([]uint32, len(request.SelectedIDs))
		copy(ids, request.SelectedIDs)
		request.SelectedIDs = ids
	}
	policies := make(map[uint32]transfer.ConflictPolicy, len(request.Policies))
	for id, value := range request.Policies {
		policies[id] = value
	}
	request.Policies = policies
	result := make(chan error, 1)
	e.decisionSent = true
	e.decision <- temporaryShareDecision{directory: directory, request: request, result: result}
	e.mu.Unlock()
	select {
	case err = <-result:
		return temporarySnapshot(temporaryRecordCopy(e)), err
	case <-ctx.Done():
		return temporarySnapshot(temporaryRecordCopy(e)), ctx.Err()
	case <-s.workCtx.Done():
		return temporarySnapshot(temporaryRecordCopy(e)), s.workCtx.Err()
	}
}

func (s *Service) confirmTemporaryPlan(ctx context.Context, e *temporaryShareEntry, t *taskRecord, attempt string, manifest transfer.Manifest) (transfer.ReceivePlan, error) {
	e.mu.Lock()
	r := cloneTemporaryRecord(e.record)
	if r.Stopped || manifest.Digest() != r.Status.Manifest || manifest.TransferID != r.Status.TaskID {
		e.mu.Unlock()
		return transfer.ReceivePlan{}, temporaryShareFailure(protocol.ShareRejected)
	}
	r.Manifest = &manifest
	r.State = "awaiting_save"
	if err := s.saveTemporaryRecord(e, r); err != nil {
		e.mu.Unlock()
		return transfer.ReceivePlan{}, err
	}
	decision := e.decision
	e.mu.Unlock()
	s.bindTemporaryTaskManifest(t, attempt, r)
	t.updateAttempt(attempt, func(v *TaskSnapshot) {
		v.State = "awaiting_acceptance"
		v.Phase = "temporary_share_save_confirmation"
		v.CanPause = false
	})
	var selected temporaryShareDecision
	select {
	case selected = <-decision:
	case <-ctx.Done():
		return transfer.ReceivePlan{}, ctx.Err()
	}
	var confirmationErr error
	defer func() { selected.result <- confirmationErr }()
	if err := ctx.Err(); err != nil {
		confirmationErr = err
		return transfer.ReceivePlan{}, err
	}
	plan, err := transfer.BuildReceivePlan(ctx, selected.directory, manifest, selected.request)
	if err != nil {
		confirmationErr = classifyTaskError(err)
		return transfer.ReceivePlan{}, confirmationErr
	}
	e.mu.Lock()
	r = cloneTemporaryRecord(e.record)
	if r.Stopped || ctx.Err() != nil || r.Status.State != sharegrant.Approved || r.Intent != nil {
		e.mu.Unlock()
		confirmationErr = temporaryShareFailure(protocol.Cancelled)
		return transfer.ReceivePlan{}, confirmationErr
	}
	r.Plan = &plan
	r.Directory = selected.directory
	r.Intent = temporaryIntent(signaling.ShareBind, signaling.ShareRequest{ID: r.Status.ID, TaskID: r.Status.TaskID, Generation: r.Status.Generation, Manifest: manifest.Digest(), Plan: plan.Digest()})
	err = s.saveTemporaryRecord(e, r)
	e.mu.Unlock()
	if err == nil {
		_, err = s.executeTemporaryIntent(ctx, e, "")
	}
	if err != nil {
		confirmationErr = err
		return transfer.ReceivePlan{}, err
	}
	r = temporaryRecordCopy(e)
	s.bindTemporaryTaskManifest(t, attempt, r)
	return plan, nil
}

func prepareTemporaryShareSource(ctx context.Context, r temporaryShareRecord) (*transfer.Prepared, error) {
	if r.Manifest == nil {
		return nil, temporaryShareFailure(protocol.ShareInvalid)
	}
	prepared, err := transfer.PrepareSelectedForResume(ctx, r.Paths, r.Manifest.ChunkSize, r.Manifest.TransferID, r.Selection)
	if err != nil {
		return prepared, err
	}
	// A directory timestamp can settle after its children have been created and
	// closed on Windows. Keep the original wire metadata only for the same
	// directory entry; the complete fresh path set and every file's metadata and
	// hashes must still match the frozen manifest below.
	for i := range prepared.Manifest.Files {
		if i >= len(r.Manifest.Files) {
			break
		}
		entry := &prepared.Manifest.Files[i]
		frozen := r.Manifest.Files[i]
		if entry.Type == "directory" && frozen.Type == "directory" && entry.ID == frozen.ID && entry.Path == frozen.Path {
			entry.Modified = frozen.Modified
		}
	}
	if prepared.Manifest.Digest() != r.Manifest.Digest() {
		return prepared, protocol.Fail(protocol.SourceChanged, "temporary share source changed; confirm a new share")
	}
	return prepared, nil
}

func (s *Service) runTemporaryShare(ctx context.Context, e *temporaryShareEntry, t *taskRecord, attempt string, cfg DirectConfig) {
	r := temporaryRecordCopy(e)
	var prepared *transfer.Prepared
	var err error
	if r.Role == "sender" {
		prepared, err = prepareTemporaryShareSource(ctx, r)
		if prepared != nil {
			defer prepared.Close()
		}
		if err != nil {
			s.finishTemporaryRun(e, t, attempt, ctx, err)
			return
		}
	}
	if r.Status.State == sharegrant.Approved {
		scope := temporaryScope(r, sharegrant.SignalPreview)
		peer, connectErr := s.connectTemporaryScope(ctx, e, scope, r.Status, cfg)
		if connectErr != nil {
			s.finishTemporaryRun(e, t, attempt, ctx, connectErr)
			return
		}
		stopMonitor := s.monitorTemporaryPeer(ctx, e, peer, scope)
		if r.Role == "sender" {
			var planDigest string
			planDigest, err = sendSharePreview(ctx, peer, scope, prepared.Manifest)
			if err == nil {
				_, err = s.refreshTemporaryShare(ctx, e, false)
				if err == nil && temporaryRecordCopy(e).Status.Plan != planDigest {
					err = transfer.ErrPlanMismatch
				}
			}
		} else {
			_, _, err = receiveSharePreview(ctx, peer, scope, func(confirmCtx context.Context, m transfer.Manifest) (transfer.ReceivePlan, error) {
				return s.confirmTemporaryPlan(confirmCtx, e, t, attempt, m)
			})
		}
		stopMonitor()
		_ = peer.Close()
		if err != nil {
			s.finishTemporaryRun(e, t, attempt, ctx, err)
			return
		}
	}
	r = temporaryRecordCopy(e)
	if r.Status.State != sharegrant.Bound || r.Manifest == nil {
		s.finishTemporaryRun(e, t, attempt, ctx, temporaryShareFailure(protocol.ShareConflict))
		return
	}
	s.bindTemporaryTaskManifest(t, attempt, r)
	if err = s.IndexInboxManifest(ctx, t.snapshot().ID, *r.Manifest); err != nil {
		s.finishTemporaryRun(e, t, attempt, ctx, err)
		return
	}
	// The offer deadline ends at Bind. A separate context now owns exactly the
	// original non-sliding recovery window; it never extends preview authority.
	transferCtx, cancel := context.WithDeadline(s.workCtx, time.Unix(r.Status.ResumeUntil, 0))
	defer cancel()
	e.mu.Lock()
	if e.record.Stopped {
		e.mu.Unlock()
		s.finishTemporaryRun(e, t, attempt, ctx, temporaryShareFailure(protocol.Cancelled))
		return
	}
	e.cancel = cancel
	e.mu.Unlock()
	t.mu.Lock()
	if t.snap.AttemptID == attempt && t.snap.State != "cancel_requested" && t.snap.State != "shutdown_requested" {
		t.cancel = cancel
	}
	t.mu.Unlock()
	// Preserve cancellation already requested on the preview owner.
	if ctx.Err() != nil {
		s.finishTemporaryRun(e, t, attempt, ctx, ctx.Err())
		return
	}
	scope := temporaryScope(r, sharegrant.SignalTransfer)
	peer, err := s.connectTemporaryScope(transferCtx, e, scope, r.Status, cfg)
	if err != nil {
		s.finishTemporaryRun(e, t, attempt, transferCtx, err)
		return
	}
	stopMonitor := s.monitorTemporaryPeer(transferCtx, e, peer, scope)
	defer func() { stopMonitor(); _ = peer.Close() }()
	if err = s.temporaryAuthorize(transferCtx, e, scope); err != nil {
		s.finishTemporaryRun(e, t, attempt, transferCtx, err)
		return
	}
	if s.beforeFileOpen != nil {
		s.beforeFileOpen(peer)
	}
	if err = s.temporaryLocalCheck(e, scope); err != nil {
		s.finishTemporaryRun(e, t, attempt, transferCtx, err)
		return
	}
	var result transfer.Result
	if r.Role == "sender" {
		stream, openErr := peer.Data.Conn.OpenStreamSync(transferCtx)
		err = openErr
		if err == nil {
			result, err = transfer.SendWithOptions(transferCtx, transport.WrapStream(stream), prepared, transfer.SendOptions{BodyBudget: temporaryBodyBudget{s, e, scope}, Hooks: transfer.SendHooks{Progress: func(p transfer.Progress) { t.progress(attempt, p) }, PreviouslySent: t.wasChunkSent, ChunkSent: func(sent transfer.ChunkTransmission) { t.recordChunkSent(attempt, sent) }, SelectionAccepted: func(selection transfer.Selection, summary transfer.PlanSummary) error {
				return t.recordSelectionAccepted(attempt, selection, summary)
			}}})
		}
	} else {
		stream, openErr := peer.Data.Conn.AcceptStream(transferCtx)
		err = openErr
		if err == nil {
			lastReceived := int64(0)
			result, err = transfer.ReceiveWithOptions(transferCtx, transport.WrapStream(stream), transfer.ReceiveOptions{Directory: r.Directory, Peer: peer.PeerID, Plan: func(planCtx context.Context, offer transfer.Offer) (transfer.ReceivePlan, error) {
				if offer.Content != nil || offer.Manifest.TransferID != scope.TaskID || offer.Manifest.Digest() != scope.Manifest || r.Plan == nil || r.Plan.Digest() != scope.Plan {
					return transfer.ReceivePlan{}, transfer.ErrPlanMismatch
				}
				if err := s.temporaryAuthorize(planCtx, e, scope); err != nil {
					return transfer.ReceivePlan{}, err
				}
				return *r.Plan, nil
			}, PlanChanged: func(plan transfer.ReceivePlan) error {
				if plan.Digest() != scope.Plan || plan.Directory != r.Directory {
					return transfer.ErrPlanMismatch
				}
				if err := s.temporaryLocalCheck(e, scope); err != nil {
					return err
				}
				t.mu.Lock()
				if t.snap.AttemptID != attempt || t.snap.State == "cancel_requested" || t.snap.State == "pause_requested" || t.snap.State == "shutdown_requested" {
					t.mu.Unlock()
					return context.Canceled
				}
				err := t.persistPlanLocked(plan, *r.Manifest)
				snap := t.snap
				t.mu.Unlock()
				if t.changed != nil {
					t.changed(snap)
				}
				if err != nil {
					return err
				}
				return s.RegisterInboxReceivePlan(snap.ID, *r.Manifest, plan)
			}, Progress: func(p transfer.Progress) {
				if p.Received > lastReceived {
					t.recordReceived(attempt, p.Received-lastReceived)
					lastReceived = p.Received
				}
				t.progress(attempt, p)
			}})
		}
	}
	if err != nil {
		s.finishTemporaryRun(e, t, attempt, transferCtx, err)
		return
	}
	// Mark local engine completion before the shared terminal notification. A
	// lost completion response remains an explicit durable reconciliation intent.
	e.mu.Lock()
	next := cloneTemporaryRecord(e.record)
	if next.Stopped {
		e.mu.Unlock()
		s.finishTemporaryRun(e, t, attempt, transferCtx, temporaryShareFailure(protocol.Cancelled))
		return
	}
	next.Completion = &result
	next.Intent = temporaryIntent(signaling.ShareComplete, temporaryStatusRequest(next))
	next.State = "ready"
	err = s.saveTemporaryRecord(e, next)
	e.mu.Unlock()
	if err == nil {
		_, err = s.executeTemporaryIntent(transferCtx, e, "")
	}
	if err != nil {
		s.finishTemporaryRun(e, t, attempt, transferCtx, err)
		return
	}
	t.completeTransfer(attempt, result, peer.SessionID)
}

func (s *Service) finishTemporaryRun(e *temporaryShareEntry, t *taskRecord, attempt string, ctx context.Context, err error) {
	if ctx.Err() == nil && (protocol.ErrorCode(err) == protocol.SourceChanged || errors.Is(err, transfer.ErrChanged)) {
		r := temporaryRecordCopy(e)
		_ = s.stopTemporaryShare(e, r.Role == "sender")
		t.finishAttempt(attempt, "failed", protocol.Fail(protocol.SourceChanged, "temporary share source changed; confirm a new share"))
		return
	}
	e.mu.Lock()
	r := cloneTemporaryRecord(e.record)
	if !r.Stopped && r.State != "completed" {
		r.State = "needs_attention"
		r.Error = protocol.ErrorCode(classifyTaskError(err))
		if t.snapshot().State == "pause_requested" {
			r.State = "paused"
		}
		_ = s.saveTemporaryRecord(e, r)
	}
	e.mu.Unlock()
	if r.Intent != nil && r.Intent.Operation == signaling.ShareComplete && r.Completion != nil && !r.Stopped {
		t.recoverWithCode(attempt, protocol.ConnectionInterrupted, "temporary_share_completion_check")
		return
	}
	handleTaskRunError(t, attempt, ctx, err)
	t.updateAttempt(attempt, func(v *TaskSnapshot) {
		v.CanRetry = false
		if r.Stopped {
			v.CanResume = false
		}
	})
}

func (s *Service) monitorTemporaryPeer(parent context.Context, e *temporaryShareEntry, peer *PeerSession, scope sharegrant.SignalScope) func() {
	ctx, cancel := context.WithCancel(parent)
	done := make(chan struct{})
	localDone := make(chan struct{})
	go func() {
		defer close(localDone)
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				r := temporaryRecordCopy(e)
				if scope.Phase == sharegrant.SignalPreview && (r.Status.State == sharegrant.Bound || r.Status.State == sharegrant.Completed) {
					continue
				}
				if r.Status.State == sharegrant.Completed {
					if r.Completion != nil {
						continue
					}
					s.tasks.mu.RLock()
					task := s.tasks.tasks[r.TaskRecordID]
					s.tasks.mu.RUnlock()
					if task != nil {
						v := task.snapshot()
						if v.TotalBytes != nil && v.CommittedBytes >= *v.TotalBytes && v.VerifiedBytes >= *v.TotalBytes {
							continue
						}
					}
				}
				if err := s.temporaryLocalCheck(e, scope); err != nil {
					e.mu.Lock()
					next := cloneTemporaryRecord(e.record)
					if !next.Stopped {
						next.Stopped = true
						next.State = "stopped"
						next.Error = protocol.ErrorCode(err)
						_ = s.saveTemporaryRecord(e, next)
					}
					e.denied = true
					if e.cancel != nil {
						e.cancel()
					}
					e.mu.Unlock()
					return
				}
			}
		}
	}()
	go func() {
		defer close(done)
		for {
			_, err := peer.signal.Read(ctx)
			if err == nil {
				continue
			}
			if ctx.Err() != nil {
				return
			}
			code := protocol.ErrorCode(err)
			switch code {
			case protocol.ShareTerminal, protocol.ShareExpired, protocol.ShareRejected, protocol.AuthenticationFailed, protocol.Replay:
				// A successful Bind intentionally retires the preview WS. Verify
				// that transition instead of treating it as a revoked bound task.
				checkCtx, stop := context.WithTimeout(s.workCtx, 3*time.Second)
				_, readErr := s.refreshTemporaryShare(checkCtx, e, false)
				stop()
				r := temporaryRecordCopy(e)
				if scope.Phase == sharegrant.SignalPreview && readErr == nil && r.Status.State == sharegrant.Bound {
					return
				}
				if r.Status.State == sharegrant.Completed || r.Intent != nil && r.Intent.Operation == signaling.ShareComplete {
					return
				}
				e.mu.Lock()
				next := cloneTemporaryRecord(e.record)
				next.Stopped = true
				next.State = "stopped"
				next.Error = code
				_ = s.saveTemporaryRecord(e, next)
				if e.cancel != nil {
					e.cancel()
				}
				e.mu.Unlock()
			}
			return
		}
	}()
	return func() { cancel(); <-done; <-localDone }
}
