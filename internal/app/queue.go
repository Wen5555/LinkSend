package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math/rand/v2"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/Wen5555/LinkSend/internal/protocol"
	"github.com/Wen5555/LinkSend/internal/transfer"
)

type QueueItem struct {
	ID            string `json:"id"`
	RequestID     string `json:"request_id"`
	PeerID        string `json:"peer_id"`
	SourceSummary string `json:"source_summary"`
	State         string `json:"state"`
	Position      int64  `json:"position"`
	TaskID        string `json:"task_id"`
	ExpiresAt     string `json:"expires_at"`
	Revision      uint64 `json:"revision"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
	LastError     string `json:"last_error"`
	WaitForPeer   bool   `json:"wait_for_peer"`
}

type EnqueueRequest struct {
	RequestID   string   `json:"request_id"`
	PeerID      string   `json:"peer_id"`
	Paths       []string `json:"paths"`
	WaitForPeer bool     `json:"wait_for_peer"`
	ExpiresAt   string   `json:"expires_at"`
}

type queueRecord struct {
	QueueItem
	paths  []string
	digest string
}
type queueManager struct {
	mu            sync.Mutex
	wake          chan struct{}
	paused        bool
	persistFailed bool
	cfg           DirectConfig
	cancel        context.CancelFunc
	done          chan struct{}
}

const queueColumns = `id,request_id,peer_id,source_paths,source_digest,state,position,task_id,expires_at,revision,created_at,updated_at,last_error,wait_for_peer`

func scanQueue(row interface{ Scan(...any) error }) (queueRecord, error) {
	var item queueRecord
	var paths string
	err := row.Scan(&item.ID, &item.RequestID, &item.PeerID, &paths, &item.digest, &item.State, &item.Position, &item.TaskID, &item.ExpiresAt, &item.Revision, &item.CreatedAt, &item.UpdatedAt, &item.LastError, &item.WaitForPeer)
	if err == nil {
		err = json.Unmarshal([]byte(paths), &item.paths)
		item.SourceSummary = sourceSummary(item.paths)
	}
	return item, err
}

func (s *Service) queueItems() ([]QueueItem, error) {
	rows, err := s.store.db.Query(`SELECT ` + queueColumns + ` FROM send_queue ORDER BY CASE WHEN state IN ('completed','cancelled','expired') THEN 1 ELSE 0 END,position,created_at,id LIMIT 1050`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]QueueItem, 0)
	for rows.Next() {
		item, e := scanQueue(rows)
		if e != nil {
			return nil, e
		}
		items = append(items, item.QueueItem)
	}
	return items, rows.Err()
}

func queueSourceDigest(m transfer.Manifest) string {
	// Content identity is independent of each new transfer/session identifier.
	m.TransferID = strings.Repeat("0", 32)
	return m.Digest()
}

func queuePaths(paths []string) ([]string, error) {
	if len(paths) == 0 || len(paths) > 4096 {
		return nil, errors.New("INVALID_ARGUMENT: queue source count")
	}
	out := make([]string, 0, len(paths))
	seen := map[string]bool{}
	for _, path := range paths {
		if !filepath.IsAbs(path) || strings.ContainsRune(path, 0) || !utf8.ValidString(path) {
			return nil, errors.New("INVALID_ARGUMENT: queue requires absolute source paths")
		}
		path = filepath.Clean(path)
		if !seen[path] {
			seen[path] = true
			out = append(out, path)
		}
	}
	return out, nil
}

func (s *Service) Enqueue(request EnqueueRequest) (QueueItem, error) {
	done, err := s.beginWorkspaceWork()
	if err != nil {
		return QueueItem{}, err
	}
	defer done()
	if request.RequestID == "" || len(request.RequestID) > 128 || !utf8.ValidString(request.RequestID) || strings.ContainsAny(request.RequestID, "\x00\r\n") {
		return QueueItem{}, errors.New("INVALID_ARGUMENT: idempotency request ID required")
	}
	if err = s.checkPeerAllowed(request.PeerID); err != nil {
		return QueueItem{}, err
	}
	paths, err := queuePaths(request.Paths)
	if err != nil {
		return QueueItem{}, err
	}
	encoded, _ := json.Marshal(paths)
	if len(encoded) > 1<<20 {
		return QueueItem{}, errors.New("RESOURCE_LIMIT: source metadata")
	}
	s.queue.mu.Lock()
	existing, err := scanQueue(s.store.db.QueryRow(`SELECT `+queueColumns+` FROM send_queue WHERE request_id=?`, request.RequestID))
	s.queue.mu.Unlock()
	if err == nil {
		saved, _ := json.Marshal(existing.paths)
		if existing.PeerID != request.PeerID || string(saved) != string(encoded) || existing.WaitForPeer != request.WaitForPeer {
			return QueueItem{}, errors.New("IDEMPOTENCY_CONFLICT")
		}
		return existing.QueueItem, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return QueueItem{}, err
	}
	expires := time.Now().UTC().Add(7 * 24 * time.Hour)
	if request.ExpiresAt != "" {
		expires, err = time.Parse(time.RFC3339Nano, request.ExpiresAt)
		if err != nil || !expires.After(time.Now()) || expires.After(time.Now().Add(30*24*time.Hour)) {
			return QueueItem{}, errors.New("INVALID_ARGUMENT: queue expiration")
		}
	}
	ctx, cancel := context.WithTimeout(s.workCtx, 5*time.Second)
	devices, deviceErr := s.Devices(ctx)
	cancel()
	known, online := false, false
	for _, device := range devices {
		if device.ID == request.PeerID {
			known = device.Trusted || device.Nearby
			online = device.Online
			break
		}
	}
	if !known {
		if deviceErr != nil {
			return QueueItem{}, deviceErr
		}
		return QueueItem{}, protocol.Fail(protocol.Unpaired, "queue target is not verified or nearby")
	}
	if !online && !request.WaitForPeer {
		return QueueItem{}, protocol.Fail(protocol.PeerOffline, "explicit waiting choice required")
	}
	prepared, err := transfer.Prepare(s.workCtx, paths, 0)
	if err != nil {
		return QueueItem{}, classifyTaskError(err)
	}
	digest := queueSourceDigest(prepared.Manifest)
	_ = prepared.Close()
	s.queue.mu.Lock()
	defer s.queue.mu.Unlock()
	if s.isClosing() {
		return QueueItem{}, errors.New("APP_CLOSING")
	}
	if err = s.checkPeerAllowed(request.PeerID); err != nil {
		return QueueItem{}, err
	}
	tx, err := s.store.db.Begin()
	if err != nil {
		return QueueItem{}, err
	}
	defer tx.Rollback()
	var count int
	if err = tx.QueryRow(`SELECT COUNT(*) FROM send_queue WHERE state NOT IN ('completed','cancelled','expired')`).Scan(&count); err != nil {
		return QueueItem{}, err
	}
	if count >= 1000 {
		return QueueItem{}, errors.New("RESOURCE_LIMIT: queue full")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	state := "queued"
	if !online {
		state = "waiting_peer"
	}
	id := protocol.RandomID()
	_, err = tx.Exec(`INSERT INTO send_queue(`+queueColumns+`) VALUES(?,?,?,?,?,?,(SELECT COALESCE(MAX(position),0)+1 FROM send_queue),'',?,1,?,?,'',?) ON CONFLICT(request_id) DO NOTHING`, id, request.RequestID, request.PeerID, string(encoded), digest, state, expires.Format(time.RFC3339Nano), now, now, request.WaitForPeer)
	if err != nil {
		return QueueItem{}, err
	}
	item, err := scanQueue(tx.QueryRow(`SELECT `+queueColumns+` FROM send_queue WHERE request_id=?`, request.RequestID))
	if err != nil {
		return QueueItem{}, err
	}
	saved, _ := json.Marshal(item.paths)
	if item.PeerID != request.PeerID || string(saved) != string(encoded) || item.WaitForPeer != request.WaitForPeer {
		return QueueItem{}, errors.New("IDEMPOTENCY_CONFLICT")
	}
	if err = tx.Commit(); err != nil {
		return QueueItem{}, err
	}
	s.notifyChange()
	return item.QueueItem, nil
}

func (s *Service) StartQueue(cfg DirectConfig) error {
	s.operationMu.Lock()
	defer s.operationMu.Unlock()
	if s.isClosing() {
		return errors.New("APP_CLOSING")
	}
	if s.store == nil || s.storeErr != nil {
		return errors.New("WORKSPACE_STORE_UNAVAILABLE")
	}
	s.queue.mu.Lock()
	defer s.queue.mu.Unlock()
	s.queue.cfg = cfg
	if s.queue.cancel != nil {
		return nil
	}
	ctx, cancel := context.WithCancel(s.workCtx)
	s.queue.cancel = cancel
	s.queue.done = make(chan struct{})
	go s.runQueue(ctx, s.queue.done)
	return nil
}

func (s *Service) runQueue(ctx context.Context, done chan struct{}) {
	defer close(done)
	for {
		if ctx.Err() != nil {
			return
		}
		s.dispatchQueue(ctx)
		timer := time.NewTimer(2*time.Second + time.Duration(rand.IntN(1000))*time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-s.queue.wake:
			timer.Stop()
		case <-timer.C:
		}
	}
}

func (s *Service) stopQueue() {
	s.queue.mu.Lock()
	cancel, done := s.queue.cancel, s.queue.done
	s.queue.mu.Unlock()
	if cancel != nil {
		cancel()
		<-done
	}
}

func (s *Service) setQueueState(id, state, reason string) error {
	result, err := s.store.db.Exec(`UPDATE send_queue SET state=?,last_error=?,revision=revision+1,updated_at=? WHERE id=? AND (state<>? OR last_error<>?)`, state, reason, time.Now().UTC().Format(time.RFC3339Nano), id, state, reason)
	if err != nil {
		s.queue.persistFailed = true
		s.queue.paused = true
		s.notifyChange()
		return err
	}
	if err == nil {
		n, e := result.RowsAffected()
		if e != nil {
			return e
		}
		if n > 0 {
			s.notifyChange()
		}
	}
	return err
}

func (s *Service) dispatchQueue(ctx context.Context) {
	s.queue.mu.Lock()
	defer s.queue.mu.Unlock()
	if s.isClosing() || ctx.Err() != nil {
		return
	}
	items, err := s.queueItems()
	if err != nil {
		return
	}
	for _, item := range items {
		if item.State != "running" {
			continue
		}
		task, ok := s.Task(item.TaskID)
		if !ok {
			_ = s.setQueueState(item.ID, "needs_attention", "TASK_NOT_FOUND")
			continue
		}
		switch task.State {
		case "completed", "cancelled":
			_ = s.setQueueState(item.ID, task.State, "")
		case "failed", "rejected":
			_ = s.setQueueState(item.ID, "needs_attention", task.ErrorCode)
		case "paused", "recovering":
			_ = s.setQueueState(item.ID, "needs_attention", "TASK_REQUIRES_RESUME")
		}
	}
	if s.queue.paused || s.tasks.hasActive() {
		return
	}
	var runnable []QueueItem
	for _, item := range items {
		if item.State != "queued" && item.State != "waiting_peer" {
			continue
		}
		if deadline, e := time.Parse(time.RFC3339Nano, item.ExpiresAt); e != nil || !deadline.After(time.Now()) {
			_ = s.setQueueState(item.ID, "expired", "QUEUE_EXPIRED")
			continue
		}
		if s.checkPeerAllowed(item.PeerID) != nil {
			_ = s.setQueueState(item.ID, "needs_attention", "PEER_BLOCKED")
			continue
		}
		runnable = append(runnable, item)
	}
	if len(runnable) == 0 {
		return
	}
	// Remote presence cannot hold the command mutex: cancellation and ordering
	// must remain available while the signaling service is slow or offline.
	s.queue.mu.Unlock()
	lookupCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	devices, _ := s.Devices(lookupCtx)
	cancel()
	s.queue.mu.Lock()
	if s.isClosing() || ctx.Err() != nil || s.queue.paused || s.tasks.hasActive() {
		return
	}
	items, err = s.queueItems()
	if err != nil {
		return
	}
	runnable = nil
	for _, item := range items {
		if item.State != "queued" && item.State != "waiting_peer" {
			continue
		}
		if deadline, e := time.Parse(time.RFC3339Nano, item.ExpiresAt); e != nil || !deadline.After(time.Now()) {
			_ = s.setQueueState(item.ID, "expired", "QUEUE_EXPIRED")
			continue
		}
		if s.checkPeerAllowed(item.PeerID) != nil {
			_ = s.setQueueState(item.ID, "needs_attention", "PEER_BLOCKED")
			continue
		}
		runnable = append(runnable, item)
	}
	online := map[string]bool{}
	for _, device := range devices {
		online[device.ID] = device.Online && !device.Blocked && (device.Trusted || device.Nearby)
	}
	for _, item := range runnable {
		if !online[item.PeerID] {
			_ = s.setQueueState(item.ID, "waiting_peer", "")
			continue
		}
		record, e := scanQueue(s.store.db.QueryRow(`SELECT `+queueColumns+` FROM send_queue WHERE id=?`, item.ID))
		if e != nil {
			return
		}
		cfg := s.queue.cfg
		cfg.expectedSourceDigest = record.digest
		cfg.beforeDispatch = func(task TaskSnapshot) error {
			if !task.HistoryPersisted {
				return errors.New("WORKSPACE_STORE_UNAVAILABLE")
			}
			result, err := s.store.db.Exec(`UPDATE send_queue SET state='running',task_id=?,last_error='',revision=revision+1,updated_at=? WHERE id=? AND revision=? AND state IN ('queued','waiting_peer')`, task.ID, time.Now().UTC().Format(time.RFC3339Nano), record.ID, record.Revision)
			if err != nil {
				return err
			}
			n, err := result.RowsAffected()
			if err != nil {
				return err
			}
			if n != 1 {
				return errors.New("QUEUE_REVISION_CONFLICT")
			}
			return nil
		}
		_, err = s.StartSend(record.PeerID, record.paths, cfg)
		if err != nil && !strings.HasPrefix(err.Error(), "BUSY") {
			_ = s.setQueueState(record.ID, "needs_attention", string(protocol.ErrorCode(err)))
		}
		return
	}
}

func (s *Service) SetQueuePaused(paused bool) error {
	done, err := s.beginWorkspaceWork()
	if err != nil {
		return err
	}
	defer done()
	s.queue.mu.Lock()
	value := "0"
	if paused {
		value = "1"
	}
	_, err = s.store.db.Exec(`INSERT INTO metadata(key,value) VALUES('queue_paused',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, value)
	if err != nil {
		s.queue.persistFailed = true
		s.queue.paused = true
		s.queue.mu.Unlock()
		s.notifyChange()
		return err
	}
	s.queue.paused = paused
	s.queue.persistFailed = false
	s.queue.mu.Unlock()
	s.notifyChange()
	return nil
}

