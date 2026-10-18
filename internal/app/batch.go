package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Wen5555/LinkSend/internal/protocol"
	"github.com/Wen5555/LinkSend/internal/transfer"
)

const maxBatchTargets = 16

var ErrBatchCancelled = errors.New("BATCH_CANCELLED")

type EnqueueBatchRequest struct {
	RequestID     string `json:"request_id"`
	DraftID       string `json:"draft_id"`
	DraftRevision uint64 `json:"draft_revision"`
	WaitForPeer   bool   `json:"wait_for_peer"`
	ExpiresAt     string `json:"expires_at"`
}

type BatchSnapshot struct {
	ID             string `json:"id"`
	RequestID      string `json:"request_id"`
	DraftID        string `json:"draft_id"`
	DraftRevision  uint64 `json:"draft_revision"`
	Revision       uint64 `json:"revision"`
	CreatedAt      string `json:"created_at"`
	Cancelled      bool   `json:"cancelled"`
	State          string `json:"state"`
	Total          int    `json:"total"`
	Completed      int    `json:"completed"`
	Waiting        int    `json:"waiting"`
	Running        int    `json:"running"`
	NeedsAttention int    `json:"needs_attention"`
	Stopped        int    `json:"stopped"`
}

func normalizeDraftPeers(peers []string) ([]string, error) {
	if len(peers) > maxBatchTargets {
		return nil, ErrMetadataInvalid
	}
	result := make([]string, 0, len(peers))
	seen := map[string]bool{}
	for _, peer := range peers {
		if !validMetadataPeerID(peer) || seen[peer] {
			return nil, ErrMetadataInvalid
		}
		seen[peer] = true
		result = append(result, peer)
	}
	return result, nil
}

func migrateBatchMetadata(tx *sql.Tx) error {
	for _, column := range []struct{ table, name, declaration string }{
		{"send_drafts", "peer_ids", "TEXT NOT NULL DEFAULT '[]'"},
		{"send_queue", "batch_id", "TEXT NOT NULL DEFAULT ''"},
		{"send_queue", "batch_position", "INTEGER NOT NULL DEFAULT 0"},
	} {
		has, err := tableHasColumn(tx, column.table, column.name)
		if err != nil {
			return err
		}
		if !has {
			if _, err = tx.Exec("ALTER TABLE " + column.table + " ADD COLUMN " + column.name + " " + column.declaration); err != nil {
				return err
			}
		}
	}
	_, err := tx.Exec(`CREATE TABLE IF NOT EXISTS send_batches (
		id TEXT PRIMARY KEY, request_id TEXT NOT NULL UNIQUE, signature TEXT NOT NULL,
		draft_id TEXT NOT NULL, draft_revision INTEGER NOT NULL, source_digest TEXT NOT NULL,
		revision INTEGER NOT NULL DEFAULT 1, cancelled INTEGER NOT NULL DEFAULT 0 CHECK(cancelled IN (0,1)), created_at TEXT NOT NULL
	); CREATE UNIQUE INDEX IF NOT EXISTS send_queue_batch_peer ON send_queue(batch_id,peer_id) WHERE batch_id<>'';
	CREATE UNIQUE INDEX IF NOT EXISTS send_queue_batch_position ON send_queue(batch_id,batch_position) WHERE batch_id<>'';`)
	return err
}

const batchSummarySQL = `SELECT b.id,b.request_id,b.draft_id,b.draft_revision,b.revision,b.created_at,b.cancelled,
	count(q.id),coalesce(sum(q.state='completed'),0),coalesce(sum(q.state IN ('queued','waiting_peer')),0),
	coalesce(sum(q.state='running' OR (b.cancelled=1 AND t.id IS NOT NULL AND coalesce(json_extract(t.snapshot,'$.can_resume'),0)=0 AND json_extract(t.snapshot,'$.state') NOT IN ('completed','no_content','cancelled','failed','rejected','paused'))),0),coalesce(sum(q.state='needs_attention'),0),coalesce(sum(q.state IN ('cancelled','expired')),0)
	FROM send_batches b LEFT JOIN send_queue q ON q.batch_id=b.id LEFT JOIN tasks t ON t.id=q.task_id `

