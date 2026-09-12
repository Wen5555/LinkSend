package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/netip"
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
	"github.com/Wen5555/LinkSend/internal/connectivity"
	"github.com/Wen5555/LinkSend/internal/protocol"
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
	closeDialog   bool
	quitReady     bool
	runtimeApp    *application.App
	window        application.Window
}

type DesktopPreferences struct {
	FormatVersion      int      `json:"format_version"`
	ServerURL          string   `json:"server_url"`
	BindAddress        string   `json:"bind_address"`
	InterfacePriority  []string `json:"interface_priority"`
	ExcludedInterfaces []string `json:"excluded_interfaces"`
	STUNURLs           []string `json:"stun_urls"`
	ReceiveDirectory   string   `json:"receive_directory"`
	DeviceName         string   `json:"device_name"`
}

type PreferencesStatus struct {
	State   string `json:"state"`
	Message string `json:"message,omitempty"`
}

type EffectiveConfig struct {
	ServerURL          string   `json:"server_url"`
	ServerSource       string   `json:"server_source"`
	BindAddress        string   `json:"bind_address"`
	BindSource         string   `json:"bind_source"`
	InterfacePriority  []string `json:"interface_priority"`
	ExcludedInterfaces []string `json:"excluded_interfaces"`
	STUNURLs           []string `json:"stun_urls"`
	STUNSource         string   `json:"stun_source"`
	NeedsRestart       bool     `json:"needs_restart"`
	Preferences        string   `json:"preferences_state"`
}

type NetworkInterfaceInfo struct {
	Name            string   `json:"name"`
	Addresses       []string `json:"addresses"`
	AddressFamilies []string `json:"address_families"`
	IsLoopback      bool     `json:"is_loopback"`
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
	explicitDataDir := dataDir != ""
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
	if a.initErr == nil && a.core != nil && !a.configBlocked {
		if strings.TrimSpace(a.prefs.ReceiveDirectory) == "" {
			if explicitDataDir {
				a.prefs.ReceiveDirectory = filepath.Join(dataDir, "received")
			} else {
				a.prefs.ReceiveDirectory = defaultReceiveDirectory()
			}
		}
		if a.prefs.ReceiveDirectory != "" {
			_ = a.core.StartInbox(a.prefs.ReceiveDirectory, a.directConfig())
		}
	}
}

func defaultReceiveDirectory() string {
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return ""
	}
	return filepath.Join(home, "Downloads", "LinkSend")
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
	a.closeMu.Lock()
	ready := a.quitReady
	a.closeMu.Unlock()
	if ready {
		return true
	}
	if a.core == nil {
		return true
	}
	active := make([]linksendapp.TaskSnapshot, 0)
	for _, task := range a.core.Tasks() {
		if !terminalTaskState(task.State) {
			active = append(active, task)
		}
	}
	if len(active) == 0 {
		return true
	}
	if a.runtimeApp == nil {
		return false
	}
	a.closeMu.Lock()
	if a.closeDialog {
		a.closeMu.Unlock()
		return false
	}
	a.closeDialog = true
	a.closeMu.Unlock()
	dialog := a.runtimeApp.Dialog.Question().
		SetTitle("LinkSend 仍有任务运行").
		SetMessage(fmt.Sprintf("当前有 %d 个未结束任务。保存并退出会保留已验证数据，下次启动需确认恢复。尚未准备完成的内容需重新发送。", len(active)))
	keep := dialog.AddButton("继续任务").OnClick(func() { a.setCloseDialog(false) })
	dialog.AddButton("保存并退出").OnClick(func() { go a.saveTasksAndQuit() })
	dialog.AddButton("取消任务并退出").OnClick(func() { go a.cancelTasksAndQuit(active) })
	dialog.SetDefaultButton(keep).SetCancelButton(keep)
	if a.window != nil {
		dialog.AttachToWindow(a.window)
	}
	dialog.Show()
	// Wails 3 message dialogs dispatch button callbacks asynchronously. The
	// original quit request must be cancelled while the native sheet is open;
	// the destructive choice explicitly calls App.Quit after cancellation has
	// reached a terminal task state.
	return false
}

func terminalTaskState(state string) bool {
	switch state {
	case "completed", "rejected", "cancelled", "failed":
		return true
	default:
		return false
	}
}

