package app

import (
	"context"
	"errors"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Wen5555/LinkSend/internal/protocol"
	"github.com/Wen5555/LinkSend/internal/transfer"
)

// TaskSnapshot is the process-lifetime application view used by Wails and CLI.
// It intentionally contains metadata only; file bytes never cross this boundary.
type TaskSnapshot struct {
	ID                 string   `json:"id"`
	Direction          string   `json:"direction"`
	PeerID             string   `json:"peer_id,omitempty"`
	SourceSummary      string   `json:"source_summary,omitempty"`
	TargetDirectory    string   `json:"target_directory,omitempty"`
	State              string   `json:"state"`
	Phase              string   `json:"phase"`
	ProcessedBytes     int64    `json:"processed_bytes"`
	TotalBytes         *int64   `json:"total_bytes,omitempty"`
	RateBytesPerSecond *float64 `json:"rate_bytes_per_second,omitempty"`
	StartedAt          string   `json:"started_at"`
	UpdatedAt          string   `json:"updated_at"`
	EndedAt            string   `json:"ended_at,omitempty"`
	ErrorCode          string   `json:"error_code,omitempty"`
	ErrorMessage       string   `json:"error_message,omitempty"`
	TransferID         string   `json:"transfer_id,omitempty"`
	SessionID          string   `json:"session_id,omitempty"`
	CanCancel          bool     `json:"can_cancel"`
	CanRetry           bool     `json:"can_retry"`
}

type taskRecord struct {
	mu       sync.RWMutex
	snap     TaskSnapshot
	cancel   context.CancelFunc
	decision chan bool
	peerID   string
	paths    []string
	cfg      DirectConfig
}

type taskManager struct {
	mu    sync.RWMutex
	seq   uint64
	tasks map[string]*taskRecord
}

func newTaskManager() *taskManager { return &taskManager{tasks: make(map[string]*taskRecord)} }

func (m *taskManager) create(s TaskSnapshot, cancel context.CancelFunc) (*taskRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, t := range m.tasks {
		t.mu.RLock()
		active := !isTerminal(t.snap.State)
		t.mu.RUnlock()
		if active {
			return nil, errors.New("BUSY: another transfer task is active")
		}
	}
	m.seq++
	s.ID = time.Now().UTC().Format("20060102T150405.000000000Z") + "-" + formatSeq(m.seq)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	s.StartedAt, s.UpdatedAt, s.State, s.CanCancel = now, now, "preparing", true
	t := &taskRecord{snap: s, cancel: cancel}
	m.tasks[s.ID] = t
	return t, nil
}

func formatSeq(n uint64) string {
	const digits = "0123456789abcdef"
	b := make([]byte, 8)
	for i := range b {
		b[len(b)-1-i] = digits[n&15]
		n >>= 4
	}
	return string(b)
}
func isTerminal(s string) bool { return s == "completed" || s == "failed" || s == "cancelled" }

func (t *taskRecord) snapshot() TaskSnapshot { t.mu.RLock(); defer t.mu.RUnlock(); return t.snap }
func (t *taskRecord) update(fn func(*TaskSnapshot)) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if isTerminal(t.snap.State) {
		return
	}
	fn(&t.snap)
	t.snap.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
}
func (t *taskRecord) progress(p transfer.Progress) {
	t.update(func(v *TaskSnapshot) {
		if v.State == "cancel_requested" {
			return
		}
		v.TransferID = p.TransferID
		v.Phase = strings.ToLower(p.State)
		switch strings.ToLower(p.State) {
		case "preparing":
			v.State = "preparing"
		case "awaitingacceptance":
			v.State = "awaiting_acceptance"
		case "transferring":
			v.State = "transferring"
		case "verifying":
			v.State = "verifying"
		}
		v.ProcessedBytes = p.Verified
		if p.Total >= 0 {
			n := p.Total
			v.TotalBytes = &n
		}
		if p.BytesPerSecond > 0 {
			rate := p.BytesPerSecond
			v.RateBytesPerSecond = &rate
		}
	})
}
func (t *taskRecord) finish(state string, err error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if isTerminal(t.snap.State) {
		return
	}
	if t.snap.State == "cancel_requested" && state == "completed" {
		state = "cancelled"
		err = protocol.Wrap(protocol.Cancelled, "task cancellation confirmed", err)
	}
	t.snap.State = state
	t.snap.CanCancel = false
	t.snap.CanRetry = state == "failed" && t.snap.Direction == "send"
	t.snap.EndedAt = time.Now().UTC().Format(time.RFC3339Nano)
	t.snap.UpdatedAt = t.snap.EndedAt
	if err != nil {
		t.snap.ErrorCode = string(protocol.ErrorCode(err))
		t.snap.ErrorMessage = userError(err)
	}
}
func userError(err error) string {
	if err == nil {
		return ""
	}
	if e, ok := err.(*protocol.Error); ok && e.Detail != "" {
		return e.Detail
	}
	return strings.TrimSpace(err.Error())
}