func scanBatch(row metadataScanner) (BatchSnapshot, error) {
	var b BatchSnapshot
	err := row.Scan(&b.ID, &b.RequestID, &b.DraftID, &b.DraftRevision, &b.Revision, &b.CreatedAt, &b.Cancelled, &b.Total, &b.Completed, &b.Waiting, &b.Running, &b.NeedsAttention, &b.Stopped)
	if err != nil {
		return b, err
	}
	if b.Total < 2 || b.Total > maxBatchTargets || b.Revision == 0 {
		return b, ErrMetadataInvalid
	}
	switch {
	case b.Completed == b.Total:
		b.State = "completed"
	case b.Cancelled && b.Running > 0:
		b.State = "cancelling"
	case b.Cancelled:
		b.State = "cancelled"
	case b.Running > 0:
		b.State = "running"
	case b.NeedsAttention > 0:
		b.State = "needs_attention"
	case b.Waiting > 0:
		b.State = "waiting"
	default:
		b.State = "stopped"
	}
	return b, nil
}

func (s *Service) batchSnapshot(id string) (BatchSnapshot, error) {
	return scanBatch(s.store.db.QueryRow(batchSummarySQL+`WHERE b.id=? GROUP BY b.id`, id))
}
func (s *Service) batchSnapshots() ([]BatchSnapshot, error) {
	rows, err := s.store.db.Query(batchSummarySQL + `GROUP BY b.id ORDER BY CASE WHEN sum(q.state IN ('queued','waiting_peer','running','needs_attention'))>0 THEN 0 ELSE 1 END,b.created_at DESC,b.id LIMIT 50`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]BatchSnapshot, 0)
	for rows.Next() {
		b, err := scanBatch(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, b)
	}
	return result, rows.Err()
}

func (s *Service) GetBatch(id string) (BatchSnapshot, error) {
	done, err := s.beginWorkspaceWork()
	if err != nil {
		return BatchSnapshot{}, err
	}
	defer done()
	if len(id) != 32 {
		return BatchSnapshot{}, ErrMetadataInvalid
	}
	return s.batchSnapshot(id)
}

func batchRequestSignature(request EnqueueBatchRequest) string {
	request.RequestID = ""
	body, _ := json.Marshal(request)
	return transfer.Sum(body)
}

func (s *Service) existingBatch(requestID, signature string) (string, error) {
	var id, saved string
	err := s.store.db.QueryRow("SELECT id,signature FROM send_batches WHERE request_id=?", requestID).Scan(&id, &saved)
	if err == nil && saved != signature {
		return "", errors.New("IDEMPOTENCY_CONFLICT")
	}
	return id, err
}

func checkBatchDraft(row *sql.Row, draft SendDraft, paths, selection, peers string) error {
	var savedPaths, savedSelection, savedPeers string
	var revision uint64
	if err := row.Scan(&savedPaths, &savedSelection, &savedPeers, &revision); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrMetadataConflict
		}
		return err
	}
	if revision != draft.Revision || savedPaths != paths || savedSelection != selection || savedPeers != peers {
		return ErrMetadataConflict
	}
	return nil
}