func (a *App) setCloseDialog(open bool) {
	a.closeMu.Lock()
	a.closeDialog = open
	a.closeMu.Unlock()
}

func (a *App) saveTasksAndQuit() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := a.core.ShutdownContext(ctx); err != nil {
		a.setCloseDialog(false)
		a.showQuitWarning("保存或资源清理尚未完成，窗口保持打开。请检查任务状态后重试退出。")
		return
	}
	a.closeMu.Lock()
	a.quitReady, a.closeDialog = true, false
	a.closeMu.Unlock()
	if a.runtimeApp != nil {
		a.runtimeApp.Quit()
	}
}

func (a *App) cancelTasksAndQuit(active []linksendapp.TaskSnapshot) {
	for _, task := range active {
		if err := a.core.CancelTask(task.ID); err != nil && !strings.Contains(err.Error(), "TASK_TERMINAL") {
			a.setCloseDialog(false)
			a.showQuitWarning("任务取消失败，窗口将保持打开。请检查任务状态后重试。")
			return
		}
	}
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	for {
		allDone := true
		for _, task := range active {
			if current, ok := a.core.Task(task.ID); ok && !terminalTaskState(current.State) {
				allDone = false
			}
		}
		if allDone {
			a.setCloseDialog(false)
			if a.runtimeApp != nil {
				a.runtimeApp.Quit()
			}
			return
		}
		select {
		case <-ticker.C:
		case <-timer.C:
			a.setCloseDialog(false)
			a.showQuitWarning("取消尚未确认，窗口将保持打开。请稍后重试。")
			return
		}
	}
}

func (a *App) showQuitWarning(message string) {
	if a.runtimeApp != nil {
		warning := a.runtimeApp.Dialog.Warning().SetTitle("任务仍在清理").SetMessage(message)
		warning.AddButton("知道了").SetAsDefault()
		if a.window != nil {
			warning.AttachToWindow(a.window)
		}
		warning.Show()
	}
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
	bind, _ := resolvedDesktopBind(os.Getenv("LINKSEND_BIND"), a.prefs.BindAddress)
	return linksendapp.DirectConfig{BindAddress: bind, InterfacePriority: append([]string(nil), a.prefs.InterfacePriority...), ExcludedInterfaces: append([]string(nil), a.prefs.ExcludedInterfaces...), STUNURLs: stun, AllowLoopback: allow, CheckTimeout: 30 * time.Second, WaitTimeout: 10 * time.Minute}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func normalizedList(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]bool)
	for _, value := range values {
		value = strings.TrimSpace(value)
		key := strings.ToLower(value)
		if value == "" || seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, value)
	}
	return result
}

func resolvedDesktopBind(environment, saved string) (string, string) {
	if value := strings.TrimSpace(environment); value != "" {
		return value, "环境变量 LINKSEND_BIND"
	}
	value := strings.TrimSpace(saved)
	if value == "" {
		return "", "自动选择"
	}
	addresses, err := connectivity.DiscoverInterfaceAddresses(true)
	if err != nil {
		return value, "已保存偏好"
	}
	if resolved, fallback := savedBindOrAutomatic(value, addresses); fallback {
		return resolved, "已保存地址当前不可用，已自动选择"
	}
	return value, "已保存偏好"
}

func savedBindOrAutomatic(value string, addresses []connectivity.InterfaceAddress) (string, bool) {
	value = strings.TrimSpace(value)
	ip, ok := configuredBindIP(value)
	if !ok {
		return value, false
	}
	for _, address := range addresses {
		candidate, err := netip.ParseAddr(address.Address)
		if err == nil && candidate.Unmap() == ip.Unmap() {
			return value, false
		}
	}
	return "", true
}

func configuredBindIP(value string) (netip.Addr, bool) {
	value = strings.TrimSpace(value)
	if ip, err := netip.ParseAddr(strings.Trim(value, "[]")); err == nil {
		return ip, true
	}
	host, _, err := net.SplitHostPort(value)
	if err != nil {
		return netip.Addr{}, false
	}
	ip, err := netip.ParseAddr(strings.Trim(host, "[]"))
	return ip, err == nil
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
	bind, bindSource := resolvedDesktopBind(envBind, a.prefs.BindAddress)
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
	return EffectiveConfig{ServerURL: server, ServerSource: serverSource, BindAddress: bind, BindSource: bindSource, InterfacePriority: append([]string(nil), a.prefs.InterfacePriority...), ExcludedInterfaces: append([]string(nil), a.prefs.ExcludedInterfaces...), STUNURLs: stun, STUNSource: stunSource, NeedsRestart: true, Preferences: a.prefsStatus.State}
}

