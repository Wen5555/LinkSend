package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	linksendapp "github.com/Wen5555/LinkSend/internal/app"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// App is the thin desktop boundary. Network and file services belong to internal/app.
type App struct {
	ctx           context.Context
	cancel        context.CancelFunc
	core          *linksendapp.Service
	initErr       error
	prefs         DesktopPreferences
	prefsStatus   PreferencesStatus
	configBlocked bool
	dataDir       string
	closeMu       sync.Mutex
	closePending  bool
	runtimeApp    *application.App
	window        application.Window
}

type DesktopPreferences struct {
	FormatVersion    int      `json:"format_version"`
	ServerURL        string   `json:"server_url"`
	BindAddress      string   `json:"bind_address"`
	STUNURLs         []string `json:"stun_urls"`
	ReceiveDirectory string   `json:"receive_directory"`
	DeviceName       string   `json:"device_name"`
}

type PreferencesStatus struct {
	State   string `json:"state"`
	Message string `json:"message,omitempty"`
}

type EffectiveConfig struct {
	ServerURL    string   `json:"server_url"`
	ServerSource string   `json:"server_source"`
	BindAddress  string   `json:"bind_address"`
	BindSource   string   `json:"bind_source"`
	STUNURLs     []string `json:"stun_urls"`
	STUNSource   string   `json:"stun_source"`
	NeedsRestart bool     `json:"needs_restart"`
	Preferences  string   `json:"preferences_state"`
}

type NetworkInterfaceInfo struct {
	Name       string   `json:"name"`
	Addresses  []string `json:"addresses"`
	IsLoopback bool     `json:"is_loopback"`
}

type DesktopStatus struct {
	Version  string `json:"version"`
	Platform string `json:"platform"`
	Relay    bool   `json:"relay"`
	Stage    string `json:"stage"`
	Ready    bool   `json:"ready"`
	Error    string `json:"error,omitempty"`
	Identity string `json:"identity,omitempty"`
}

var errBackendUnavailable = errors.New("BACKEND_UNAVAILABLE: desktop core is not initialized")

func NewApp() *App { return &App{} }

// attachRuntime is called by the Wails 3 host before Run. Keeping the host
// reference here lets bound methods use native dialogs without exposing the
// framework object to the frontend.
func (a *App) attachRuntime(host *application.App, window application.Window) {
	a.runtimeApp = host
	a.window = window
}

func (a *App) startup(ctx context.Context) {
	a.ctx, a.cancel = context.WithCancel(ctx)
	dataDir := os.Getenv("LINKSEND_DATA_DIR")
	if dataDir == "" {
		if root, err := os.UserConfigDir(); err == nil {
			dataDir = filepath.Join(root, "LinkSend")
		} else {
			dataDir = filepath.Join(os.TempDir(), "LinkSend")
		}
	}
	a.dataDir = dataDir
	a.prefs, a.prefsStatus = loadPreferencesDetailed(dataDir)
	a.configBlocked = a.prefsStatus.State != "missing" && a.prefsStatus.State != "valid"
	serverURL := firstNonEmpty(os.Getenv("LINKSEND_SERVER_URL"), a.prefs.ServerURL, "https://linksend.oooai.de")
	name := firstNonEmpty(a.prefs.DeviceName, "LinkSend desktop")
	allowLoopback, _ := strconv.ParseBool(os.Getenv("LINKSEND_ALLOW_INSECURE_LOOPBACK"))
	a.core, a.initErr = linksendapp.New(linksendapp.Config{DataDir: dataDir, ServerURL: serverURL, AllowInsecureLoopback: allowLoopback, Name: name})
}
func (a *App) shutdown() {
	if a.core != nil {
		a.core.Shutdown()
	}
	if a.cancel != nil {
		a.cancel()
	}
}

// ServiceStartup/ServiceShutdown are the Wails 3 lifecycle hooks. Startup
// errors are retained in Status so a desktop window can still open and expose
// actionable diagnostics when the signaling service is unavailable.
func (a *App) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	a.startup(ctx)
	return nil
}

func (a *App) ServiceShutdown() error {
	a.shutdown()
	return nil
}