func (s *Service) CancelQueue(id string, revision uint64) error {
	done, err := s.beginWorkspaceWork()
	if err != nil {
		return err
	}
	defer done()
	s.queue.mu.Lock()
	defer s.queue.mu.Unlock()
	item, err := scanQueue(s.store.db.QueryRow(`SELECT `+queueColumns+` FROM send_queue WHERE id=?`, id))
	if err != nil {
		return err
	}
	if item.Revision != revision {
		return errors.New("QUEUE_REVISION_CONFLICT")
	}
	if item.State == "completed" || item.State == "cancelled" || item.State == "expired" {
		return errors.New("QUEUE_TERMINAL")
	}
	if item.State == "running" {
		if err = s.CancelTask(item.TaskID); err != nil {
			return err
		}
	}
	if err = s.setQueueState(id, "cancelled", ""); err == nil {
		s.notifyChange()
	}
	return err
}

func (s *Service) ConfirmQueue(id string, revision uint64) error {
	done, err := s.beginWorkspaceWork()
	if err != nil {
		return err
	}
	defer done()
	s.queue.mu.Lock()
	defer s.queue.mu.Unlock()
	item, err := scanQueue(s.store.db.QueryRow(`SELECT `+queueColumns+` FROM send_queue WHERE id=?`, id))
	if err != nil {
		return err
	}
	if item.Revision != revision || item.State != "needs_attention" {
		return errors.New("QUEUE_REVISION_CONFLICT")
	}
	if err = s.checkPeerAllowed(item.PeerID); err != nil {
		return err
	}
	if item.TaskID != "" {
		if task, ok := s.Task(item.TaskID); ok && task.CanResume {
			cfg := s.queue.cfg
			cfg.beforeDispatch = func(_ TaskSnapshot) error { return s.setQueueState(id, "running", "") }
			_, err = s.ResumeTask(task.ID, cfg)
			return err
		}
	}
	prepared, err := transfer.Prepare(s.workCtx, item.paths, 0)
	if err != nil {
		return classifyTaskError(err)
	}
	digest := queueSourceDigest(prepared.Manifest)
	_ = prepared.Close()
	_, err = s.store.db.Exec(`UPDATE send_queue SET state='queued',task_id='',source_digest=?,last_error='',revision=revision+1,updated_at=? WHERE id=? AND revision=?`, digest, time.Now().UTC().Format(time.RFC3339Nano), id, revision)
	if err == nil {
		s.notifyChange()
	}
	return err
}

func (s *Service) ReorderQueue(ids []string) error {
	done, err := s.beginWorkspaceWork()
	if err != nil {
		return err
	}
	defer done()
	if len(ids) > 1000 {
		return errors.New("RESOURCE_LIMIT")
	}
	s.queue.mu.Lock()
	defer s.queue.mu.Unlock()
	tx, err := s.store.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	seen := map[string]bool{}
	for index, id := range ids {
		if seen[id] {
			return errors.New("INVALID_ARGUMENT: duplicate queue item")
		}
		seen[id] = true
		result, e := tx.Exec(`UPDATE send_queue SET position=?,revision=revision+1,updated_at=? WHERE id=? AND state IN ('queued','waiting_peer','needs_attention')`, index, time.Now().UTC().Format(time.RFC3339Nano), id)
		if e != nil {
			return e
		}
		n, e := result.RowsAffected()
		if e != nil {
			return e
		}
		if n != 1 {
			return errors.New("QUEUE_REVISION_CONFLICT")
		}
	}
	if err = tx.Commit(); err == nil {
		s.notifyChange()
	}
	return err
}
