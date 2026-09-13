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
	"sync/atomic"
	"time"

	"github.com/Wen5555/LinkSend/internal/connectivity"
	"github.com/Wen5555/LinkSend/internal/discovery"
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
	profileLock       *profileLock
	cfg               Config
	identity          *identity.Identity
	signal            *signaling.Client
	mu                sync.RWMutex
	trustMu           sync.Mutex
	tasks             *taskManager
	inbox             inboxManager
	networkMu         sync.Mutex
	network           cachedNetworkSelection
	lanMu             sync.RWMutex
	lan               *lanRuntime
	lanError          string
	operationMu       sync.Mutex
	closing           atomic.Bool
	lanLifecycleMu    sync.Mutex
	listenerWorkers   sync.WaitGroup
	shutdownOnce      sync.Once
	shutdownDone      chan struct{}
	store             *desktopStore
	storeErr          error
	queue             queueManager
	changes           changeHub
	epoch             string
	workCtx           context.Context
	workCancel        context.CancelFunc
	content           contentManager
	lanPair           lanPairCoordinator
	recoveryMu        sync.Mutex
	lastNetworkChange time.Time
	directPoolMu      sync.Mutex
	directPool        map[string]*pooledPeerSession
}

type cachedNetworkSelection struct {
	key     string
	address connectivity.InterfaceAddress
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
	ID                 string        `json:"id"`
	GroupID            string        `json:"group_id"`
	Name               string        `json:"name"`
	PublicKey          string        `json:"public_key_hex"`
	Admin              bool          `json:"admin"`
	Online             bool          `json:"online"`
	Trusted            bool          `json:"trusted"`
	AlwaysAccept       bool          `json:"always_accept"`
	Nearby             bool          `json:"nearby"`
	Blocked            bool          `json:"blocked"`
	Profile            DeviceProfile `json:"profile"`
	Incarnation        string        `json:"incarnation,omitempty"`
	MembershipRevision uint64        `json:"membership_revision,omitempty"`
	Relationship       string        `json:"relationship"`
	ServiceState       string        `json:"service_state"`
	LANControlState    string        `json:"lan_control_state"`
	ConnectionState    string        `json:"connection_state"`
}

