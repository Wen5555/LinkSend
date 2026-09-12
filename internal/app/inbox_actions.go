package app

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/Wen5555/LinkSend/internal/transfer"
)

type InboxRecordRef struct {
	TaskID   string `json:"task_id"`
	Revision uint64 `json:"revision"`
}
type ResendInboxRequest struct {
	TaskID      string `json:"task_id"`
	RequestID   string `json:"request_id"`
	WaitForPeer bool   `json:"wait_for_peer"`
}

// ResendInbox verifies the old source identity, then queues a new logical send.
// ResumeTask keeps its original task/transfer IDs; this operation never does.
func (s *Service) ResendInbox(ctx context.Context, request ResendInboxRequest) (QueueItem, error) {
	done, err := s.beginWorkspaceWork()
	if err != nil {
		return QueueItem{}, err
	}
	defer done()
	if !validInboxID(request.RequestID) || !validInboxID(request.TaskID) {
		return QueueItem{}, ErrInboxQuery
	}
	hash := sha256.Sum256([]byte("inbox-resend\x00" + request.TaskID + "\x00" + request.RequestID))
	requestID := hex.EncodeToString(hash[:])
	existing, existingErr := scanQueue(s.store.db.QueryRowContext(ctx, `SELECT `+queueColumns+` FROM send_queue WHERE request_id=?`, requestID))
	if existingErr == nil {
		if existing.WaitForPeer != request.WaitForPeer {
			return QueueItem{}, errors.New("IDEMPOTENCY_CONFLICT")
		}
		return existing.QueueItem, nil
	}
	if !errors.Is(existingErr, sql.ErrNoRows) {
		return QueueItem{}, existingErr
	}
	snap, recovery, err := s.loadInboxTask(ctx, request.TaskID)
	if err != nil {
		return QueueItem{}, err
	}
	if snap.Direction != "send" || !isTerminal(snap.State) || len(recovery.SourcePaths) == 0 || !recoveryUsable(recovery) {
		return QueueItem{}, errors.New("INBOX_NOT_RESENDABLE")
	}
	if err = s.checkPeerAllowed(recovery.PeerID); err != nil {
		return QueueItem{}, err
	}
	prepared, err := transfer.PrepareForResume(ctx, recovery.SourcePaths, recovery.ChunkSize, recovery.TransferID)
	if err != nil {
		return QueueItem{}, classifyTaskError(err)
	}
	defer prepared.Close()
	if prepared.Manifest.Digest() != recovery.ManifestDigest {
		return QueueItem{}, classifyTaskError(transfer.ErrChanged)
	}
	return s.Enqueue(EnqueueRequest{RequestID: requestID, PeerID: recovery.PeerID, Paths: append([]string(nil), recovery.SourcePaths...), WaitForPeer: request.WaitForPeer, ExpectedSourceDigest: inboxSourceDigest(prepared.Manifest)})
}

// Content identity survives the new task's independently chosen chunk layout.
func inboxSourceDigest(m transfer.Manifest) string {
	m.TransferID = strings.Repeat("0", 32)
	m.ChunkSize = 0
	m.Files = append([]transfer.FileEntry(nil), m.Files...)
	for i := range m.Files {
		m.Files[i].Chunks = nil
	}
	return m.Digest()
}

