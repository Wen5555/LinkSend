package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Wen5555/LinkSend/internal/protocol"
	"github.com/Wen5555/LinkSend/internal/transfer"
)

const maxIncomingPage = 200

// incomingOffer is private, attempt-scoped memory. Neither chunk hashes nor
// the full manifest are serialized into history snapshots or JavaScript IPC.
type incomingOffer struct {
	ctx             context.Context
	manifest        transfer.Manifest
	subsetSupported bool
	resume          bool
	preview         *transfer.ReceivePlan
	decided         bool
}

type IncomingFilesRequest struct {
	ExpectedRevision uint64 `json:"expected_revision"`
	Offset           int    `json:"offset"`
	Limit            int    `json:"limit"`
}
type IncomingFile struct {
	ID         uint32 `json:"id"`
	Path       string `json:"path"`
	Type       string `json:"type"`
	Size       int64  `json:"size"`
	Selected   bool   `json:"selected"`
	TargetPath string `json:"target_path,omitempty"`
}
type IncomingFilesPage struct {
	Revision        uint64         `json:"revision"`
	Files           []IncomingFile `json:"files"`
	Offset          int            `json:"offset"`
	TotalEntries    int            `json:"total_entries"`
	OriginalTotal   int64          `json:"original_total"`
	Directory       string         `json:"directory"`
	SubsetSupported bool           `json:"subset_supported"`
	Resume          bool           `json:"resume"`
}
type IncomingPlanRequest struct {
	ExpectedRevision uint64                             `json:"expected_revision"`
	Directory        string                             `json:"directory"`
	SelectedIDs      []uint32                           `json:"selected_ids"`
	ConflictPolicy   transfer.ConflictPolicy            `json:"conflict_policy"`
	Policies         map[uint32]transfer.ConflictPolicy `json:"policies"`
}
type IncomingPlanPreview struct {
	Revision        uint64               `json:"revision"`
	PlanDigest      string               `json:"plan_digest"`
	Directory       string               `json:"directory"`
	Summary         transfer.PlanSummary `json:"summary"`
	AvailableBytes  uint64               `json:"available_bytes"`
	ReservedBytes   uint64               `json:"reserved_bytes"`
	RequiredBytes   uint64               `json:"required_bytes"`
	SpaceSufficient bool                 `json:"space_sufficient"`
	SpaceEstimate   string               `json:"space_estimate"`
}

// This is a conservative free-space admission check, not a filesystem quota
// guarantee: metadata/inodes and safe resume copy peaks depend on the volume.
// Count full selected content and reserve 16 MiB for checkpoint/metadata writes.
func receiveSpaceRequirement(summary transfer.PlanSummary) (required, reserved uint64) {
	reserved = 16 << 20
	return uint64(summary.SelectedTotal) + reserved, reserved
}

func (s *Service) incomingTask(id string) (*taskRecord, error) {
	s.tasks.mu.RLock()
	t := s.tasks.tasks[id]
	s.tasks.mu.RUnlock()
	if t == nil {
		return nil, errors.New("TASK_NOT_FOUND")
	}
	return t, nil
}

func (t *taskRecord) checkIncomingLocked(revision uint64) error {
	if t.snap.Direction != "receive" || t.snap.State != "awaiting_acceptance" || t.incoming == nil || t.incoming.decided {
		return errors.New("TASK_NOT_AWAITING_ACCEPTANCE")
	}
	if revision != t.snap.Revision {
		return errors.New("REVISION_CONFLICT")
	}
	return t.incoming.ctx.Err()
}