func (s *Service) StartSend(peerID string, paths []string, cfg DirectConfig) (TaskSnapshot, error) {
	if strings.TrimSpace(peerID) == "" || len(paths) == 0 {
		return TaskSnapshot{}, errors.New("INVALID_ARGUMENT: peer and source paths are required")
	}
	ctx, cancel := context.WithCancel(context.Background())
	base := make([]string, len(paths))
	copy(base, paths)
	t, err := s.tasks.create(TaskSnapshot{Direction: "send", PeerID: peerID, SourceSummary: sourceSummary(paths)}, cancel)
	if err != nil {
		cancel()
		return TaskSnapshot{}, err
	}
	t.peerID, t.paths, t.cfg = peerID, base, cfg
	cfg.onPhase = func(phase string) {
		t.update(func(v *TaskSnapshot) {
			if v.State != "cancel_requested" {
				v.Phase = phase
			}
		})
	}
	cfg.onSession = func(sessionID, _ string) { t.update(func(v *TaskSnapshot) { v.SessionID = sessionID }) }
	go func() {
		result, runErr := s.SendFilesDetailed(ctx, peerID, base, cfg, t.progress)
		if runErr != nil {
			if errors.Is(ctx.Err(), context.Canceled) {
				t.finish("cancelled", protocol.Wrap(protocol.Cancelled, "task cancellation confirmed", runErr))
			} else {
				t.finish("failed", runErr)
			}
			return
		}
		t.update(func(v *TaskSnapshot) {
			v.TransferID = result.Transfer.TransferID
			v.SessionID = result.Evidence.SessionID
			v.Phase = "completed"
			v.ProcessedBytes = result.Transfer.Bytes
			n := result.Transfer.Bytes
			v.TotalBytes = &n
		})
		t.finish("completed", nil)
	}()
	return t.snapshot(), nil
}

func (s *Service) StartReceive(expectedPeerID, directory string, cfg DirectConfig) (TaskSnapshot, error) {
	if strings.TrimSpace(directory) == "" {
		return TaskSnapshot{}, errors.New("INVALID_ARGUMENT: receive directory is required")
	}
	ctx, cancel := context.WithCancel(context.Background())
	t, err := s.tasks.create(TaskSnapshot{Direction: "receive", PeerID: expectedPeerID, TargetDirectory: filepath.Clean(directory)}, cancel)
	if err != nil {
		cancel()
		return TaskSnapshot{}, err
	}
	t.cfg = cfg
	t.decision = make(chan bool, 1)
	cfg.onPhase = func(phase string) {
		t.update(func(v *TaskSnapshot) {
			if v.State != "cancel_requested" {
				v.Phase = phase
			}
		})
	}
	cfg.onSession = func(sessionID, _ string) { t.update(func(v *TaskSnapshot) { v.SessionID = sessionID }) }
	go func() {
		result, runErr := s.ReceiveOnceDetailed(ctx, expectedPeerID, directory, cfg, func(m transfer.Manifest) bool {
			n := m.TotalBytes()
			t.update(func(v *TaskSnapshot) {
				v.State = "awaiting_acceptance"
				v.Phase = "awaiting_acceptance"
				v.TotalBytes = &n
				v.CanCancel = true
			})
			select {
			case accepted := <-t.decision:
				return accepted
			case <-ctx.Done():
				return false
			}
		}, t.progress)
		if runErr != nil {
			if errors.Is(ctx.Err(), context.Canceled) {
				t.finish("cancelled", protocol.Wrap(protocol.Cancelled, "task cancellation confirmed", runErr))
			} else {
				t.finish("failed", runErr)
			}
			return
		}
		t.update(func(v *TaskSnapshot) {
			v.TransferID = result.Transfer.TransferID
			v.SessionID = result.Evidence.SessionID
			v.Phase = "completed"
			v.ProcessedBytes = result.Transfer.Bytes
			n := result.Transfer.Bytes
			v.TotalBytes = &n
		})
		t.finish("completed", nil)
	}()
	return t.snapshot(), nil
}

