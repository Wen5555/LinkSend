package main

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strconv"

	linksendapp "github.com/Wen5555/LinkSend/internal/app"
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
	if a.cancel != nil {
		a.cancel()
	}
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