func (s *Service) IncomingFiles(id string, req IncomingFilesRequest) (IncomingFilesPage, error) {
	t, err := s.incomingTask(id)
	if err != nil {
		return IncomingFilesPage{}, err
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	if err := t.checkIncomingLocked(req.ExpectedRevision); err != nil {
		return IncomingFilesPage{}, err
	}
	if req.Offset < 0 || req.Offset > len(t.incoming.manifest.Files) || req.Limit < 0 || req.Limit > maxIncomingPage {
		return IncomingFilesPage{}, errors.New("INVALID_ARGUMENT")
	}
	if req.Limit == 0 {
		req.Limit = maxIncomingPage
	}
	in := t.incoming
	page := IncomingFilesPage{Revision: t.snap.Revision, Files: make([]IncomingFile, 0, req.Limit), Offset: req.Offset, TotalEntries: len(in.manifest.Files), OriginalTotal: in.manifest.TotalBytes(), Directory: t.snap.TargetDirectory, SubsetSupported: in.subsetSupported, Resume: in.resume}
	selected := make(map[uint32]string)
	if in.preview != nil {
		page.Directory = in.preview.Directory
		for _, entry := range in.preview.Entries {
			selected[entry.FileID] = entry.Path
		}
	}
	end := min(len(in.manifest.Files), req.Offset+req.Limit)
	for _, entry := range in.manifest.Files[req.Offset:end] {
		path, yes := selected[entry.ID]
		if in.preview == nil {
			path, yes = entry.Path, true
		}
		page.Files = append(page.Files, IncomingFile{ID: entry.ID, Path: entry.Path, Type: entry.Type, Size: entry.Size, Selected: yes, TargetPath: path})
	}
	return page, nil
}

func (s *Service) IncomingPlan(id string, req IncomingPlanRequest) (IncomingPlanPreview, error) {
	done, err := s.beginProfileWork()
	if err != nil {
		return IncomingPlanPreview{}, err
	}
	defer done()
	return s.previewIncomingPlan(id, req, false)
}

func validateReceiveDirectory(directory string) error {
	info, err := os.Stat(directory)
	if err != nil || !info.IsDir() {
		return protocol.Wrap(protocol.ReceiveDirectoryUnavailable, "保存的接收目录不可用，请恢复原目录后重试。", err)
	}
	return nil
}

func (s *Service) previewIncomingPlan(id string, req IncomingPlanRequest, createDefault bool) (IncomingPlanPreview, error) {
	t, err := s.incomingTask(id)
	if err != nil {
		return IncomingPlanPreview{}, err
	}
	t.mu.RLock()
	err = t.checkIncomingLocked(req.ExpectedRevision)
	in, attemptID, directory, peer := t.incoming, t.snap.AttemptID, t.snap.TargetDirectory, t.snap.PeerID
	t.mu.RUnlock()
	if err != nil {
		return IncomingPlanPreview{}, err
	}
	if err = s.checkPeerAllowed(peer); err != nil {
		return IncomingPlanPreview{}, err
	}
	if strings.TrimSpace(req.Directory) != "" {
		directory = req.Directory
	}
	if len(directory) > 32768 || strings.ContainsRune(directory, 0) {
		return IncomingPlanPreview{}, transfer.ErrPlanInvalid
	}
	directory, err = filepath.Abs(directory)
	if err != nil {
		return IncomingPlanPreview{}, err
	}
	if createDefault && !in.resume {
		if err = os.MkdirAll(directory, 0700); err != nil {
			return IncomingPlanPreview{}, classifyTaskError(err)
		}
	}
	info, err := os.Stat(directory)
	if err != nil || !info.IsDir() {
		return IncomingPlanPreview{}, protocol.Wrap(protocol.ReceiveDirectoryUnavailable, "接收目录不可用，请重新选择已存在的目录。", err)
	}
	plan, err := transfer.BuildReceivePlan(in.ctx, directory, in.manifest, transfer.PlanRequest{SelectedIDs: req.SelectedIDs, ConflictPolicy: req.ConflictPolicy, Policies: req.Policies})
	if err != nil {
		return IncomingPlanPreview{}, classifyTaskError(err)
	}
	if !in.subsetSupported && plan.Selection().Digest != transfer.FullReceivePlan(in.manifest).Selection().Digest {
		return IncomingPlanPreview{}, transfer.ErrPlanUnsupported
	}
	available, err := availableDiskBytes(directory)
	if err != nil {
		return IncomingPlanPreview{}, classifyTaskError(err)
	}
	summary := plan.Summary(in.manifest)
	required, reserved := receiveSpaceRequirement(summary)
	preview := IncomingPlanPreview{PlanDigest: plan.Digest(), Directory: directory, Summary: summary, AvailableBytes: available, ReservedBytes: reserved, RequiredBytes: required, SpaceSufficient: available >= required, SpaceEstimate: "full_selected_content_plus_metadata_reserve_not_a_quota_guarantee"}
	t.mu.Lock()
	if err = t.checkIncomingLocked(req.ExpectedRevision); err == nil && t.snap.AttemptID != attemptID {
		err = errors.New("REVISION_CONFLICT")
	}
	if err != nil {
		t.mu.Unlock()
		return IncomingPlanPreview{}, err
	}
	t.incoming.preview = &plan
	t.snap.Revision++
	t.snap.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	preview.Revision = t.snap.Revision
	snap := t.snap
	t.mu.Unlock()
	if t.changed != nil {
		t.changed(snap)
	}
	return preview, nil
}

func (s *Service) AcceptReceivePlan(id string, revision uint64, digest string) error {
	done, err := s.beginProfileWork()
	if err != nil {
		return err
	}
	defer done()
	return s.acceptReceivePlan(id, revision, digest)
}

func (s *Service) acceptReceivePlan(id string, revision uint64, digest string) error {
	t, err := s.incomingTask(id)
	if err != nil {
		return err
	}
	t.mu.Lock()
	if err = t.checkIncomingLocked(revision); err != nil {
		t.mu.Unlock()
		return err
	}
	if err = s.checkPeerAllowed(t.snap.PeerID); err != nil {
		t.mu.Unlock()
		return err
	}
	plan := t.incoming.preview
	if plan == nil || plan.Digest() != digest {
		t.mu.Unlock()
		return transfer.ErrPlanMismatch
	}
	available, err := availableDiskBytes(plan.Directory)
	if err != nil {
		t.mu.Unlock()
		return classifyTaskError(err)
	}
	required, _ := receiveSpaceRequirement(plan.Summary(t.incoming.manifest))
	if available < required {
		t.mu.Unlock()
		return protocol.Fail(protocol.DiskFull, "接收磁盘空间不足，请清理空间或选择其他目录。")
	}
	err = t.persistPlanLocked(*plan, t.incoming.manifest)
	if err == nil {
		t.incoming.decided = true
		t.decision <- true
	}
	snap := t.snap
	t.mu.Unlock()
	if t.changed != nil {
		t.changed(snap)
	}
	return err
}

// This critical write is serialized with cancel/pause and completes before an
// acceptance can be emitted. Unlike UI progress persistence, failures abort.
func (t *taskRecord) persistPlanLocked(plan transfer.ReceivePlan, m transfer.Manifest) error {
	snap, recovery := t.snap, t.recovery
	snap.TargetDirectory = plan.Directory
	snap.ReceivePlanDigest = plan.Digest()
	snap.SelectionDigest = plan.Selection().Digest
	applySelectionSummary(&snap, plan.Summary(m))
	snap.Revision++
	snap.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	recovery.TargetDirectory = plan.Directory
	recovery.ReceivePlanDigest = snap.ReceivePlanDigest
	recovery.SelectionDigest = snap.SelectionDigest
	if t.persist == nil {
		return transfer.ErrPlanPersistence
	}
	if err := t.persist(snap, recovery); err != nil {
		t.snap.HistoryPersisted = false
		return errors.Join(transfer.ErrPlanPersistence, err)
	}
	t.snap, t.recovery = snap, recovery
	return nil
}

func applySelectionSummary(snap *TaskSnapshot, summary transfer.PlanSummary) {
	snap.OriginalTotal = summary.OriginalTotal
	snap.SelectedFiles = summary.SelectedFiles
	snap.SelectedEntries = summary.SelectedEntries
	snap.SkippedFiles = summary.SkippedFiles
	snap.SkippedEntries = summary.SkippedEntries
	snap.SkippedBytes = summary.SkippedBytes
	n := summary.SelectedTotal
	snap.TotalBytes = &n
}

func (s *Service) taskReceiveOptions(ctx context.Context, t *taskRecord, attemptID, directory string, resume *taskRecovery, autoAccept bool) transfer.ReceiveOptions {
	var manifest transfer.Manifest
	var lastReceived int64
	return transfer.ReceiveOptions{Directory: directory, Plan: func(planCtx context.Context, offer transfer.Offer) (transfer.ReceivePlan, error) {
		manifest = offer.Manifest
		m := manifest
		if resume != nil && (m.TransferID != resume.TransferID || m.Digest() != resume.ManifestDigest || m.ChunkSize != resume.ChunkSize || m.TotalBytes() != resume.TotalBytes || len(m.Files) != resume.FileCount) {
			return transfer.ReceivePlan{}, transfer.ErrResumeMismatch
		}
		in := &incomingOffer{ctx: planCtx, manifest: m, resume: resume != nil}
		in.decided = resume != nil // ResumeTask already provided explicit consent.
		for _, capability := range offer.Capabilities {
			if capability == transfer.CapabilityReceivePlan {
				in.subsetSupported = true
			}
		}
		if !t.updateRecordAttempt(attemptID, func(v *TaskSnapshot, r *taskRecovery) {
			t.incoming = in
			v.State = "awaiting_acceptance"
			v.Phase = "awaiting_acceptance"
			if resume != nil {
				v.State = "recovering"
				v.Phase = "revalidating_receive_plan"
			}
			n := m.TotalBytes()
			v.TotalBytes = &n
			v.OriginalTotal = n
			v.FileCount = len(m.Files)
			v.ManifestSummary = manifestSummary(m)
			v.TransferID = m.TransferID
			v.ManifestDigest = m.Digest()
			v.ChunkSize = m.ChunkSize
			v.CanCancel = true
			v.CanPause = false
			v.RestartRecoverySupported = v.HistoryPersisted
			v.ByteResumeSupported = true
			r.Direction = "receive"
			r.PeerID = v.PeerID
			r.PeerFingerprint = v.PeerID
			r.TargetDirectory = directory
			r.TransferID = m.TransferID
			r.ManifestDigest = m.Digest()
			r.ChunkSize = m.ChunkSize
			r.TotalBytes = n
			r.FileCount = len(m.Files)
		}) {
			return transfer.ReceivePlan{}, context.Canceled
		}
		if err := s.checkPeerAllowed(t.snapshot().PeerID); err != nil {
			return transfer.ReceivePlan{}, err
		}
		if err := s.IndexInboxManifest(planCtx, t.snapshot().ID, m); err != nil {
			return transfer.ReceivePlan{}, errors.Join(transfer.ErrPlanPersistence, err)
		}
		if resume != nil {
			info, err := os.Stat(directory)
			if err != nil || !info.IsDir() {
				return transfer.ReceivePlan{}, protocol.Wrap(protocol.ReceiveDirectoryUnavailable, "保存的接收目录不可用，请恢复原目录后重试。", err)
			}
			plan := offer.ResumePlan
			if plan == nil {
				if resume.ReceivePlanDigest != "" {
					return transfer.ReceivePlan{}, transfer.ErrResumeMismatch
				}
				legacy := transfer.FullReceivePlan(m)
				legacy.Directory = directory
				plan = &legacy
			}
			if resume.ReceivePlanDigest != "" && (plan.Digest() != resume.ReceivePlanDigest || plan.Selection().Digest != resume.SelectionDigest) {
				return transfer.ReceivePlan{}, transfer.ErrResumeMismatch
			}
			return *plan, nil
		}
		if autoAccept {
			if err := s.decideTask(t.snapshot().ID, true); err != nil {
				return transfer.ReceivePlan{}, err
			}
		}
		t.mu.RLock()
		decision := t.decision
		t.mu.RUnlock()
		select {
		case accepted := <-decision:
			if !accepted {
				return transfer.ReceivePlan{}, transfer.ErrRejected
			}
			t.mu.RLock()
			plan := in.preview
			t.mu.RUnlock()
			if plan == nil {
				return transfer.ReceivePlan{}, transfer.ErrPlanInvalid
			}
			return *plan, nil
		case <-ctx.Done():
			return transfer.ReceivePlan{}, ctx.Err()
		}
	}, PlanChanged: func(plan transfer.ReceivePlan) error {
		t.mu.Lock()
		if t.snap.AttemptID != attemptID || isTerminal(t.snap.State) || t.snap.State == "cancel_requested" || t.snap.State == "pause_requested" || t.snap.State == "shutdown_requested" {
			t.mu.Unlock()
			return context.Canceled
		}
		err := t.persistPlanLocked(plan, manifest)
		snap := t.snap
		t.mu.Unlock()
		if t.changed != nil {
			t.changed(snap)
		}
		if err != nil {
			return err
		}
		if err = s.RegisterInboxReceivePlan(snap.ID, manifest, plan); err != nil {
			return errors.Join(transfer.ErrPlanPersistence, err)
		}
		return nil
	}, Progress: func(p transfer.Progress) {
		if p.Received >= lastReceived {
			t.recordReceived(attemptID, p.Received-lastReceived)
			lastReceived = p.Received
		}
		t.progress(attemptID, p)
	}}
}

func (t *taskRecord) recordSelectionAccepted(attemptID string, selection transfer.Selection, summary transfer.PlanSummary) error {
	t.mu.Lock()
	if t.snap.AttemptID != attemptID || isTerminal(t.snap.State) || t.snap.State == "cancel_requested" || t.snap.State == "pause_requested" || t.snap.State == "shutdown_requested" {
		t.mu.Unlock()
		return context.Canceled
	}
	if t.recovery.SelectionDigest != "" && t.recovery.SelectionDigest != selection.Digest {
		t.mu.Unlock()
		return transfer.ErrPlanMismatch
	}
	snap, recovery := t.snap, t.recovery
	snap.SelectionDigest = selection.Digest
	recovery.SelectionDigest = selection.Digest
	applySelectionSummary(&snap, summary)
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

func (t *taskRecord) completeTransfer(attemptID string, result transfer.Result, sessionID string) {
	state := "completed"
	if result.State == "NoContent" {
		state = "no_content"
	}
	t.updateRecordAttempt(attemptID, func(v *TaskSnapshot, r *taskRecovery) {
		v.TransferID = result.TransferID
		if sessionID != "" {
			v.SessionID = sessionID
		}
		v.Phase = state
		v.ProcessedBytes = result.Bytes
		v.VerifiedBytes = result.Bytes
		v.CommittedBytes = result.Bytes
		v.CommittedFiles = result.SelectedFiles
		v.BilateralConfirmed = true
		r.LogicalCompleted = result.Bytes
		applySelectionSummary(v, transfer.PlanSummary{OriginalTotal: result.OriginalTotal, SelectedTotal: result.Bytes, SelectedFiles: result.SelectedFiles, SelectedEntries: result.SelectedEntries, SkippedFiles: result.SkippedFiles, SkippedEntries: result.SkippedEntries, SkippedBytes: result.SkippedBytes})
	})
	t.finishAttempt(attemptID, state, nil)
}