func sourceSummary(paths []string) string {
	names := make([]string, 0, len(paths))
	for _, p := range paths {
		names = append(names, filepath.Base(filepath.Clean(p)))
	}
	return strings.Join(names, ", ")
}

func (s *Service) Tasks() []TaskSnapshot {
	s.tasks.mu.RLock()
	out := make([]TaskSnapshot, 0, len(s.tasks.tasks))
	for _, t := range s.tasks.tasks {
		out = append(out, t.snapshot())
	}
	s.tasks.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].StartedAt < out[j].StartedAt })
	return out
}
func (s *Service) Task(id string) (TaskSnapshot, bool) {
	s.tasks.mu.RLock()
	t := s.tasks.tasks[id]
	s.tasks.mu.RUnlock()
	if t == nil {
		return TaskSnapshot{}, false
	}
	return t.snapshot(), true
}
func (s *Service) CancelTask(id string) error {
	s.tasks.mu.RLock()
	t := s.tasks.tasks[id]
	s.tasks.mu.RUnlock()
	if t == nil {
		return errors.New("TASK_NOT_FOUND")
	}
	snap := t.snapshot()
	if isTerminal(snap.State) {
		return errors.New("TASK_TERMINAL: task is already finished")
	}
	if snap.State == "cancel_requested" {
		return errors.New("TASK_CANCEL_ALREADY_REQUESTED: cancellation is still pending")
	}
	t.update(func(v *TaskSnapshot) { v.State = "cancel_requested"; v.Phase = "cancelling"; v.CanCancel = false })
	t.cancel()
	return nil
}
func (s *Service) AcceptTask(id string) error { return s.decideTask(id, true) }
func (s *Service) RejectTask(id string) error { return s.decideTask(id, false) }
func (s *Service) decideTask(id string, accepted bool) error {
	s.tasks.mu.RLock()
	t := s.tasks.tasks[id]
	s.tasks.mu.RUnlock()
	if t == nil || t.decision == nil {
		return errors.New("TASK_NOT_AWAITING_ACCEPTANCE")
	}
	snap := t.snapshot()
	if snap.State != "awaiting_acceptance" {
		return errors.New("TASK_NOT_AWAITING_ACCEPTANCE")
	}
	select {
	case t.decision <- accepted:
		return nil
	default:
		return errors.New("TASK_DECISION_ALREADY_SET")
	}
}
func (s *Service) RetryTask(id string) (TaskSnapshot, error) {
	s.tasks.mu.RLock()
	t := s.tasks.tasks[id]
	s.tasks.mu.RUnlock()
	if t == nil {
		return TaskSnapshot{}, errors.New("TASK_NOT_FOUND")
	}
	snap := t.snapshot()
	if snap.State != "failed" || t.peerID == "" || len(t.paths) == 0 {
		return TaskSnapshot{}, errors.New("TASK_NOT_RETRYABLE")
	}
	return s.StartSend(t.peerID, t.paths, t.cfg)
}

// Shutdown cancels active in-process tasks. It does not claim restart recovery.
func (s *Service) Shutdown() {
	s.tasks.mu.RLock()
	for _, t := range s.tasks.tasks {
		if !isTerminal(t.snapshot().State) {
			t.cancel()
		}
	}
	s.tasks.mu.RUnlock()
}
