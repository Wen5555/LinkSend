package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/Wen5555/LinkSend/internal/protocol"
)

// Export is an allowlist projection, never a scrubbed serialization of private
// service/task/recovery/cache records. Unknown strings become fixed literals.
type DiagnosticGuidance struct {
	Code     string `json:"code"`
	Stage    string `json:"stage"`
	Reason   string `json:"reason"`
	NextStep string `json:"next_step"`
	Action   string `json:"action"`
}

type DiagnosticTask struct {
	ProductVersion string             `json:"product_version"`
	Label          string             `json:"label"`
	Direction      string             `json:"direction"`
	State          string             `json:"state"`
	Method         string             `json:"method"`
	LastAttempt    string             `json:"last_attempt,omitempty"`
	CanRetry       bool               `json:"can_retry"`
	CanResume      bool               `json:"can_resume"`
	Guidance       DiagnosticGuidance `json:"guidance"`
}

type DiagnosticReport struct {
	Version                  int              `json:"report_version"`
	ProductVersion           string           `json:"product_version"`
	Platform                 string           `json:"platform"`
	GeneratedAt              string           `json:"generated_at"`
	RelayAvailable           bool             `json:"relay_available"`
	ServerHealth             string           `json:"server_health"`
	HistoryPersisted         bool             `json:"history_persisted"`
	RestartRecoverySupported bool             `json:"restart_recovery_supported"`
	ByteResumeSupported      bool             `json:"byte_resume_supported"`
	Tasks                    []DiagnosticTask `json:"tasks"`
	TasksTruncated           bool             `json:"tasks_truncated"`
}

