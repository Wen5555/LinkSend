//go:build windows

package nativeclipboard

import (
	"context"
	"runtime"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

func TestWindowsClipboardListenerUsesNativeWindowSubclass(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	className, _ := windows.UTF16PtrFromString("STATIC")
	windowName, _ := windows.UTF16PtrFromString("LinkSend clipboard listener test")
	createWindow := user32.NewProc("CreateWindowExW")
	destroyWindow := user32.NewProc("DestroyWindow")
	sendMessage := user32.NewProc("SendMessageW")
	hwnd, _, callErr := createWindow.Call(0, uintptr(unsafe.Pointer(className)), uintptr(unsafe.Pointer(windowName)), 0, 0, 0, 0, 0, 0, 0, 0, 0)
	if hwnd == 0 {
		t.Fatal(callErr)
	}
	defer destroyWindow.Call(hwnd)
	changes := make(chan Change, 2)
	stop, err := Watch(context.Background(), hwnd, func(change Change) { changes <- change })
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	sendMessage.Call(hwnd, wmClipboardUpdate, 0, 0)
	select {
	case change := <-changes:
		t.Fatalf("unchanged registration baseline was reported: %+v", change)
	case <-time.After(25 * time.Millisecond):
	}
}

func TestWindowsClipboardListenerKeepsLatestSequence(t *testing.T) {
	watch := &windowsClipboardWatch{changes: make(chan Change, 1)}
	watch.offer(Change{Sequence: 11})
	watch.offer(Change{Sequence: 12})
	if got := <-watch.changes; got.Sequence != 12 {
		t.Fatalf("latest sequence = %d, want 12", got.Sequence)
	}
	watch.last.Store(20)
	if watch.acceptSequence(19) || watch.acceptSequence(20) || !watch.acceptSequence(21) {
		t.Fatal("baseline accepted stale or rejected fresh sequence")
	}
}
