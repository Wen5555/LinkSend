package main

import (
	"bytes"
	"errors"
	"image/png"
	"sync/atomic"

	"github.com/wailsapp/wails/v3/pkg/application"
)

type nativeTrayCallbacks struct {
	Show       func()
	Inbox      func()
	PauseQueue func(bool)
	Quit       func()
}

type nativeTray struct {
	tray      *application.SystemTray
	menu      *application.Menu
	pauseItem *application.MenuItem
	paused    atomic.Bool
	closed    atomic.Bool
	callbacks nativeTrayCallbacks
}

// Create before host.Run; Wails defers the real tray until its native loop is
// ready. iconPNG must be an application-owned PNG asset, not a user file path.
func newNativeTray(host *application.App, iconPNG []byte, callbacks nativeTrayCallbacks) (*nativeTray, error) {
	if host == nil || len(iconPNG) == 0 || len(iconPNG) > 1<<20 {
		return nil, errors.New("TRAY_ICON_REQUIRED")
	}
	config, err := png.DecodeConfig(bytes.NewReader(iconPNG))
	if err != nil || config.Width > 1024 || config.Height > 1024 {
		return nil, errors.New("TRAY_ICON_INVALID")
	}
	handle := &nativeTray{tray: host.SystemTray.New(), menu: application.NewMenu(), callbacks: callbacks}
	handle.tray.SetIcon(append([]byte(nil), iconPNG...))
	handle.tray.SetTooltip("LinkSend")
	handle.menu.Add("显示 LinkSend").OnClick(func(*application.Context) { handle.invoke(callbacks.Show) })
	handle.menu.Add("打开收件箱").OnClick(func(*application.Context) { handle.invoke(callbacks.Inbox) })
	handle.menu.AddSeparator()
	// A normal menu item deliberately does not toggle a native checkbox before
	// the core has committed its queue pause command.
	handle.pauseItem = handle.menu.Add("暂停队列派发").OnClick(func(*application.Context) {
		if !handle.closed.Load() && callbacks.PauseQueue != nil {
			callbacks.PauseQueue(!handle.paused.Load())
		}
	})
	handle.menu.AddSeparator()
	handle.menu.Add("退出应用").OnClick(func(*application.Context) { handle.invoke(callbacks.Quit) })
	handle.tray.SetMenu(handle.menu)
	handle.tray.OnClick(func() { handle.invoke(callbacks.Show) })
	host.OnShutdown(handle.Close)
	return handle, nil
}

func (h *nativeTray) invoke(callback func()) {
	if !h.closed.Load() && callback != nil {
		callback()
	}
}

// Call only after the Go queue command has committed. This can run before or
// after host.Run: updates are dispatched without waiting on a not-yet-live loop.
func (h *nativeTray) SetQueuePaused(paused bool) {
	if h.closed.Load() {
		return
	}
	h.paused.Store(paused)
	application.InvokeAsync(func() {
		if h.closed.Load() {
			return
		}
		label := "暂停队列派发"
		if h.paused.Load() {
			label = "继续队列派发"
		}
		h.pauseItem.SetLabel(label)
	})
}

// Close while the native event loop is alive; App.OnShutdown is the fallback.
// Removing menu handlers and the native icon does not terminate the Go core.
func (h *nativeTray) Close() {
	if !h.closed.CompareAndSwap(false, true) {
		return
	}
	application.InvokeSync(func() {
		h.tray.OnClick(func() {})
		h.tray.Destroy()
		h.menu.Destroy()
	})
}
