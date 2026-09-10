package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/Wen5555/LinkSend/internal/protocol"
	"github.com/Wen5555/LinkSend/internal/transfer"
)

// TaskSnapshot is the process-lifetime application view used by Wails and CLI.
// It intentionally contains metadata only; file bytes never cross this boundary.
type TaskSnapshot struct {
	ID                       string   `json:"id"`
	Direction                string   `json:"direction"`
	PeerID                   string   `json:"peer_id,omitempty"`
	SourceSummary            string   `json:"source_summary,omitempty"`
	ManifestSummary          string   `json:"manifest_summary,omitempty"`
	FileCount                int      `json:"file_count,omitempty"`
	TargetDirectory          string   `json:"target_directory,omitempty"`
	State                    string   `json:"state"`
	Phase                    string   `json:"phase"`
	ProcessedBytes           int64    `json:"processed_bytes"`
	TotalBytes               *int64   `json:"total_bytes,omitempty"`
	RateBytesPerSecond       *float64 `json:"rate_bytes_per_second,omitempty"`
	StartedAt                string   `json:"started_at"`
	UpdatedAt                string   `json:"updated_at"`
	EndedAt                  string   `json:"ended_at,omitempty"`
	ErrorCode                string   `json:"error_code,omitempty"`
	ErrorMessage             string   `json:"error_message,omitempty"`
	TransferID               string   `json:"transfer_id,omitempty"`
	SessionID                string   `json:"session_id,omitempty"`
	ConnectionMethod         string   `json:"connection_method,omitempty"`
	TransportProtocol        string   `json:"transport_protocol,omitempty"`
	Relay                    bool     `json:"relay"`
	CanCancel                bool     `json:"can_cancel"`
	CanRetry                 bool     `json:"can_retry"`
	CanPause                 bool     `json:"can_pause"`
	CanResume                bool     `json:"can_resume"`
	Revision                 uint64   `json:"revision"`
	HistoryPersisted         bool     `json:"history_persisted"`
	RestartRecoverySupported bool     `json:"restart_recovery_supported"`
	ByteResumeSupported      bool     `json:"byte_resume_supported"`
}

type taskRecord struct {
	mu       sync.RWMutex
	snap     TaskSnapshot
	cancel   context.CancelFunc
	decision chan bool
	peerID   string
	paths    []string
	cfg      DirectConfig
	persist  func(TaskSnapshot)
}

type taskManager struct {
	mu          sync.RWMutex
	seq         uint64
	tasks       map[string]*taskRecord
	historyPath string
}

type taskHistory struct {
	SchemaVersion int            `json:"schema_version"`
	Tasks         []TaskSnapshot `json:"tasks"`
}

func (m *taskManager) configureHistory(path string) {
	m.historyPath = path
	b, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var h taskHistory
	if json.Unmarshal(b, &h) != nil || h.SchemaVersion != 1 {
		return
	}
	for _, snap := range h.Tasks {
		if snap.ID == "" {
			continue
		}
		if !isTerminal(snap.State) {
			snap.State = "recovering"
			snap.Phase = "recovering"
			snap.CanCancel = false
			snap.CanResume = snap.Direction == "receive"
		}
		snap.HistoryPersisted = true
		snap.RestartRecoverySupported = true
		snap.ByteResumeSupported = true
		m.tasks[snap.ID] = &taskRecord{snap: snap}
	}
}

func (m *taskManager) persistSnapshot(_ TaskSnapshot) {
	if m.historyPath == "" {
		return
	}
	m.mu.RLock()
	h := taskHistory{SchemaVersion: 1, Tasks: make([]TaskSnapshot, 0, len(m.tasks))}
	for _, t := range m.tasks {
		h.Tasks = append(h.Tasks, t.snapshot())
	}
	m.mu.RUnlock()
	b, err := json.MarshalIndent(h, "", "  ")
	if err != nil {
		return
	}
	tmp := m.historyPath + ".tmp"
	if err = os.WriteFile(tmp, b, 0600); err == nil {
		_ = os.Rename(tmp, m.historyPath)
	}
}

func newTaskManager() *taskManager { return &taskManager{tasks: make(map[string]*taskRecord)} }

