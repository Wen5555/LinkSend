// Package app exposes the shared application service used by the CLI and Wails.
// It owns profile and trust policy, while connectivity and transfer remain
// separate packages. No file bytes pass through this service boundary.
package app

import (
	"context"
	"encoding/hex"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/Wen5555/LinkSend/internal/identity"
	"github.com/Wen5555/LinkSend/internal/protocol"
	"github.com/Wen5555/LinkSend/internal/signaling"
)

var ErrNotImplemented = errors.New("NOT_IMPLEMENTED: requested application operation is not available")

type Config struct {
	DataDir               string
	ServerURL             string
	Name                  string
	AllowInsecureLoopback bool
	Identity              *identity.Identity
}

type Service struct {
	cfg      Config
	identity *identity.Identity
	signal   *signaling.Client
	mu       sync.RWMutex
	tasks    *taskManager
}

type IdentityInfo struct {
	ID        string `json:"id"`
	PublicKey string `json:"public_key_hex"`
	DataDir   string `json:"data_dir"`
}

// DiagnosticIdentity intentionally omits local filesystem locations. Identity()
// is an explicit local profile view; diagnostics are safe to export by default.
type DiagnosticIdentity struct {
	ID        string `json:"id"`
	PublicKey string `json:"public_key_hex"`
}

type DeviceInfo struct {
	ID        string `json:"id"`
	GroupID   string `json:"group_id"`
	Name      string `json:"name"`
	PublicKey string `json:"public_key_hex"`
	Admin     bool   `json:"admin"`
	Online    bool   `json:"online"`
	Trusted   bool   `json:"trusted"`
}

type InvitationInfo struct {
	Token     string `json:"token"`
	ExpiresAt string `json:"expires_at"`
}

type MembershipStatus struct {
	State   string `json:"state"` // pending, member, not_member, auth_failed, unavailable
	Role    string `json:"role"`  // unknown, member, admin
	Message string `json:"message,omitempty"`
}

type Diagnostics struct {
	HistoryPersisted         bool                  `json:"history_persisted"`
	RestartRecoverySupported bool                  `json:"restart_recovery_supported"`
	ByteResumeSupported      bool                  `json:"byte_resume_supported"`
	HistoryError             string                `json:"history_error,omitempty"`
	Version                  string                `json:"version"`
	Platform                 string                `json:"platform"`
	Relay                    bool                  `json:"relay"`
	Identity                 DiagnosticIdentity    `json:"identity"`
	ServerURL                string                `json:"server_url,omitempty"`
	ServerHealth             string                `json:"server_health"`
	Capabilities             protocol.Capabilities `json:"capabilities"`
	TrustedPeers             int                   `json:"trusted_peers"`
	GeneratedAt              string                `json:"generated_at"`
	HealthFailure            string                `json:"health_failure,omitempty"`
}

func New(cfg Config) (*Service, error) {
	if strings.TrimSpace(cfg.DataDir) == "" {
		return nil, errors.New("data directory is required")
	}
	if err := os.MkdirAll(cfg.DataDir, 0700); err != nil {
		return nil, err
	}
	id := cfg.Identity
	var err error
	if id == nil {
		id, err = identity.LoadOrCreate(cfg.DataDir)
		if err != nil {
			return nil, err
		}
	}
	s := &Service{cfg: cfg, identity: id, tasks: newTaskManager()}
	s.tasks.configureHistory(filepath.Join(cfg.DataDir, "task-history.sqlite"))
	if strings.TrimSpace(cfg.ServerURL) != "" {
		s.signal, err = signaling.New(signaling.Config{ServerURL: cfg.ServerURL, Identity: id, AllowInsecureLoopback: cfg.AllowInsecureLoopback})
		if err != nil {
			return nil, err
		}
	}
	return s, nil
}

func (s *Service) client() (*signaling.Client, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.signal == nil {
		return nil, errors.New("SIGNALING_UNREACHABLE: server URL is not configured")
	}
	return s.signal, nil
}

func (s *Service) Identity() IdentityInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return IdentityInfo{ID: s.identity.ID(), PublicKey: hex.EncodeToString(s.identity.PublicKey()), DataDir: s.cfg.DataDir}
}

func (s *Service) Bootstrap(ctx context.Context, token, name string) (DeviceInfo, error) {
	c, err := s.client()
	if err != nil {
		return DeviceInfo{}, err
	}
	d, err := c.Bootstrap(ctx, token, s.name(name))
	return s.device(d), err
}

func (s *Service) Join(ctx context.Context, token, name string) (DeviceInfo, error) {
	c, err := s.client()
	if err != nil {
		return DeviceInfo{}, err
	}
	d, err := c.Join(ctx, token, s.name(name))
	return s.device(d), err
}

func (s *Service) CreateInvitation(ctx context.Context) (InvitationInfo, error) {
	c, err := s.client()
	if err != nil {
		return InvitationInfo{}, err
	}
	i, err := c.CreateInvitation(ctx)
	return InvitationInfo{Token: i.Token, ExpiresAt: i.ExpiresAt.UTC().Format(time.RFC3339)}, err
}

func (s *Service) Devices(ctx context.Context) ([]DeviceInfo, error) {
	c, err := s.client()
	if err != nil {
		return nil, err
	}
	devices, err := c.Devices(ctx)
	if err != nil {
		return nil, err
	}
	peers, err := identity.LoadTrust(s.cfg.DataDir)
	if err != nil {
		return nil, err
	}
	trusted := make(map[string]bool, len(peers))
	for _, p := range peers {
		trusted[p.ID] = true
	}
	out := make([]DeviceInfo, 0, len(devices))
	for _, d := range devices {
		item := s.device(d)
		item.Trusted = trusted[d.ID] || d.ID == s.identity.ID()
		out = append(out, item)
	}
	return out, nil
}

