//go:build windows

package nativeclipboard

import (
	"context"
	"encoding/binary"
	"runtime"
	"time"
	"unicode/utf16"
	"unsafe"

	"github.com/Wen5555/LinkSend/internal/clipboardsync"
	"golang.org/x/sys/windows"
)

var dragQueryFile = windows.NewLazySystemDLL("shell32.dll").NewProc("DragQueryFileW")

func readClipboardFiles(ctx context.Context, expected uint64) ([]string, uint64, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	deadline := time.Now().Add(750 * time.Millisecond)
	for {
		if clipboardGeneration() != expected {
			return nil, clipboardGeneration(), ErrChanged
		}
		if ok, _, _ := openClipboard.Call(clipboardOwner.Load()); ok != 0 {
			break
		}
		if !time.Now().Before(deadline) {
			return nil, clipboardGeneration(), ErrUnavailable
		}
		select {
		case <-ctx.Done():
			return nil, clipboardGeneration(), ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}
	}
	defer closeClipboard.Call()
	handle, _, _ := getClipboardData.Call(15) // CF_HDROP
	if handle == 0 {
		return nil, clipboardGeneration(), ErrUnsupported
	}
	count, _, _ := dragQueryFile.Call(handle, 0xffffffff, 0, 0)
	if count == 0 || count > 128 {
		return nil, clipboardGeneration(), clipboardsync.ErrLimit
	}
	paths := make([]string, 0, count)
	for index := uintptr(0); index < count; index++ {
		size, _, _ := dragQueryFile.Call(handle, index, 0, 0)
		if size == 0 || size > 32767 {
			return nil, clipboardGeneration(), clipboardsync.ErrLimit
		}
		buffer := make([]uint16, size+1)
		read, _, _ := dragQueryFile.Call(handle, index, uintptr(unsafe.Pointer(&buffer[0])), uintptr(len(buffer)))
		if read != size {
			return nil, clipboardGeneration(), ErrChanged
		}
		paths = append(paths, windows.UTF16ToString(buffer))
	}
	current := clipboardGeneration()
	if current != expected {
		return nil, current, ErrChanged
	}
	return paths, current, nil
}

func clipboardDropData(paths []string) []byte {
	// DROPFILES: pFiles=20, POINT=(0,0), fNC=0, fWide=1, double NUL list.
	units := make([]uint16, 0)
	for _, path := range paths {
		units = append(units, utf16.Encode([]rune(path))...)
		units = append(units, 0)
	}
	units = append(units, 0)
	data := make([]byte, 20+2*len(units))
	binary.LittleEndian.PutUint32(data, 20)
	binary.LittleEndian.PutUint32(data[16:], 1)
	for index, unit := range units {
		binary.LittleEndian.PutUint16(data[20+2*index:], unit)
	}
	return data
}

func writeClipboardFiles(paths []string, expected uint64, deadline time.Time) (uint64, error) {
	data := clipboardDropData(paths)
	handle, _, _ := globalAlloc.Call(0x42, uintptr(len(data)))
	if handle == 0 {
		return clipboardGeneration(), ErrUnavailable
	}
	address, _, _ := globalLock.Call(handle)
	if address == 0 {
		globalFree.Call(handle)
		return clipboardGeneration(), ErrUnavailable
	}
	moveMemory.Call(address, uintptr(unsafe.Pointer(&data[0])), uintptr(len(data)))
	globalUnlock.Call(handle)
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	owner := clipboardOwner.Load()
	if owner == 0 {
		globalFree.Call(handle)
		return clipboardGeneration(), ErrUnavailable
	}
	if ok, _, _ := openClipboard.Call(owner); ok == 0 {
		globalFree.Call(handle)
		return clipboardGeneration(), ErrUnavailable
	}
	if clipboardGeneration() != expected || (!deadline.IsZero() && !time.Now().Before(deadline)) {
		closeClipboard.Call()
		globalFree.Call(handle)
		return clipboardGeneration(), ErrChanged
	}
	if ok, _, _ := emptyClipboard.Call(); ok == 0 {
		closeClipboard.Call()
		globalFree.Call(handle)
		return clipboardGeneration(), ErrUnavailable
	}
	if ok, _, _ := setClipboardData.Call(15, handle); ok == 0 {
		closeClipboard.Call()
		globalFree.Call(handle)
		return clipboardGeneration(), ErrUnavailable
	}
	closeClipboard.Call()
	return clipboardGeneration(), nil
}