func (m *taskManager) create(s TaskSnapshot, cancel context.CancelFunc) (*taskRecord, error) {
	m.mu.Lock()
	for _, t := range m.tasks {
		t.mu.RLock()
		active := !isTerminal(t.snap.State)
		t.mu.RUnlock()
		if active {
			m.mu.Unlock()
			return nil, errors.New("BUSY: another transfer task is active")
		}
	}
	m.seq++
	s.ID = time.Now().UTC().Format("20060102T150405.000000000Z") + "-" + formatSeq(m.seq)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	s.StartedAt, s.UpdatedAt, s.State, s.CanCancel = now, now, "preparing", true
	s.Revision = 1
	s.HistoryPersisted = true
	s.RestartRecoverySupported = true
	s.ByteResumeSupported = true
	t := &taskRecord{snap: s, cancel: cancel}
	t.persist = m.persistSnapshot
	m.tasks[s.ID] = t
	m.mu.Unlock()
	m.persistSnapshot(s)
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
	if isTerminal(t.snap.State) {
		t.mu.Unlock()
		return
	}
	fn(&t.snap)
	t.snap.Revision++
	t.snap.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	t.mu.Unlock()
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
			v.CanPause = true
			v.CanResume = false
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
	if isTerminal(t.snap.State) {
		t.mu.Unlock()
		return
	}
	if t.snap.State == "cancel_requested" && state == "completed" {
		state = "cancelled"
		err = protocol.Wrap(protocol.Cancelled, "task cancellation confirmed", err)
	}
	t.snap.State = state
	t.snap.Revision++
	t.snap.CanCancel = false
	t.snap.CanPause = false
	t.snap.CanResume = false
	t.snap.CanRetry = state == "failed" && t.snap.Direction == "send"
	t.snap.EndedAt = time.Now().UTC().Format(time.RFC3339Nano)
	t.snap.UpdatedAt = t.snap.EndedAt
	if err != nil {
		t.snap.ErrorCode = string(protocol.ErrorCode(err))
		t.snap.ErrorMessage = userError(err)
	}
	t.mu.Unlock()
}
func userError(err error) string {
	if err == nil {
		return ""
	}
	var e *protocol.Error
	if errors.As(err, &e) {
		if detail := userErrorForCode(e.Code); detail != "" {
			return detail
		}
		if e.Detail != "" {
			return e.Detail
		}
	}
	return "传输未完成，请检查设备状态、网络和接收目录后重试。"
}

// classifyTaskError converts internal transfer/system errors into stable,
// user-actionable protocol codes while retaining the original cause for
// errors.Is/errors.As callers. Raw HTTP, filesystem and stack details must not
// cross the Wails task DTO boundary.
func classifyTaskError(err error) error {
	if err == nil {
		return nil
	}
	var existing *protocol.Error
	if errors.As(err, &existing) {
		return err
	}
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, transfer.ErrCancelled):
		return protocol.Wrap(protocol.Cancelled, "传输已取消", err)
	case errors.Is(err, transfer.ErrRejected):
		return protocol.Wrap(protocol.ReceiveRejected, "接收方拒绝了本次传输，请确认对方已准备接收。", err)
	case errors.Is(err, transfer.ErrChanged):
		return protocol.Wrap(protocol.SourceChanged, "源文件在传输过程中发生变化，请重新选择文件后重试。", err)
	case errors.Is(err, transfer.ErrIntegrity):
		return protocol.Wrap(protocol.IntegrityFailed, "文件完整性校验失败，请重试并检查磁盘或网络。", err)
	case errors.Is(err, transfer.ErrPath):
		return protocol.Wrap(protocol.UnsafePath, "目标路径不安全或包含不支持的名称，请选择其他目录。", err)
	case errors.Is(err, transfer.ErrConflict):
		return protocol.Wrap(protocol.DirectFailed, "目标文件已存在且不会覆盖，请选择空目录后重试。", err)
	case errors.Is(err, syscall.ENOSPC):
		return protocol.Wrap(protocol.DiskFull, "接收磁盘空间不足，请清理空间后重试。", err)
	case errors.Is(err, fs.ErrPermission):
		return protocol.Wrap(protocol.DiskFull, "接收目录没有写入权限，请选择可写目录。", err)
	default:
		return protocol.Wrap(protocol.DirectFailed, "传输未完成，请根据当前阶段检查设备在线、网络和接收目录。", err)
	}
}

