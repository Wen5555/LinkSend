package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Wen5555/LinkSend/internal/transfer"
)

type inboxStage struct {
	TaskID         string
	TransferID     string
	PeerID         string
	RootPath       string
	Identity       string
	ManifestDigest string
	Parts          map[string]string
}

type InboxLocation struct {
	TaskID string `json:"task_id"`
	FileID uint32 `json:"file_id"`
	Name   string `json:"name"`
	Path   string `json:"-"`
}

type InboxCleanupIssue struct {
	TaskID string `json:"task_id"`
	Code   string `json:"code"`
}
type InboxCleanupResult struct {
	Removed   []string            `json:"removed"`
	Protected []string            `json:"protected"`
	Issues    []InboxCleanupIssue `json:"issues"`
}

// RegisterInboxReceivePlan is part of the same serialized PlanChanged callback
// as the app's plan persistence. Call only after openReceiver has durably
// checkpointed this plan, and propagate failures before accept/body/renamed Link.
func (s *Service) RegisterInboxReceivePlan(taskID string, m transfer.Manifest, plan transfer.ReceivePlan) error {
	done, err := s.beginWorkspaceWork()
	if err != nil {
		return err
	}
	defer done()
	if err = m.Validate(); err != nil {
		return err
	}
	if err = plan.Validate(m); err != nil {
		return err
	}
	s.tasks.mu.RLock()
	task := s.tasks.tasks[taskID]
	s.tasks.mu.RUnlock()
	if task == nil {
		return ErrMetadataNotFound
	}
	task.mu.RLock()
	snap, recovery, active := task.snap, task.recovery, task.cancel != nil
	task.mu.RUnlock()
	if !active || snap.Direction != "receive" || snap.TransferID != m.TransferID || snap.ManifestDigest != m.Digest() {
		return ErrInboxProtected
	}
	directory := plan.Directory
	if directory == "" {
		directory = recovery.TargetDirectory
	}
	if directory == "" || !filepath.IsAbs(directory) {
		return ErrInboxOwnership
	}
	root, stage, state, err := openInboxStage(directory, m.TransferID, recovery.PeerID, m.Digest())
	if err != nil {
		return err
	}
	defer root.Close()
	defer stage.Close()
	if state.Plan == nil || state.Plan.Digest() != plan.Digest() || state.PlanDigest != state.Plan.Digest() {
		return transfer.ErrPlanMismatch
	}
	dirFile, err := stage.Open(".")
	if err != nil {
		return err
	}
	identity, err := inboxFileIdentity(dirFile)
	_ = dirFile.Close()
	if err != nil {
		return err
	}
	parts, err := stagePartIdentities(stage, m)
	if err != nil {
		return err
	}
	encoded, _ := json.Marshal(parts)
	tx, err := s.store.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = indexInboxManifest(tx, taskID, m); err != nil {
		return err
	}
	if _, err = tx.Exec(`UPDATE inbox_files SET saved_path='' WHERE task_id=?`, taskID); err != nil {
		return err
	}
	statement, err := tx.Prepare(`UPDATE inbox_files SET saved_path=? WHERE task_id=? AND file_id=?`)
	if err != nil {
		return err
	}
	defer statement.Close()
	for _, entry := range plan.Entries {
		if _, err = statement.Exec(entry.Path, taskID, entry.FileID); err != nil {
			return err
		}
	}
	_, err = tx.Exec(`INSERT INTO inbox_staging(task_id,transfer_id,peer_id,root_path,stage_identity,manifest_digest,parts,registered_at) VALUES(?,?,?,?,?,?,?,?) ON CONFLICT(task_id) DO UPDATE SET transfer_id=excluded.transfer_id,peer_id=excluded.peer_id,root_path=excluded.root_path,stage_identity=excluded.stage_identity,manifest_digest=excluded.manifest_digest,parts=excluded.parts,registered_at=excluded.registered_at`, taskID, m.TransferID, recovery.PeerID, directory, identity, m.Digest(), encoded, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return err
	}
	return tx.Commit()
}

