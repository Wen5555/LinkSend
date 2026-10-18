package main

import (
	"errors"
	"os"

	coreapp "github.com/Wen5555/LinkSend/internal/app"
	"github.com/wailsapp/wails/v3/pkg/application"
)

func init() { application.RegisterEvent[uint64]("quick-send:open") }

const quickSendDraftID = coreapp.QuickSendDraftID

// Revision is a request to show one panel in the existing process/window. It
// does not enqueue work and does not mutate either persistent draft.
func (a *App) OpenQuickSend() error {
	if err := a.workspaceAvailable(); err != nil {
		return err
	}
	if a.background == nil {
		return errors.New("DESKTOP_NOT_READY")
	}
	a.background.mu.Lock()
	if a.background.closed || a.background.status.QuickSendRevision == ^uint64(0) {
		a.background.mu.Unlock()
		return errors.New("APP_CLOSING")
	}
	a.background.status.QuickSendRevision++
	revision := a.background.status.QuickSendRevision
	a.background.mu.Unlock()
	a.showEntryWindow()
	if a.runtimeApp != nil {
		a.runtimeApp.Event.Emit("quick-send:open", revision)
	}
	return nil
}

func (a *App) QuickSendDraft() (coreapp.SendDraft, error) {
	if err := a.workspaceAvailable(); err != nil {
		return coreapp.SendDraft{}, err
	}
	return a.core.SendDraft(quickSendDraftID)
}

func (a *App) SaveQuickSendDraft(draft coreapp.SendDraft) (coreapp.SendDraft, error) {
	if err := a.workspaceAvailable(); err != nil {
		return coreapp.SendDraft{}, err
	}
	if draft.ID != quickSendDraftID || len(draft.PeerIDs) != 0 || draft.SourceSelection != nil {
		return coreapp.SendDraft{}, errors.New("INVALID_ARGUMENT: quick send uses its own single-target full-content draft")
	}
	dir, err := os.Getwd()
	if err != nil {
		return coreapp.SendDraft{}, err
	}
	return a.core.SaveDraft(draft, dir)
}

func (a *App) EnqueueQuickSend(revision uint64, waitForPeer bool) (coreapp.QueueItem, error) {
	if err := a.workspaceAvailable(); err != nil {
		return coreapp.QueueItem{}, err
	}
	return a.core.EnqueueQuickSend(revision, waitForPeer)
}
