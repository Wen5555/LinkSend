package main

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	coreapp "github.com/Wen5555/LinkSend/internal/app"
	"github.com/wailsapp/wails/v3/pkg/application"
)

type DesktopEntryStatus struct {
	Revision        uint64               `json:"revision"`
	Error           string               `json:"error,omitempty"`
	DraftRevision   uint64               `json:"draft_revision"`
	SendToSupported bool                 `json:"send_to_supported"`
	FinderServices  bool                 `json:"finder_services"`
	PendingShares   []NativeSharePending `json:"pending_shares,omitempty"`
}

type NativeSharePending struct {
	RequestID string `json:"request_id"`
	PeerID    string `json:"peer_id"`
}

type desktopEntries struct {
	mu        sync.Mutex
	consumeMu sync.Mutex
	wake      chan bool
	done      chan struct{}
	status    DesktopEntryStatus
	cleanup   []func()
	closed    bool
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

func (a *App) wakeEntries(showWindow ...bool) {
	show := len(showWindow) == 0 || showWindow[0]
	select {
	case a.entries.wake <- show:
	default:
		if show {
			select {
			case <-a.entries.wake:
			default:
			}
			select {
			case a.entries.wake <- true:
			default:
			}
		}
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
		lastDevicePublish := time.Time{}
		for {
			activate := false
			select {
			case <-a.ctx.Done():
				return
			case activate = <-a.entries.wake:
			case <-ticker.C:
			}
			if a.ctx.Err() != nil {
				return
			}
			if time.Since(lastDevicePublish) >= 15*time.Second {
				if devices, err := a.core.Devices(a.ctx); err == nil {
					if publishErr := publishNativeShareDevices(a.dataDir, devices); publishErr != nil {
						a.nativeEntryError(publishErr)
					}
					lastDevicePublish = time.Now()
				}
			}
			var draft coreapp.SendDraft
			showDraft := false
			count := 0
			var consumeErr error
			pending := make(map[string]NativeSharePending)
			a.entries.consumeMu.Lock()
			if workspace, err := a.core.Workspace(); err == nil {
				terminal := make(map[string]bool)
				for _, item := range workspace.Queue {
					switch item.State {
					case "completed", "cancelled", "expired":
						terminal[item.RequestID] = true
					}
				}
				for _, root := range nativeShareRoots(a.dataDir) {
					_, _ = reapShareActivations(root, terminal)
				}
			}
			consume := func(activation fileActivation) error {
				if err := a.workspaceAvailable(); err != nil {
					return err
				}
				if activation.Version == 2 {
					resolved, err := resolveNativeShareActivation(activation)
					if err != nil {
						pending[activation.RequestID] = NativeSharePending{RequestID: activation.RequestID, PeerID: activation.PeerID}
						return err
					}
					_, err = a.core.Enqueue(coreapp.EnqueueRequest{RequestID: resolved.RequestID, PeerID: resolved.PeerID, Paths: resolved.Paths, WaitForPeer: resolved.WaitForPeer})
					if err != nil {
						pending[activation.RequestID] = NativeSharePending{RequestID: activation.RequestID, PeerID: activation.PeerID}
						return err
					}
					for _, root := range nativeShareRoots(a.dataDir) {
						if err := publishNativeShareReceipt(root, resolved.RequestID); err != nil {
							return err
						}
					}
					return errActivationRetained
				}
				showDraft = true
				var mergeErr error
				draft, mergeErr = a.core.MergeDraftPaths(activation.Paths, "")
				return mergeErr
			}
			for _, root := range nativeShareRoots(a.dataDir) {
				consumed, err := consumeActivations(root, consume)
				count += consumed
				if err != nil && consumeErr == nil {
					consumeErr = err
				}
			}
			a.entries.consumeMu.Unlock()
			if consumeErr != nil || count > 0 {
				a.nativeEntryError(consumeErr)
			}
			nextPending := make([]NativeSharePending, 0, len(pending))
			for _, item := range pending {
				nextPending = append(nextPending, item)
			}
			slices.SortFunc(nextPending, func(a, b NativeSharePending) int { return strings.Compare(a.RequestID, b.RequestID) })
			a.entries.mu.Lock()
			if !slices.Equal(a.entries.status.PendingShares, nextPending) {
				a.entries.status.PendingShares = nextPending
				a.entries.status.Revision++
			}
			a.entries.mu.Unlock()
			if count > 0 && showDraft {
				a.entries.mu.Lock()
				a.entries.status.DraftRevision = draft.Revision
				a.entries.status.Revision++
				a.entries.mu.Unlock()
			}
			if count > 0 && showDraft || activate {
				a.showEntryWindow()
			}
		}
	}()
}

func (a *App) DiscardNativeShare(requestID string) error {
	if !activationName.MatchString(requestID + ".json") {
		return errors.New("SYSTEM_SHARE_INVALID: request ID")
	}
	a.entries.consumeMu.Lock()
	defer a.entries.consumeMu.Unlock()
	workspace, err := a.core.Workspace()
	if err != nil {
		return err
	}
	for _, item := range workspace.Queue {
		if item.RequestID == requestID {
			return errors.New("SYSTEM_SHARE_ALREADY_QUEUED: 请在发送队列中取消该任务")
		}
	}
	if err := discardNativeShare(a.dataDir, requestID); err != nil {
		return err
	}
	a.wakeEntries(false)
	return nil
}

func discardNativeShare(dataDir, requestID string) error {
	removed := 0
	for _, root := range nativeShareRoots(dataDir) {
		n, err := reapShareActivations(root, map[string]bool{requestID: true})
		if err != nil {
			return err
		}
		removed += n
	}
	if removed == 0 {
		return errors.New("SYSTEM_SHARE_NOT_FOUND")
	}
	return nil
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
	closeNativeShareAccess()
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
