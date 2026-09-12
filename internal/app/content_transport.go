package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/Wen5555/LinkSend/internal/content"
	"github.com/Wen5555/LinkSend/internal/transfer"
)

func copyContentDescriptor(d *transfer.ContentDescriptor) *transfer.ContentDescriptor {
	if d == nil {
		return nil
	}
	value := *d
	return &value
}

func (s *Service) recordContentAcceptance(ctx context.Context, t *taskRecord, attemptID string, a transfer.ContentAcceptance) error {
	record, err := s.loadContentTask(ctx, t.snapshot().ID)
	if err != nil {
		return err
	}
	if record.mode == "file" {
		a.Content = nil
		a.ContentDigest = ""
		a.FileFallback = true
	}
	mode := "native"
	if a.FileFallback {
		mode = "file"
	}
	if mode == "native" && a.Content == nil {
		return transfer.ErrContentMismatch
	}
	if record.mode != "pending" && (record.mode != mode || record.binding != a.ContentDigest) {
		return transfer.ErrContentMismatch
	}
	if record.descriptor != nil && a.Content != nil && *record.descriptor != *a.Content {
		return transfer.ErrContentMismatch
	}
	var descriptor []byte
	if a.Content != nil {
		descriptor, _ = json.Marshal(a.Content)
	}
	_, err = s.store.db.ExecContext(ctx, `UPDATE content_tasks SET descriptor=?,binding=?,mode=? WHERE task_id=?`, descriptor, a.ContentDigest, mode, record.taskID)
	if err != nil {
		return errors.Join(transfer.ErrPlanPersistence, err)
	}
	// Record the negotiated interpretation first. A later history failure can
	// retain extra metadata, but cannot leave recovery claiming a binding that
	// was never durably registered. Both writes precede the accepted barrier.
	if err = t.persistContentRecovery(attemptID, record.snapshotID, mode, a.ContentDigest); err != nil {
		return err
	}
	s.notifyChange()
	return nil
}

func (t *taskRecord) persistContentRecovery(attemptID, snapshotID, mode, binding string) error {
	t.mu.Lock()
	if t.snap.AttemptID != attemptID || isTerminal(t.snap.State) || t.snap.State == "cancel_requested" || t.snap.State == "pause_requested" || t.snap.State == "shutdown_requested" {
		t.mu.Unlock()
		return context.Canceled
	}
	if t.recovery.ContentMode != "" && (t.recovery.ContentMode != mode || t.recovery.ContentBinding != binding || t.recovery.ContentSnapshotID != snapshotID) {
		t.mu.Unlock()
		return transfer.ErrContentMismatch
	}
	snap, recovery := t.snap, t.recovery
	recovery.ContentMode = mode
	recovery.ContentBinding = binding
	recovery.ContentSnapshotID = snapshotID
	snap.Revision++
	snap.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if t.persist == nil {
		t.mu.Unlock()
		return transfer.ErrPlanPersistence
	}
	if err := t.persist(snap, recovery); err != nil {
		t.snap.HistoryPersisted = false
		t.mu.Unlock()
		return errors.Join(transfer.ErrPlanPersistence, err)
	}
	t.snap, t.recovery = snap, recovery
	t.mu.Unlock()
	if t.changed != nil {
		t.changed(snap)
	}
	return nil
}

func (s *Service) registerIncomingContent(ctx context.Context, t *taskRecord, attemptID string, offer transfer.Offer) error {
	snapshot := t.snapshot()
	prior, err := s.loadContentTask(ctx, snapshot.ID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if offer.Content == nil && !offer.FileFallback && errors.Is(err, sql.ErrNoRows) {
		if t.recoverySnapshot().ContentMode != "" {
			return transfer.ErrContentMismatch
		}
		return nil
	}
	mode := "native"
	if offer.Content == nil {
		mode = "file"
	}
	if err == nil && (prior.mode != mode || prior.binding != offer.ContentDigest) {
		return transfer.ErrContentMismatch
	}
	var descriptor []byte
	if offer.Content != nil {
		descriptor, _ = json.Marshal(offer.Content)
	}
	_, err = s.store.db.ExecContext(ctx, `INSERT INTO content_tasks(task_id,direction,descriptor,binding,mode,allow_file_fallback)VALUES(?,'receive',?,?,?,?) ON CONFLICT(task_id) DO UPDATE SET descriptor=excluded.descriptor,binding=excluded.binding,mode=excluded.mode`, snapshot.ID, descriptor, offer.ContentDigest, mode, mode == "file")
	if err != nil {
		return errors.Join(transfer.ErrPlanPersistence, err)
	}
	return t.persistContentRecovery(attemptID, "", mode, offer.ContentDigest)
}

// Called before creating a resumed attempt. The private recovery record makes
// a missing content row a hard error instead of silently becoming a plain file.
func (s *Service) configureContentResume(ctx context.Context, id string, recovery taskRecovery, cfg *DirectConfig) error {
	record, err := s.loadContentTask(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		if recovery.ContentMode != "" || recovery.ContentSnapshotID != "" {
			return transfer.ErrContentMismatch
		}
		return nil
	}
	if err != nil {
		return err
	}
	if recovery.ContentMode != "" && (recovery.ContentMode != record.mode || recovery.ContentBinding != record.binding || recovery.ContentSnapshotID != record.snapshotID) {
		return transfer.ErrContentMismatch
	}
	if record.direction != "send" {
		if record.mode == "native" && !s.content.enabled.Load() {
			return transfer.ErrContentUnsupported
		}
		return nil
	}
	if record.snapshot == nil || record.snapshotID != record.snapshot.ID {
		return transfer.ErrContentMismatch
	}
	store, err := s.contentStoreLocked()
	if err != nil {
		return err
	}
	actual, err := store.Metadata(record.snapshotID)
	if err != nil {
		return err
	}
	if actual != *record.snapshot {
		return content.ErrChanged
	}
	if _, err = store.OwnedPath(ctx, record.snapshotID); err != nil {
		return err
	}
	cfg.contentSnapshot = record.snapshot
	cfg.contentAllowFallback = record.allowFallback
	cfg.contentForceFile = record.mode == "file"
	cfg.contentTaskID = id
	return nil
}

func (s *Service) sendContentOptions(ctx context.Context, p *transfer.Prepared, cfg DirectConfig, hooks transfer.SendHooks) (transfer.SendOptions, error) {
	options := transfer.SendOptions{Hooks: hooks}
	if cfg.contentSnapshot == nil {
		return options, nil
	}
	if cfg.contentTaskID == "" || cfg.contentAttemptID == "" {
		return options, transfer.ErrContentMismatch
	}
	d, err := transfer.NewContentDescriptor(p.Manifest, *cfg.contentSnapshot)
	if err != nil {
		return options, err
	}
	if !cfg.contentForceFile {
		options.Content = &d
		options.AllowFileFallback = cfg.contentAllowFallback
	}
	t, err := s.incomingTask(cfg.contentTaskID)
	if err != nil {
		return options, err
	}
	priorHook := hooks.ContentAccepted
	options.Hooks.ContentAccepted = func(acceptance transfer.ContentAcceptance) error {
		if cfg.contentForceFile {
			acceptance = transfer.ContentAcceptance{FileFallback: true}
		}
		if err := s.recordContentAcceptance(ctx, t, cfg.contentAttemptID, acceptance); err != nil {
			return err
		}
		if priorHook != nil {
			return priorHook(acceptance)
		}
		return nil
	}
	return options, nil
}
