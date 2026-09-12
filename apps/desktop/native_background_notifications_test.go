package main

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"
)

type testNativeNotifications struct {
	mu         sync.Mutex
	allowed    bool
	startupErr error
	sendErr    error
	requests   int
	sent       []notifications.NotificationOptions
	response   func(notifications.NotificationResult)
}

func (b *testNativeNotifications) ServiceStartup(context.Context, application.ServiceOptions) error {
	return b.startupErr
}
func (b *testNativeNotifications) ServiceShutdown() error { return nil }
func (b *testNativeNotifications) CheckNotificationAuthorization() (bool, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.allowed, nil
}
func (b *testNativeNotifications) RequestNotificationAuthorization() (bool, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.requests++
	return b.allowed, nil
}
func (b *testNativeNotifications) SendNotification(options notifications.NotificationOptions) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.sendErr != nil {
		return b.sendErr
	}
	b.sent = append(b.sent, options)
	return nil
}
func (b *testNativeNotifications) OnNotificationResponse(callback func(notifications.NotificationResult)) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.response = callback
}
func testNativeNotifier(t *testing.T, backend *testNativeNotifications, platform string) *nativeNotifier {
	t.Helper()
	n := &nativeNotifier{backend: backend, platform: platform, permission: "unknown", recent: make(map[string]*nativeNoticeRecord)}
	if err := n.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = n.ServiceShutdown() })
	return n
}

func TestNativeNotificationsDeduplicateConcurrentRevisions(t *testing.T) {
	backend := &testNativeNotifications{allowed: true}
	n := testNativeNotifier(t, backend, "darwin")
	note := nativeNotification{TaskID: "task-123", Revision: 4, Title: "任务已完成", Body: "打开 LinkSend 查看详情"}
	var workers sync.WaitGroup
	for j := 0; j < 32; j++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			result, err := n.notify(note)
			if err != nil || result.State != "submitted" && result.State != "duplicate" {
				t.Errorf("duplicate dispatch: %+v %v", result, err)
			}
		}()
	}
	workers.Wait()
	if len(backend.sent) != 1 || backend.requests != 0 {
		t.Fatalf("duplicate/implicit-permission calls: sends=%d requests=%d", len(backend.sent), backend.requests)
	}
	note.Revision = 3
	if result, _ := n.notify(note); result.State != "duplicate" {
		t.Fatal("stale revision was submitted")
	}
	note.Revision = 5
	if result, err := n.notify(note); err != nil || result.State != "submitted" || len(backend.sent) != 2 {
		t.Fatalf("new terminal revision lost: %+v %v", result, err)
	}
}

func TestNativeNotificationDeniedAndFailedAttemptCanRetry(t *testing.T) {
	backend := &testNativeNotifications{}
	n := testNativeNotifier(t, backend, "darwin")
	note := nativeNotification{TaskID: "task-denied", Revision: 1, Title: "新接收请求"}
	if result, err := n.notify(note); err != nil || result.State != "not_authorized" || backend.requests != 0 {
		t.Fatalf("denied OS state interrupted core or prompted: %+v %v", result, err)
	}
	if result := n.requestNotificationPermission(); result.State != "denied" || backend.requests != 1 {
		t.Fatalf("explicit denial not reported: %+v", result)
	}
	backend.allowed = true
	backend.sendErr = errors.New("OS temporarily unavailable")
	if result, err := n.notify(note); err != nil || result.State != "unavailable" {
		t.Fatalf("OS failure must stay non-fatal: %+v %v", result, err)
	}
	backend.sendErr = nil
	if result, err := n.notify(note); err != nil || result.State != "submitted" {
		t.Fatalf("failed delivery incorrectly consumed revision: %+v %v", result, err)
	}
}

func TestNativeNotificationStartupFailureAndWindowsPermissionStub(t *testing.T) {
	unavailable := testNativeNotifier(t, &testNativeNotifications{startupErr: errors.New("no bundle")}, "darwin")
	if result := unavailable.notificationStatus(); result.State != "unavailable" {
		t.Fatalf("optional startup failure hidden: %+v", result)
	}
	backend := &testNativeNotifications{allowed: false}
	n := testNativeNotifier(t, backend, "windows")
	if result := n.requestNotificationPermission(); result.Permission != "unknown" || backend.requests != 0 {
		t.Fatalf("Windows permission stub treated as authorization: %+v", result)
	}
	if result, err := n.notify(nativeNotification{TaskID: "task-win", Revision: 1, Title: "任务已完成"}); err != nil || result.State != "submitted" || result.Permission != "unknown" {
		t.Fatalf("Windows submission state: %+v %v", result, err)
	}
}

func TestNativeNotificationClicksUseOpaqueIDsAndStopOnShutdown(t *testing.T) {
	backend := &testNativeNotifications{allowed: true}
	n := testNativeNotifier(t, backend, "darwin")
	var clicked []string
	n.onTaskClick = func(taskID string) { clicked = append(clicked, taskID) }
	for _, id := range []string{"../private", "linksend.C:\\private.1", "linksend.task.0", "linksend.task.invalid"} {
		n.handleResponse(notifications.NotificationResult{Response: notifications.NotificationResponse{ID: id}})
	}
	if len(clicked) != 0 {
		t.Fatal("invalid notification was treated as an open-path command")
	}
	result := notifications.NotificationResult{Response: notifications.NotificationResponse{ID: "linksend.task-safe.7", UserInfo: map[string]interface{}{"task_id": "task-safe"}}}
	n.handleResponse(result)
	if len(clicked) != 1 || clicked[0] != "task-safe" {
		t.Fatal("valid click did not map to its task lookup key")
	}
	if err := n.ServiceShutdown(); err != nil {
		t.Fatal(err)
	}
	n.handleResponse(result)
	if len(clicked) != 1 || backend.response != nil {
		t.Fatal("click handler survived shutdown")
	}
	if result, err := n.notify(nativeNotification{TaskID: "task-closed", Revision: 1, Title: "任务"}); err != nil || result.State != "closed" {
		t.Fatal("notification was submitted after shutdown")
	}
}
