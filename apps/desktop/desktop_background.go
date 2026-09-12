package main

import (
	"context"
	"errors"
	"os"
	"sync"
	"time"

	coreapp "github.com/Wen5555/LinkSend/internal/app"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

type BackgroundOptions struct {
	CloseMode     string `json:"close_mode"` // empty = ask once, exit, background
	Notifications bool   `json:"notifications"`
	PreventSleep  bool   `json:"prevent_sleep"`
}

type BackgroundStatus struct {
	Options                BackgroundOptions `json:"options"`
	TrayAvailable          bool              `json:"tray_available"`
	SleepInhibited         bool              `json:"sleep_inhibited"`
	NotificationPermission string            `json:"notification_permission"`
	LastNotification       string            `json:"last_notification"`
	Error                  string            `json:"error,omitempty"`
	NavigationRevision     uint64            `json:"navigation_revision"`
	TaskID                 string            `json:"task_id,omitempty"`
	Autostart              bool              `json:"autostart"`
}

type desktopBackground struct {
	mu       sync.Mutex
	tray     *nativeTray
	notifier *nativeNotifier
	sleep    *nativeSleepInhibitor
	status   BackgroundStatus
	tracker  backgroundTaskTracker
	cancel   context.CancelFunc
	done     chan struct{}
	cleanup  []func()
	sleeping bool
	closed   bool
}

type backgroundTaskTracker struct {
	started time.Time
	last    time.Time
	active  map[string]string
}

func (t *backgroundTaskTracker) observe(tasks []coreapp.TaskSnapshot, now time.Time) ([]nativeNotification, bool) {
	if t.active == nil {
		t.active = make(map[string]string)
	}
	next := make(map[string]string)
	notes := make([]nativeNotification, 0)
	transferring := false
	for _, task := range tasks {
		previous := t.active[task.ID]
		key := task.AttemptID + ":" + task.State
		if task.State == "transferring" {
			transferring = true
		}
		if !terminalTaskState(task.State) || task.State == "completed" && !task.BilateralConfirmed {
			if len(next) < 4096 {
				next[task.ID] = key
			}
		}
		if task.Direction == "receive" && task.State == "awaiting_acceptance" && previous != key {
			notes = append(notes, nativeNotification{TaskID: task.ID, Revision: task.Revision, Title: "收到接收请求", Body: "请在 LinkSend 中核对发送者和内容后确认。"})
			continue
		}
		if !terminalTaskState(task.State) || task.State == "completed" && !task.BilateralConfirmed {
			continue
		}
		ended, err := time.Parse(time.RFC3339Nano, task.EndedAt)
		started, _ := time.Parse(time.RFC3339Nano, task.StartedAt)
		if err != nil || ended.After(now) || (!ended.After(t.last) && previous == "") || (started.Before(t.started) && previous == "") {
			continue
		}
		title := "传输任务已结束"
		if task.State == "completed" {
			title = "传输已完成"
		}
		if task.State == "failed" {
			title = "传输未完成"
		}
		if task.State == "no_content" {
			title = "本次未接收内容"
		}
		notes = append(notes, nativeNotification{TaskID: task.ID, Revision: task.Revision, Title: title, Body: "点击在 LinkSend 中查看任务详情。"})
	}
	t.active = next
	t.last = now
	return notes, transferring
}

func (a *App) attachBackground(host *application.App, window *application.WebviewWindow, icon []byte) {
	b := a.background
	b.notifier = newNativeNotifier(a.navigateToTask)
	b.sleep = newNativeSleepInhibitor()
	b.tray, _ = newNativeTray(host, icon, nativeTrayCallbacks{
		Show: a.showEntryWindow, Inbox: func() { a.navigateToTask("") },
		PauseQueue: func(paused bool) {
			go func() {
				if err := a.SetQueuePaused(paused); err != nil {
					a.backgroundError("队列状态未保存，请在窗口中重试。")
				}
			}()
		},
		Quit: func() { host.Quit() },
	})
	b.status.TrayAvailable = b.tray != nil
	b.cleanup = append(b.cleanup, window.RegisterHook(events.Common.WindowClosing, a.onWindowClosing))
	b.cleanup = append(b.cleanup, host.Event.OnApplicationEvent(events.Common.SystemWillSleep, func(*application.ApplicationEvent) {
		b.mu.Lock()
		b.sleeping = true
		b.mu.Unlock()
		_ = b.sleep.Release()
	}))
	b.cleanup = append(b.cleanup, host.Event.OnApplicationEvent(events.Common.SystemDidWake, func(*application.ApplicationEvent) {
		b.mu.Lock()
		b.sleeping = false
		b.mu.Unlock()
	}))
}

func (a *App) startBackground() {
	b := a.background
	if b == nil || b.notifier == nil || a.core == nil {
		return
	}
	if a.Preferences().Background.Notifications {
		_ = b.notifier.ServiceStartup(a.ctx, application.ServiceOptions{})
	}
	now := time.Now()
	b.tracker = backgroundTaskTracker{started: now, last: now}
	// Seed existing tasks without issuing notifications for restored history.
	_, _ = b.tracker.observe(a.core.Tasks(), now)
	ctx, cancel := context.WithCancel(a.ctx)
	b.cancel = cancel
	b.done = make(chan struct{})
	go func() {
		defer close(b.done)
		defer b.sleep.Release()
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
			if ctx.Err() != nil {
				return
			}
			now := time.Now()
			notes, active := b.tracker.observe(a.core.Tasks(), now)
			options := a.Preferences().Background
			b.mu.Lock()
			sleeping := b.sleeping
			b.mu.Unlock()
			if active && options.PreventSleep && !sleeping {
				if err := b.sleep.Acquire(); err != nil {
					a.backgroundError("无法阻止系统自动睡眠；传输仍可继续。")
				}
			} else {
				_ = b.sleep.Release()
			}
			if options.Notifications {
				_ = b.notifier.ServiceStartup(a.ctx, application.ServiceOptions{})
				for _, note := range notes {
					if ctx.Err() != nil {
						return
					}
					result, _ := b.notifier.notify(note)
					b.mu.Lock()
					b.status.LastNotification = result.State
					b.status.NotificationPermission = result.Permission
					b.mu.Unlock()
				}
			}
		}
	}()
}