// beforeClose is the native Wails close hook. It protects every non-terminal
// task, including waiting/connecting phases, rather than relying on browser
// unload events which do not cover the native window.
func (a *App) shouldQuit() bool {
	if a.core == nil {
		return true
	}
	a.closeMu.Lock()
	if a.closePending {
		a.closeMu.Unlock()
		return true
	}
	a.closePending = true
	a.closeMu.Unlock()
	defer func() { a.closeMu.Lock(); a.closePending = false; a.closeMu.Unlock() }()
	active := make([]linksendapp.TaskSnapshot, 0)
	for _, task := range a.core.Tasks() {
		if task.State != "completed" && task.State != "failed" && task.State != "cancelled" {
			active = append(active, task)
		}
	}
	if len(active) == 0 {
		return false
	}
	if a.runtimeApp == nil {
		return false
	}
	choice := make(chan bool, 1)
	dialog := a.runtimeApp.Dialog.Question().
		SetTitle("LinkSend 仍有任务运行").
		SetMessage(fmt.Sprintf("当前有 %d 个任务处于准备、等待确认、连接或传输阶段。请选择继续任务，或取消任务并退出。", len(active)))
	keep := dialog.AddButton("继续任务").OnClick(func() { choice <- false })
	dialog.AddButton("取消任务并退出").OnClick(func() { choice <- true })
	dialog.SetDefaultButton(keep).SetCancelButton(keep)
	dialog.Show()
	shouldExit := false
	select {
	case shouldExit = <-choice:
	default:
		// A native dialog that closes without a callback is treated as cancel.
	}
	if !shouldExit {
		return false
	}
	for _, task := range active {
		if err := a.core.CancelTask(task.ID); err != nil && !strings.Contains(err.Error(), "TASK_TERMINAL") {
			return false
		}
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		allDone := true
		for _, task := range active {
			if current, ok := a.core.Task(task.ID); ok && current.State != "completed" && current.State != "failed" && current.State != "cancelled" {
				allDone = false
			}
		}
		if allDone {
			return false
		}
		time.Sleep(50 * time.Millisecond)
	}
	if a.runtimeApp != nil {
		warning := a.runtimeApp.Dialog.Warning().SetTitle("任务仍在清理").SetMessage("取消尚未确认，窗口将保持打开。请稍后重试。")
		warning.AddButton("知道了").SetAsDefault()
		warning.Show()
	}
	return false
}

