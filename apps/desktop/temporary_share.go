package main

import (
	"errors"

	"github.com/Wen5555/LinkSend/apps/desktop/nativeclipboard"
	coreapp "github.com/Wen5555/LinkSend/internal/app"
	"github.com/Wen5555/LinkSend/internal/clipboardsync"
	"github.com/Wen5555/LinkSend/internal/transfer"
	"github.com/wailsapp/wails/v3/pkg/application"
)

func (a *App) TemporaryShares() ([]coreapp.TemporaryShareSnapshot, error) {
	if err := a.workspaceAvailable(); err != nil {
		return nil, err
	}
	return a.core.TemporaryShares()
}

// The desktop passes only the already saved main-draft revision. File paths
// and source selection are reloaded in Go, not reconstructed by JavaScript.
func (a *App) CreateTemporaryShare(expectedDraftRevision uint64) (coreapp.TemporaryShareCreation, error) {
	if err := a.workspaceAvailable(); err != nil {
		return coreapp.TemporaryShareCreation{}, err
	}
	draft, err := a.core.SendDraft("main")
	if err != nil {
		return coreapp.TemporaryShareCreation{}, err
	}
	if draft.Revision != expectedDraftRevision || len(draft.Paths) == 0 {
		return coreapp.TemporaryShareCreation{}, coreapp.ErrMetadataConflict
	}
	return a.core.CreateTemporaryShare(a.contentContext(), draft.Paths, draft.SourceSelection)
}

func (a *App) ClaimTemporaryShare(code string) (coreapp.TemporaryShareSnapshot, error) {
	if err := a.workspaceAvailable(); err != nil {
		return coreapp.TemporaryShareSnapshot{}, err
	}
	return a.core.ClaimTemporaryShare(a.contentContext(), code)
}

func (a *App) RefreshTemporaryShare(id string) (coreapp.TemporaryShareSnapshot, error) {
	if err := a.workspaceAvailable(); err != nil {
		return coreapp.TemporaryShareSnapshot{}, err
	}
	return a.core.RefreshTemporaryShare(a.contentContext(), id)
}

func (a *App) ApproveTemporaryShare(id, claimID, receiverKey string, generation uint64) (coreapp.TemporaryShareSnapshot, error) {
	if err := a.workspaceAvailable(); err != nil {
		return coreapp.TemporaryShareSnapshot{}, err
	}
	return a.core.ApproveTemporaryShare(a.contentContext(), id, claimID, receiverKey, generation)
}

func (a *App) DenyTemporaryShare(id, claimID string, generation uint64) (coreapp.TemporaryShareSnapshot, error) {
	if err := a.workspaceAvailable(); err != nil {
		return coreapp.TemporaryShareSnapshot{}, err
	}
	return a.core.DenyTemporaryShare(a.contentContext(), id, claimID, generation)
}

func (a *App) StartTemporaryShare(id string) (coreapp.TaskSnapshot, error) {
	if err := a.workspaceAvailable(); err != nil {
		return coreapp.TaskSnapshot{}, err
	}
	return a.core.StartTemporaryShare(a.contentContext(), id, a.directConfig())
}

func (a *App) ConfirmTemporaryShareSave(id, directory, conflictPolicy string) (coreapp.TemporaryShareSnapshot, error) {
	if err := a.workspaceAvailable(); err != nil {
		return coreapp.TemporaryShareSnapshot{}, err
	}
	return a.core.ConfirmTemporaryShareSave(a.contentContext(), id, directory, transfer.PlanRequest{ConflictPolicy: transfer.ConflictPolicy(conflictPolicy)})
}

func (a *App) CancelTemporaryShare(id string) (coreapp.TemporaryShareSnapshot, error) {
	if err := a.workspaceAvailable(); err != nil {
		return coreapp.TemporaryShareSnapshot{}, err
	}
	return a.core.CancelTemporaryShare(a.contentContext(), id)
}

func (a *App) RevokeTemporaryShare(id string) (coreapp.TemporaryShareSnapshot, error) {
	if err := a.workspaceAvailable(); err != nil {
		return coreapp.TemporaryShareSnapshot{}, err
	}
	return a.core.RevokeTemporaryShare(a.contentContext(), id)
}

func (a *App) ResumeTemporaryShare(id string) (coreapp.TaskSnapshot, error) {
	if err := a.workspaceAvailable(); err != nil {
		return coreapp.TaskSnapshot{}, err
	}
	return a.core.ResumeTemporaryShare(a.contentContext(), id, a.directConfig())
}

func (a *App) RetryTemporaryShareAction(id string) (coreapp.TemporaryShareSnapshot, error) {
	if err := a.workspaceAvailable(); err != nil {
		return coreapp.TemporaryShareSnapshot{}, err
	}
	return a.core.RetryTemporaryShareAction(a.contentContext(), id)
}

func (a *App) RetryTemporaryShareClaim(id, code string) (coreapp.TemporaryShareSnapshot, error) {
	if err := a.workspaceAvailable(); err != nil {
		return coreapp.TemporaryShareSnapshot{}, err
	}
	return a.core.RetryTemporaryShareClaim(a.contentContext(), id, code)
}

func (a *App) CopyTemporaryShareCode(id, code string) error {
	if err := a.workspaceAvailable(); err != nil {
		return err
	}
	if a.runtimeApp == nil {
		return errors.New("NATIVE_RUNTIME_UNAVAILABLE")
	}
	return a.copyTemporaryShareCode(id, code, func(value string) bool {
		return application.InvokeSyncWithResult(func() bool { return a.runtimeApp.Clipboard.SetText(value) })
	}, nativeclipboard.Generation)
}

// Native writes and actual watcher delivery use the same owner lock. A native
// callback only queues generation metadata; it must not take this lock while
// SetText is running on the UI thread. The secret never becomes a sync payload.
func (a *App) copyTemporaryShareCode(id, code string, write func(string) bool, generation func() uint64) error {
	a.clipboardOwnerMu.Lock()
	defer a.clipboardOwnerMu.Unlock()
	if err := a.core.CheckTemporaryShareCode(id, code); err != nil {
		return err
	}
	a.clipboardMu.Lock()
	closed := a.clipboardClosed
	a.clipboardMu.Unlock()
	if closed {
		return errors.New("APP_CLOSING")
	}
	if !write(code) {
		return errors.New("CLIPBOARD_WRITE_FAILED")
	}
	actual := generation()
	if actual == 0 {
		return errors.New("CLIPBOARD_UNAVAILABLE")
	}
	err := a.core.ClipboardChanged(a.contentContext(), coreapp.ClipboardChange{Generation: actual})
	// Paused delivery already ignores this generation. Resuming uses the real
	// current OS baseline, so it cannot turn this secret into a future copy.
	if errors.Is(err, clipboardsync.ErrStale) {
		return nil
	}
	return err
}
