package main

import (
	"context"
	"errors"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"
)

type nativeNotification struct {
	TaskID   string
	Revision uint64
	Title    string
	Body     string // Only a generic task summary; never clipboard/file content.
}

type nativeNotificationResult struct {
	State      string `json:"state"`      // submitted, duplicate, not_authorized, denied, unavailable, closed
	Permission string `json:"permission"` // granted, not_authorized, denied, unknown, unavailable
	Reason     string `json:"reason,omitempty"`
}

type nativeNotificationBackend interface {
	ServiceStartup(context.Context, application.ServiceOptions) error
	ServiceShutdown() error
	CheckNotificationAuthorization() (bool, error)
	RequestNotificationAuthorization() (bool, error)
	SendNotification(notifications.NotificationOptions) error
	OnNotificationResponse(func(notifications.NotificationResult))
}

type nativeNoticeRecord struct {
	revision uint64
	order    uint64
	pending  map[uint64]struct{}
}

type nativeNotifier struct {
	mu          sync.Mutex
	lifecycleMu sync.Mutex
	backend     nativeNotificationBackend
	platform    string
	onTaskClick func(string)
	started     bool
	closed      bool
	permission  string
	reason      string
	recent      map[string]*nativeNoticeRecord
	order       uint64
}

func newNativeNotifier(onTaskClick func(string)) *nativeNotifier {
	return &nativeNotifier{backend: notifications.New(), platform: runtime.GOOS, onTaskClick: onTaskClick, permission: "unknown", recent: make(map[string]*nativeNoticeRecord)}
}

func (n *nativeNotifier) ServiceName() string { return "LinkSend native notifications" }

// Register application.NewService(notifier), not a second service for its
// underlying backend. An optional notification startup failure is non-fatal to
// the application and is returned by notificationStatus instead.
func (n *nativeNotifier) ServiceStartup(ctx context.Context, options application.ServiceOptions) error {
	n.lifecycleMu.Lock()
	defer n.lifecycleMu.Unlock()
	n.mu.Lock()
	if n.started || n.closed {
		n.mu.Unlock()
		return nil
	}
	n.mu.Unlock()
	err := n.backend.ServiceStartup(ctx, options)
	n.mu.Lock()
	if err != nil {
		n.permission, n.reason = "unavailable", err.Error()
	} else if !n.closed {
		n.started = true
	}
	n.mu.Unlock()
	if err == nil {
		n.backend.OnNotificationResponse(n.handleResponse)
	}
	return nil
}

func (n *nativeNotifier) ServiceShutdown() error {
	n.lifecycleMu.Lock()
	defer n.lifecycleMu.Unlock()
	n.mu.Lock()
	if n.closed {
		n.mu.Unlock()
		return nil
	}
	n.closed = true
	started := n.started
	n.mu.Unlock()
	n.backend.OnNotificationResponse(nil)
	if started {
		return n.backend.ServiceShutdown()
	}
	return nil
}

func (n *nativeNotifier) notificationStatus() nativeNotificationResult {
	n.mu.Lock()
	defer n.mu.Unlock()
	state := "ready"
	if n.closed {
		state = "closed"
	} else if !n.started {
		state = "unavailable"
	}
	return nativeNotificationResult{State: state, Permission: n.permission, Reason: n.reason}
}

// Request only after the user's explicit enable-notifications action. Windows
// beta.18 returns true from a stub; that cannot prove OS policy permits a toast.
// This call is never made by startup or transfer events.
func (n *nativeNotifier) requestNotificationPermission() nativeNotificationResult {
	if state := n.notificationStatus(); state.State != "ready" {
		return state
	}
	if n.platform == "windows" {
		return nativeNotificationResult{State: "ready", Permission: "unknown", Reason: "Windows notification and focus settings determine whether submitted notifications appear"}
	}
	allowed, err := n.backend.RequestNotificationAuthorization()
	return n.recordPermission(allowed, err, true)
}

func (n *nativeNotifier) recordPermission(allowed bool, err error, requested bool) nativeNotificationResult {
	permission, state, reason := "not_authorized", "not_authorized", ""
	if requested {
		permission, state = "denied", "denied"
	}
	if allowed {
		permission, state = "granted", "ready"
	}
	if err != nil {
		permission, state, reason = "unavailable", "unavailable", err.Error()
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.closed {
		return nativeNotificationResult{State: "closed", Permission: n.permission}
	}
	n.permission, n.reason = permission, reason
	return nativeNotificationResult{State: state, Permission: permission, Reason: reason}
}

func validNativeTaskID(taskID string) bool {
	if len(taskID) == 0 || len(taskID) > 128 {
		return false
	}
	for _, c := range taskID {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
			return false
		}
	}
	return true
}