func (a *App) directConfig() linksendapp.DirectConfig {
	stun := make([]string, 0)
	rawSTUN := firstNonEmpty(os.Getenv("LINKSEND_STUN"), strings.Join(a.prefs.STUNURLs, ","), "stun:stun.oooai.de:3478")
	for _, value := range strings.Split(rawSTUN, ",") {
		if value = strings.TrimSpace(value); value != "" {
			stun = append(stun, value)
		}
	}
	allow, _ := strconv.ParseBool(os.Getenv("LINKSEND_ALLOW_INSECURE_LOOPBACK"))
	return linksendapp.DirectConfig{BindAddress: firstNonEmpty(os.Getenv("LINKSEND_BIND"), a.prefs.BindAddress), STUNURLs: stun, AllowLoopback: allow, CheckTimeout: 30 * time.Second, WaitTimeout: 10 * time.Minute}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func loadPreferences(dataDir string) DesktopPreferences {
	p, _ := loadPreferencesDetailed(dataDir)
	return p
}

func loadPreferencesDetailed(dataDir string) (DesktopPreferences, PreferencesStatus) {
	p := DesktopPreferences{FormatVersion: 1}
	b, err := os.ReadFile(filepath.Join(dataDir, "desktop-preferences.json"))
	if errors.Is(err, os.ErrNotExist) {
		return p, PreferencesStatus{State: "missing"}
	}
	if err != nil {
		return p, PreferencesStatus{State: "unreadable", Message: "偏好文件无法读取，请检查权限后重试"}
	}
	var loaded DesktopPreferences
	if err := json.Unmarshal(b, &loaded); err != nil {
		return p, PreferencesStatus{State: "corrupt", Message: "偏好文件格式损坏；原文件已保留，可重试或仅重置桌面偏好"}
	}
	if loaded.FormatVersion != 1 {
		return p, PreferencesStatus{State: "unsupported", Message: "偏好文件版本不受支持；原文件已保留，请重置桌面偏好"}
	}
	return loaded, PreferencesStatus{State: "valid"}
}

func (a *App) Preferences() DesktopPreferences      { return a.prefs }
func (a *App) PreferencesStatus() PreferencesStatus { return a.prefsStatus }

func (a *App) EffectiveConfig() EffectiveConfig {
	envServer, envBind, envSTUN := os.Getenv("LINKSEND_SERVER_URL"), os.Getenv("LINKSEND_BIND"), os.Getenv("LINKSEND_STUN")
	server, serverSource := a.prefs.ServerURL, "已保存偏好"
	if strings.TrimSpace(envServer) != "" {
		server, serverSource = strings.TrimSpace(envServer), "环境变量 LINKSEND_SERVER_URL"
	}
	if strings.TrimSpace(server) == "" {
		server, serverSource = "https://linksend.oooai.de", "默认值"
	}
	bind, bindSource := a.prefs.BindAddress, "已保存偏好"
	if strings.TrimSpace(envBind) != "" {
		bind, bindSource = strings.TrimSpace(envBind), "环境变量 LINKSEND_BIND"
	}
	stun := append([]string(nil), a.prefs.STUNURLs...)
	stunSource := "已保存偏好"
	if strings.TrimSpace(envSTUN) != "" {
		stun = make([]string, 0, 4)
		for _, value := range strings.Split(envSTUN, ",") {
			if value = strings.TrimSpace(value); value != "" {
				stun = append(stun, value)
			}
		}
		stunSource = "环境变量 LINKSEND_STUN"
	}
	if len(stun) == 0 {
		stun, stunSource = []string{"stun:stun.oooai.de:3478"}, "默认值"
	}
	return EffectiveConfig{ServerURL: server, ServerSource: serverSource, BindAddress: bind, BindSource: bindSource, STUNURLs: stun, STUNSource: stunSource, NeedsRestart: true, Preferences: a.prefsStatus.State}
}

func (a *App) SavePreferences(next DesktopPreferences) error {
	if a.dataDir == "" {
		return errBackendUnavailable
	}
	next.FormatVersion = 1
	next.ServerURL = strings.TrimSpace(next.ServerURL)
	next.BindAddress = strings.TrimSpace(next.BindAddress)
	next.ReceiveDirectory = strings.TrimSpace(next.ReceiveDirectory)
	next.DeviceName = strings.TrimSpace(next.DeviceName)
	if next.ServerURL != "" {
		u, err := url.Parse(next.ServerURL)
		if err != nil || (u.Scheme != "https" && u.Scheme != "wss" && u.Scheme != "http" && u.Scheme != "ws") {
			return errors.New("INVALID_CONFIG: 服务地址必须是 http(s)/ws(s) URL")
		}
	}
	tmp := filepath.Join(a.dataDir, fmt.Sprintf("desktop-preferences.json.tmp-%d", time.Now().UnixNano()))
	var recoveryBackup string
	if a.configBlocked {
		original := filepath.Join(a.dataDir, "desktop-preferences.json")
		if _, err := os.Stat(original); err == nil {
			recoveryBackup = filepath.Join(a.dataDir, fmt.Sprintf("desktop-preferences.json.recovery-%d.bak", time.Now().UnixNano()))
			contents, readErr := os.ReadFile(original)
			writeErr := error(nil)
			if readErr == nil {
				writeErr = os.WriteFile(recoveryBackup, contents, 0600)
			}
			if readErr != nil || writeErr != nil {
				return fmt.Errorf("PREFERENCES_BACKUP_FAILED: 无法保留原偏好文件: %v", errors.Join(readErr, writeErr))
			}
		}
	}
	b, _ := json.MarshalIndent(next, "", "  ")
	if err := os.WriteFile(tmp, append(b, '\n'), 0600); err != nil {
		_ = recoveryBackup
		return err
	}
	if err := os.Rename(tmp, filepath.Join(a.dataDir, "desktop-preferences.json")); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	a.prefs = next
	a.prefsStatus = PreferencesStatus{State: "valid"}
	a.configBlocked = false
	return nil
}

func (a *App) ensureConfig() error {
	if a.configBlocked {
		return errors.New("CONFIG_BLOCKED: 偏好文件异常，先在设置页重置或修复桌面偏好")
	}
	return nil
}

func (a *App) NetworkInterfaces() []NetworkInterfaceInfo {
	ifs, err := net.Interfaces()
	if err != nil {
		return []NetworkInterfaceInfo{}
	}
	out := make([]NetworkInterfaceInfo, 0, len(ifs))
	for _, in := range ifs {
		addrs, _ := in.Addrs()
		item := NetworkInterfaceInfo{Name: in.Name, IsLoopback: in.Flags&net.FlagLoopback != 0}
		for _, addr := range addrs {
			item.Addresses = append(item.Addresses, addr.String())
		}
		if len(item.Addresses) > 0 {
			out = append(out, item)
		}
	}
	return out
}

func (a *App) StartSend(peerID string, paths []string) (linksendapp.TaskSnapshot, error) {
	if err := a.ensureConfig(); err != nil {
		return linksendapp.TaskSnapshot{}, err
	}
	if a.core == nil {
		if a.initErr != nil {
			return linksendapp.TaskSnapshot{}, a.initErr
		}
		return linksendapp.TaskSnapshot{}, errBackendUnavailable
	}
	return a.core.StartSend(peerID, paths, a.directConfig())
}
func (a *App) StartReceive(expectedPeerID, directory string) (linksendapp.TaskSnapshot, error) {
	if err := a.ensureConfig(); err != nil {
		return linksendapp.TaskSnapshot{}, err
	}
	if a.core == nil {
		if a.initErr != nil {
			return linksendapp.TaskSnapshot{}, a.initErr
		}
		return linksendapp.TaskSnapshot{}, errBackendUnavailable
	}
	return a.core.StartReceive(expectedPeerID, directory, a.directConfig())
}
func (a *App) Tasks() []linksendapp.TaskSnapshot {
	if a.core == nil {
		return []linksendapp.TaskSnapshot{}
	}
	return a.core.Tasks()
}
func (a *App) GetTask(id string) (linksendapp.TaskSnapshot, error) {
	if a.core == nil {
		if a.initErr != nil {
			return linksendapp.TaskSnapshot{}, a.initErr
		}
		return linksendapp.TaskSnapshot{}, errBackendUnavailable
	}
	if task, ok := a.core.Task(id); ok {
		return task, nil
	}
	return linksendapp.TaskSnapshot{}, errors.New("TASK_NOT_FOUND")
}
func (a *App) CancelTask(id string) error {
	if a.core == nil {
		if a.initErr != nil {
			return a.initErr
		}
		return errBackendUnavailable
	}
	return a.core.CancelTask(id)
}
func (a *App) AcceptTask(id string) error {
	if a.core == nil {
		if a.initErr != nil {
			return a.initErr
		}
		return errBackendUnavailable
	}
	return a.core.AcceptTask(id)
}
func (a *App) RejectTask(id string) error {
	if a.core == nil {
		if a.initErr != nil {
			return a.initErr
		}
		return errBackendUnavailable
	}
	return a.core.RejectTask(id)
}
func (a *App) RetryTask(id string) (linksendapp.TaskSnapshot, error) {
	if a.core == nil {
		if a.initErr != nil {
			return linksendapp.TaskSnapshot{}, a.initErr
		}
		return linksendapp.TaskSnapshot{}, errBackendUnavailable
	}
	return a.core.RetryTask(id)
}

func (a *App) JoinGroup(token, name string) (linksendapp.DeviceInfo, error) {
	if err := a.ensureConfig(); err != nil {
		return linksendapp.DeviceInfo{}, err
	}
	if a.core == nil {
		if a.initErr != nil {
			return linksendapp.DeviceInfo{}, a.initErr
		}
		return linksendapp.DeviceInfo{}, errBackendUnavailable
	}
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	return a.core.Join(ctx, token, name)
}

func (a *App) TrustDevice(deviceID, fingerprint string) error {
	if err := a.ensureConfig(); err != nil {
		return err
	}
	if a.core == nil {
		if a.initErr != nil {
			return a.initErr
		}
		return errBackendUnavailable
	}
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	return a.core.Trust(ctx, deviceID, fingerprint)
}

func (a *App) OpenTaskDirectory(taskID string) error {
	if a.core == nil {
		return errBackendUnavailable
	}
	task, ok := a.core.Task(taskID)
	if !ok || task.Direction != "receive" || task.State != "completed" || strings.TrimSpace(task.TargetDirectory) == "" {
		return errors.New("INVALID_TASK_DIRECTORY: 仅允许打开已完成接收任务的目录")
	}
	dir, err := filepath.Abs(filepath.Clean(task.TargetDirectory))
	if err != nil {
		return err
	}
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return errors.New("INVALID_DIRECTORY: 目录不存在")
	}
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	launcher := "xdg-open"
	if runtime.GOOS == "windows" {
		launcher = "explorer.exe"
	} else if runtime.GOOS == "darwin" {
		launcher = "open"
	}
	return exec.CommandContext(ctx, launcher, dir).Start()
}

