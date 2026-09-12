package main

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	coreapp "github.com/Wen5555/LinkSend/internal/app"
	"github.com/wailsapp/wails/v3/pkg/application"
)

type DesktopEntryStatus struct {
	Revision        uint64 `json:"revision"`
	Error           string `json:"error,omitempty"`
	DraftRevision   uint64 `json:"draft_revision"`
	SendToSupported bool   `json:"send_to_supported"`
	FinderServices  bool   `json:"finder_services"`
}

type desktopEntries struct {
	mu      sync.Mutex
	wake    chan struct{}
	done    chan struct{}
	status  DesktopEntryStatus
	cleanup []func()
	closed  bool
}

func desktopProfileDirectory() string {
	if directory := os.Getenv("LINKSEND_DATA_DIR"); directory != "" {
		return directory
	}
	if root, err := os.UserConfigDir(); err == nil {
		return filepath.Join(root, "LinkSend")
	}
	return filepath.Join(os.TempDir(), "LinkSend")
}

func (a *App) nativeEntryError(err error) {
	a.entries.mu.Lock()
	defer a.entries.mu.Unlock()
	message := ""
	if err != nil {
		message = "系统入口尚未保存到草稿，已落盘的入口会保留并重试。请检查磁盘、草稿上限和本机配置。"
	}
	if a.entries.status.Error != message {
		a.entries.status.Error = message
		a.entries.status.Revision++
	}
}

func (a *App) DesktopEntries() DesktopEntryStatus {
	a.entries.mu.Lock()
	defer a.entries.mu.Unlock()
	return a.entries.status
}

func (a *App) wakeEntries() {
	select {
	case a.entries.wake <- struct{}{}:
	default:
	}
}

func (a *App) receiveNativePaths(paths []string, workingDir string) error {
	a.entries.mu.Lock()
	closed := a.entries.closed
	a.entries.mu.Unlock()
	if closed {
		return errors.New("SHUTTING_DOWN: system entry is closing")
	}
	err := stageFileActivation(a.dataDir, paths, workingDir)
	a.nativeEntryError(err)
	if err == nil {
		a.wakeEntries()
	}
	return err
}

func (a *App) startNativeEntries() {
	if a.entries == nil || a.core == nil {
		return
	}
	a.entries.done = make(chan struct{})
	go func() {
		defer close(a.entries.done)
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			activate := false
			select {
			case <-a.ctx.Done():
				return
			case <-a.entries.wake:
				activate = true
			case <-ticker.C:
			}
			if a.ctx.Err() != nil {
				return
			}
			var draft coreapp.SendDraft
			count, err := consumeFileActivations(a.dataDir, func(paths []string) error {
				if err := a.workspaceAvailable(); err != nil {
					return err
				}
				var err error
				draft, err = a.core.MergeDraftPaths(paths, "")
				return err
			})
			if err != nil || count > 0 {
				a.nativeEntryError(err)
			}
			if count > 0 {
				a.entries.mu.Lock()
				a.entries.status.DraftRevision = draft.Revision
				a.entries.status.Revision++
				a.entries.mu.Unlock()
			}
			if count > 0 || activate {
				a.showEntryWindow()
			}
		}
	}()
}

func (a *App) showEntryWindow() {
	if a.entries != nil {
		a.entries.mu.Lock()
		closed := a.entries.closed
		a.entries.mu.Unlock()
		if closed {
			return
		}
	}
	if a.window != nil {
		a.window.Show()
		a.window.Focus()
	}
}

func (a *App) registerNativeEntries(window *application.WebviewWindow) {
	cleanup := attachFileDrop(window, func(paths []string, dir string) { _ = a.receiveNativePaths(paths, dir) })
	a.entries.mu.Lock()
	a.entries.cleanup = append(a.entries.cleanup, cleanup)
	a.entries.mu.Unlock()
}

func (a *App) runtimeEntriesReady() {
	if runtime.GOOS != "darwin" {
		return
	}
	a.entries.mu.Lock()
	if a.entries.closed || a.entries.status.FinderServices {
		a.entries.mu.Unlock()
		return
	}
	a.entries.mu.Unlock()
	cleanup, err := registerFinderServices(a.receiveNativePaths)
	a.nativeEntryError(err)
	if err != nil {
		return
	}
	a.entries.mu.Lock()
	a.entries.status.FinderServices = true
	a.entries.cleanup = append(a.entries.cleanup, cleanup)
	a.entries.mu.Unlock()
}

// Called while the native loop is still alive. Stop ingress before saving core
// state; a racing second process leaves its journal record for the next launch.
func (a *App) closeNativeEntries() {
	if a.entries == nil {
		return
	}
	a.entries.mu.Lock()
	a.entries.closed = true
	cleanup := a.entries.cleanup
	a.entries.cleanup = nil
	a.entries.mu.Unlock()
	for _, close := range cleanup {
		close()
	}
}

func (a *App) ConfigureSendTo(enabled bool) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if enabled {
		return installSendTo(exe)
	}
	return uninstallSendTo(exe)
}

// Setup-only arguments run before application.New and never acquire the core
// profile, so an installer can remove its own SendTo entry during app shutdown.
func runDesktopSetup(args []string) (bool, error) {
	if len(args) != 1 {
		return false, nil
	}
	if args[0] != "--install-sendto" && args[0] != "--uninstall-sendto" && args[0] != "--uninstall-autostart" {
		return false, nil
	}
	exe, err := os.Executable()
	if err != nil {
		return true, err
	}
	if args[0] == "--install-sendto" {
		return true, installSendTo(exe)
	}
	if args[0] == "--uninstall-autostart" {
		return true, uninstallNativeAutostart(exe)
	}
	return true, uninstallSendTo(exe)
}
