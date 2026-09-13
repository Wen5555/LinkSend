package main

import (
	"context"
	"runtime"
	"time"

	"github.com/Wen5555/LinkSend/apps/desktop/nativeclipboard"
	coreapp "github.com/Wen5555/LinkSend/internal/app"
	"github.com/Wen5555/LinkSend/internal/clipboardsync"
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
	go a.ensureClipboardSessions()
	return grant, nil
}

func (a *App) ensureClipboardSessions() {
	if a.ctx == nil || a.core == nil {
		return
	}
	ctx, cancel := context.WithTimeout(a.ctx, 10*time.Second)
	defer cancel()
	_ = a.core.EnsureClipboardSessions(ctx, a.directConfig())
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
	if a.clipboardClosed {
		a.clipboardMu.Unlock()
		return
	}
	paused := a.clipboardSleeping || a.clipboardLocked || a.clipboardUserPaused
	enabled := a.core.ClipboardSyncEnabled() && !paused
	active := a.clipboardStop != nil
	a.clipboardMu.Unlock()
	if !enabled {
		a.stopClipboardWatchOwned()
		return
	}
	if active {
		return
	}
	var stop func()
	var err error
	if a.clipboardWatchStart != nil {
		stop, err = a.clipboardWatchStart()
	} else if a.runtimeApp == nil || a.window == nil {
		return
	} else {
		stop, err = application.InvokeSyncWithResultAndError(func() (func(), error) {
			var nativeWindow uintptr
			if runtime.GOOS == "windows" {
				nativeWindow = uintptr(a.window.NativeWindow())
			}
			return nativeclipboard.Watch(a.ctx, nativeWindow, a.recordClipboardChange)
		})
	}
	a.clipboardMu.Lock()
	if err != nil {
		a.clipboardErr = err.Error()
		a.clipboardMu.Unlock()
		return
	}
	paused = a.clipboardSleeping || a.clipboardLocked || a.clipboardUserPaused
	stale := a.clipboardClosed || !a.core.ClipboardSyncEnabled() || paused || a.clipboardStop != nil
	if !stale {
		a.clipboardStop = stop
		a.clipboardErr = ""
	}
	a.clipboardMu.Unlock()
	if stale {
		a.invokeClipboardStop(stop)
	}
}

func (a *App) recordClipboardChange(change nativeclipboard.Change) {
	a.clipboardMu.Lock()
	if a.clipboardStop != nil && !a.clipboardSleeping && !a.clipboardLocked && !a.clipboardUserPaused {
		a.clipboardLast = change
	}
	a.clipboardMu.Unlock()
	if a.clipboardChanges != nil {
		select {
		case a.clipboardChanges <- change:
		default:
			select {
			case <-a.clipboardChanges:
			default:
			}
			select {
			case a.clipboardChanges <- change:
			default:
			}
		}
	}
}

func (a *App) startClipboardDelivery() {
	if a.clipboardChanges != nil || a.ctx == nil || a.core == nil {
		return
	}
	a.clipboardChanges = make(chan nativeclipboard.Change, 1)
	a.clipboardDone = make(chan struct{})
	go func() {
		defer close(a.clipboardDone)
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		ctx, cancel := context.WithTimeout(a.ctx, 10*time.Second)
		_ = a.core.EnsureClipboardSessions(ctx, a.directConfig())
		cancel()
		for {
			select {
			case <-a.ctx.Done():
				return
			case <-ticker.C:
				ctx, cancel := context.WithTimeout(a.ctx, 10*time.Second)
				_ = a.core.EnsureClipboardSessions(ctx, a.directConfig())
				cancel()
			case change := <-a.clipboardChanges:
				ctx, cancel := context.WithTimeout(a.ctx, 10*time.Second)
				_ = a.core.EnsureClipboardSessions(ctx, a.directConfig())
				cancel()
				kinds := make([]clipboardsync.Kind, 0, 3)
				if change.Link || change.Text {
					kinds = append(kinds, clipboardsync.Link)
				}
				if change.Image {
					kinds = append(kinds, clipboardsync.Image)
				}
				if change.Text {
					kinds = append(kinds, clipboardsync.Text)
				}
				if len(kinds) > 0 {
					_ = a.core.ClipboardChanged(a.ctx, coreapp.ClipboardChange{Generation: change.Sequence, Kinds: kinds})
				}
			}
		}
	}()
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
		a.invokeClipboardStop(stop)
	}
}

func (a *App) invokeClipboardStop(stop func()) {
	if a.clipboardWatchStart != nil {
		stop()
		return
	}
	application.InvokeSync(stop)
}

func (a *App) setClipboardSuspended(reason string, suspended bool) {
	a.clipboardOwnerMu.Lock()
	defer a.clipboardOwnerMu.Unlock()
	a.clipboardMu.Lock()
	if a.clipboardClosed {
		a.clipboardMu.Unlock()
		return
	}
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
	if a.core != nil {
		a.core.ResetClipboard(paused, nativeclipboard.Generation())
	}
	if paused {
		a.stopClipboardWatchOwned()
	} else {
		a.refreshClipboardWatchOwned()
	}
}

func (a *App) SetClipboardPaused(paused bool) {
	a.setClipboardSuspended("user", paused)
	if a.background != nil && a.background.tray != nil {
		a.background.tray.SetClipboardPaused(paused)
	}
}
func (a *App) pauseClipboardWatch()  { a.setClipboardSuspended("sleep", true) }
func (a *App) resumeClipboardWatch() { a.setClipboardSuspended("sleep", false) }

func (a *App) closeClipboardOwner() {
	a.clipboardOwnerMu.Lock()
	defer a.clipboardOwnerMu.Unlock()
	a.clipboardMu.Lock()
	a.clipboardClosed = true
	a.clipboardLast = nativeclipboard.Change{}
	a.clipboardMu.Unlock()
	if a.core != nil {
		a.core.ResetClipboard(true, nativeclipboard.Generation())
	}
	a.stopClipboardWatchOwned()
}