func (a *App) SavePreferences(next DesktopPreferences) error {
	if a.dataDir == "" {
		return errBackendUnavailable
	}
	next.FormatVersion = 1
	next.ServerURL = strings.TrimSpace(next.ServerURL)
	next.BindAddress = strings.TrimSpace(next.BindAddress)
	next.InterfacePriority = normalizedList(next.InterfacePriority)
	next.ExcludedInterfaces = normalizedList(next.ExcludedInterfaces)
	next.STUNURLs = normalizedList(next.STUNURLs)
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
	if a.core != nil && next.ReceiveDirectory != "" {
		if err := a.core.StartInbox(next.ReceiveDirectory, a.directConfig()); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) ensureConfig() error {
	if a.configBlocked {
		return errors.New("CONFIG_BLOCKED: 偏好文件异常，先在设置页重置或修复桌面偏好")
	}
	return nil
}

func (a *App) NetworkInterfaces() []NetworkInterfaceInfo {
	addresses, err := connectivity.DiscoverInterfaceAddresses(true)
	if err != nil {
		return []NetworkInterfaceInfo{}
	}
	out := make([]NetworkInterfaceInfo, 0, len(addresses))
	indexByName := make(map[string]int)
	for _, address := range addresses {
		index, ok := indexByName[address.Interface]
		if !ok {
			index = len(out)
			indexByName[address.Interface] = index
			out = append(out, NetworkInterfaceInfo{Name: address.Interface, IsLoopback: address.Loopback})
		}
		out[index].Addresses = append(out[index].Addresses, address.Address)
		out[index].AddressFamilies = append(out[index].AddressFamilies, address.Family)
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
func (a *App) PauseTask(id string) error {
	if a.core == nil {
		if a.initErr != nil {
			return a.initErr
		}
		return errBackendUnavailable
	}
	return a.core.PauseTask(id)
}
func (a *App) ResumeTask(id string) (linksendapp.TaskSnapshot, error) {
	if err := a.ensureConfig(); err != nil {
		return linksendapp.TaskSnapshot{}, err
	}
	if a.core == nil {
		if a.initErr != nil {
			return linksendapp.TaskSnapshot{}, a.initErr
		}
		return linksendapp.TaskSnapshot{}, errBackendUnavailable
	}
	return a.core.ResumeTask(id, a.directConfig())
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
func (a *App) AcceptTaskAlways(id string) error {
	if a.core == nil {
		if a.initErr != nil {
			return a.initErr
		}
		return errBackendUnavailable
	}
	return a.core.AcceptTaskAlways(id)
}
func (a *App) SetAlwaysAccept(deviceID string, enabled bool) error {
	if a.core == nil {
		if a.initErr != nil {
			return a.initErr
		}
		return errBackendUnavailable
	}
	return a.core.SetAlwaysAccept(deviceID, enabled)
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

// PairDevice is the user-facing alias for the simplified pairing-code flow.
// JoinGroup remains for CLI/binding compatibility with existing clients.
func (a *App) PairDevice(code, name string) (linksendapp.DeviceInfo, error) {
	return a.JoinGroup(code, name)
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

func (a *App) InboxStatus() linksendapp.InboxStatus {
	if a.core == nil {
		return linksendapp.InboxStatus{}
	}
	return a.core.InboxStatus()
}

func (a *App) RefreshLANDiscovery() error {
	if a.core == nil {
		return errBackendUnavailable
	}
	return a.core.RefreshLANDiscovery()
}

func (a *App) ProbeLANAddress(address string) error {
	if a.core == nil {
		return errBackendUnavailable
	}
	return a.core.ProbeLANAddress(address)
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
	status := DesktopStatus{Version: protocol.ProductVersion, Platform: runtime.GOOS + "/" + runtime.GOARCH, Relay: false, Stage: "service", Ready: a.core != nil}
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
		return linksendapp.Diagnostics{Version: protocol.ProductVersion, Platform: runtime.GOOS + "/" + runtime.GOARCH, Relay: false, ServerHealth: "unavailable"}
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
