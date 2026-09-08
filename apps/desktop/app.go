package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	linksendapp "github.com/Wen5555/LinkSend/internal/app"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// App is the thin desktop boundary. Network and file services belong to internal/app.
type App struct {
	ctx     context.Context
	cancel  context.CancelFunc
	core    *linksendapp.Service
	initErr error
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
	allowLoopback, _ := strconv.ParseBool(os.Getenv("LINKSEND_ALLOW_INSECURE_LOOPBACK"))
	a.core, a.initErr = linksendapp.New(linksendapp.Config{DataDir: dataDir, ServerURL: os.Getenv("LINKSEND_SERVER_URL"), AllowInsecureLoopback: allowLoopback, Name: "LinkSend desktop"})
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
	for _, value := range strings.Split(os.Getenv("LINKSEND_STUN"), ",") {
		if value = strings.TrimSpace(value); value != "" {
			stun = append(stun, value)
		}
	}
	allow, _ := strconv.ParseBool(os.Getenv("LINKSEND_ALLOW_INSECURE_LOOPBACK"))
	return linksendapp.DirectConfig{BindAddress: strings.TrimSpace(os.Getenv("LINKSEND_BIND")), STUNURLs: stun, AllowLoopback: allow}
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
	if a.core == nil { if a.initErr != nil { return linksendapp.TaskSnapshot{}, a.initErr }; return linksendapp.TaskSnapshot{}, errBackendUnavailable }
	if task, ok := a.core.Task(id); ok { return task, nil }
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