func openInboxStage(directory, transferID, peer, digest string) (*os.Root, *os.Root, transfer.ResumeState, error) {
	var state transfer.ResumeState
	if len(transferID) != 32 {
		return nil, nil, state, ErrInboxOwnership
	}
	// Transfer IDs must be lowercase hexadecimal, not arbitrary UI paths.
	for _, r := range transferID {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return nil, nil, state, ErrInboxOwnership
		}
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, nil, state, err
	}
	fail := func(e error) (*os.Root, *os.Root, transfer.ResumeState, error) {
		_ = root.Close()
		return nil, nil, state, e
	}
	name := ".linksend-" + transferID
	info, err := root.Lstat(name)
	if err != nil {
		return fail(err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fail(ErrInboxOwnership)
	}
	stage, err := root.OpenRoot(name)
	if err != nil {
		return fail(err)
	}
	state, err = readInboxStageState(stage, "state.json")
	if err != nil {
		_ = stage.Close()
		return fail(err)
	}
	if state.Peer != peer || state.Digest != digest || state.Manifest.TransferID != transferID || state.Manifest.Digest() != digest || state.Manifest.Validate() != nil {
		_ = stage.Close()
		return fail(ErrInboxOwnership)
	}
	return root, stage, state, nil
}

func readInboxStageState(stage *os.Root, name string) (transfer.ResumeState, error) {
	var state transfer.ResumeState
	info, err := stage.Lstat(name)
	if err != nil {
		return state, err
	}
	if !info.Mode().IsRegular() || info.Size() > 2*transfer.MaxMetadata {
		return state, ErrInboxOwnership
	}
	f, err := stage.Open(name)
	if err != nil {
		return state, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, 2*transfer.MaxMetadata+1))
	if err != nil {
		return state, err
	}
	if len(data) > 2*transfer.MaxMetadata || json.Unmarshal(data, &state) != nil {
		return state, ErrInboxOwnership
	}
	return state, nil
}

func stagePartIdentities(stage *os.Root, m transfer.Manifest) (map[string]string, error) {
	f, err := stage.Open(".")
	if err != nil {
		return nil, err
	}
	defer f.Close()
	parts := make(map[string]string)
	count := 0
	for {
		entries, readErr := f.ReadDir(128)
		for _, entry := range entries {
			count++
			if count > transfer.MaxEntries+2 {
				return nil, ErrInboxOwnership
			}
			if entry.Name() == "state.json" || entry.Name() == "checkpoint.tmp" {
				continue
			}
			if !strings.HasSuffix(entry.Name(), ".part") {
				return nil, ErrInboxOwnership
			}
			id, err := strconv.ParseUint(strings.TrimSuffix(entry.Name(), ".part"), 10, 32)
			if err != nil || id >= uint64(len(m.Files)) || m.Files[id].Type != "file" || entry.Name() != strconv.FormatUint(id, 10)+".part" {
				return nil, ErrInboxOwnership
			}
			info, err := stage.Lstat(entry.Name())
			if err != nil {
				return nil, err
			}
			if !info.Mode().IsRegular() {
				return nil, ErrInboxOwnership
			}
			part, err := stage.Open(entry.Name())
			if err != nil {
				return nil, err
			}
			identity, err := inboxFileIdentity(part)
			_ = part.Close()
			if err != nil {
				return nil, err
			}
			parts[entry.Name()] = identity
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return nil, readErr
		}
	}
	return parts, nil
}