var diagnosticGuidance = map[protocol.Code]DiagnosticGuidance{
	protocol.LANDiscoveryUnavailable:     {Stage: "discovery", Reason: "尚未发现附近设备", NextStep: "确认两端已打开LinkSend并连接同一局域网，再在设备页刷新附近设备。", Action: "open_devices"},
	protocol.LANControlUnreachable:       {Stage: "control", Reason: "无法连接对方的局域网入口", NextStep: "确认对方应用正在运行，并在设备页刷新当前地址。", Action: "open_devices"},
	protocol.Unpaired:                    {Stage: "pairing", Reason: "这台设备尚未获准连接", NextStep: "在设备页完成双方配对与接收同意。", Action: "open_devices"},
	protocol.PeerOffline:                 {Stage: "presence", Reason: "对方当前不在线", NextStep: "等待对方打开LinkSend并确认设备页上线。", Action: "open_devices"},
	protocol.SignalingUnreachable:        {Stage: "signaling", Reason: "无法连接协调服务", NextStep: "检查网络设置中的服务地址和网络连通性；同局域网仍可使用附近设备入口。", Action: "open_network"},
	protocol.SignalingTimeout:            {Stage: "signaling", Reason: "协调服务响应超时", NextStep: "确认服务和网络恢复后，再重试原任务。", Action: "open_network"},
	protocol.CandidateTimeout:            {Stage: "candidates", Reason: "连接地址交换未完成", NextStep: "确认两端在线并检查协调服务连接，再重试原任务。", Action: "open_network"},
	protocol.NoCandidates:                {Stage: "candidates", Reason: "未取得可用的直连地址", NextStep: "检查网络设置中的可用网卡和STUN配置。", Action: "open_network"},
	protocol.NoViableCandidate:           {Stage: "ice", Reason: "当前地址无法建立直连", NextStep: "检查两端网络是否允许UDP；本版本没有文件中继。", Action: "open_network"},
	protocol.CheckTimeout:                {Stage: "ice", Reason: "直连探测超时", NextStep: "确认两端网络允许UDP后重试；受限网络可能无法直连，本版本没有文件中继。", Action: "open_network"},
	protocol.ICEFailed:                   {Stage: "ice", Reason: "直连探测失败", NextStep: "查看网络设置与对方在线状态，确认网络恢复后再重试。", Action: "open_network"},
	protocol.QUICHandshakeTimeout:        {Stage: "quic", Reason: "加密连接握手超时", NextStep: "确认两端仍在线及网络稳定后，再重试原任务。", Action: "open_network"},
	protocol.QUICHandshakeFailed:         {Stage: "authentication", Reason: "加密连接握手失败", NextStep: "核对双方设备身份和版本；身份不一致时重新确认配对。", Action: "open_devices"},
	protocol.AuthenticationFailed:        {Stage: "authentication", Reason: "设备身份验证失败", NextStep: "核对对方身份及配对状态，不要跳过身份验证。", Action: "open_devices"},
	protocol.PairingIdentityConflict:     {Stage: "authentication", Reason: "配对记录中的设备身份已变化", NextStep: "核对设备身份后，在设备页重新完成明确授权。", Action: "open_devices"},
	protocol.VersionIncompatible:         {Stage: "compatibility", Reason: "双方版本或能力不兼容", NextStep: "在软件更新中检查双方可用版本后重试。", Action: "open_updates"},
	protocol.ReceiveRejected:             {Stage: "confirmation", Reason: "对方拒绝了本次接收", NextStep: "确认对方愿意接收后，再次发起发送。", Action: "details"},
	protocol.SourceChanged:               {Stage: "source", Reason: "发送源文件已经变化", NextStep: "检查文件内容，在原任务中确认新清单后继续。", Action: "review_source"},
	protocol.DiskFull:                    {Stage: "storage", Reason: "接收磁盘空间不足", NextStep: "为接收磁盘释放其他可用空间后重试；剪贴板缓存由程序自动管理。", Action: "open_receive"},
	protocol.PermissionDenied:            {Stage: "storage", Reason: "接收位置没有写入权限", NextStep: "在接收设置中选择当前用户可写的位置。", Action: "open_receive"},
	protocol.ReceiveDirectoryUnavailable: {Stage: "storage", Reason: "接收位置当前不可用", NextStep: "确认磁盘已连接，或在接收设置中选择可用位置。", Action: "open_receive"},
	protocol.FileConflict:                {Stage: "commit", Reason: "目标位置有同名内容或已被修改", NextStep: "检查目标内容，在原任务中重新确认保存方案。", Action: "details"},
	protocol.UnsafePath:                  {Stage: "storage", Reason: "文件路径未通过安全检查", NextStep: "检查发送目录中的链接及特殊文件，不要绕过路径检查。", Action: "details"},
	protocol.IntegrityFailed:             {Stage: "verification", Reason: "文件完整性校验失败", NextStep: "检查源文件和磁盘状态，再通过原任务重试或恢复。", Action: "details"},
	protocol.ConnectionInterrupted:       {Stage: "transfer", Reason: "文件连接已中断", NextStep: "等待对方和网络恢复，在原任务中选择可用的继续操作。", Action: "details"},
	protocol.ResumeMismatch:              {Stage: "recovery", Reason: "恢复任务的身份或内容不一致", NextStep: "检查原设备和源文件；重新确认后发起新任务。", Action: "review_source"},
	protocol.ReceivePlanPersistence:      {Stage: "storage", Reason: "无法安全保存本次接收状态", NextStep: "检查接收磁盘是否可写且有可用空间，再重试原任务。", Action: "open_receive"},
	protocol.Cancelled:                   {Stage: "control", Reason: "任务已取消", NextStep: "需要再次发送时，明确创建新的发送任务。", Action: "details"},
	protocol.RateLimited:                 {Stage: "control", Reason: "请求过于频繁", NextStep: "稍后再重试，不要连续重复提交。", Action: "details"},
	protocol.SessionConflict:             {Stage: "control", Reason: "连接请求正在协调", NextStep: "等待当前尝试结束后，再检查原任务状态。", Action: "details"},
	protocol.ShareRejected:               {Stage: "authorization", Reason: "本次分享授权核验未通过", NextStep: "核对原分享状态和接收人，不要跳过身份与授权检查。", Action: "details"},
	protocol.ShareExpired:                {Stage: "authorization", Reason: "本次分享授权已到期", NextStep: "查看原分享的当前状态；到期授权不能继续连接。", Action: "details"},
	protocol.ShareTerminal:               {Stage: "authorization", Reason: "本次分享已结束", NextStep: "查看原分享的当前状态；结束的授权不能继续连接。", Action: "details"},
	protocol.ShareConflict:               {Stage: "authorization", Reason: "本次分享状态已变化", NextStep: "在原分享中核对当前状态和原操作，不要重复领取或创建任务。", Action: "details"},
}