type InvitationInfo struct {
	Token            string `json:"token"`
	ExpiresAt        string `json:"expires_at"`
	ExpiresInSeconds int64  `json:"expires_in_seconds"`
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
	lock, err := acquireProfileLock(cfg.DataDir)
	if err != nil {
		return nil, err
	}
	keepLock := false
	defer func() {
		if !keepLock {
			_ = lock.Close()
		}
	}()
	id := cfg.Identity
	if id == nil {
		id, err = identity.LoadOrCreate(cfg.DataDir)
		if err != nil {
			return nil, err
		}
	}
	s := &Service{cfg: cfg, identity: id, tasks: newTaskManager(), profileLock: lock, directPool: make(map[string]*pooledPeerSession)}
	s.tasks.configureHistory(filepath.Join(cfg.DataDir, "task-history.sqlite"))
	keepHistory := false
	defer func() {
		if !keepHistory {
			_ = s.tasks.closeHistory()
		}
	}()
	if strings.TrimSpace(cfg.ServerURL) != "" {
		s.signal, err = signaling.New(signaling.Config{ServerURL: cfg.ServerURL, Identity: id, AllowInsecureLoopback: cfg.AllowInsecureLoopback})
		if err != nil {
			return nil, err
		}
	}
	s.initializeWorkspace()
	keepLock = true
	keepHistory = true
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

func (s *Service) InitializeMembership(ctx context.Context, name string) (DeviceInfo, error) {
	c, err := s.client()
	if err != nil {
		return DeviceInfo{}, err
	}
	d, err := c.Initialize(ctx, s.name(name))
	if err != nil {
		return DeviceInfo{}, err
	}
	return s.device(d), nil
}

func (s *Service) Join(ctx context.Context, token, name string) (DeviceInfo, error) {
	done, workErr := s.beginProfileWork()
	if workErr != nil {
		return DeviceInfo{}, workErr
	}
	defer done()
	c, err := s.client()
	if err != nil {
		return DeviceInfo{}, err
	}
	d, err := c.Join(ctx, token, s.name(name))
	if err != nil {
		return DeviceInfo{}, err
	}
	devices, err := c.Devices(ctx)
	if err != nil {
		return DeviceInfo{}, err
	}
	if err = s.syncPairedDevices(devices); err != nil {
		return DeviceInfo{}, err
	}
	return s.device(d), nil
}

func (s *Service) SwitchMembership(ctx context.Context, token, name string) (DeviceInfo, error) {
	done, workErr := s.beginProfileWork()
	if workErr != nil {
		return DeviceInfo{}, workErr
	}
	defer done()
	c, err := s.client()
	if err != nil {
		return DeviceInfo{}, err
	}
	devices, err := c.Devices(ctx)
	if err != nil {
		return DeviceInfo{}, err
	}
	var current signaling.Device
	for _, device := range devices {
		if device.ID == s.identity.ID() {
			current = device
			break
		}
	}
	if current.ID == "" {
		return DeviceInfo{}, errors.New("UNPAIRED: current membership required for explicit switch")
	}
	d, err := c.SwitchGroup(ctx, token, s.name(name), current)
	if err != nil {
		return DeviceInfo{}, err
	}
	devices, err = c.Devices(ctx)
	if err != nil {
		return DeviceInfo{}, err
	}
	if err = s.syncPairedDevices(devices); err != nil {
		return DeviceInfo{}, err
	}
	return s.device(d), nil
}

func (s *Service) CreateInvitation(ctx context.Context) (InvitationInfo, error) {
	c, err := s.client()
	if err != nil {
		return InvitationInfo{}, err
	}
	i, err := c.CreateInvitation(ctx)
	if err != nil {
		return InvitationInfo{}, err
	}
	remaining := int64((i.Remaining + time.Second - 1) / time.Second)
	return InvitationInfo{Token: i.Token, ExpiresAt: i.ExpiresAt.UTC().Format(time.RFC3339), ExpiresInSeconds: remaining}, nil
}

func (s *Service) Devices(ctx context.Context) ([]DeviceInfo, error) {
	done, workErr := s.beginProfileWork()
	if workErr != nil {
		return nil, workErr
	}
	defer done()
	c, err := s.client()
	var devices []signaling.Device
	serverErr := err
	if err == nil {
		s.flushPendingRevocations(ctx, c)
		devices, serverErr = c.Devices(ctx)
		if serverErr == nil {
			if err = s.syncPairedDevices(devices); err != nil {
				return nil, err
			}
		}
	}
	peers, err := identity.LoadTrust(s.cfg.DataDir)
	if err != nil {
		return nil, err
	}
	trusted := make(map[string]identity.TrustedPeer, len(peers))
	for _, p := range peers {
		trusted[p.ID] = p
	}
	nearby := s.lanPeers()
	nearbyByID := make(map[string]discovery.Device, len(nearby))
	for _, peer := range nearby {
		nearbyByID[peer.ID] = peer
	}
	out := make([]DeviceInfo, 0, len(devices)+len(peers)+len(nearby))
	seen := make(map[string]bool, cap(out))
	for _, d := range devices {
		item := s.device(d)
		peer, ok := trusted[d.ID]
		item.Trusted = ok || d.ID == s.identity.ID()
		item.AlwaysAccept = ok && peer.AutoAccept
		_, item.Nearby = nearbyByID[d.ID]
		if item.Nearby {
			item.LANControlState = "discovered_unverified"
		}
		out = append(out, item)
		seen[d.ID] = true
	}
	for _, peer := range peers {
		if seen[peer.ID] || peer.ID == s.identity.ID() {
			continue
		}
		_, isNearby := nearbyByID[peer.ID]
		relationship := "lan_paired"
		if peer.GrantKind == "group" {
			relationship = "group_paired"
		}
		out = append(out, DeviceInfo{ID: peer.ID, Name: peer.Name, PublicKey: hex.EncodeToString(peer.PublicKey), Online: false, Trusted: true, AlwaysAccept: peer.AutoAccept, Nearby: isNearby, Relationship: relationship, ServiceState: "unavailable", LANControlState: map[bool]string{true: "discovered_unverified", false: "not_seen"}[isNearby], ConnectionState: "not_connected"})
		seen[peer.ID] = true
	}
	for _, peer := range nearby {
		if seen[peer.ID] || peer.ID == s.identity.ID() {
			continue
		}
		out = append(out, DeviceInfo{ID: peer.ID, Name: peer.Name, PublicKey: hex.EncodeToString(peer.PublicKey), Online: false, Nearby: true, Relationship: "unpaired", ServiceState: "unknown", LANControlState: "discovered_unverified", ConnectionState: "not_connected"})
		seen[peer.ID] = true
	}
	denied, err := identity.LoadDeniedPeers(s.cfg.DataDir)
	if err != nil {
		return nil, err
	}
	for _, blocked := range denied {
		found := false
		for i := range out {
			if out[i].ID == blocked.ID {
				out[i].Blocked, out[i].Trusted, out[i].AlwaysAccept, out[i].Relationship = true, false, false, "removed"
				if blocked.PendingSync {
					out[i].ServiceState = "pending_revoke_sync"
				} else {
					out[i].ServiceState = "revoked"
				}
				out[i].LANControlState, out[i].ConnectionState = "blocked", "blocked"
				found = true
			}
		}
		if !found {
			out = append(out, DeviceInfo{ID: blocked.ID, Name: blocked.Name, Blocked: true, Relationship: "removed", ServiceState: map[bool]string{true: "pending_revoke_sync", false: "revoked"}[blocked.PendingSync], LANControlState: "blocked", ConnectionState: "blocked"})
		}
	}
	if len(out) == 0 && serverErr != nil {
		return nil, serverErr
	}
	connected := map[string]bool{}
	for _, task := range s.Tasks() {
		if task.PeerID != "" && task.SessionID != "" && !isTerminal(task.State) {
			connected[task.PeerID] = true
		}
	}
	for index := range out {
		if connected[out[index].ID] {
			out[index].ConnectionState = "connected"
		} else if out[index].ConnectionState == "" {
			out[index].ConnectionState = "not_connected"
		}
	}
	return s.decorateDevices(out), nil
}

func (s *Service) flushPendingRevocations(ctx context.Context, c *signaling.Client) {
	denied, err := identity.LoadDeniedPeers(s.cfg.DataDir)
	if err != nil {
		return
	}
	for _, pending := range denied {
		if !pending.PendingSync || pending.RequestID == "" || pending.TargetIncarnation == "" || pending.SeenRevision == 0 {
			continue
		}
		revision, revokeErr := c.RevokeMembership(ctx, pending.ID, pending.TargetIncarnation, pending.SeenRevision, pending.RequestID)
		if revokeErr != nil {
			continue
		}
		s.trustMu.Lock()
		_ = identity.MarkMembershipRevokeSynced(s.cfg.DataDir, pending.ID, pending.RequestID, revision)
		s.trustMu.Unlock()
	}
}

// syncPairedDevices intentionally implements the simplified trust model:
// successful pairing-service membership pins every returned device key. This
// removes the manual fingerprint step while still rejecting later key changes.
func (s *Service) syncPairedDevices(devices []signaling.Device) error {
	s.trustMu.Lock()
	before, beforeErr := identity.LoadTrust(s.cfg.DataDir)
	var local signaling.Device
	for _, d := range devices {
		if d.ID == s.identity.ID() {
			local = d
			break
		}
	}
	if local.ID == "" {
		s.trustMu.Unlock()
		return protocol.Fail(protocol.VersionIncompatible, "membership_v2 local incarnation missing")
	}
	snapshot := make([]identity.TrustedPeer, 0, len(devices))
	for _, d := range devices {
		snapshot = append(snapshot, identity.TrustedPeer{ID: d.ID, Name: d.Name, PublicKey: d.PublicKey, GroupID: d.GroupID, PeerIncarnation: d.Incarnation, LocalIncarnation: local.Incarnation, MembershipRevision: d.MembershipRevision})
	}
	err := identity.ApplyMembershipSnapshot(s.cfg.DataDir, s.identity.ID(), snapshot)
	after, afterErr := identity.LoadTrust(s.cfg.DataDir)
	s.trustMu.Unlock()
	if err != nil {
		return err
	}
	if beforeErr == nil && afterErr == nil {
		current := make(map[string]uint64, len(after))
		for _, peer := range after {
			current[peer.ID] = peer.GrantGeneration
		}
		for _, peer := range before {
			if generation, ok := current[peer.ID]; !ok || generation != peer.GrantGeneration {
				s.closePooledSessions(peer.ID)
				s.cancelPeerTasks(peer.ID)
			}
		}
	}
	return nil
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
	return MembershipStatus{State: "not_member", Role: "unknown", Message: "当前设备尚未配对，请在另一台设备上生成配对码"}
}

func membershipFailure(err error) MembershipStatus {
	switch protocol.ErrorCode(err) {
	case protocol.SignalingUnreachable, protocol.SignalingTimeout:
		return MembershipStatus{State: "unavailable", Role: "unknown", Message: "暂时无法连接信令服务，请检查服务地址和网络。"}
	case protocol.VersionIncompatible:
		return MembershipStatus{State: "auth_failed", Role: "unknown", Message: "服务版本或能力不兼容，请升级后重试。"}
	case protocol.AuthenticationFailed:
		return MembershipStatus{State: "auth_failed", Role: "unknown", Message: "当前设备尚未完成配对，请输入另一台设备生成的配对码。"}
	default:
		return MembershipStatus{State: "unavailable", Role: "unknown", Message: "配对状态暂时无法读取，请检查服务和网络。"}
	}
}

func (s *Service) Trust(ctx context.Context, deviceID, fingerprint string) error {
	done, workErr := s.beginProfileWork()
	if workErr != nil {
		return workErr
	}
	defer done()
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

// SetAlwaysAccept changes receiver consent for one paired device. Transport
// identity checks remain mandatory even when the user confirmation is skipped.
func (s *Service) SetAlwaysAccept(deviceID string, enabled bool) error {
	done, workErr := s.beginProfileWork()
	if workErr != nil {
		return workErr
	}
	defer done()
	if len(deviceID) != 64 || deviceID == s.identity.ID() {
		return errors.New("UNPAIRED: paired peer is required")
	}
	s.trustMu.Lock()
	defer s.trustMu.Unlock()
	return identity.SetAutoAccept(s.cfg.DataDir, deviceID, enabled)
}

func (s *Service) alwaysAccept(deviceID string) bool {
	s.trustMu.Lock()
	defer s.trustMu.Unlock()
	peers, err := identity.LoadTrust(s.cfg.DataDir)
	if err != nil {
		return false
	}
	for _, peer := range peers {
		if peer.ID == deviceID {
			return peer.AutoAccept
		}
	}
	return false
}

func (s *Service) Revoke(ctx context.Context, deviceID string) error {
	peers, err := identity.LoadTrust(s.cfg.DataDir)
	if err != nil {
		return err
	}
	var peer identity.TrustedPeer
	for _, candidate := range peers {
		if candidate.ID == deviceID {
			peer = candidate
			break
		}
	}
	if peer.ID == "" {
		denied, loadErr := identity.LoadDeniedPeers(s.cfg.DataDir)
		if loadErr != nil {
			return loadErr
		}
		for _, prior := range denied {
			if prior.ID != deviceID {
				continue
			}
			if !prior.PendingSync {
				return nil
			}
			c, clientErr := s.client()
			if clientErr != nil {
				return clientErr
			}
			revision, retryErr := c.RevokeMembership(ctx, deviceID, prior.TargetIncarnation, prior.SeenRevision, prior.RequestID)
			if retryErr != nil {
				return retryErr
			}
			return identity.MarkMembershipRevokeSynced(s.cfg.DataDir, deviceID, prior.RequestID, revision)
		}
		return errors.New("UNPAIRED: current membership grant required")
	}
	if peer.GrantKind != "group" || len(peer.PeerIncarnation) != 32 || peer.MembershipRevision == 0 {
		if err = s.BlockPeer(deviceID); err != nil {
			return err
		}
		c, clientErr := s.client()
		if clientErr != nil {
			return clientErr
		}
		return c.Revoke(ctx, deviceID)
	}
	requestID := protocol.RandomID()
	s.trustMu.Lock()
	err = identity.RevokeMembershipPeer(s.cfg.DataDir, deviceID, peer.PeerIncarnation, requestID, peer.MembershipRevision)
	s.trustMu.Unlock()
	if err != nil {
		return err
	}
	for _, task := range s.Tasks() {
		if task.PeerID == deviceID && !isTerminal(task.State) {
			_ = s.CancelTask(task.ID)
		}
	}
	c, err := s.client()
	if err != nil {
		return err
	}
	revision, err := c.RevokeMembership(ctx, deviceID, peer.PeerIncarnation, peer.MembershipRevision, requestID)
	if err != nil {
		return err
	}
	s.trustMu.Lock()
	syncErr := identity.MarkMembershipRevokeSynced(s.cfg.DataDir, deviceID, requestID, revision)
	s.trustMu.Unlock()
	return syncErr
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
	return DeviceInfo{ID: d.ID, GroupID: d.GroupID, Name: d.Name, PublicKey: hex.EncodeToString(d.PublicKey), Admin: d.Admin, Online: d.Online, Incarnation: d.Incarnation, MembershipRevision: d.MembershipRevision, Relationship: "group_paired", ServiceState: "membership_synced", ConnectionState: "not_connected"}
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