func (s *Service) ResolveInboxFile(ctx context.Context, taskID string, fileID uint32) (InboxLocation, error) {
	done, err := s.beginWorkspaceWork()
	if err != nil {
		return InboxLocation{}, err
	}
	defer done()
	snap, recovery, err := s.loadInboxTask(ctx, taskID)
	if err != nil {
		return InboxLocation{}, err
	}
	if snap.State != "completed" || !snap.BilateralConfirmed || snap.Direction != "receive" {
		return InboxLocation{}, ErrInboxFileUnavailable
	}
	var relative, kind string
	var size int64
	err = s.store.db.QueryRowContext(ctx, `SELECT saved_path,kind,size FROM inbox_files WHERE task_id=? AND file_id=?`, taskID, fileID).Scan(&relative, &kind, &size)
	if err != nil || relative == "" {
		return InboxLocation{}, ErrInboxFileUnavailable
	}
	if transfer.ValidatePath(relative) != nil || recovery.TargetDirectory == "" {
		return InboxLocation{}, ErrInboxOwnership
	}
	root, err := os.OpenRoot(recovery.TargetDirectory)
	if errors.Is(err, fs.ErrNotExist) {
		return InboxLocation{}, ErrInboxFileMissing
	}
	if err != nil {
		return InboxLocation{}, err
	}
	defer root.Close()
	for parent := path.Dir(relative); parent != "."; parent = path.Dir(parent) {
		info, e := root.Lstat(parent)
		if errors.Is(e, fs.ErrNotExist) {
			return InboxLocation{}, ErrInboxFileMissing
		}
		if e != nil {
			return InboxLocation{}, e
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return InboxLocation{}, ErrInboxOwnership
		}
	}
	info, err := root.Lstat(relative)
	if errors.Is(err, fs.ErrNotExist) {
		return InboxLocation{}, ErrInboxFileMissing
	}
	if err != nil {
		return InboxLocation{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 || (kind == "file" && !info.Mode().IsRegular()) || (kind == "directory" && !info.IsDir()) {
		return InboxLocation{}, ErrInboxOwnership
	}
	if kind == "file" && info.Size() != size {
		return InboxLocation{}, ErrInboxFileChanged
	}
	if err = ctx.Err(); err != nil {
		return InboxLocation{}, err
	}
	return InboxLocation{TaskID: taskID, FileID: fileID, Name: relative, Path: filepath.Join(recovery.TargetDirectory, filepath.FromSlash(relative))}, nil
}

func (s *Service) loadInboxTask(ctx context.Context, id string) (TaskSnapshot, taskRecovery, error) {
	var snap TaskSnapshot
	var recovery taskRecovery
	if !validInboxID(id) {
		return snap, recovery, ErrInboxQuery
	}
	var snapshotData, recoveryData []byte
	err := s.store.db.QueryRowContext(ctx, `SELECT snapshot,recovery FROM tasks WHERE id=?`, id).Scan(&snapshotData, &recoveryData)
	if errors.Is(err, sql.ErrNoRows) {
		return snap, recovery, ErrMetadataNotFound
	}
	if err != nil {
		return snap, recovery, err
	}
	if len(snapshotData) > 1<<20 || len(recoveryData) > 2<<20 || json.Unmarshal(snapshotData, &snap) != nil || snap.ID != id || (len(recoveryData) > 0 && json.Unmarshal(recoveryData, &recovery) != nil) {
		return snap, recovery, errors.New("INVALID_HISTORY_METADATA")
	}
	return snap, recovery, nil
}

func inboxTaskProtected(snap TaskSnapshot, recovery taskRecovery) bool {
	return !isTerminal(snap.State) || snap.CanResume || (snap.State == "failed" && recoveryUsable(recovery))
}

// CleanupInboxStaging only visits locations registered by an authenticated
// receiver's PlanChanged callback. It never scans a user's receive directory or
// recursively deletes anything, and does not remove received destination files.
func (s *Service) CleanupInboxStaging(ctx context.Context, limit int) (InboxCleanupResult, error) {
	done, err := s.beginWorkspaceWork()
	if err != nil {
		return InboxCleanupResult{}, err
	}
	defer done()
	if limit == 0 {
		limit = 20
	}
	if limit < 1 || limit > 100 {
		return InboxCleanupResult{}, ErrInboxQuery
	}
	// Same order as queue dispatch. Prevent new attempts between reference
	// checks and unlinking staging; existing active attempts are protected.
	s.queue.mu.Lock()
	defer s.queue.mu.Unlock()
	s.operationMu.Lock()
	defer s.operationMu.Unlock()
	rows, err := s.store.db.QueryContext(ctx, `SELECT task_id,transfer_id,peer_id,root_path,stage_identity,manifest_digest,parts FROM inbox_staging ORDER BY registered_at,task_id LIMIT ?`, limit)
	if err != nil {
		return InboxCleanupResult{}, err
	}
	var stages []inboxStage
	for rows.Next() {
		var stage inboxStage
		var parts []byte
		if err = rows.Scan(&stage.TaskID, &stage.TransferID, &stage.PeerID, &stage.RootPath, &stage.Identity, &stage.ManifestDigest, &parts); err != nil {
			_ = rows.Close()
			return InboxCleanupResult{}, err
		}
		if len(parts) > 2<<20 || json.Unmarshal(parts, &stage.Parts) != nil {
			_ = rows.Close()
			return InboxCleanupResult{}, ErrInboxOwnership
		}
		stages = append(stages, stage)
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return InboxCleanupResult{}, err
	}
	result := InboxCleanupResult{Removed: []string{}, Protected: []string{}, Issues: []InboxCleanupIssue{}}
	for _, stage := range stages {
		if err = ctx.Err(); err != nil {
			return result, err
		}
		protected, e := s.inboxStageReferenced(ctx, stage)
		if e != nil {
			return result, e
		}
		if protected {
			result.Protected = append(result.Protected, stage.TaskID)
			continue
		}
		if e = s.cleanRegisteredStage(ctx, stage); e != nil {
			code := "INBOX_STAGING_CLEANUP_FAILED"
			if errors.Is(e, ErrInboxOwnership) {
				code = ErrInboxOwnership.Error()
			}
			result.Issues = append(result.Issues, InboxCleanupIssue{TaskID: stage.TaskID, Code: code})
			continue
		}
		if _, e = s.store.db.ExecContext(ctx, `DELETE FROM inbox_staging WHERE task_id=? AND stage_identity=?`, stage.TaskID, stage.Identity); e != nil {
			return result, e
		}
		result.Removed = append(result.Removed, stage.TaskID)
	}
	return result, nil
}

func (s *Service) inboxReferenced(ctx context.Context, id string) (bool, error) {
	s.tasks.mu.RLock()
	task := s.tasks.tasks[id]
	s.tasks.mu.RUnlock()
	if task != nil {
		task.mu.RLock()
		protected := task.cancel != nil || inboxTaskProtected(task.snap, task.recovery)
		task.mu.RUnlock()
		if protected {
			return true, nil
		}
	}
	snap, recovery, err := s.loadInboxTask(ctx, id)
	if err == nil && inboxTaskProtected(snap, recovery) {
		return true, nil
	}
	if err != nil && !errors.Is(err, ErrMetadataNotFound) {
		return false, err
	}
	var references int
	err = s.store.db.QueryRowContext(ctx, `SELECT count(*) FROM send_queue WHERE task_id=? AND state NOT IN ('completed','cancelled','expired')`, id).Scan(&references)
	return references != 0, err
}

func (s *Service) cleanRegisteredStage(ctx context.Context, saved inboxStage) error {
	root, stage, state, err := openInboxStage(saved.RootPath, saved.TransferID, saved.PeerID, saved.ManifestDigest)
	if errors.Is(err, fs.ErrNotExist) {
		if _, statErr := os.Lstat(filepath.Join(saved.RootPath, ".linksend-"+saved.TransferID)); errors.Is(statErr, fs.ErrNotExist) {
			return nil
		}
		return ErrInboxOwnership
	}
	if err != nil {
		return err
	}
	defer root.Close()
	defer stage.Close()
	f, err := stage.Open(".")
	if err != nil {
		return err
	}
	identity, err := inboxFileIdentity(f)
	_ = f.Close()
	if err != nil {
		return err
	}
	if identity != saved.Identity {
		return ErrInboxOwnership
	}
	parts, err := stagePartIdentities(stage, state.Manifest)
	if err != nil {
		return err
	}
	if len(parts) > len(saved.Parts) {
		return ErrInboxOwnership
	}
	for name, identity := range parts {
		if saved.Parts[name] != identity {
			return ErrInboxOwnership
		}
	}
	if info, e := stage.Lstat("checkpoint.tmp"); e == nil {
		if !info.Mode().IsRegular() {
			return ErrInboxOwnership
		}
		pending, e := readInboxStageState(stage, "checkpoint.tmp")
		if e != nil || pending.Peer != saved.PeerID || pending.Digest != saved.ManifestDigest || pending.Manifest.Digest() != saved.ManifestDigest {
			return ErrInboxOwnership
		}
	} else if !errors.Is(e, fs.ErrNotExist) {
		return e
	}
	// All validation precedes any deletion. Only registered hard-link names in
	// staging are unlinked; their completed destination links remain untouched.
	for name := range parts {
		if err = ctx.Err(); err != nil {
			return err
		}
		if err = stage.Remove(name); err != nil {
			return err
		}
	}
	if err = stage.Remove("checkpoint.tmp"); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if err = stage.Remove("state.json"); err != nil {
		return err
	}
	_ = stage.Close()
	return root.Remove(".linksend-" + saved.TransferID)
}
