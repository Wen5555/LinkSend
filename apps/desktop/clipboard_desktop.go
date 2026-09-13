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
	paused := a.clipboardSleeping || a.clipboardLocked || a.clipboardUserPaused
	return ClipboardWatchStatus{Enabled: enabled, Active: a.clipboardStop != nil, Paused: paused, Last: a.clipboardLast, Error: a.clipboardErr}
}

func (a *App) refreshClipboardWatch() {
	a.clipboardOwnerMu.Lock()
	defer a.clipboardOwnerMu.Unlock()
	a.refreshClipboardWatchOwned()
}

func (a *App) refreshClipboardWatchOwned() {
	if a.core == nil || a.ctx == nil {
		return
	}
	a.clipboardMu.Lock()
	paused := a.clipboardSleeping || a.clipboardLocked || a.clipboardUserPaused
	enabled := a.core.ClipboardSyncEnabled() && !paused
	active := a.clipboardStop != nil
	a.clipboardMu.Unlock()
	if !enabled {
		a.stopClipboardWatchOwned()
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
	paused = a.clipboardSleeping || a.clipboardLocked || a.clipboardUserPaused
	stale := !a.core.ClipboardSyncEnabled() || paused || a.clipboardStop != nil
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
	if a.clipboardStop != nil && !a.clipboardSleeping && !a.clipboardLocked && !a.clipboardUserPaused {
		a.clipboardLast = change
	}
	a.clipboardMu.Unlock()
}

func (a *App) stopClipboardWatch() {
	a.clipboardOwnerMu.Lock()
	defer a.clipboardOwnerMu.Unlock()
	a.stopClipboardWatchOwned()
}

func (a *App) stopClipboardWatchOwned() {
	a.clipboardMu.Lock()
	stop := a.clipboardStop
	a.clipboardStop = nil
	a.clipboardMu.Unlock()
	if stop != nil {
		application.InvokeSync(stop)
	}
}

func (a *App) setClipboardSuspended(reason string, suspended bool) {
	a.clipboardOwnerMu.Lock()
	defer a.clipboardOwnerMu.Unlock()
	a.clipboardMu.Lock()
	switch reason {
	case "sleep":
		a.clipboardSleeping = suspended
	case "lock":
		a.clipboardLocked = suspended
	case "user":
		a.clipboardUserPaused = suspended
	}
	a.clipboardLast = nativeclipboard.Change{}
	paused := a.clipboardSleeping || a.clipboardLocked || a.clipboardUserPaused
	a.clipboardMu.Unlock()
	if paused {
		a.stopClipboardWatchOwned()
	} else {
		a.refreshClipboardWatchOwned()
	}
}

func (a *App) SetClipboardPaused(paused bool) { a.setClipboardSuspended("user", paused) }
func (a *App) pauseClipboardWatch()           { a.setClipboardSuspended("sleep", true) }
func (a *App) resumeClipboardWatch()          { a.setClipboardSuspended("sleep", false) }
