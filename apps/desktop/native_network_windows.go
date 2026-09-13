//go:build windows

package main

import (
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	ipHelperDLL               = windows.NewLazySystemDLL("iphlpapi.dll")
	notifyIPInterfaceChange   = ipHelperDLL.NewProc("NotifyIpInterfaceChange")
	cancelMibChangeNotifyProc = ipHelperDLL.NewProc("CancelMibChangeNotify2")
)

func startNativeNetworkMonitor(notify func()) (func(), error) {
	if notify == nil {
		return nil, fmt.Errorf("NATIVE_NETWORK_MONITOR_FAILED: callback is required")
	}
	var active atomic.Bool
	active.Store(true)
	callback := syscall.NewCallback(func(_ uintptr, _ uintptr, _ uintptr) uintptr {
		if active.Load() {
			notify()
		}
		return 0
	})
	var handle uintptr
	result, _, callErr := notifyIPInterfaceChange.Call(
		windows.AF_UNSPEC,
		callback,
		0,
		0,
		uintptr(unsafe.Pointer(&handle)),
	)
	if result != 0 {
		active.Store(false)
		return nil, nativeNetworkWindowsError("NotifyIpInterfaceChange", result, callErr)
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			active.Store(false)
			_, _, _ = cancelMibChangeNotifyProc.Call(handle)
			runtime.KeepAlive(callback)
		})
	}, nil
}

func nativeNetworkWindowsError(operation string, code uintptr, callErr error) error {
	if callErr == nil || callErr == syscall.Errno(0) {
		return fmt.Errorf("NATIVE_NETWORK_MONITOR_FAILED: %s code=%d", operation, code)
	}
	return fmt.Errorf("NATIVE_NETWORK_MONITOR_FAILED: %s code=%d: %w", operation, code, callErr)
}