func nativeNotificationID(note nativeNotification) string {
	return "linksend." + note.TaskID + "." + strconv.FormatUint(note.Revision, 10)
}

func (n *nativeNotifier) notify(note nativeNotification) (nativeNotificationResult, error) {
	if !validNativeTaskID(note.TaskID) || note.Revision == 0 || strings.TrimSpace(note.Title) == "" || !utf8.ValidString(note.Title) || !utf8.ValidString(note.Body) || len(note.Title) > 512 || len(note.Body) > 2048 {
		return nativeNotificationResult{State: "invalid"}, errors.New("NOTIFICATION_INVALID: use an opaque task ID, positive revision and bounded summary")
	}
	n.mu.Lock()
	if n.closed || !n.started {
		state := "unavailable"
		if n.closed {
			state = "closed"
		}
		result := nativeNotificationResult{State: state, Permission: n.permission, Reason: n.reason}
		n.mu.Unlock()
		return result, nil
	}
	record, exists := n.recent[note.TaskID]
	if exists {
		if record.revision >= note.Revision {
			n.mu.Unlock()
			return nativeNotificationResult{State: "duplicate"}, nil
		}
		for pending := range record.pending {
			if pending >= note.Revision {
				n.mu.Unlock()
				return nativeNotificationResult{State: "duplicate"}, nil
			}
		}
		if len(record.pending) >= 8 {
			n.mu.Unlock()
			return nativeNotificationResult{State: "unavailable", Reason: "notification capacity reached"}, nil
		}
	}
	if !exists && len(n.recent) >= 4096 {
		oldestID := ""
		var oldest uint64
		for id, candidate := range n.recent {
			if len(candidate.pending) == 0 && (oldestID == "" || candidate.order < oldest) {
				oldestID, oldest = id, candidate.order
			}
		}
		if oldestID == "" {
			n.mu.Unlock()
			return nativeNotificationResult{State: "unavailable", Reason: "notification capacity reached"}, nil
		}
		delete(n.recent, oldestID)
	}
	n.order++
	if !exists {
		record = &nativeNoticeRecord{pending: make(map[uint64]struct{})}
		n.recent[note.TaskID] = record
	}
	record.order = n.order
	record.pending[note.Revision] = struct{}{}
	n.mu.Unlock()
	committed := false
	defer func() {
		n.mu.Lock()
		defer n.mu.Unlock()
		delete(record.pending, note.Revision)
		if committed && note.Revision > record.revision {
			record.revision = note.Revision
		}
		if record.revision == 0 && len(record.pending) == 0 {
			delete(n.recent, note.TaskID)
		}
	}()
	permission := "unknown"
	if n.platform != "windows" {
		allowed, err := n.backend.CheckNotificationAuthorization()
		result := n.recordPermission(allowed, err, false)
		if result.State != "ready" {
			return result, nil
		}
		permission = result.Permission
	}
	if state := n.notificationStatus(); state.State != "ready" {
		return state, nil
	}
	err := n.backend.SendNotification(notifications.NotificationOptions{
		ID: nativeNotificationID(note), Title: note.Title, Body: note.Body,
		Data: map[string]interface{}{"task_id": note.TaskID, "revision": strconv.FormatUint(note.Revision, 10)},
	})
	if err != nil {
		return nativeNotificationResult{State: "unavailable", Permission: permission, Reason: err.Error()}, nil
	}
	committed = true
	return nativeNotificationResult{State: "submitted", Permission: permission}, nil
}

func (n *nativeNotifier) handleResponse(result notifications.NotificationResult) {
	if result.Error != nil {
		return
	}
	parts := strings.Split(result.Response.ID, ".")
	if len(parts) != 3 || parts[0] != "linksend" || !validNativeTaskID(parts[1]) {
		return
	}
	if revision, err := strconv.ParseUint(parts[2], 10, 64); err != nil || revision == 0 {
		return
	}
	if id, ok := result.Response.UserInfo["task_id"].(string); ok && id != parts[1] {
		return
	}
	n.mu.Lock()
	callback := n.onTaskClick
	closed := n.closed
	n.mu.Unlock()
	if !closed && callback != nil {
		callback(parts[1]) // An opaque lookup key, never a path or command.
	}
}