func (a *App) PickFiles() ([]string, error) {
	if a.runtimeApp == nil {
		return nil, errors.New("DESKTOP_NOT_READY: 原生窗口尚未就绪")
	}
	return a.runtimeApp.Dialog.OpenFile().
		SetTitle("选择要发送的文件").
		CanChooseFiles(true).
		CanChooseDirectories(false).
		PromptForMultipleSelection()
}
func (a *App) PickDirectory() (string, error) {
	if a.runtimeApp == nil {
		return "", errors.New("DESKTOP_NOT_READY: 原生窗口尚未就绪")
	}
	return a.runtimeApp.Dialog.OpenFile().
		SetTitle("选择接收目录").
		CanChooseFiles(false).
		CanChooseDirectories(true).
		PromptForSingleSelection()
}
func (a *App) PickSourceDirectory() (string, error) {
	if a.runtimeApp == nil {
		return "", errors.New("DESKTOP_NOT_READY: 原生窗口尚未就绪")
	}
	return a.runtimeApp.Dialog.OpenFile().
		SetTitle("选择要发送的目录").
		CanChooseFiles(false).
		CanChooseDirectories(true).
		PromptForSingleSelection()
}

// Status reports this running shell only; it does not claim a P2P connection.
func (a *App) Status() DesktopStatus {
	status := DesktopStatus{Version: "0.1.0-dev", Platform: runtime.GOOS + "/" + runtime.GOARCH, Relay: false, Stage: "service", Ready: a.core != nil}
	if a.configBlocked {
		status.Ready = false
		status.Error = a.prefsStatus.Message
	}
	if a.core != nil {
		status.Identity = a.core.Identity().ID
	}
	if a.initErr != nil {
		status.Ready = false
		status.Error = a.initErr.Error()
	}
	return status
}

