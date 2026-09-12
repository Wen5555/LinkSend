package main

import (
	"os"
	"time"

	coreapp "github.com/Wen5555/LinkSend/internal/app"
	"github.com/wailsapp/wails/v3/pkg/application"
)

func init() { application.RegisterEvent[coreapp.WorkspaceChange]("workspace:changed") }

func (a *App) workspaceAvailable() error {
	if err := a.ensureConfig(); err != nil {
		return err
	}
	if a.core == nil {
		if a.initErr != nil {
			return a.initErr
		}
		return errBackendUnavailable
	}
	return nil
}

func (a *App) Workspace() (coreapp.WorkspaceSnapshot, error) {
	if err := a.workspaceAvailable(); err != nil {
		return coreapp.WorkspaceSnapshot{}, err
	}
	return a.core.Workspace()
}
func (a *App) SaveDraft(draft coreapp.SendDraft) (coreapp.SendDraft, error) {
	if err := a.workspaceAvailable(); err != nil {
		return coreapp.SendDraft{}, err
	}
	dir, err := os.Getwd()
	if err != nil {
		return coreapp.SendDraft{}, err
	}
	return a.core.SaveDraft(draft, dir)
}
func (a *App) SaveDeviceProfile(profile coreapp.DeviceProfile) (coreapp.DeviceProfile, error) {
	if err := a.workspaceAvailable(); err != nil {
		return coreapp.DeviceProfile{}, err
	}
	return a.core.SaveDeviceProfile(profile)
}
func (a *App) Enqueue(request coreapp.EnqueueRequest) (coreapp.QueueItem, error) {
	if err := a.workspaceAvailable(); err != nil {
		return coreapp.QueueItem{}, err
	}
	return a.core.Enqueue(request)
}
func (a *App) CancelQueue(id string, revision uint64) error {
	if err := a.workspaceAvailable(); err != nil {
		return err
	}
	return a.core.CancelQueue(id, revision)
}
func (a *App) ConfirmQueue(id string, revision uint64) error {
	if err := a.workspaceAvailable(); err != nil {
		return err
	}
	return a.core.ConfirmQueue(id, revision)
}
func (a *App) ReorderQueue(ids []string) error {
	if err := a.workspaceAvailable(); err != nil {
		return err
	}
	return a.core.ReorderQueue(ids)
}
func (a *App) SetQueuePaused(paused bool) error {
	if err := a.workspaceAvailable(); err != nil {
		return err
	}
	return a.core.SetQueuePaused(paused)
}
func (a *App) BlockPeer(id string) error {
	if err := a.workspaceAvailable(); err != nil {
		return err
	}
	return a.core.BlockPeer(id)
}
func (a *App) UnblockPeer(id string) error {
	if err := a.workspaceAvailable(); err != nil {
		return err
	}
	return a.core.UnblockPeer(id)
}

func (a *App) startWorkspaceEvents() {
	if a.core == nil || a.runtimeApp == nil {
		return
	}
	changes, unsubscribe := a.core.SubscribeChanges()
	a.eventsDone = make(chan struct{})
	go func() {
		defer close(a.eventsDone)
		defer unsubscribe()
		ticker := time.NewTicker(200 * time.Millisecond)
		defer ticker.Stop()
		pending := false
		for {
			select {
			case <-a.ctx.Done():
				return
			case _, ok := <-changes:
				if !ok {
					return
				}
				pending = true
			case <-ticker.C:
				if pending {
					pending = false
					a.runtimeApp.Event.Emit("workspace:changed", a.core.WorkspaceChange())
				}
			}
		}
	}()
}