func taskDiagnosticGuidance(snapshot TaskSnapshot) DiagnosticGuidance {
	if snapshot.State == "completed" {
		return DiagnosticGuidance{Code: "NONE", Stage: "completed", Reason: "传输已完成", Action: "none"}
	}
	if snapshot.ErrorCode == "" {
		if snapshot.Phase == "waiting_share_peer" && (snapshot.State == "preparing" || snapshot.State == "connecting" || snapshot.State == "recovering") {
			return DiagnosticGuidance{Code: "NONE", Stage: "presence", Reason: "正在等待对方进入本次分享", NextStep: "请对方在原分享中明确继续；等待仍受本次授权期限限制。", Action: "wait"}
		}
		switch snapshot.State {
		case "failed":
			return DiagnosticGuidance{Code: "UNKNOWN", Stage: "unknown", Reason: "任务失败，具体阶段尚未确认", NextStep: "查看原任务状态，必要时导出本地诊断供检查。", Action: "details"}
		case "waiting_accept", "awaiting_acceptance":
			return DiagnosticGuidance{Code: "NONE", Stage: "confirmation", Reason: "正在等待对方确认接收", NextStep: "请对方在LinkSend中确认本次保存位置。", Action: "wait"}
		case "paused":
			return DiagnosticGuidance{Code: "NONE", Stage: "paused", Reason: "任务已暂停", NextStep: "在原任务中选择继续，仍会核对身份和源文件。", Action: "details"}
		default:
			return DiagnosticGuidance{Code: "NONE", Stage: "unknown", Reason: "暂无已确认的错误", NextStep: "查看原任务的当前状态。", Action: "details"}
		}
	}
	if snapshot.Phase == "waiting_share_peer" && snapshot.ErrorCode == string(protocol.SignalingTimeout) {
		return DiagnosticGuidance{Code: snapshot.ErrorCode, Stage: "presence", Reason: "本次分享等待对方进入已超时", NextStep: "确认对方已在原分享中继续，再通过原分享手动继续；不会自动重新领取或创建任务。", Action: "details"}
	}
	if guidance, ok := diagnosticGuidance[protocol.Code(snapshot.ErrorCode)]; ok {
		guidance.Code = snapshot.ErrorCode
		return guidance
	}
	if snapshot.ErrorCode == string(protocol.DirectFailed) && snapshot.Phase == "signaling_connect" {
		return DiagnosticGuidance{Code: snapshot.ErrorCode, Stage: "signaling", Reason: "连接协调未完成", NextStep: "确认对方仍在等待本次连接，检查协调服务后，再通过原任务或原分享手动继续。", Action: "open_network"}
	}
	switch protocol.Code(snapshot.ErrorCode) {
	case protocol.LANProbeInvalid, protocol.PairingCodeInvalid, protocol.PairingCodeExpired, protocol.PairingCodeUsed,
		protocol.DirectFailed, protocol.RelayNotImplemented, protocol.InvalidMessage, protocol.Replay,
		protocol.ReceivePlanUnsupported, protocol.ReceivePlanMismatch, protocol.ReceivePlanInvalid,
		protocol.ContentUnsupported, protocol.ContentInvalid, protocol.ContentMismatch:
		return DiagnosticGuidance{Code: snapshot.ErrorCode, Stage: "unknown", Reason: "任务未完成，具体原因需要进一步检查", NextStep: "查看原任务状态，必要时导出本地诊断供检查。", Action: "details"}
	}
	return DiagnosticGuidance{Code: "UNKNOWN", Stage: "unknown", Reason: "任务遇到未分类的问题", NextStep: "查看原任务状态，必要时导出本地诊断供检查。", Action: "details"}
}