// ForgetInboxRecords removes only local history and filename indexes. Persistent
// tombstones reject late observers even after restart. Staging registrations and
// owned content references are preserved for the separate cleanup operation.
func (s *Service) ForgetInboxRecords(refs []InboxRecordRef) error {
	done, err := s.beginWorkspaceWork()
	if err != nil {
		return err
	}
	defer done()
	if len(refs) == 0 || len(refs) > 100 {
		return ErrInboxQuery
	}
	seen := make(map[string]bool)
	for _, ref := range refs {
		if !validInboxID(ref.TaskID) || ref.Revision == 0 || seen[ref.TaskID] {
			return ErrInboxQuery
		}
		seen[ref.TaskID] = true
	}
	s.queue.mu.Lock()
	defer s.queue.mu.Unlock()
	s.operationMu.Lock()
	defer s.operationMu.Unlock()
	for _, ref := range refs {
		protected, e := s.inboxReferenced(s.workCtx, ref.TaskID)
		if e != nil {
			return e
		}
		if protected {
			return ErrInboxProtected
		}
	}
	tx, err := s.store.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, ref := range refs {
		var revision uint64
		if err = tx.QueryRow(`SELECT revision FROM tasks WHERE id=?`, ref.TaskID).Scan(&revision); err != nil {
			return err
		}
		if revision != ref.Revision {
			return ErrMetadataConflict
		}
		if _, err = tx.Exec(`INSERT INTO inbox_tombstones(task_id,revision,forgotten_at) VALUES(?,?,?)`, ref.TaskID, ref.Revision, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			return err
		}
		if _, err = tx.Exec(`DELETE FROM inbox_files WHERE task_id=?`, ref.TaskID); err != nil {
			return err
		}
		if _, err = tx.Exec(`DELETE FROM inbox_manifests WHERE task_id=?`, ref.TaskID); err != nil {
			return err
		}
		if _, err = tx.Exec(`DELETE FROM tasks WHERE id=? AND revision=?`, ref.TaskID, ref.Revision); err != nil {
			return err
		}
		// Historical queue entries keep their idempotency result but no longer
		// point at a deleted task. Live/attention queue references were refused.
		if _, err = tx.Exec(`UPDATE send_queue SET task_id='',revision=revision+1,updated_at=? WHERE task_id=? AND state IN ('completed','cancelled','expired')`, time.Now().UTC().Format(time.RFC3339Nano), ref.TaskID); err != nil {
			return err
		}
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	s.tasks.mu.Lock()
	for _, ref := range refs {
		delete(s.tasks.tasks, ref.TaskID)
	}
	s.tasks.mu.Unlock()
	s.notifyChange()
	return nil
}

func (s *Service) inboxStageReferenced(ctx context.Context, stage inboxStage) (bool, error) {
	protected, err := s.inboxReferenced(ctx, stage.TaskID)
	if err != nil || protected {
		return protected, err
	}
	// A reconnect may have a second task ID for the same offered transfer.
	// Protect the receiver before its first PlanChanged registration exists.
	s.tasks.mu.RLock()
	for _, task := range s.tasks.tasks {
		task.mu.RLock()
		active := task.cancel != nil || inboxTaskProtected(task.snap, task.recovery)
		directory := task.recovery.TargetDirectory
		if directory == "" {
			directory = task.snap.TargetDirectory
		}
		transferID := task.recovery.TransferID
		matches := active && task.snap.Direction == "receive" && (transferID == "" || transferID == stage.TransferID) && sameInboxDirectory(directory, stage.RootPath)
		task.mu.RUnlock()
		if matches {
			s.tasks.mu.RUnlock()
			return true, nil
		}
	}
	s.tasks.mu.RUnlock()
	rows, scanErr := s.store.db.QueryContext(ctx, `SELECT snapshot,recovery FROM tasks WHERE inbox_direction='receive' AND inbox_state NOT IN ('completed','cancelled','rejected','no_content') AND json_valid(recovery) AND json_extract(recovery,'$.transfer_id')=?`, stage.TransferID)
	if scanErr != nil {
		return false, scanErr
	}
	for rows.Next() {
		var snapshotData, recoveryData []byte
		if scanErr = rows.Scan(&snapshotData, &recoveryData); scanErr != nil {
			_ = rows.Close()
			return false, scanErr
		}
		var snap TaskSnapshot
		var recovery taskRecovery
		if len(snapshotData) > 1<<20 || len(recoveryData) > 2<<20 || json.Unmarshal(snapshotData, &snap) != nil || json.Unmarshal(recoveryData, &recovery) != nil {
			_ = rows.Close()
			return false, errors.New("INVALID_HISTORY_METADATA")
		}
		if inboxTaskProtected(snap, recovery) && sameInboxDirectory(recovery.TargetDirectory, stage.RootPath) {
			_ = rows.Close()
			return true, nil
		}
	}
	scanErr = rows.Err()
	_ = rows.Close()
	if scanErr != nil {
		return false, scanErr
	}
	stagePath := filepath.Join(stage.RootPath, ".linksend-"+stage.TransferID)
	// Query private source arrays inside SQLite; only one bounded path crosses
	// into Go at a time. No recovery path is exposed through the Inbox DTO.
	queries := []string{
		`SELECT j.value FROM send_queue q,json_each(q.source_paths) j WHERE q.state NOT IN ('completed','cancelled','expired')`,
		`SELECT j.value FROM send_drafts d,json_each(d.source_paths) j`,
		`SELECT j.value FROM tasks t,json_each(t.recovery,'$.source_paths') j WHERE t.inbox_state NOT IN ('completed','cancelled','rejected','no_content') AND json_valid(t.recovery)`,
	}
	for _, query := range queries {
		rows, e := s.store.db.QueryContext(ctx, query)
		if e != nil {
			return false, e
		}
		count := 0
		for rows.Next() {
			count++
			if count > 100000 {
				_ = rows.Close()
				return false, errors.New("INBOX_REFERENCE_SCAN_LIMIT")
			}
			var candidate string
			if e = rows.Scan(&candidate); e != nil {
				_ = rows.Close()
				return false, e
			}
			if pathsOverlapForCleanup(stagePath, candidate) {
				_ = rows.Close()
				return true, nil
			}
		}
		e = rows.Err()
		_ = rows.Close()
		if e != nil {
			return false, e
		}
	}
	return false, nil
}

func sameInboxDirectory(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	a, b = filepath.Clean(a), filepath.Clean(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

func pathsOverlapForCleanup(stage, source string) bool {
	if !filepath.IsAbs(source) {
		return true
	} // malformed private reference: fail closed
	stage, source = filepath.Clean(stage), filepath.Clean(source)
	if runtime.GOOS == "windows" {
		stage, source = strings.ToLower(stage), strings.ToLower(source)
	}
	for _, pair := range [][2]string{{stage, source}, {source, stage}} {
		relative, err := filepath.Rel(pair[0], pair[1])
		if err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return true
		}
	}
	return false
}
