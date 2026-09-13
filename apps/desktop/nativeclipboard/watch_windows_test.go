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
	changes := make(chan Change, 1)
	stop, err := Watch(context.Background(), hwnd, func(change Change) { changes <- change })
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	expectedSequence, _, _ := getSequence.Call()
	sendMessage.Call(hwnd, wmClipboardUpdate, 0, 0)
	select {
	case change := <-changes:
		if change.Sequence != uint64(uint32(expectedSequence)) {
			t.Fatalf("native callback sequence = %d, want %d", change.Sequence, uint32(expectedSequence))
		}
	case <-time.After(time.Second):
		t.Fatal("WM_CLIPBOARDUPDATE did not reach subclass callback")
	}
}