// One Go transaction owns the parent and every child. Preflight and hashing
// happen outside queue/operation locks; no per-target frontend enqueue loop.
func (s *Service) EnqueueBatch(request EnqueueBatchRequest) (BatchSnapshot, error) {
	done, err := s.beginWorkspaceWork()
	if err != nil {
		return BatchSnapshot{}, err
	}
	defer done()
	if request.RequestID == "" || len(request.RequestID) > 128 || !utf8.ValidString(request.RequestID) || strings.ContainsAny(request.RequestID, "\x00\r\n") || request.DraftRevision == 0 || !validMetadataRevision(request.DraftRevision) {
		return BatchSnapshot{}, ErrMetadataInvalid
	}
	request.DraftID, err = normalizeDraftID(request.DraftID)
	if err != nil {
		return BatchSnapshot{}, err
	}
	signature := batchRequestSignature(request)
	if id, err := s.existingBatch(request.RequestID, signature); err == nil {
		return s.batchSnapshot(id)
	} else if !errors.Is(err, sql.ErrNoRows) {
		return BatchSnapshot{}, err
	}
	draft, err := s.store.Draft(request.DraftID)
	if err != nil {
		return BatchSnapshot{}, err
	}
	if draft.Revision != request.DraftRevision || len(draft.PeerIDs) < 2 {
		return BatchSnapshot{}, ErrMetadataConflict
	}
	paths, err := queuePaths(draft.Paths)
	if err != nil {
		return BatchSnapshot{}, err
	}
	encoded, _ := json.Marshal(paths)
	encodedPeers, _ := json.Marshal(draft.PeerIDs)
	selection, err := encodeSourceSelection(draft.SourceSelection)
	if err != nil {
		return BatchSnapshot{}, err
	}
	generations := make(map[string]uint64, len(draft.PeerIDs))
	for _, peer := range draft.PeerIDs {
		generation, err := s.currentAuthorizationGeneration(peer)
		if err != nil {
			return BatchSnapshot{}, protocol.Wrap(protocol.AuthenticationFailed, "batch requires current paired targets", err)
		}
		generations[peer] = generation
	}
	ctx, cancel := context.WithTimeout(s.workCtx, 5*time.Second)
	devices, deviceErr := s.Devices(ctx)
	cancel()
	online := map[string]bool{}
	for _, device := range devices {
		online[device.ID] = device.Trusted && !device.Blocked && (device.Online || device.Nearby || device.ConnectionState == "connected")
	}
	for _, peer := range draft.PeerIDs {
		if !online[peer] && !request.WaitForPeer {
			if deviceErr != nil {
				return BatchSnapshot{}, deviceErr
			}
			return BatchSnapshot{}, protocol.Fail(protocol.PeerOffline, "all batch targets must be reachable or explicitly wait")
		}
	}
	expires := time.Now().UTC().Add(7 * 24 * time.Hour)
	if request.ExpiresAt != "" {
		expires, err = time.Parse(time.RFC3339Nano, request.ExpiresAt)
		if err != nil || !expires.After(time.Now()) || expires.After(time.Now().Add(30*24*time.Hour)) {
			return BatchSnapshot{}, ErrMetadataInvalid
		}
	}
	prepared, err := transfer.PrepareSelected(s.workCtx, paths, 0, draft.SourceSelection)
	if err != nil {
		return BatchSnapshot{}, classifyTaskError(err)
	}
	digest := selectedQueueSourceDigest(prepared.Manifest, draft.SourceSelection)
	_ = prepared.Close()
	s.queue.mu.Lock()
	defer s.queue.mu.Unlock()
	if s.isClosing() {
		return BatchSnapshot{}, errors.New("APP_CLOSING")
	}
	for _, peer := range draft.PeerIDs {
		if err = s.checkAuthorizationGeneration(peer, generations[peer]); err != nil {
			return BatchSnapshot{}, err
		}
	}
	tx, err := s.store.db.Begin()
	if err != nil {
		return BatchSnapshot{}, err
	}
	defer tx.Rollback()
	var existingID, existingSignature string
	err = tx.QueryRow("SELECT id,signature FROM send_batches WHERE request_id=?", request.RequestID).Scan(&existingID, &existingSignature)
	if err == nil {
		_ = tx.Rollback()
		if existingSignature != signature {
			return BatchSnapshot{}, errors.New("IDEMPOTENCY_CONFLICT")
		}
		return s.batchSnapshot(existingID)
	} else if !errors.Is(err, sql.ErrNoRows) {
		return BatchSnapshot{}, err
	}
	if err = checkBatchDraft(tx.QueryRow("SELECT source_paths,source_selection,peer_ids,revision FROM send_drafts WHERE id=?", draft.ID), draft, string(encoded), selection, string(encodedPeers)); err != nil {
		return BatchSnapshot{}, err
	}
	var count int
	var position int64
	if err = tx.QueryRow("SELECT count(*) FROM send_queue WHERE state NOT IN ('completed','cancelled','expired')").Scan(&count); err != nil {
		return BatchSnapshot{}, err
	}
	if count+len(draft.PeerIDs) > 1000 {
		return BatchSnapshot{}, errors.New("RESOURCE_LIMIT: queue full")
	}
	if err = tx.QueryRow("SELECT coalesce(max(position),0) FROM send_queue").Scan(&position); err != nil {
		return BatchSnapshot{}, err
	}
	if position > 1<<62 {
		return BatchSnapshot{}, ErrMetadataInvalid
	}
	id, now := protocol.RandomID(), time.Now().UTC().Format(time.RFC3339Nano)
	if _, err = tx.Exec("INSERT INTO send_batches(id,request_id,signature,draft_id,draft_revision,source_digest,created_at) VALUES(?,?,?,?,?,?,?)", id, request.RequestID, signature, draft.ID, draft.Revision, digest, now); err != nil {
		return BatchSnapshot{}, err
	}
	for index, peer := range draft.PeerIDs {
		state := "queued"
		if !online[peer] {
			state = "waiting_peer"
		}
		childRequest := transfer.Sum([]byte("batch-child-v1\x00" + request.RequestID + "\x00" + peer))
		_, err = tx.Exec(`INSERT INTO send_queue(`+queueColumns+`) VALUES(?,?,?,?,?,?,?,'',?,1,?,?,'',?,?,?,?,?,?,?)`, protocol.RandomID(), childRequest, peer, string(encoded), digest, state, position+int64(index)+1, expires.Format(time.RFC3339Nano), now, now, request.WaitForPeer, generations[peer], selection, draft.ID, draft.Revision, id, index+1)
		if err != nil {
			return BatchSnapshot{}, err
		}
	}
	if err = tx.Commit(); err != nil {
		return BatchSnapshot{}, err
	}
	s.notifyChange()
	return s.batchSnapshot(id)
}

