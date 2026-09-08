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
	"time"

	linksendapp "github.com/Wen5555/LinkSend/internal/app"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// App is the thin desktop boundary. Network and file services belong to internal/app.
type App struct {
	ctx     context.Context
	cancel  context.CancelFunc
	core    *linksendapp.Service
	initErr error
	prefs   DesktopPreferences
	dataDir string
}

type DesktopPreferences struct {
	FormatVersion    int      `json:"format_version"`
	ServerURL        string   `json:"server_url"`
	BindAddress      string   `json:"bind_address"`
	STUNURLs         []string `json:"stun_urls"`
	ReceiveDirectory string   `json:"receive_directory"`
	DeviceName       string   `json:"device_name"`
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
	a.prefs = loadPreferences(dataDir)
	serverURL := firstNonEmpty(os.Getenv("LINKSEND_SERVER_URL"), a.prefs.ServerURL, "https://linksend.oooai.de")
	name := firstNonEmpty(a.prefs.DeviceName, "LinkSend desktop")
	allowLoopback, _ := strconv.ParseBool(os.Getenv("LINKSEND_ALLOW_INSECURE_LOOPBACK"))
	a.core, a.initErr = linksendapp.New(linksendapp.Config{DataDir: dataDir, ServerURL: serverURL, AllowInsecureLoopback: allowLoopback, Name: name})
}
func (a *App) shutdown(context.Context) {
	if a.core != nil {
		a.core.Shutdown()
	}
	if a.cancel != nil {
		a.cancel()
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
	p := DesktopPreferences{FormatVersion: 1}
	b, err := os.ReadFile(filepath.Join(dataDir, "desktop-preferences.json"))
	if err == nil {
		if json.Unmarshal(b, &p) != nil || p.FormatVersion != 1 {
			p = DesktopPreferences{FormatVersion: 1}
		}
	}
	return p
}

func (a *App) Preferences() DesktopPreferences { return a.prefs }

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
	b, _ := json.MarshalIndent(next, "", "  ")
	if err := os.WriteFile(tmp, append(b, '\n'), 0600); err != nil {
		return err
	}
	if err := os.Rename(tmp, filepath.Join(a.dataDir, "desktop-preferences.json")); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	a.prefs = next
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
	if a.core == nil {
		if a.initErr != nil {
			return linksendapp.TaskSnapshot{}, a.initErr
		}
		return linksendapp.TaskSnapshot{}, errBackendUnavailable
	}
	return a.core.StartSend(peerID, paths, a.directConfig())
}
func (a *App) StartReceive(expectedPeerID, directory string) (linksendapp.TaskSnapshot, error) {
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
	if runtime.GOOS == "windows" {
		return exec.CommandContext(ctx, "explorer.exe", dir).Start()
	}
	return exec.CommandContext(ctx, "xdg-open", dir).Start()
}

func (a *App) PickFiles() ([]string, error) {
	if a.ctx == nil {
		return nil, os.ErrInvalid
	}
	return wailsruntime.OpenMultipleFilesDialog(a.ctx, wailsruntime.OpenDialogOptions{Title: "选择要发送的文件"})
}
func (a *App) PickDirectory() (string, error) {
	if a.ctx == nil {
		return "", os.ErrInvalid
	}
	return wailsruntime.OpenDirectoryDialog(a.ctx, wailsruntime.OpenDialogOptions{Title: "选择接收目录"})
}
func (a *App) PickSourceDirectory() (string, error) {
	if a.ctx == nil {
		return "", os.ErrInvalid
	}
	return wailsruntime.OpenDirectoryDialog(a.ctx, wailsruntime.OpenDialogOptions{Title: "选择要发送的目录"})
}

// Status reports this running shell only; it does not claim a P2P connection.
func (a *App) Status() DesktopStatus {
	status := DesktopStatus{Version: "0.1.0-dev", Platform: runtime.GOOS + "/" + runtime.GOARCH, Relay: false, Stage: "service", Ready: a.core != nil}
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

func (a *App) CreateInvitation() (linksendapp.InvitationInfo, error) {
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
