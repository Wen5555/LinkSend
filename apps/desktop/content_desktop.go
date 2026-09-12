package main

import (
	"context"
	"errors"

	"github.com/Wen5555/LinkSend/apps/desktop/nativeclipboard"
	coreapp "github.com/Wen5555/LinkSend/internal/app"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// The host calls this after attaching the real runtime and enabling these
// actions in its UI. It is unbound and requires no new App field.
func (a *App) initializeContentActions() error {
	if err := a.workspaceAvailable(); err != nil {
		return err
	}
	if a.runtimeApp == nil {
		return errors.New("NATIVE_RUNTIME_UNAVAILABLE")
	}
	a.core.EnableNativeContentActions()
	return nil
}

func (a *App) contentContext() context.Context {
	if a.ctx != nil {
		return a.ctx
	}
	return context.Background()
}

func (a *App) CreateContentText(request coreapp.ContentTextRequest) (coreapp.ContentDraft, error) {
	if err := a.workspaceAvailable(); err != nil {
		return coreapp.ContentDraft{}, err
	}
	return a.core.CreateContentText(a.contentContext(), request)
}
func (a *App) CaptureClipboardImage(requestID string) (coreapp.ContentDraft, error) {
	if err := a.workspaceAvailable(); err != nil {
		return coreapp.ContentDraft{}, err
	}
	return a.core.CreateClipboardImage(a.contentContext(), requestID, nativeclipboard.CaptureImage)
}
func (a *App) ContentDrafts() ([]coreapp.ContentDraft, error) {
	if err := a.workspaceAvailable(); err != nil {
		return nil, err
	}
	return a.core.ContentDrafts(a.contentContext())
}
func (a *App) DiscardContentDraft(id string, revision uint64) error {
	if err := a.workspaceAvailable(); err != nil {
		return err
	}
	return a.core.DiscardContentDraft(a.contentContext(), id, revision)
}
func (a *App) EnqueueContent(request coreapp.EnqueueContentRequest) (coreapp.QueueItem, error) {
	if err := a.workspaceAvailable(); err != nil {
		return coreapp.QueueItem{}, err
	}
	return a.core.EnqueueContent(a.contentContext(), request)
}
func (a *App) ContentTask(taskID string) (coreapp.ContentTaskInfo, error) {
	if err := a.workspaceAvailable(); err != nil {
		return coreapp.ContentTaskInfo{}, err
	}
	return a.core.ContentTask(a.contentContext(), taskID)
}
func (a *App) ContentSettings() (coreapp.ContentSettings, error) {
	if err := a.workspaceAvailable(); err != nil {
		return coreapp.ContentSettings{}, err
	}
	return a.core.ContentSettings()
}
func (a *App) SetContentSettings(settings coreapp.ContentSettings) error {
	if err := a.workspaceAvailable(); err != nil {
		return err
	}
	return a.core.SetContentSettings(settings)
}
func (a *App) CleanupContentSnapshots() (coreapp.ContentCleanupResult, error) {
	if err := a.workspaceAvailable(); err != nil {
		return coreapp.ContentCleanupResult{}, err
	}
	return a.core.CleanupContentSnapshots(a.contentContext())
}
func (a *App) nativeContentAvailable() error {
	if err := a.workspaceAvailable(); err != nil {
		return err
	}
	if a.runtimeApp == nil {
		return errors.New("NATIVE_RUNTIME_UNAVAILABLE")
	}
	return nil
}
func (a *App) PreviewReceivedText(taskID string) (coreapp.ContentActionResult, error) {
	if err := a.nativeContentAvailable(); err != nil {
		return coreapp.ContentActionResult{}, err
	}
	return a.core.PreviewReceivedText(a.contentContext(), taskID, func(value string) error {
		a.runtimeApp.Dialog.Info().SetTitle("收到的文字").SetMessage(value).Show()
		return nil
	})
}
func (a *App) CopyReceivedText(taskID string) (coreapp.ContentActionResult, error) {
	if err := a.nativeContentAvailable(); err != nil {
		return coreapp.ContentActionResult{}, err
	}
	return a.core.CopyReceivedText(a.contentContext(), taskID, func(value string) error {
		// The manager lazily creates its clipboard. Include that creation in
		// the main-thread dispatch, so concurrent actions cannot race it.
		if !application.InvokeSyncWithResult(func() bool { return a.runtimeApp.Clipboard.SetText(value) }) {
			return errors.New("CLIPBOARD_WRITE_FAILED")
		}
		return nil
	})
}
func (a *App) OpenReceivedURL(taskID string) (coreapp.ContentActionResult, error) {
	if err := a.nativeContentAvailable(); err != nil {
		return coreapp.ContentActionResult{}, err
	}
	return a.core.OpenReceivedURL(a.contentContext(), taskID, a.runtimeApp.Browser.OpenURL)
}
func (a *App) SaveReceivedImage(taskID string) (coreapp.ContentActionResult, error) {
	if err := a.nativeContentAvailable(); err != nil {
		return coreapp.ContentActionResult{}, err
	}
	info, err := a.core.ContentTask(a.contentContext(), taskID)
	if err != nil {
		return coreapp.ContentActionResult{}, err
	}
	if !info.CanSave {
		return coreapp.ContentActionResult{}, errors.New("CONTENT_IMAGE_NOT_AVAILABLE")
	}
	path, err := a.runtimeApp.Dialog.SaveFile().SetFilename("LinkSend-image.png").AddFilter("PNG 图片", "*.png").PromptForSingleSelection()
	if err != nil {
		return coreapp.ContentActionResult{}, err
	}
	if path == "" {
		return coreapp.ContentActionResult{TaskID: taskID, Action: "save_image", State: "cancelled"}, nil
	}
	return a.core.SaveReceivedImage(a.contentContext(), taskID, path)
}
