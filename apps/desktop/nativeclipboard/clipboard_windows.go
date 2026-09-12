//go:build windows

package nativeclipboard

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"runtime"
	"time"
	"unsafe"

	"github.com/Wen5555/LinkSend/internal/content"
	"golang.org/x/sys/windows"
)

var (
	user32                   = windows.NewLazySystemDLL("user32.dll")
	kernel32                 = windows.NewLazySystemDLL("kernel32.dll")
	openClipboard            = user32.NewProc("OpenClipboard")
	closeClipboard           = user32.NewProc("CloseClipboard")
	getClipboardData         = user32.NewProc("GetClipboardData")
	clipboardFormatAvailable = user32.NewProc("IsClipboardFormatAvailable")
	registerClipboardFormat  = user32.NewProc("RegisterClipboardFormatW")
	globalSize               = kernel32.NewProc("GlobalSize")
	globalLock               = kernel32.NewProc("GlobalLock")
	globalUnlock             = kernel32.NewProc("GlobalUnlock")
	moveMemory               = kernel32.NewProc("RtlMoveMemory")
)

func captureImage(ctx context.Context) (image.Image, error) {
	// Open/CloseClipboard must execute on the same OS thread. Never empty or
	// replace the user's clipboard; copying is complete before it is unlocked.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	deadline := time.Now().Add(750 * time.Millisecond)
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if ok, _, _ := openClipboard.Call(0); ok != 0 {
			break
		}
		if time.Now().After(deadline) {
			return nil, ErrUnavailable
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(15 * time.Millisecond):
		}
	}
	var data []byte
	var encoded bool
	var readErr error
	func() {
		defer closeClipboard.Call()
		pngName, _ := windows.UTF16PtrFromString("PNG")
		pngFormat, _, _ := registerClipboardFormat.Call(uintptr(unsafe.Pointer(pngName)))
		formats := []uintptr{17, 8} // CF_DIBV5, CF_DIB
		if pngFormat != 0 {
			formats = append([]uintptr{pngFormat}, formats...)
		}
		for _, format := range formats {
			if available, _, _ := clipboardFormatAvailable.Call(format); available == 0 {
				continue
			}
			handle, _, err := getClipboardData.Call(format)
			if handle == 0 {
				readErr = fmt.Errorf("%w: %v", ErrUnavailable, err)
				return
			}
			size, _, _ := globalSize.Call(handle)
			limit := uintptr(content.MaxImagePixels*4 + 1024)
			encoded = format == pngFormat
			if encoded {
				limit = content.MaxImageBytes
			}
			if size == 0 || size > limit {
				readErr = content.ErrLimit
				return
			}
			address, _, err := globalLock.Call(handle)
			if address == 0 {
				readErr = fmt.Errorf("%w: %v", ErrUnavailable, err)
				return
			}
			data = copyNativeMemory(address, int(size))
			globalUnlock.Call(handle)
			return
		}
		readErr = ErrNoImage
	}()
	if readErr != nil {
		return nil, readErr
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if encoded {
		return content.DecodeImage(ctx, bytes.NewReader(data))
	}
	return decodeDIB(data)
}

func copyNativeMemory(address uintptr, size int) []byte {
	data := make([]byte, size)
	// The native address is never converted into a Go pointer. The OS copies
	// the locked memory into our bounded Go allocation.
	moveMemory.Call(uintptr(unsafe.Pointer(&data[0])), address, uintptr(size))
	runtime.KeepAlive(data)
	return data
}
