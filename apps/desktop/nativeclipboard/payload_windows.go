//go:build windows

package nativeclipboard

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"net/url"
	"runtime"
	"time"
	"unicode/utf16"
	"unicode/utf8"
	"unsafe"

	"github.com/Wen5555/LinkSend/internal/clipboardsync"
	"github.com/Wen5555/LinkSend/internal/content"
	"golang.org/x/sys/windows"
)

var (
	emptyClipboard   = user32.NewProc("EmptyClipboard")
	setClipboardData = user32.NewProc("SetClipboardData")
	globalAlloc      = kernel32.NewProc("GlobalAlloc")
	globalFree       = kernel32.NewProc("GlobalFree")
)

func clipboardGeneration() uint64 { value, _, _ := getSequence.Call(); return uint64(uint32(value)) }

func readClipboardText(ctx context.Context, kind clipboardsync.Kind, expected uint64) ([]byte, uint64, error) {
	if kind != clipboardsync.Text && kind != clipboardsync.Link {
		return nil, clipboardGeneration(), ErrUnsupported
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := ctx.Err(); err != nil {
		return nil, clipboardGeneration(), err
	}
	if ok, _, err := openClipboard.Call(clipboardOwner.Load()); ok == 0 {
		return nil, clipboardGeneration(), errors.Join(ErrUnavailable, err)
	}
	defer closeClipboard.Call()
	if current := clipboardGeneration(); current != expected {
		return nil, current, ErrChanged
	}
	handle, _, err := getClipboardData.Call(13)
	if handle == 0 {
		return nil, clipboardGeneration(), errors.Join(ErrUnavailable, err)
	}
	size, _, _ := globalSize.Call(handle)
	if size == 0 || size > 2*(clipboardsync.MaxTextBytes+1) {
		return nil, clipboardGeneration(), clipboardsync.ErrLimit
	}
	address, _, err := globalLock.Call(handle)
	if address == 0 {
		return nil, clipboardGeneration(), errors.Join(ErrUnavailable, err)
	}
	raw := make([]byte, int(size))
	moveMemory.Call(uintptr(unsafe.Pointer(&raw[0])), address, size)
	globalUnlock.Call(handle)
	units := make([]uint16, len(raw)/2)
	for i := range units {
		units[i] = binary.LittleEndian.Uint16(raw[i*2:])
	}
	for len(units) > 0 && units[len(units)-1] == 0 {
		units = units[:len(units)-1]
	}
	data := []byte(string(utf16.Decode(units)))
	if !utf8.Valid(data) || len(data) == 0 || len(data) > clipboardsync.MaxTextBytes {
		return nil, clipboardGeneration(), clipboardsync.ErrLimit
	}
	if kind == clipboardsync.Link {
		parsed, e := url.Parse(string(data))
		if e != nil || parsed.Scheme == "" {
			return nil, clipboardGeneration(), ErrUnsupported
		}
	}
	current := clipboardGeneration()
	if current != expected {
		return nil, current, ErrChanged
	}
	return data, current, nil
}

func writeClipboardPayload(kind clipboardsync.Kind, payload []byte, expected uint64, deadline time.Time) (uint64, error) {
	if kind == clipboardsync.Image {
		if _, err := content.DecodeImage(context.Background(), bytes.NewReader(payload)); err != nil {
			return clipboardGeneration(), err
		}
	} else if !utf8.Valid(payload) || len(payload) == 0 || len(payload) > clipboardsync.MaxTextBytes {
		return clipboardGeneration(), clipboardsync.ErrLimit
	}
	if kind == clipboardsync.Link {
		parsed, err := url.Parse(string(payload))
		if err != nil || parsed.Scheme == "" {
			return clipboardGeneration(), ErrUnsupported
		}
	}
	format := uintptr(13)
	data := payload
	var units []uint16
	if kind == clipboardsync.Image {
		name, _ := windows.UTF16PtrFromString("PNG")
		format, _, _ = registerClipboardFormat.Call(uintptr(unsafe.Pointer(name)))
		if format == 0 {
			return clipboardGeneration(), ErrUnavailable
		}
	} else {
		units = utf16.Encode([]rune(string(payload)))
		units = append(units, 0)
		data = unsafe.Slice((*byte)(unsafe.Pointer(&units[0])), len(units)*2)
	}
	handle, _, err := globalAlloc.Call(0x42, uintptr(len(data)))
	if handle == 0 {
		return clipboardGeneration(), errors.Join(ErrUnavailable, err)
	}
	address, _, err := globalLock.Call(handle)
	if address == 0 {
		globalFree.Call(handle)
		return clipboardGeneration(), errors.Join(ErrUnavailable, err)
	}
	moveMemory.Call(address, uintptr(unsafe.Pointer(&data[0])), uintptr(len(data)))
	globalUnlock.Call(handle)
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	owner := clipboardOwner.Load()
	if owner == 0 {
		globalFree.Call(handle)
		return clipboardGeneration(), errors.New("CLIPBOARD_NATIVE_WINDOW_REQUIRED")
	}
	if ok, _, err := openClipboard.Call(owner); ok == 0 {
		globalFree.Call(handle)
		return clipboardGeneration(), errors.Join(ErrUnavailable, err)
	}
	defer closeClipboard.Call()
	if current := clipboardGeneration(); current != expected {
		globalFree.Call(handle)
		return current, ErrChanged
	}
	if !deadline.IsZero() && !time.Now().Before(deadline) {
		globalFree.Call(handle)
		return clipboardGeneration(), clipboardsync.ErrExpired
	}
	if ok, _, err := emptyClipboard.Call(); ok == 0 {
		globalFree.Call(handle)
		return clipboardGeneration(), errors.Join(ErrUnavailable, err)
	}
	if result, _, err := setClipboardData.Call(format, handle); result == 0 {
		globalFree.Call(handle)
		return clipboardGeneration(), errors.Join(ErrUnavailable, err)
	}
	return clipboardGeneration(), nil
}