func diagnosticEnum(value string, allowed ...string) string {
	for _, candidate := range allowed {
		if value == candidate {
			return value
		}
	}
	return "unknown"
}

func safeDiagnosticTime(value string) string {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil || parsed.Year() < 2000 || parsed.Year() > 2200 {
		return ""
	}
	return parsed.UTC().Format(time.RFC3339)
}

func makeDiagnosticReport(d Diagnostics, tasks []TaskSnapshot, now time.Time) DiagnosticReport {
	report := DiagnosticReport{Version: 1, ProductVersion: protocol.ProductVersion, Platform: runtime.GOOS + "/" + runtime.GOARCH, GeneratedAt: now.UTC().Format(time.RFC3339),
		ServerHealth: diagnosticEnum(d.ServerHealth, "ok", "error", "not_configured", "unavailable"), HistoryPersisted: d.HistoryPersisted,
		RestartRecoverySupported: d.RestartRecoverySupported, ByteResumeSupported: d.ByteResumeSupported, Tasks: []DiagnosticTask{}, TasksTruncated: len(tasks) > 32}
	for i, snapshot := range tasks[:min(32, len(tasks))] {
		report.Tasks = append(report.Tasks, makeDiagnosticTask(snapshot, fmt.Sprintf("task-%d", i+1)))
	}
	return report
}

func makeDiagnosticTask(snapshot TaskSnapshot, label string) DiagnosticTask {
	return DiagnosticTask{ProductVersion: protocol.ProductVersion, Label: label, Direction: diagnosticEnum(snapshot.Direction, "send", "receive"),
		State:  diagnosticEnum(snapshot.State, "queued", "preparing", "connecting", "waiting_accept", "awaiting_acceptance", "transferring", "verifying", "completed", "failed", "paused", "cancelled", "cancel_requested", "pause_requested", "shutdown_requested", "recovering"),
		Method: diagnosticEnum(snapshot.ConnectionMethod, "lan_direct", "internet_p2p", "direct_unknown"), LastAttempt: safeDiagnosticTime(snapshot.AttemptStartedAt), CanRetry: snapshot.CanRetry, CanResume: snapshot.CanResume, Guidance: taskDiagnosticGuidance(snapshot)}
}

func (s *Service) DiagnosticReport(ctx context.Context) (DiagnosticReport, error) {
	done, err := s.beginProfileWork()
	if err != nil {
		return DiagnosticReport{}, err
	}
	defer done()
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	return makeDiagnosticReport(s.Diagnostics(ctx), s.workspaceTasks(), time.Now()), nil
}

// The caller supplies a native user-selected local .json destination. Link is
// atomic and no-replace: an existing user file is never overwritten. Only safe
// DTO bytes are ever written, and no upload/network send is performed here.
func (s *Service) ExportDiagnosticReport(ctx context.Context, destination string) error {
	done, err := s.beginProfileWork()
	if err != nil {
		return err
	}
	defer done()
	if !filepath.IsAbs(destination) || filepath.Ext(destination) != ".json" {
		return errors.New("DIAGNOSTIC_EXPORT_DESTINATION_INVALID")
	}
	report, err := s.DiagnosticReport(ctx)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	if len(data) > 128<<10 {
		return errors.New("DIAGNOSTIC_EXPORT_LIMIT")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	root, err := os.OpenRoot(filepath.Dir(destination))
	if err != nil {
		return err
	}
	defer root.Close()
	temporary := ".linksend-diagnostics-" + protocol.RandomID() + ".tmp"
	f, err := root.OpenFile(temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer root.Remove(temporary)
	if _, err = f.Write(append(data, '\n')); err != nil {
		_ = f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return root.Link(temporary, filepath.Base(destination))
}