func (s *Service) requireBatchAllowed(id string) error {
	if id == "" {
		return nil
	}
	var cancelled bool
	if err := s.store.db.QueryRow("SELECT cancelled FROM send_batches WHERE id=?", id).Scan(&cancelled); err != nil {
		return err
	}
	if cancelled {
		return ErrBatchCancelled
	}
	return nil
}

func (s *Service) requireTaskBatchAllowed(id string) error {
	batch, err := s.taskBatchID(id)
	if err != nil {
		return err
	}
	return s.requireBatchAllowed(batch)
}

func (s *Service) taskBatchID(id string) (string, error) {
	s.tasks.mu.RLock()
	record := s.tasks.tasks[id]
	s.tasks.mu.RUnlock()
	if record != nil {
		if batch := record.recoverySnapshot().BatchID; batch != "" {
			return batch, nil
		}
	}
	if s.store == nil {
		return "", nil
	}
	var batch string
	err := s.store.db.QueryRow("SELECT batch_id FROM send_queue WHERE task_id=? AND batch_id<>'' LIMIT 1", id).Scan(&batch)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return batch, nil
}

// A durable completed task wins over a late cancellation and its queue receipt.
// This also closes the crash window between task commit and queue acknowledgement.
func (s *Service) reconcileCancelledBatchReceipts() error {
	result, err := s.store.db.Exec(`UPDATE send_queue SET state='completed',last_error='',revision=revision+1,updated_at=?
	WHERE state<>'completed' AND batch_id IN (SELECT id FROM send_batches WHERE cancelled=1)
	AND task_id<>'' AND EXISTS(SELECT 1 FROM tasks WHERE tasks.id=send_queue.task_id AND json_extract(tasks.snapshot,'$.state') IN ('completed','no_content'))`, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err == nil && changed > 0 {
		s.notifyChange()
	}
	return err
}

func (s *Service) CancelBatch(id string, revision uint64) (BatchSnapshot, error) {
	done, err := s.beginWorkspaceWork()
	if err != nil {
		return BatchSnapshot{}, err
	}
	defer done()
	if len(id) != 32 || revision == 0 || !validMetadataRevision(revision) {
		return BatchSnapshot{}, ErrMetadataInvalid
	}
	s.queue.mu.Lock()
	defer s.queue.mu.Unlock()
	batch, err := s.batchSnapshot(id)
	if err != nil {
		return BatchSnapshot{}, err
	}
	if batch.Cancelled {
		return batch, nil
	}
	if batch.Revision != revision {
		return BatchSnapshot{}, ErrMetadataConflict
	}
	s.operationMu.Lock()
	ownerLocked := true
	defer func() {
		if ownerLocked {
			s.operationMu.Unlock()
		}
	}()
	if s.isClosing() {
		return BatchSnapshot{}, errors.New("APP_CLOSING")
	}
	rows, err := s.store.db.Query("SELECT task_id FROM send_queue WHERE batch_id=? AND task_id<>'' AND state<>'completed'", id)
	if err != nil {
		return BatchSnapshot{}, err
	}
	var tasks []string
	for rows.Next() {
		var task string
		if err = rows.Scan(&task); err != nil {
			break
		}
		tasks = append(tasks, task)
	}
	err = errors.Join(err, rows.Err(), rows.Close())
	if err != nil {
		return BatchSnapshot{}, err
	}
	tx, err := s.store.db.Begin()
	if err != nil {
		return BatchSnapshot{}, err
	}
	defer tx.Rollback()
	result, err := tx.Exec("UPDATE send_batches SET cancelled=1,revision=revision+1 WHERE id=? AND revision=? AND cancelled=0", id, revision)
	if err != nil {
		return BatchSnapshot{}, err
	}
	if err = requireMetadataUpdate(result); err != nil {
		return BatchSnapshot{}, err
	}
	_, err = tx.Exec("UPDATE send_queue SET state='cancelled',last_error='',revision=revision+1,updated_at=? WHERE batch_id=? AND state NOT IN ('completed','cancelled','expired')", time.Now().UTC().Format(time.RFC3339Nano), id)
	if err != nil {
		return BatchSnapshot{}, err
	}
	if err = tx.Commit(); err != nil {
		return BatchSnapshot{}, err
	}
	s.operationMu.Unlock()
	ownerLocked = false
	var cancelErr error
	for _, task := range tasks {
		if snap, ok := s.Task(task); ok && !isTerminal(snap.State) {
			cancelErr = errors.Join(cancelErr, s.CancelTask(task))
		}
	}
	cancelErr = errors.Join(cancelErr, s.reconcileCancelledBatchReceipts())
	s.notifyChange()
	current, readErr := s.batchSnapshot(id)
	return current, errors.Join(cancelErr, readErr)
}

func (s *Service) restoreCancelledBatches() error {
	if err := s.reconcileCancelledBatchReceipts(); err != nil {
		return err
	}
	rows, err := s.store.db.Query(`SELECT q.task_id FROM send_queue q JOIN send_batches b ON b.id=q.batch_id WHERE b.cancelled=1 AND q.state<>'completed' AND q.task_id<>''`)
	if err != nil {
		return err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			break
		}
		ids = append(ids, id)
	}
	err = errors.Join(err, rows.Err(), rows.Close())
	if err != nil {
		return err
	}
	for _, id := range ids {
		if task := s.tasks.tasks[id]; task != nil && !isTerminal(task.snapshot().State) {
			task.finish("cancelled", protocol.Fail(protocol.Cancelled, "batch cancelled before restart"))
		}
	}
	_, err = s.store.db.Exec(`UPDATE send_queue SET state='cancelled',last_error='',revision=revision+1,updated_at=? WHERE batch_id IN (SELECT id FROM send_batches WHERE cancelled=1) AND state NOT IN ('completed','cancelled','expired')`, time.Now().UTC().Format(time.RFC3339Nano))
	return err
}
