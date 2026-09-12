package app

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/Wen5555/LinkSend/internal/connectivity"
	"github.com/Wen5555/LinkSend/internal/identity"
	"github.com/Wen5555/LinkSend/internal/protocol"
	"github.com/Wen5555/LinkSend/internal/transfer"
)

// TaskSnapshot is the process-lifetime application view used by Wails and CLI.
// It intentionally contains metadata only; file bytes never cross this boundary.
type TaskPhaseEvent struct {
	Phase string `json:"phase"`
	At    string `json:"at"`
}

type TaskSnapshot struct {
	ID                       string                       `json:"id"` // compatibility alias for task_id
	TaskID                   string                       `json:"task_id"`
	AttemptID                string                       `json:"attempt_id"`
	Direction                string                       `json:"direction"`
	PeerID                   string                       `json:"peer_id,omitempty"`
	SourceSummary            string                       `json:"source_summary,omitempty"`
	ManifestSummary          string                       `json:"manifest_summary,omitempty"`
	FileCount                int                          `json:"file_count,omitempty"`
	TargetDirectory          string                       `json:"target_directory,omitempty"`
	State                    string                       `json:"state"`
	Phase                    string                       `json:"phase"`
	ProcessedBytes           int64                        `json:"processed_bytes"`
	TotalBytes               *int64                       `json:"total_bytes,omitempty"`
	RateBytesPerSecond       *float64                     `json:"rate_bytes_per_second,omitempty"`
	StartedAt                string                       `json:"started_at"`
	UpdatedAt                string                       `json:"updated_at"`
	EndedAt                  string                       `json:"ended_at,omitempty"`
	ErrorCode                string                       `json:"error_code,omitempty"`
	ErrorMessage             string                       `json:"error_message,omitempty"`
	TransferID               string                       `json:"transfer_id,omitempty"`
	SessionID                string                       `json:"session_id,omitempty"`
	ICEGeneration            uint64                       `json:"ice_generation,omitempty"`
	ManifestDigest           string                       `json:"manifest_digest,omitempty"`
	ChunkSize                int                          `json:"chunk_size,omitempty"`
	SentBytes                int64                        `json:"sent_bytes"`
	ReceivedBytes            int64                        `json:"received_bytes"`
	RetransmittedBytes       int64                        `json:"retransmitted_bytes"`
	VerifiedBytes            int64                        `json:"verified_bytes"`
	CommittedBytes           int64                        `json:"committed_bytes"`
	CommittedFiles           int                          `json:"committed_files"`
	BilateralConfirmed       bool                         `json:"bilateral_confirmed"`
	ConnectionMethod         string                       `json:"connection_method,omitempty"`
	TransportProtocol        string                       `json:"transport_protocol,omitempty"`
	Relay                    bool                         `json:"relay"`
	BaseSocket               string                       `json:"base_socket,omitempty"`
	NetworkInterface         string                       `json:"network_interface,omitempty"`
	AddressFamily            string                       `json:"address_family,omitempty"`
	LocalCandidate           string                       `json:"local_candidate,omitempty"`
	RemoteCandidate          string                       `json:"remote_candidate,omitempty"`
	LocalCandidateType       string                       `json:"local_candidate_type,omitempty"`
	RemoteCandidateType      string                       `json:"remote_candidate_type,omitempty"`
	STUNRequestsSent         uint64                       `json:"stun_requests_sent"`
	STUNResponsesReceived    uint64                       `json:"stun_responses_received"`
	SignalingBytesSent       uint64                       `json:"signaling_bytes_sent"`
	SignalingBytesReceived   uint64                       `json:"signaling_bytes_received"`
	ICEStateTimeline         []connectivity.ICEStateEvent `json:"ice_state_timeline,omitempty"`
	PhaseTimeline            []TaskPhaseEvent             `json:"phase_timeline,omitempty"`
	TLSVersion               uint16                       `json:"tls_version,omitempty"`
	ALPN                     string                       `json:"alpn,omitempty"`
	ConnectTimings           DirectTimings                `json:"connect_timings"`
	CanCancel                bool                         `json:"can_cancel"`
	CanRetry                 bool                         `json:"can_retry"`
	CanPause                 bool                         `json:"can_pause"`
	CanResume                bool                         `json:"can_resume"`
	Revision                 uint64                       `json:"revision"`
	HistoryPersisted         bool                         `json:"history_persisted"`
	RestartRecoverySupported bool                         `json:"restart_recovery_supported"`
	ByteResumeSupported      bool                         `json:"byte_resume_supported"`
}

type taskRecord struct {
	mu       sync.RWMutex
	snap     TaskSnapshot
	recovery taskRecovery
	cancel   context.CancelFunc
	decision chan bool
	peerID   string
	paths    []string
	cfg      DirectConfig
	persist  func(TaskSnapshot, taskRecovery) error
	changed  func(TaskSnapshot)
}

type taskRecovery struct {
	Version          int               `json:"version"`
	Direction        string            `json:"direction"`
	PeerID           string            `json:"peer_id"`
	PeerFingerprint  string            `json:"peer_fingerprint"`
	SourcePaths      []string          `json:"source_paths,omitempty"`
	TargetDirectory  string            `json:"target_directory,omitempty"`
	TransferID       string            `json:"transfer_id,omitempty"`
	ManifestDigest   string            `json:"manifest_digest,omitempty"`
	ChunkSize        int               `json:"chunk_size,omitempty"`
	TotalBytes       int64             `json:"total_bytes,omitempty"`
	FileCount        int               `json:"file_count,omitempty"`
	ActualSentBytes  int64             `json:"actual_sent_bytes,omitempty"`
	ReceivedBytes    int64             `json:"received_bytes,omitempty"`
	RetransmitBytes  int64             `json:"retransmit_bytes,omitempty"`
	LogicalCompleted int64             `json:"logical_completed_bytes,omitempty"`
	SentChunks       map[uint32][]bool `json:"sent_chunks,omitempty"`
}

