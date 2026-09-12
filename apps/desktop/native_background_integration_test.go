package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// Explicit opt-in executable harness. Ordinary go test never starts a GUI,
// creates a toast registration, requests permission or edits real startup keys.
func TestMain(m *testing.M) {
	if os.Getenv("LINKSEND_NATIVE_BACKGROUND_TEST") != "1" {
		os.Exit(m.Run())
	}
	if err := runNativeBackgroundHarness(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(0)
}

func runNativeBackgroundHarness() error {
	icon, err := os.ReadFile(os.Getenv("LINKSEND_NATIVE_BACKGROUND_ICON"))
	if err != nil {
		return err
	}
	name := "LinkSend Native Test " + strconv.Itoa(os.Getpid()) + "-" + strconv.FormatInt(time.Now().UnixNano(), 16)
	cleanup, err := prepareNativeNotificationHarness(name)
	if err != nil {
		return err
	}
	defer cleanup()
	notifier := newNativeNotifier(func(id string) { fmt.Println("NATIVE_NOTIFICATION_CLICK_TASK=" + id) })
	host := application.New(application.Options{
		Name: name,
		Services: []application.Service{
			application.NewService(notifier),
		},
	})
	tray, err := newNativeTray(host, icon, nativeTrayCallbacks{})
	if err != nil {
		return err
	}
	inhibitor := newNativeSleepInhibitor()
	defer inhibitor.Close()
	done := make(chan struct{})
	var harnessErr error
	host.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
		go func() {
			if err := inhibitor.Acquire(); err != nil {
				harnessErr = err
			} else {
				fmt.Println("NATIVE_SLEEP_ACTIVE=" + strconv.FormatBool(inhibitor.Active()))
			}
			result, err := notifier.notify(nativeNotification{TaskID: "native-test", Revision: 1, Title: "LinkSend 原生适配验证", Body: "仅验证本机通知接口，不含文件或剪贴板内容。"})
			encoded, _ := json.Marshal(result)
			fmt.Println("NATIVE_NOTIFICATION_RESULT=" + string(encoded))
			if err != nil {
				harnessErr = err
			} else if runtime.GOOS == "windows" && result.State != "submitted" {
				harnessErr = errors.New("native Windows notification submission unavailable")
			} else if runtime.GOOS == "darwin" && result.State != "submitted" && result.State != "not_authorized" && result.State != "denied" {
				harnessErr = errors.New("native macOS notification service unavailable")
			}
			tray.SetQueuePaused(true)
			time.Sleep(250 * time.Millisecond)
			if err := inhibitor.Release(); err != nil {
				harnessErr = err
			}
			fmt.Println("NATIVE_SLEEP_RELEASED=" + strconv.FormatBool(!inhibitor.Active()))
			tray.Close()
			fmt.Println("NATIVE_TRAY_RUNTIME_AND_CLEANUP=PASS")
			close(done)
			host.Quit()
		}()
	})
	go func() {
		select {
		case <-done:
		case <-time.After(25 * time.Second):
			fmt.Fprintln(os.Stderr, "NATIVE_BACKGROUND_HARNESS_TIMEOUT")
			host.Quit()
		}
	}()
	if err := host.Run(); err != nil {
		return err
	}
	select {
	case <-done:
		return harnessErr
	default:
		return errors.New("native background harness did not complete")
	}
}
