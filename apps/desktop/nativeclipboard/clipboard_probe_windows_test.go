//go:build windows

package nativeclipboard

import (
	"fmt"
	"github.com/Wen5555/LinkSend/internal/clipboardsync"
	"golang.org/x/sys/windows"
	"os"
	"runtime"
	"testing"
	"time"
	"unsafe"
)

// Explicit, read-only diagnostic against a user-authorized fixture. Default
// tests never inspect the general clipboard through this diagnostic.
func TestWindowsClipboardReadOnlyDeliveryProbe(t *testing.T) {
	if os.Getenv("LINKSEND_TEST_CLIPBOARD_PROBE") != "1" {
		t.Skip("explicit read-only fixture diagnostic")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	class, _ := windows.UTF16PtrFromString("STATIC")
	name, _ := windows.UTF16PtrFromString("LinkSend clipboard probe")
	hwnd, _, err := user32.NewProc("CreateWindowExW").Call(0, uintptr(unsafe.Pointer(class)), uintptr(unsafe.Pointer(name)), 0, 0, 0, 0, 0, 0, 0, 0, 0)
	if hwnd == 0 {
		t.Fatal(err)
	}
	defer user32.NewProc("DestroyWindow").Call(hwnd)
	changes := make(chan Change, 8)
	stop, err := Watch(t.Context(), hwnd, func(c Change) { changes <- c })
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	type message struct {
		Hwnd           uintptr
		Message        uint32
		WParam, LParam uintptr
		Time           uint32
		X, Y           int32
		Private        uint32
	}
	var msg message
	peek, dispatch := user32.NewProc("PeekMessageW"), user32.NewProc("DispatchMessageW")
	fmt.Println("READ_ONLY_CLIPBOARD_PROBE_READY")
	end := time.Now().Add(30 * time.Second)
	for time.Now().Before(end) {
		for {
			ok, _, _ := peek.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0, 1)
			if ok == 0 {
				break
			}
			dispatch.Call(uintptr(unsafe.Pointer(&msg)))
		}
		select {
		case c := <-changes:
			fmt.Printf("READ_ONLY_CHANGE=%+v\n", c)
			if c.Image {
				payload, current, err := ReadPayload(t.Context(), clipboardsync.Image, c.Sequence)
				fmt.Printf("READ_ONLY_PNG_BYTES=%d GENERATION=%d ERROR=%v\n", len(payload), current, err)
				if err != nil {
					t.Fatal(err)
				}
				return
			}
		default:
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("no image notification observed")
}
