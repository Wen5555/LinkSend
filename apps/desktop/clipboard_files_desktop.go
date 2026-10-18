package main

import (
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

func (a *App) CancelClipboardFile(id string) {
	if a.core != nil {
		a.core.CancelClipboardFile(id)
	}
}

func (a *App) clipboardFilePopupNeeded() bool {
	if a.core == nil {
		return false
	}
	for _, progress := range a.core.ClipboardFileProgresses() {
		if progress.Large && progress.State != "completed" && progress.State != "cancelled" {
			return true
		}
	}
	return false
}

func (a *App) refreshClipboardFileWindow() {
	if a.runtimeApp == nil || a.ctx == nil || a.ctx.Err() != nil {
		return
	}
	wanted := a.clipboardFilePopupNeeded()
	a.clipboardProgressMu.Lock()
	if a.clipboardProgressQueued || wanted == a.clipboardProgressShown {
		a.clipboardProgressMu.Unlock()
		return
	}
	a.clipboardProgressQueued = true
	a.clipboardProgressMu.Unlock()
	application.InvokeAsync(func() {
		a.clipboardProgressMu.Lock()
		defer a.clipboardProgressMu.Unlock()
		a.clipboardProgressQueued = false
		if a.ctx.Err() != nil {
			return
		}
		show := a.clipboardFilePopupNeeded()
		if show && a.clipboardProgressWindow == nil {
			window := a.runtimeApp.Window.NewWithOptions(application.WebviewWindowOptions{Name: "clipboard-copy", Title: "正在复制", Width: 440, Height: 240, DisableResize: true, AlwaysOnTop: true, Hidden: true, InitialPosition: application.WindowCentered, URL: "/?clipboard-progress=1", BackgroundColour: application.NewRGB(247, 249, 251)})
			window.OnWindowEvent(events.Common.WindowClosing, func(event *application.WindowEvent) {
				event.Cancel()
				for _, progress := range a.core.ClipboardFileProgresses() {
					if progress.Large {
						a.core.CancelClipboardFile(progress.ID)
					}
				}
				window.Hide()
				a.clipboardProgressMu.Lock()
				a.clipboardProgressShown = false
				a.clipboardProgressMu.Unlock()
			})
			a.clipboardProgressWindow = window
		}
		if a.clipboardProgressWindow != nil {
			if show {
				a.clipboardProgressWindow.Center()
				a.clipboardProgressWindow.Show()
			} else {
				a.clipboardProgressWindow.Hide()
			}
		}
		a.clipboardProgressShown = show
	})
}