func (a *App) backgroundError(message string) {
	a.background.mu.Lock()
	a.background.status.Error = message
	a.background.mu.Unlock()
}

func (a *App) Background() BackgroundStatus {
	b := a.background
	b.mu.Lock()
	status := b.status
	b.mu.Unlock()
	status.Options = a.Preferences().Background
	if b.sleep != nil {
		status.SleepInhibited = b.sleep.Active()
	}
	if b.notifier != nil {
		status.NotificationPermission = b.notifier.notificationStatus().Permission
	}
	if exe, err := os.Executable(); err == nil {
		status.Autostart, _ = nativeAutostartEnabled(exe)
	}
	return status
}

func (a *App) SetBackgroundOptions(options BackgroundOptions) error {
	a.prefsWriteMu.Lock()
	defer a.prefsWriteMu.Unlock()
	if options.CloseMode != "" && options.CloseMode != "exit" && options.CloseMode != "background" {
		return errors.New("INVALID_CONFIG: unknown close behavior")
	}
	if options.CloseMode == "background" && a.background.tray == nil {
		return errors.New("TRAY_UNAVAILABLE: 请保留窗口以继续接收")
	}
	prefs := a.Preferences()
	prefs.Background = options
	if err := a.savePreferencesLocked(prefs, false); err != nil {
		return err
	}
	if !options.PreventSleep && a.background.sleep != nil {
		_ = a.background.sleep.Release()
	}
	return nil
}

