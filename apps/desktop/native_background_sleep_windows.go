//go:build windows

package main

import (
	"fmt"
	"runtime"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	nativePowerDLL    = windows.NewLazySystemDLL("kernel32.dll")
	nativePowerCreate = nativePowerDLL.NewProc("PowerCreateRequest")
	nativePowerSet    = nativePowerDLL.NewProc("PowerSetRequest")
	nativePowerClear  = nativePowerDLL.NewProc("PowerClearRequest")
)

// Windows SDK REASON_CONTEXT: Version/Flags followed by the pointer-aligned
// union. Four uintptr words are enough for the complete union on 32/64 bit.
type nativePowerReason struct {
	Version uint32
	Flags   uint32
	Reason  [4]uintptr
}

func acquireNativeSleepRequest() (func() error, error) {
	const powerRequestSystemRequired = 1 // POWER_REQUEST_TYPE in winnt.h.
	reason, err := windows.UTF16FromString("LinkSend active file transfer")
	if err != nil {
		return nil, err
	}
	context := nativePowerReason{Version: 0, Flags: 1}
	context.Reason[0] = uintptr(unsafe.Pointer(&reason[0]))
	handle, _, callErr := nativePowerCreate.Call(uintptr(unsafe.Pointer(&context)))
	runtime.KeepAlive(reason)
	if handle == 0 || handle == ^uintptr(0) {
		return nil, nativePowerError("PowerCreateRequest", callErr)
	}
	if ok, _, callErr := nativePowerSet.Call(handle, powerRequestSystemRequired); ok == 0 {
		_ = windows.CloseHandle(windows.Handle(handle))
		return nil, nativePowerError("PowerSetRequest", callErr)
	}
	return func() error {
		// Closing our unique handle releases its requests even if Clear fails.
		// Keep the handle in the owner only when Close itself failed.
		_, _, _ = nativePowerClear.Call(handle, powerRequestSystemRequired)
		return windows.CloseHandle(windows.Handle(handle))
	}, nil
}

func nativePowerError(operation string, err error) error {
	if err == nil || err == syscall.Errno(0) {
		return fmt.Errorf("SLEEP_INHIBITOR_FAILED: %s returned failure", operation)
	}
	return fmt.Errorf("SLEEP_INHIBITOR_FAILED: %s: %w", operation, err)
}
