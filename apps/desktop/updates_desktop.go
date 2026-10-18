package main

import (
	"context"
	"os"
	"path/filepath"
	"runtime"

	"github.com/Wen5555/LinkSend/apps/desktop/updater"
	"github.com/Wen5555/LinkSend/internal/protocol"
)

func (a *App) desktopUpdater() *updater.Manager {
	a.updateMu.Lock()
	defer a.updateMu.Unlock()
	if a.updates == nil {
		ctx := a.ctx
		if ctx == nil {
			ctx = context.Background()
		}
		cache, err := os.UserCacheDir()
		if err == nil {
			cache = filepath.Join(cache, "LinkSend", "updates")
		}
		a.updates = updater.New(ctx, protocol.ProductVersion, runtime.GOOS, runtime.GOARCH, cache)
	}
	return a.updates
}

func (a *App) UpdateStatus() updater.Status    { return a.desktopUpdater().Status() }
func (a *App) CheckForUpdates() updater.Status { return a.desktopUpdater().Check() }
func (a *App) DownloadUpdate() updater.Status  { return a.desktopUpdater().Download() }
func (a *App) CancelUpdate() updater.Status    { return a.desktopUpdater().Cancel() }
func (a *App) OpenUpdatePackage() updater.Status {
	return a.desktopUpdater().OpenPackage(openNativeInboxFile)
}
func (a *App) OpenUpdateReleasePage() error {
	return openNativeInboxFile(a.desktopUpdater().Status().ReleaseURL)
}

func (a *App) closeUpdater() {
	a.updateMu.Lock()
	manager := a.updates
	a.updateMu.Unlock()
	if manager != nil {
		manager.Close()
	}
}