// Membership reports only evidence returned by the authenticated server. A
// merged 401 remains auth_failed; it is never guessed to mean not_member.
func (s *Service) Membership(ctx context.Context) MembershipStatus {
	c, err := s.client()
	if err != nil {
		return MembershipStatus{State: "unavailable", Role: "unknown", Message: "服务地址尚未配置或当前不可达"}
	}
	devices, err := c.Devices(ctx)
	if err != nil {
		return membershipFailure(err)
	}
	for _, d := range devices {
		if d.ID == s.identity.ID() {
			role := "member"
			if d.Admin {
				role = "admin"
			}
			return MembershipStatus{State: "member", Role: role}
		}
	}
	return MembershipStatus{State: "not_member", Role: "unknown", Message: "当前身份不在该设备组中，请使用管理员邀请加入"}
}

func membershipFailure(err error) MembershipStatus {
	switch protocol.ErrorCode(err) {
	case protocol.SignalingUnreachable, protocol.SignalingTimeout:
		return MembershipStatus{State: "unavailable", Role: "unknown", Message: "暂时无法连接信令服务，请检查服务地址和网络。"}
	case protocol.VersionIncompatible:
		return MembershipStatus{State: "auth_failed", Role: "unknown", Message: "服务版本或能力不兼容，请升级后重试。"}
	case protocol.AuthenticationFailed:
		return MembershipStatus{State: "auth_failed", Role: "unknown", Message: "当前身份未通过该服务的成员或签名验证，请先使用一次性邀请加入。"}
	default:
		return MembershipStatus{State: "unavailable", Role: "unknown", Message: "设备组状态暂时无法读取，请检查服务和网络。"}
	}
}

func (s *Service) Trust(ctx context.Context, deviceID, fingerprint string) error {
	if len(deviceID) != 64 || deviceID != fingerprint {
		return errors.New("UNPAIRED: full device fingerprint confirmation is required")
	}
	c, err := s.client()
	if err != nil {
		return err
	}
	devices, err := c.Devices(ctx)
	if err != nil {
		return err
	}
	for _, d := range devices {
		if d.ID == deviceID {
			return identity.TrustPeer(s.cfg.DataDir, identity.TrustedPeer{ID: d.ID, Name: d.Name, PublicKey: d.PublicKey}, fingerprint)
		}
	}
	return errors.New("UNPAIRED: device is not a current member of the server group")
}

func (s *Service) Revoke(ctx context.Context, deviceID string) error {
	c, err := s.client()
	if err != nil {
		return err
	}
	return c.Revoke(ctx, deviceID)
}

func (s *Service) Health(ctx context.Context) (protocol.Capabilities, error) {
	c, err := s.client()
	if err != nil {
		return protocol.Capabilities{}, err
	}
	return c.Health(ctx)
}

func (s *Service) Diagnostics(ctx context.Context) Diagnostics {
	info := s.Identity()
	peers, _ := identity.LoadTrust(s.cfg.DataDir)
	d := Diagnostics{Version: protocol.ProductVersion, Platform: runtime.GOOS + "/" + runtime.GOARCH, Relay: false, Identity: DiagnosticIdentity{ID: info.ID, PublicKey: info.PublicKey}, ServerURL: redactURL(s.cfg.ServerURL), ServerHealth: "not_configured", Capabilities: protocol.Supported(), TrustedPeers: len(peers), GeneratedAt: time.Now().UTC().Format(time.RFC3339)}
	d.HistoryPersisted = s.tasks.historyAvailable()
	d.RestartRecoverySupported = d.HistoryPersisted
	d.ByteResumeSupported = true
	if s.tasks.historyError() != nil {
		d.HistoryError = "TASK_STORE_UNAVAILABLE"
	}
	if strings.TrimSpace(s.cfg.ServerURL) == "" {
		return d
	}
	capabilities, err := s.Health(ctx)
	if err != nil {
		d.ServerHealth = "error"
		d.HealthFailure = safeHealthFailure(err)
		return d
	}
	d.ServerHealth = "ok"
	d.Capabilities = capabilities
	return d
}

func safeHealthFailure(err error) string {
	if err == nil {
		return ""
	}
	code := protocol.ErrorCode(err)
	if code == protocol.DirectFailed {
		return "server health check failed"
	}
	return string(code)
}

func (s *Service) Send(context.Context, []string, string) error { return ErrNotImplemented }
func (s *Service) Receive(context.Context, string) error        { return ErrNotImplemented }
func (s *Service) Accept(context.Context, string) error         { return ErrNotImplemented }
func (s *Service) Pause(context.Context, string) error          { return ErrNotImplemented }
func (s *Service) Resume(context.Context, string) error         { return ErrNotImplemented }
func (s *Service) Cancel(context.Context, string) error         { return ErrNotImplemented }

func (s *Service) name(value string) string {
	if strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	if strings.TrimSpace(s.cfg.Name) != "" {
		return strings.TrimSpace(s.cfg.Name)
	}
	return "LinkSend device"
}

func (s *Service) device(d signaling.Device) DeviceInfo {
	return DeviceInfo{ID: d.ID, GroupID: d.GroupID, Name: d.Name, PublicKey: hex.EncodeToString(d.PublicKey), Admin: d.Admin, Online: d.Online}
}

func redactURL(raw string) string {
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "<invalid>"
	}
	u.User = nil
	u.RawQuery = ""
	u.Fragment = ""
	return u.String()
}

// DataPath is kept in one place so CLI and desktop profiles do not drift.
func DataPath(base string) string { return filepath.Clean(base) }