func (a *App) RequestNotificationPermission() BackgroundStatus {
	if a.background.notifier != nil {
		_ = a.background.notifier.ServiceStartup(a.ctx, application.ServiceOptions{})
		a.background.notifier.requestNotificationPermission()
	}
	return a.Background()
}

func (a *App) ConfigureAutostart(enabled bool) error {
	if enabled && os.Getenv("LINKSEND_DATA_DIR") != "" {
		return errors.New("AUTOSTART_PROFILE: 自启动仅用于默认本机配置")
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if enabled {
		return installNativeAutostart(exe)
	}
	return uninstallNativeAutostart(exe)
}

func (a *App) navigateToTask(id string) {
	if id != "" {
		if a.core == nil {
			return
		}
		if _, ok := a.core.Task(id); !ok {
			// Old history is paged from SQLite and may not be resident in Tasks.
			go func(taskID string) {
				ctx, cancel := context.WithTimeout(a.ctx, 5*time.Second)
				defer cancel()
				if _, err := a.core.InboxFiles(ctx, taskID, "", 1); err != nil {
					a.backgroundError("该任务记录不可用，请在收件箱查看现有记录。")
					taskID = ""
				}
				a.publishInboxNavigation(taskID)
			}(id)
			return
		}
	}
	a.publishInboxNavigation(id)
}

func (a *App) publishInboxNavigation(id string) {
	a.background.mu.Lock()
	if a.background.closed {
		a.background.mu.Unlock()
		return
	}
	a.background.status.TaskID = id
	a.background.status.NavigationRevision++
	a.background.mu.Unlock()
	a.showEntryWindow()
}

func (a *App) QuitApplication() {
	if a.runtimeApp != nil {
		a.runtimeApp.Quit()
	}
}

func (a *App) onWindowClosing(event *application.WindowEvent) {
	a.closeMu.Lock()
	ready := a.quitReady
	dialogOpen := a.closeDialog
	a.closeMu.Unlock()
	if ready {
		return
	}
	event.Cancel()
	if dialogOpen {
		return
	}
	options := a.Preferences().Background
	if options.CloseMode == "background" && a.background.tray != nil {
		a.window.Hide()
		return
	}
	if options.CloseMode == "exit" || a.background.tray == nil {
		go a.runtimeApp.Quit()
		return
	}
	a.setCloseDialog(true)
	showNativeChoice(a.window, "关闭窗口后继续接收？", "留在后台会保留托盘入口；退出应用会停止接收。你可随时在设置中修改。", []string{"留在后台", "退出应用", "取消"}, 2, 2, func(choice int, err error) {
		if err != nil {
			a.backgroundError("关闭确认窗口暂不可用，窗口保持打开。")
			a.setCloseDialog(false)
			return
		}
		switch choice {
		case 0:
			options.CloseMode = "background"
			if err := a.SetBackgroundOptions(options); err != nil {
				a.backgroundError("关闭方式未保存，窗口保持打开。")
			} else {
				a.window.Hide()
			}
			a.setCloseDialog(false)
		case 1:
			options.CloseMode = "exit"
			if err := a.SetBackgroundOptions(options); err != nil {
				a.backgroundError("关闭方式未保存，窗口保持打开。")
				a.setCloseDialog(false)
				return
			}
			a.setCloseDialog(false)
			a.runtimeApp.Quit()
		default:
			a.setCloseDialog(false)
		}
	})
}

func (a *App) closeBackground() {
	b := a.background
	if b == nil {
		return
	}
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return
	}
	b.closed = true
	b.mu.Unlock()
	if b.cancel != nil {
		b.cancel()
	}
	if b.done != nil {
		<-b.done
	}
	for _, cleanup := range b.cleanup {
		cleanup()
	}
	if b.sleep != nil {
		_ = b.sleep.Close()
	}
	if b.notifier != nil {
		_ = b.notifier.ServiceShutdown()
	}
	if b.tray != nil {
		b.tray.Close()
	}
}