func userErrorForCode(code protocol.Code) string {
	switch code {
	case protocol.SignalingUnreachable:
		return "无法连接信令服务，请检查服务地址和网络后重试。"
	case protocol.SignalingTimeout, protocol.CandidateTimeout:
		return "信令或候选交换超时，请检查网络后重试。"
	case protocol.PeerOffline:
		return "对端当前离线，请让对端保持 LinkSend 运行。"
	case protocol.Unpaired:
		return "设备尚未完成指纹信任，请在设备页核对完整指纹。"
	case protocol.AuthenticationFailed:
		return "身份验证失败，请确认设备组成员资格和已保存指纹。"
	case protocol.VersionIncompatible:
		return "双方版本或传输能力不兼容，请升级到兼容版本。"
	case protocol.NoCandidates, protocol.NoViableCandidate, protocol.CheckTimeout, protocol.ICEFailed:
		return "未能建立直连，请检查绑定地址、UDP 防火墙和网络切换。"
	case protocol.QUICHandshakeTimeout, protocol.QUICHandshakeFailed:
		return "安全直连握手失败，请确认双方指纹一致并重试。"
	case protocol.DirectFailed:
		return "直连或传输未完成，请检查设备在线、网络和接收目录后重试。"
	case protocol.ReceiveRejected:
		return "接收方拒绝了本次传输。"
	case protocol.SourceChanged:
		return "源文件在传输过程中发生变化，请重新选择文件后重试。"
	case protocol.IntegrityFailed:
		return "文件完整性校验失败，请重试并检查磁盘或网络。"
	case protocol.DiskFull:
		return "接收磁盘空间不足或目录不可写，请选择其他目录。"
	case protocol.UnsafePath:
		return "目标路径不安全或包含不支持的名称，请选择其他目录。"
	case protocol.Cancelled:
		return "传输已取消。"
	case protocol.RelayNotImplemented:
		return "当前网络需要中继，但 LinkSend 暂未实现中继。"
	case protocol.InvalidMessage, protocol.Replay, protocol.RateLimited:
		return "对端或服务返回了无效请求，请刷新状态后重试。"
	default:
		return ""
	}
}

// UserError returns a stable, actionable message suitable for CLI/Wails
// surfaces. It intentionally omits private causes and filesystem details.
func UserError(err error) string { return userError(err) }

// ClassifyError exposes the same boundary used by in-process task snapshots
// for callers such as the standalone CLI. The returned error retains its
// original cause for internal errors.Is/errors.As checks.
func ClassifyError(err error) error { return classifyTaskError(err) }

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
	cfg.onSession = func(sessionID, peerID string) {
		t.update(func(v *TaskSnapshot) { v.SessionID = sessionID; v.PeerID = peerID })
	}
	cfg.onEvidence = func(e DirectEvidence) {
		t.update(func(v *TaskSnapshot) {
			v.ConnectionMethod = e.ConnectionMethod
			v.TransportProtocol = e.TransportProtocol
			v.Relay = e.Relay
		})
	}
	go func() {
		result, runErr := s.SendFilesDetailed(ctx, peerID, base, cfg, t.progress)
		if runErr != nil {
			runErr = classifyTaskError(runErr)
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
	cfg.onSession = func(sessionID, peerID string) {
		t.update(func(v *TaskSnapshot) { v.SessionID = sessionID; v.PeerID = peerID })
	}
	cfg.onEvidence = func(e DirectEvidence) {
		t.update(func(v *TaskSnapshot) {
			v.ConnectionMethod = e.ConnectionMethod
			v.TransportProtocol = e.TransportProtocol
			v.Relay = e.Relay
		})
	}
	go func() {
		result, runErr := s.ReceiveOnceDetailed(ctx, expectedPeerID, directory, cfg, func(m transfer.Manifest) bool {
			n := m.TotalBytes()
			t.update(func(v *TaskSnapshot) {
				v.State = "awaiting_acceptance"
				v.Phase = "awaiting_acceptance"
				v.TotalBytes = &n
				v.FileCount = len(m.Files)
				v.ManifestSummary = manifestSummary(m)
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
			runErr = classifyTaskError(runErr)
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

func manifestSummary(m transfer.Manifest) string {
	if len(m.Files) == 0 {
		return "空内容"
	}
	const maxNames = 3
	names := make([]string, 0, maxNames)
	for _, f := range m.Files {
		if f.Type != "file" {
			continue
		}
		name := filepath.Base(filepath.Clean(f.Path))
		if name == "." || name == "" {
			continue
		}
		names = append(names, name)
		if len(names) == maxNames {
			break
		}
	}
	if len(names) == 0 {
		return fmt.Sprintf("%d 个目录", len(m.Files))
	}
	if len(m.Files) > len(names) {
		return strings.Join(names, "、") + fmt.Sprintf(" 等 %d 项", len(m.Files))
	}
	return strings.Join(names, "、")
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
