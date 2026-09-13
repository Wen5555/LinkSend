package main

import (
	"context"
	"runtime"

	"github.com/Wen5555/LinkSend/apps/desktop/nativeclipboard"
	coreapp "github.com/Wen5555/LinkSend/internal/app"
	"github.com/wailsapp/wails/v3/pkg/application"
)

type ClipboardWatchStatus struct {
	Enabled bool                   `json:"enabled"`
	Active  bool                   `json:"active"`
	Paused  bool                   `json:"paused"`
	Last    nativeclipboard.Change `json:"last"`
	Error   string                 `json:"error,omitempty"`
}

func (a *App) ClipboardGrants(peerID string) ([]coreapp.ClipboardGrant, error) {
	if a.core == nil {
		return nil, errBackendUnavailable
	}
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	return a.core.ClipboardGrants(ctx, peerID)
}

func (a *App) SetClipboardGrant(patch coreapp.ClipboardGrantPatch) (coreapp.ClipboardGrant, error) {
	if a.core == nil {
		return coreapp.ClipboardGrant{}, errBackendUnavailable
	}
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	grant, err := a.core.SetClipboardGrant(ctx, patch)
	if err != nil {
		return coreapp.ClipboardGrant{}, err
	}
	a.refreshClipboardWatch()
	return grant, nil
}

func (a *App) ClipboardWatcher() ClipboardWatchStatus {
	a.clipboardMu.Lock()
	defer a.clipboardMu.Unlock()
	enabled := a.core != nil && a.core.ClipboardSyncEnabled()
	return ClipboardWatchStatus{Enabled: enabled, Active: a.clipboardStop != nil, Paused: a.clipboardPaused, Last: a.clipboardLast, Error: a.clipboardErr}
}

func (a *App) refreshClipboardWatch() {
	if a.core == nil || a.ctx == nil {
		return
	}
	a.clipboardMu.Lock()
	enabled := a.core.ClipboardSyncEnabled() && !a.clipboardPaused
	active := a.clipboardStop != nil
	a.clipboardMu.Unlock()
	if !enabled {
		a.stopClipboardWatch()
		return
	}
	if active || a.runtimeApp == nil || a.window == nil {
		return
	}
	stop, err := application.InvokeSyncWithResultAndError(func() (func(), error) {
		var nativeWindow uintptr
		if runtime.GOOS == "windows" {
			nativeWindow = uintptr(a.window.NativeWindow())
		}
		return nativeclipboard.Watch(a.ctx, nativeWindow, a.recordClipboardChange)
	})
	a.clipboardMu.Lock()
	if err != nil {
		a.clipboardErr = err.Error()
		a.clipboardMu.Unlock()
		return
	}
	stale := !a.core.ClipboardSyncEnabled() || a.clipboardPaused || a.clipboardStop != nil
	if !stale {
		a.clipboardStop = stop
		a.clipboardErr = ""
	}
	a.clipboardMu.Unlock()
	if stale {
		application.InvokeSync(stop)
	}
}

func (a *App) recordClipboardChange(change nativeclipboard.Change) {
	a.clipboardMu.Lock()
	if a.clipboardStop != nil && !a.clipboardPaused {
		a.clipboardLast = change
	}
	a.clipboardMu.Unlock()
}

func (a *App) stopClipboardWatch() {
	a.clipboardMu.Lock()
	stop := a.clipboardStop
	a.clipboardStop = nil
	a.clipboardMu.Unlock()
	if stop != nil {
		application.InvokeSync(stop)
	}
}

func (a *App) pauseClipboardWatch() {
	a.clipboardMu.Lock()
	a.clipboardPaused = true
	a.clipboardLast = nativeclipboard.Change{}
	a.clipboardMu.Unlock()
	a.stopClipboardWatch()
}

func (a *App) resumeClipboardWatch() {
	a.clipboardMu.Lock()
	a.clipboardPaused = false
	a.clipboardLast = nativeclipboard.Change{}
	a.clipboardMu.Unlock()
	a.refreshClipboardWatch()
}