type taskManager struct {
	mu          sync.RWMutex
	seq         uint64
	tasks       map[string]*taskRecord
	historyPath string
	historyMu   sync.RWMutex
	historyErr  error
	workers     sync.WaitGroup
	onChange    func(TaskSnapshot)
}

func newTaskManager() *taskManager { return &taskManager{tasks: make(map[string]*taskRecord)} }

func (m *taskManager) hasActive() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, task := range m.tasks {
		task.mu.RLock()
		active := task.cancel != nil
		task.mu.RUnlock()
		if active {
			return true
		}
	}
	return false
}

func (m *taskManager) create(s TaskSnapshot, cancel context.CancelFunc) (*taskRecord, error) {
	m.mu.Lock()
	for _, t := range m.tasks {
		t.mu.RLock()
		active := t.cancel != nil
		t.mu.RUnlock()
		if active {
			m.mu.Unlock()
			return nil, errors.New("BUSY: another transfer task is active")
		}
	}
	m.seq++
	s.ID = time.Now().UTC().Format("20060102T150405.000000000Z") + "-" + formatSeq(m.seq)
	s.TaskID = s.ID
	s.AttemptID = protocol.RandomID()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	s.StartedAt, s.UpdatedAt, s.State, s.CanCancel = now, now, "preparing", true
	if s.Phase == "" {
		s.Phase = "preparing"
	}
	s.PhaseTimeline = append(s.PhaseTimeline, TaskPhaseEvent{Phase: s.Phase, At: now})
	s.Revision = 1
	s.HistoryPersisted = m.historyAvailable()
	t := &taskRecord{snap: s, cancel: cancel, recovery: taskRecovery{Version: 1, Direction: s.Direction, PeerID: s.PeerID, PeerFingerprint: s.PeerID, TargetDirectory: s.TargetDirectory}}
	t.persist = m.persistRecordTracked
	t.changed = m.onChange
	m.tasks[s.ID] = t
	m.mu.Unlock()
	if err := m.persistRecord(s, t.recovery); err != nil {
		t.mu.Lock()
		t.snap.HistoryPersisted = false
		t.mu.Unlock()
	}
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
func isTerminal(s string) bool {
	return s == "completed" || s == "rejected" || s == "failed" || s == "cancelled"
}

func (t *taskRecord) snapshot() TaskSnapshot {
	t.mu.RLock()
	defer t.mu.RUnlock()
	snapshot := t.snap
	snapshot.ICEStateTimeline = append([]connectivity.ICEStateEvent(nil), t.snap.ICEStateTimeline...)
	snapshot.PhaseTimeline = append([]TaskPhaseEvent(nil), t.snap.PhaseTimeline...)
	return snapshot
}
func (t *taskRecord) recoverySnapshot() taskRecovery {
	t.mu.RLock()
	defer t.mu.RUnlock()
	recovery := t.recovery
	recovery.SourcePaths = append([]string(nil), recovery.SourcePaths...)
	return recovery
}
func (t *taskRecord) update(fn func(*TaskSnapshot)) {
	t.updateAttempt("", fn)
}
func (t *taskRecord) updateAttempt(attemptID string, fn func(*TaskSnapshot)) bool {
	return t.updateRecordAttempt(attemptID, func(snap *TaskSnapshot, _ *taskRecovery) { fn(snap) })
}
func (t *taskRecord) updateAttemptTransient(attemptID string, fn func(*TaskSnapshot)) bool {
	return t.updateRecordAttemptMode(attemptID, false, func(snap *TaskSnapshot, _ *taskRecovery) { fn(snap) })
}
func (t *taskRecord) updateRecovery(attemptID string, fn func(*taskRecovery)) bool {
	return t.updateRecordAttempt(attemptID, func(_ *TaskSnapshot, recovery *taskRecovery) { fn(recovery) })
}
func (t *taskRecord) updateRecordAttempt(attemptID string, fn func(*TaskSnapshot, *taskRecovery)) bool {
	return t.updateRecordAttemptMode(attemptID, true, fn)
}
func (t *taskRecord) updateRecordAttemptMode(attemptID string, persist bool, fn func(*TaskSnapshot, *taskRecovery)) bool {
	t.mu.Lock()
	if isTerminal(t.snap.State) || (attemptID != "" && t.snap.AttemptID != attemptID) {
		t.mu.Unlock()
		return false
	}
	previousPhase := t.snap.Phase
	previousState := t.snap.State
	fn(&t.snap, &t.recovery)
	if previousState == "pause_requested" || previousState == "cancel_requested" || previousState == "shutdown_requested" {
		t.snap.State, t.snap.Phase = previousState, previousPhase
		t.snap.CanPause, t.snap.CanCancel = false, false
	}
	t.snap.Revision++
	t.snap.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if t.snap.Phase != "" && t.snap.Phase != previousPhase && len(t.snap.PhaseTimeline) < 64 {
		t.snap.PhaseTimeline = append(t.snap.PhaseTimeline, TaskPhaseEvent{Phase: t.snap.Phase, At: t.snap.UpdatedAt})
	}
	snap := t.snap
	recovery := t.recovery
	t.mu.Unlock()
	if persist {
		t.save(snap, recovery)
	} else if t.changed != nil {
		t.changed(snap)
	}
	return true
}
func (t *taskRecord) progress(attemptID string, p transfer.Progress) {
	// Progress is process-live UI state. Durable byte checkpoints are written by
	// recordChunkSent/recordReceived and terminal transitions; synchronously
	// committing every UI callback would add disk latency to every QUIC ACK.
	t.updateRecordAttemptMode(attemptID, false, func(v *TaskSnapshot, recovery *taskRecovery) {
		if v.State == "cancel_requested" {
			return
		}
		v.TransferID = p.TransferID
		// A progress callback may arrive after PauseTask has cancelled the
		// attempt (for example, the ACK for the last body frame already written).
		// Keep its verified-byte accounting, but never let it roll the control
		// state back from pause_requested to transferring/verifying.
		if v.State != "pause_requested" {
			phase := strings.ToLower(p.State)
			if phase == "awaitingacceptance" {
				phase = "awaiting_acceptance"
			}
			v.Phase = phase
			switch phase {
			case "preparing":
				if v.State != "recovering" {
					v.State = "preparing"
				}
			case "awaiting_acceptance":
				v.State = "awaiting_acceptance"
			case "transferring":
				v.State = "transferring"
				v.CanPause = true
				v.CanResume = false
			case "verifying":
				v.State = "verifying"
				v.CanPause = false
			}
		}
		v.ProcessedBytes = p.Verified
		v.VerifiedBytes = p.Verified
		recovery.LogicalCompleted = p.Verified
		v.SentBytes = recovery.ActualSentBytes
		v.RetransmittedBytes = recovery.RetransmitBytes
		v.ReceivedBytes = recovery.ReceivedBytes
		v.CommittedBytes = p.Committed
		v.CommittedFiles = p.CommittedFiles
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

func (t *taskRecord) wasChunkSent(fileID uint32, index int) bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	bits := t.recovery.SentChunks[fileID]
	return index >= 0 && index < len(bits) && bits[index]
}

func (t *taskRecord) recordChunkSent(attemptID string, sent transfer.ChunkTransmission) {
	t.updateRecordAttempt(attemptID, func(v *TaskSnapshot, recovery *taskRecovery) {
		bits := recovery.SentChunks[sent.FileID]
		if sent.Index >= 0 && sent.Index < len(bits) {
			bits[sent.Index] = true
			recovery.SentChunks[sent.FileID] = bits
		}
		recovery.ActualSentBytes += sent.Bytes
		if sent.Retransmitted {
			recovery.RetransmitBytes += sent.Bytes
		}
		v.SentBytes = recovery.ActualSentBytes
		v.RetransmittedBytes = recovery.RetransmitBytes
	})
}

func (t *taskRecord) recordReceived(attemptID string, bytes int64) {
	if bytes <= 0 {
		return
	}
	t.updateRecordAttempt(attemptID, func(v *TaskSnapshot, recovery *taskRecovery) {
		recovery.ReceivedBytes += bytes
		v.ReceivedBytes = recovery.ReceivedBytes
	})
}

func applyDirectEvidence(snapshot *TaskSnapshot, evidence DirectEvidence) {
	snapshot.ConnectionMethod = evidence.ConnectionMethod
	snapshot.TransportProtocol = evidence.TransportProtocol
	snapshot.Relay = evidence.Relay
	snapshot.BaseSocket = evidence.BaseSocket
	snapshot.NetworkInterface = evidence.Interface
	snapshot.AddressFamily = evidence.AddressFamily
	snapshot.LocalCandidate = evidence.LocalCandidate
	snapshot.RemoteCandidate = evidence.RemoteCandidate
	snapshot.LocalCandidateType = evidence.LocalType
	snapshot.RemoteCandidateType = evidence.RemoteType
	snapshot.STUNRequestsSent = evidence.STUNRequestsSent
	snapshot.STUNResponsesReceived = evidence.STUNResponsesReceived
	snapshot.SignalingBytesSent = evidence.SignalingBytesSent
	snapshot.SignalingBytesReceived = evidence.SignalingBytesReceived
	snapshot.ICEStateTimeline = append([]connectivity.ICEStateEvent(nil), evidence.ICEStateTimeline...)
	snapshot.TLSVersion = evidence.TLSVersion
	snapshot.ALPN = evidence.ALPN
	snapshot.ConnectTimings = evidence.Timings
}

func clearDirectEvidence(snapshot *TaskSnapshot) {
	applyDirectEvidence(snapshot, DirectEvidence{})
}
func (t *taskRecord) finish(state string, err error) {
	t.finishAttempt("", state, err)
}
func (t *taskRecord) finishAttempt(attemptID, state string, err error) {
	t.mu.Lock()
	if isTerminal(t.snap.State) || (attemptID != "" && t.snap.AttemptID != attemptID) {
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
	t.snap.CanRetry = (state == "failed" || state == "rejected") && t.snap.Direction == "send"
	t.cancel = nil
	t.snap.EndedAt = time.Now().UTC().Format(time.RFC3339Nano)
	t.snap.UpdatedAt = t.snap.EndedAt
	if err != nil {
		t.snap.ErrorCode = string(protocol.ErrorCode(err))
		t.snap.ErrorMessage = userError(err)
	}
	snap := t.snap
	recovery := t.recovery
	t.mu.Unlock()
	t.save(snap, recovery)
}

func (t *taskRecord) pauseAttempt(attemptID string) {
	t.mu.Lock()
	if isTerminal(t.snap.State) || t.snap.AttemptID != attemptID {
		t.mu.Unlock()
		return
	}
	t.snap.State = "paused"
	t.snap.Phase = "paused"
	t.snap.Revision++
	t.snap.CanCancel = true
	t.snap.CanPause = false
	t.snap.CanResume = recoveryUsable(t.recovery)
	t.snap.RestartRecoverySupported = t.snap.CanResume && t.snap.HistoryPersisted
	t.snap.ByteResumeSupported = t.snap.CanResume
	t.snap.ErrorCode = ""
	t.snap.ErrorMessage = ""
	t.snap.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	t.cancel = nil
	snap, recovery := t.snap, t.recovery
	t.mu.Unlock()
	t.save(snap, recovery)
}

func (t *taskRecord) recoverAttempt(attemptID string, err error) {
	t.mu.Lock()
	if isTerminal(t.snap.State) || t.snap.AttemptID != attemptID {
		t.mu.Unlock()
		return
	}
	t.snap.State = "recovering"
	t.snap.Phase = "connection_interrupted"
	t.snap.Revision++
	t.snap.CanCancel = true
	t.snap.CanPause = false
	t.snap.CanResume = recoveryUsable(t.recovery)
	t.snap.RestartRecoverySupported = t.snap.CanResume && t.snap.HistoryPersisted
	t.snap.ByteResumeSupported = t.snap.CanResume
	t.snap.ErrorCode = string(protocol.ConnectionInterrupted)
	t.snap.ErrorMessage = userErrorForCode(protocol.ConnectionInterrupted)
	t.snap.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	t.cancel = nil
	snap, recovery := t.snap, t.recovery
	t.mu.Unlock()
	t.save(snap, recovery)
}

func recoveryUsable(recovery taskRecovery) bool {
	if recovery.Version != 1 || recovery.Direction == "" || recovery.PeerID == "" || recovery.PeerFingerprint != recovery.PeerID || len(recovery.TransferID) != 32 || len(recovery.ManifestDigest) != 64 || recovery.ChunkSize < 64<<10 || recovery.ChunkSize > 8<<20 || recovery.TotalBytes < 0 {
		return false
	}
	if recovery.Direction == "send" {
		return len(recovery.SourcePaths) > 0
	}
	return recovery.Direction == "receive" && strings.TrimSpace(recovery.TargetDirectory) != ""
}

func (s *Service) validateRecoveryPeer(recovery taskRecovery) error {
	if err := s.checkPeerAllowed(recovery.PeerID); err != nil {
		return err
	}
	peers, err := identity.LoadTrust(s.cfg.DataDir)
	if err != nil {
		return protocol.Wrap(protocol.ResumeMismatch, "cannot validate persisted peer fingerprint", err)
	}
	for _, peer := range peers {
		if peer.ID != recovery.PeerID {
			continue
		}
		if identity.DeviceID(peer.PublicKey) != recovery.PeerFingerprint {
			return protocol.Fail(protocol.ResumeMismatch, "persisted peer fingerprint changed")
		}
		return nil
	}
	return protocol.Fail(protocol.ResumeMismatch, "persisted peer fingerprint is no longer trusted")
}

func (t *taskRecord) save(snap TaskSnapshot, recovery taskRecovery) {
	if t.persist != nil {
		if err := t.persist(snap, recovery); err != nil {
			t.mu.Lock()
			t.snap.HistoryPersisted = false
			t.mu.Unlock()
		}
	}
	if t.changed != nil {
		t.changed(t.snapshot())
	}
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
		return protocol.Wrap(protocol.FileConflict, "目标文件已存在且不会覆盖，请选择空目录后重试。", err)
	case errors.Is(err, syscall.ENOSPC):
		return protocol.Wrap(protocol.DiskFull, "接收磁盘空间不足，请清理空间后重试。", err)
	case errors.Is(err, fs.ErrPermission):
		return protocol.Wrap(protocol.PermissionDenied, "接收目录没有写入权限，请选择可写目录。", err)
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
		return "设备尚未完成配对，请在设备页输入新的配对码。"
	case protocol.AuthenticationFailed:
		return "设备认证失败，请重新生成配对码完成配对。"
	case protocol.VersionIncompatible:
		return "双方版本或传输能力不兼容，请升级到兼容版本。"
	case protocol.NoCandidates, protocol.NoViableCandidate, protocol.CheckTimeout, protocol.ICEFailed:
		return "未能建立直连，请检查绑定地址、UDP 防火墙和网络切换。"
	case protocol.QUICHandshakeTimeout, protocol.QUICHandshakeFailed:
		return "安全直连握手失败，请确认双方在线且版本兼容；如设备密钥已变化请重新配对。"
	case protocol.DirectFailed:
		return "直连或传输未完成，请检查设备在线、网络和接收目录后重试。"
	case protocol.ConnectionInterrupted:
		return "连接已中断，已验证数据仍保留；请让双方确认后恢复。"
	case protocol.ResumeMismatch:
		return "恢复身份不匹配，已停止传输；请核对源文件、目标目录和已配对设备。"
	case protocol.SessionConflict:
		return "双方同时发起连接，LinkSend 正在合并为同一会话；如未继续请重试。"
	case protocol.ReceiveRejected:
		return "接收方拒绝了本次传输。"
	case protocol.SourceChanged:
		return "源文件在传输过程中发生变化，请重新选择文件后重试。"
	case protocol.IntegrityFailed:
		return "文件完整性校验失败，请重试并检查磁盘或网络。"
	case protocol.DiskFull:
		return "接收磁盘空间不足，请清理空间后重试。"
	case protocol.PermissionDenied:
		return "接收目录没有写入权限，请选择可写目录。"
	case protocol.FileConflict:
		return "目标文件已存在且不会覆盖，请选择空目录后重试。"
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
	if err := s.checkPeerAllowed(peerID); err != nil {
		return TaskSnapshot{}, err
	}
	if s.tasks.hasActive() {
		return TaskSnapshot{}, errors.New("BUSY: another transfer task is active")
	}
	if err := s.stopInbox(false); err != nil {
		return TaskSnapshot{}, err
	}
	s.operationMu.Lock()
	defer s.operationMu.Unlock()
	if s.isClosing() {
		return TaskSnapshot{}, errors.New("APP_CLOSING")
	}
	ctx, cancel := context.WithCancel(context.Background())
	base := make([]string, len(paths))
	copy(base, paths)
	t, err := s.tasks.create(TaskSnapshot{Direction: "send", PeerID: peerID, SourceSummary: sourceSummary(paths)}, cancel)
	if err != nil {
		cancel()
		s.ensureInbox()
		return TaskSnapshot{}, err
	}
	t.peerID, t.paths, t.cfg = peerID, base, cfg
	if cfg.beforeDispatch != nil {
		if err = cfg.beforeDispatch(t.snapshot()); err != nil {
			cancel()
			t.finish("failed", err)
			s.ensureInbox()
			return TaskSnapshot{}, err
		}
	}
	attemptID := t.snapshot().AttemptID
	t.updateRecovery(attemptID, func(recovery *taskRecovery) {
		recovery.SourcePaths = append([]string(nil), base...)
		recovery.PeerID = peerID
		recovery.PeerFingerprint = peerID
	})
	cfg.onPhase = func(phase string) {
		t.updateAttemptTransient(attemptID, func(v *TaskSnapshot) {
			if v.State != "cancel_requested" {
				v.Phase = phase
			}
		})
	}
	cfg.onSession = func(sessionID, peerID string) {
		t.updateAttemptTransient(attemptID, func(v *TaskSnapshot) {
			v.SessionID = sessionID
			v.PeerID = peerID
			v.ICEGeneration = firstGeneration
		})
	}
	cfg.onEvidence = func(e DirectEvidence) {
		t.updateAttempt(attemptID, func(v *TaskSnapshot) {
			applyDirectEvidence(v, e)
		})
	}
	s.tasks.workers.Add(1)
	go func() {
		defer s.tasks.workers.Done()
		defer s.ensureInbox()
		var result DirectTransferResult
		prepared, peer, runErr := s.prepareAndConnect(ctx, peerID, base, cfg)
		if runErr == nil && cfg.expectedSourceDigest != "" && queueSourceDigest(prepared.Manifest) != cfg.expectedSourceDigest {
			_ = prepared.Close()
			_ = peer.Close()
			runErr = transfer.ErrChanged
		}
		if runErr == nil {
			defer prepared.Close()
			sentChunks := make(map[uint32][]bool)
			for _, entry := range prepared.Manifest.Files {
				if entry.Type == "file" {
					sentChunks[entry.ID] = make([]bool, len(entry.Chunks))
				}
			}
			t.updateRecordAttempt(attemptID, func(v *TaskSnapshot, recovery *taskRecovery) {
				v.TransferID = prepared.Manifest.TransferID
				v.ManifestDigest = prepared.Manifest.Digest()
				v.ChunkSize = prepared.Manifest.ChunkSize
				v.FileCount = len(prepared.Manifest.Files)
				v.ManifestSummary = manifestSummary(prepared.Manifest)
				v.CanPause = true
				v.RestartRecoverySupported = v.HistoryPersisted
				v.ByteResumeSupported = true
				n := prepared.Manifest.TotalBytes()
				v.TotalBytes = &n
				recovery.TransferID = prepared.Manifest.TransferID
				recovery.ManifestDigest = prepared.Manifest.Digest()
				recovery.ChunkSize = prepared.Manifest.ChunkSize
				recovery.TotalBytes = n
				recovery.FileCount = len(prepared.Manifest.Files)
				recovery.SentChunks = sentChunks
			})
			if indexErr := s.IndexInboxManifest(ctx, t.snapshot().ID, prepared.Manifest); indexErr != nil {
				handleTaskRunError(t, attemptID, ctx, errors.Join(indexErr, peer.Close()))
				return
			}
			result, runErr = s.sendPreparedOverPeer(ctx, peer, prepared, cfg, transfer.SendHooks{
				Progress:       func(p transfer.Progress) { t.progress(attemptID, p) },
				PreviouslySent: t.wasChunkSent,
				ChunkSent: func(sent transfer.ChunkTransmission) {
					t.recordChunkSent(attemptID, sent)
					if cfg.onChunkSent != nil {
						cfg.onChunkSent(sent)
					}
				},
			})
			if runErr == nil {
				runErr = s.closePeerAfterTransfer(peer)
			} else {
				runErr = errors.Join(runErr, peer.Close())
			}
		}
		if runErr != nil {
			handleTaskRunError(t, attemptID, ctx, runErr)
			return
		}
		t.updateAttempt(attemptID, func(v *TaskSnapshot) {
			v.TransferID = result.Transfer.TransferID
			v.SessionID = result.Evidence.SessionID
			v.Phase = "completed"
			v.ProcessedBytes = result.Transfer.Bytes
			v.VerifiedBytes = result.Transfer.Bytes
			v.CommittedBytes = result.Transfer.Bytes
			v.BilateralConfirmed = true
			n := result.Transfer.Bytes
			v.TotalBytes = &n
		})
		t.finishAttempt(attemptID, "completed", nil)
	}()
	return t.snapshot(), nil
}

func (s *Service) StartReceive(expectedPeerID, directory string, cfg DirectConfig) (TaskSnapshot, error) {
	if strings.TrimSpace(directory) == "" {
		return TaskSnapshot{}, errors.New("INVALID_ARGUMENT: receive directory is required")
	}
	if s.tasks.hasActive() {
		return TaskSnapshot{}, errors.New("BUSY: another transfer task is active")
	}
	if err := s.stopInbox(false); err != nil {
		return TaskSnapshot{}, err
	}
	s.operationMu.Lock()
	defer s.operationMu.Unlock()
	if s.isClosing() {
		return TaskSnapshot{}, errors.New("APP_CLOSING")
	}
	ctx, cancel := context.WithCancel(context.Background())
	t, err := s.tasks.create(TaskSnapshot{Direction: "receive", PeerID: expectedPeerID, TargetDirectory: filepath.Clean(directory)}, cancel)
	if err != nil {
		cancel()
		s.ensureInbox()
		return TaskSnapshot{}, err
	}
	t.cfg = cfg
	t.decision = make(chan bool, 1)
	attemptID := t.snapshot().AttemptID
	cfg.onPhase = func(phase string) {
		t.updateAttemptTransient(attemptID, func(v *TaskSnapshot) {
			if v.State != "cancel_requested" {
				v.Phase = phase
			}
		})
	}
	cfg.onSession = func(sessionID, peerID string) {
		t.updateAttemptTransient(attemptID, func(v *TaskSnapshot) {
			v.SessionID = sessionID
			v.PeerID = peerID
			v.ICEGeneration = firstGeneration
		})
	}
	cfg.onEvidence = func(e DirectEvidence) {
		t.updateAttempt(attemptID, func(v *TaskSnapshot) {
			applyDirectEvidence(v, e)
		})
	}
	s.tasks.workers.Add(1)
	go func() {
		defer s.tasks.workers.Done()
		defer s.ensureInbox()
		var lastReceived int64
		result, runErr := s.ReceiveOnceDetailed(ctx, expectedPeerID, directory, cfg, func(m transfer.Manifest) bool {
			n := m.TotalBytes()
			t.updateRecordAttempt(attemptID, func(v *TaskSnapshot, recovery *taskRecovery) {
				v.State = "awaiting_acceptance"
				v.Phase = "awaiting_acceptance"
				v.TotalBytes = &n
				v.FileCount = len(m.Files)
				v.ManifestSummary = manifestSummary(m)
				v.TransferID = m.TransferID
				v.ManifestDigest = m.Digest()
				v.ChunkSize = m.ChunkSize
				v.CanCancel = true
				v.CanPause = false
				v.RestartRecoverySupported = v.HistoryPersisted
				v.ByteResumeSupported = true
				recovery.Direction = "receive"
				recovery.PeerID = v.PeerID
				recovery.PeerFingerprint = v.PeerID
				recovery.TargetDirectory = directory
				recovery.TransferID = m.TransferID
				recovery.ManifestDigest = m.Digest()
				recovery.ChunkSize = m.ChunkSize
				recovery.TotalBytes = n
				recovery.FileCount = len(m.Files)
			})
			select {
			case accepted := <-t.decision:
				return accepted
			case <-ctx.Done():
				return false
			}
		}, func(p transfer.Progress) {
			if p.Received >= lastReceived {
				t.recordReceived(attemptID, p.Received-lastReceived)
				lastReceived = p.Received
			}
			t.progress(attemptID, p)
		})
		if runErr != nil {
			handleTaskRunError(t, attemptID, ctx, runErr)
			return
		}
		t.updateAttempt(attemptID, func(v *TaskSnapshot) {
			v.TransferID = result.Transfer.TransferID
			v.SessionID = result.Evidence.SessionID
			v.Phase = "completed"
			v.ProcessedBytes = result.Transfer.Bytes
			v.VerifiedBytes = result.Transfer.Bytes
			v.ReceivedBytes = result.Transfer.Bytes
			v.CommittedBytes = result.Transfer.Bytes
			v.CommittedFiles = v.FileCount
			v.BilateralConfirmed = true
			n := result.Transfer.Bytes
			v.TotalBytes = &n
		})
		t.finishAttempt(attemptID, "completed", nil)
	}()
	return t.snapshot(), nil
}

func handleTaskRunError(t *taskRecord, attemptID string, ctx context.Context, runErr error) {
	runErr = classifyTaskError(runErr)
	snap := t.snapshot()
	if errors.Is(ctx.Err(), context.Canceled) {
		if snap.State == "shutdown_requested" {
			shutdownTask(t, attemptID, runErr)
			return
		}
		if snap.State == "pause_requested" {
			t.pauseAttempt(attemptID)
			return
		}
		t.finishAttempt(attemptID, "cancelled", protocol.Wrap(protocol.Cancelled, "task cancellation confirmed", runErr))
		return
	}
	if errors.Is(runErr, transfer.ErrRejected) {
		t.finishAttempt(attemptID, "rejected", runErr)
		return
	}
	code := protocol.ErrorCode(runErr)
	if recoveryUsable(t.recoverySnapshot()) && (snap.State == "awaiting_acceptance" || snap.State == "transferring") {
		switch code {
		case protocol.SignalingUnreachable, protocol.SignalingTimeout, protocol.CandidateTimeout,
			protocol.NoCandidates, protocol.NoViableCandidate, protocol.CheckTimeout, protocol.ICEFailed,
			protocol.QUICHandshakeTimeout, protocol.QUICHandshakeFailed, protocol.DirectFailed:
			t.recoverAttempt(attemptID, runErr)
			return
		}
	}
	t.finishAttempt(attemptID, "failed", runErr)
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
	done, workErr := s.beginProfileWork()
	if workErr != nil {
		return workErr
	}
	defer done()
	s.tasks.mu.RLock()
	t := s.tasks.tasks[id]
	s.tasks.mu.RUnlock()
	if t == nil {
		return errors.New("TASK_NOT_FOUND")
	}
	t.mu.Lock()
	if isTerminal(t.snap.State) {
		t.mu.Unlock()
		return errors.New("TASK_TERMINAL: task is already finished")
	}
	if t.snap.State == "cancel_requested" {
		t.mu.Unlock()
		return errors.New("TASK_CANCEL_ALREADY_REQUESTED: cancellation is still pending")
	}
	if t.cancel == nil {
		t.mu.Unlock()
		t.finish("cancelled", protocol.Fail(protocol.Cancelled, "task cancelled"))
		return nil
	}
	t.snap.State = "cancel_requested"
	t.snap.Phase = "cancelling"
	t.snap.CanCancel = false
	t.snap.CanPause = false
	t.snap.Revision++
	t.snap.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	cancel := t.cancel
	snap, recovery := t.snap, t.recovery
	t.mu.Unlock()
	t.save(snap, recovery)
	cancel()
	return nil
}

func (s *Service) PauseTask(id string) error {
	done, workErr := s.beginProfileWork()
	if workErr != nil {
		return workErr
	}
	defer done()
	s.tasks.mu.RLock()
	t := s.tasks.tasks[id]
	s.tasks.mu.RUnlock()
	if t == nil {
		return errors.New("TASK_NOT_FOUND")
	}
	t.mu.Lock()
	if !t.snap.CanPause || t.cancel == nil || (t.snap.State != "awaiting_acceptance" && t.snap.State != "transferring") {
		t.mu.Unlock()
		return errors.New("TASK_NOT_PAUSABLE")
	}
	t.snap.State = "pause_requested"
	t.snap.Phase = "pausing"
	t.snap.CanPause = false
	t.snap.CanCancel = false
	t.snap.Revision++
	t.snap.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	cancel := t.cancel
	snap, recovery := t.snap, t.recovery
	t.mu.Unlock()
	t.save(snap, recovery)
	cancel()
	return nil
}

func (s *Service) ResumeTask(id string, cfg DirectConfig) (TaskSnapshot, error) {
	if err := s.stopInbox(false); err != nil {
		return TaskSnapshot{}, err
	}
	s.operationMu.Lock()
	defer s.operationMu.Unlock()
	if s.isClosing() {
		return TaskSnapshot{}, errors.New("APP_CLOSING")
	}
	s.tasks.mu.RLock()
	t := s.tasks.tasks[id]
	for otherID, other := range s.tasks.tasks {
		if otherID == id {
			continue
		}
		other.mu.RLock()
		running := other.cancel != nil
		other.mu.RUnlock()
		if running {
			s.tasks.mu.RUnlock()
			s.ensureInbox()
			return TaskSnapshot{}, errors.New("BUSY: another transfer task is active")
		}
	}
	s.tasks.mu.RUnlock()
	if t == nil {
		s.ensureInbox()
		return TaskSnapshot{}, errors.New("TASK_NOT_FOUND")
	}
	t.mu.Lock()
	if t.cancel != nil || !t.snap.CanResume || !recoveryUsable(t.recovery) {
		t.mu.Unlock()
		s.ensureInbox()
		return TaskSnapshot{}, errors.New("TASK_NOT_RESUMABLE")
	}
	if err := s.validateRecoveryPeer(t.recovery); err != nil {
		t.mu.Unlock()
		s.ensureInbox()
		return TaskSnapshot{}, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	attemptID := protocol.RandomID()
	t.snap.AttemptID = attemptID
	t.snap.SessionID = ""
	t.snap.ICEGeneration = 0
	clearDirectEvidence(&t.snap)
	t.snap.State = "recovering"
	t.snap.Phase = "recovering"
	t.snap.CanCancel = true
	t.snap.CanPause = false
	t.snap.CanResume = false
	t.snap.CanRetry = false
	t.snap.ErrorCode = ""
	t.snap.ErrorMessage = ""
	t.snap.EndedAt = ""
	t.snap.RateBytesPerSecond = nil
	t.snap.Revision++
	t.snap.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	t.cancel = cancel
	t.cfg = cfg
	if t.recovery.Direction == "receive" {
		t.decision = make(chan bool, 1)
	}
	snap, recovery := t.snap, t.recovery
	t.mu.Unlock()
	t.save(snap, recovery)
	if cfg.beforeDispatch != nil {
		if err := cfg.beforeDispatch(snap); err != nil {
			cancel()
			t.recoverAttempt(attemptID, err)
			return TaskSnapshot{}, err
		}
	}
	s.tasks.workers.Add(1)
	if recovery.Direction == "send" {
		go s.runResumeSend(ctx, t, attemptID, recovery, cfg)
	} else {
		go s.runResumeReceive(ctx, t, attemptID, recovery, cfg)
	}
	return t.snapshot(), nil
}

func taskAttemptConfig(t *taskRecord, attemptID string, cfg DirectConfig) DirectConfig {
	cfg.onPhase = func(phase string) {
		t.updateAttemptTransient(attemptID, func(v *TaskSnapshot) {
			if v.State != "cancel_requested" && v.State != "pause_requested" {
				v.Phase = phase
			}
		})
	}
	cfg.onSession = func(sessionID, peerID string) {
		t.updateAttemptTransient(attemptID, func(v *TaskSnapshot) {
			v.SessionID = sessionID
			v.PeerID = peerID
			v.ICEGeneration = firstGeneration
		})
	}
	cfg.onEvidence = func(e DirectEvidence) {
		t.updateAttempt(attemptID, func(v *TaskSnapshot) {
			applyDirectEvidence(v, e)
		})
	}
	return cfg
}

func (s *Service) runResumeSend(ctx context.Context, t *taskRecord, attemptID string, recovery taskRecovery, cfg DirectConfig) {
	defer s.tasks.workers.Done()
	defer s.ensureInbox()
	prepared, err := transfer.PrepareForResume(ctx, recovery.SourcePaths, recovery.ChunkSize, recovery.TransferID)
	if prepared != nil {
		defer prepared.Close()
	}
	if err == nil && prepared.Manifest.Digest() != recovery.ManifestDigest {
		err = protocol.Fail(protocol.SourceChanged, "persisted source manifest changed")
	}
	if err != nil {
		handleTaskRunError(t, attemptID, ctx, err)
		return
	}
	t.updateAttempt(attemptID, func(v *TaskSnapshot) {
		v.CanPause = true
		v.TransferID = recovery.TransferID
		v.ManifestDigest = recovery.ManifestDigest
		v.ChunkSize = recovery.ChunkSize
	})
	if err = s.IndexInboxManifest(ctx, t.snapshot().ID, prepared.Manifest); err != nil {
		handleTaskRunError(t, attemptID, ctx, err)
		return
	}
	cfg = taskAttemptConfig(t, attemptID, cfg)
	result, err := s.SendPreparedWithHooksDetailed(ctx, recovery.PeerID, prepared, cfg, transfer.SendHooks{
		Progress:       func(p transfer.Progress) { t.progress(attemptID, p) },
		PreviouslySent: t.wasChunkSent,
		ChunkSent: func(sent transfer.ChunkTransmission) {
			t.recordChunkSent(attemptID, sent)
			if cfg.onChunkSent != nil {
				cfg.onChunkSent(sent)
			}
		},
	})
	if err != nil {
		handleTaskRunError(t, attemptID, ctx, err)
		return
	}
	t.updateRecordAttempt(attemptID, func(v *TaskSnapshot, saved *taskRecovery) {
		v.TransferID = result.Transfer.TransferID
		v.SessionID = result.Evidence.SessionID
		v.Phase = "completed"
		v.ProcessedBytes = result.Transfer.Bytes
		v.VerifiedBytes = result.Transfer.Bytes
		v.CommittedBytes = result.Transfer.Bytes
		v.BilateralConfirmed = true
		saved.LogicalCompleted = result.Transfer.Bytes
		n := result.Transfer.Bytes
		v.TotalBytes = &n
	})
	t.finishAttempt(attemptID, "completed", nil)
}

func (s *Service) runResumeReceive(ctx context.Context, t *taskRecord, attemptID string, recovery taskRecovery, cfg DirectConfig) {
	defer s.tasks.workers.Done()
	defer s.ensureInbox()
	cfg = taskAttemptConfig(t, attemptID, cfg)
	var lastReceived int64
	var resumeMismatch error
	result, err := s.ReceiveOnceDetailed(ctx, recovery.PeerID, recovery.TargetDirectory, cfg, func(manifest transfer.Manifest) bool {
		if manifest.TransferID != recovery.TransferID || manifest.Digest() != recovery.ManifestDigest || manifest.ChunkSize != recovery.ChunkSize || manifest.TotalBytes() != recovery.TotalBytes || len(manifest.Files) != recovery.FileCount {
			resumeMismatch = protocol.Fail(protocol.ResumeMismatch, "resume manifest does not match persisted recovery identity")
			return false
		}
		// ResumeTask is the explicit user confirmation. No body transfer is
		// accepted before this method is invoked and the identity matches.
		return true
	}, func(p transfer.Progress) {
		if p.Received >= lastReceived {
			t.recordReceived(attemptID, p.Received-lastReceived)
			lastReceived = p.Received
		}
		t.progress(attemptID, p)
	})
	if resumeMismatch != nil {
		err = resumeMismatch
	}
	if err != nil {
		handleTaskRunError(t, attemptID, ctx, err)
		return
	}
	t.updateRecordAttempt(attemptID, func(v *TaskSnapshot, saved *taskRecovery) {
		v.TransferID = result.Transfer.TransferID
		v.SessionID = result.Evidence.SessionID
		v.Phase = "completed"
		v.ProcessedBytes = result.Transfer.Bytes
		v.VerifiedBytes = result.Transfer.Bytes
		v.CommittedBytes = result.Transfer.Bytes
		v.CommittedFiles = recovery.FileCount
		v.BilateralConfirmed = true
		saved.LogicalCompleted = result.Transfer.Bytes
		n := result.Transfer.Bytes
		v.TotalBytes = &n
	})
	t.finishAttempt(attemptID, "completed", nil)
}
func (s *Service) AcceptTask(id string) error { return s.decideTask(id, true) }
func (s *Service) AcceptTaskAlways(id string) error {
	s.tasks.mu.RLock()
	t := s.tasks.tasks[id]
	s.tasks.mu.RUnlock()
	if t == nil {
		return errors.New("TASK_NOT_FOUND")
	}
	snap := t.snapshot()
	if snap.State != "awaiting_acceptance" || snap.Direction != "receive" || snap.PeerID == "" {
		return errors.New("TASK_NOT_AWAITING_ACCEPTANCE")
	}
	if err := s.SetAlwaysAccept(snap.PeerID, true); err != nil {
		return err
	}
	return s.decideTask(id, true)
}
func (s *Service) RejectTask(id string) error { return s.decideTask(id, false) }
func (s *Service) decideTask(id string, accepted bool) error {
	done, workErr := s.beginProfileWork()
	if workErr != nil {
		return workErr
	}
	defer done()
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
	if accepted {
		if err := s.checkPeerAllowed(snap.PeerID); err != nil {
			return err
		}
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

// Shutdown preserves resumable tasks and joins all application workers.
func (s *Service) Shutdown() {
	_ = s.ShutdownContext(context.Background())
}