func (a *App) Identity() linksendapp.IdentityInfo {
	if a.core == nil {
		return linksendapp.IdentityInfo{}
	}
	return a.core.Identity()
}

func (a *App) Diagnostics() linksendapp.Diagnostics {
	if a.core == nil {
		return linksendapp.Diagnostics{Version: "0.1.0-dev", Platform: runtime.GOOS + "/" + runtime.GOARCH, Relay: false, ServerHealth: "unavailable"}
	}
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	return a.core.Diagnostics(ctx)
}

func (a *App) Devices() ([]linksendapp.DeviceInfo, error) {
	if err := a.ensureConfig(); err != nil {
		return nil, err
	}
	if a.core == nil {
		if a.initErr != nil {
			return nil, a.initErr
		}
		return nil, linksendapp.ErrNotImplemented
	}
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	return a.core.Devices(ctx)
}

func (a *App) Membership() linksendapp.MembershipStatus {
	if a.core == nil {
		return linksendapp.MembershipStatus{State: "unavailable", Role: "unknown", Message: "桌面后端尚未就绪"}
	}
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	return a.core.Membership(ctx)
}

func (a *App) CreateInvitation() (linksendapp.InvitationInfo, error) {
	if err := a.ensureConfig(); err != nil {
		return linksendapp.InvitationInfo{}, err
	}
	if a.core == nil {
		if a.initErr != nil {
			return linksendapp.InvitationInfo{}, a.initErr
		}
		return linksendapp.InvitationInfo{}, linksendapp.ErrNotImplemented
	}
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	return a.core.CreateInvitation(ctx)
}
