//go:build windows

package main

import (
	"errors"
	"fmt"
	"log/slog"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

type nativeNetworkWindowsRegister func(uint16, uintptr, unsafe.Pointer, bool, *windows.Handle) error

type nativeNetworkWindowsAPI struct {
	interfaceChange nativeNetworkWindowsRegister
	addressChange   nativeNetworkWindowsRegister
	routeChange     nativeNetworkWindowsRegister
	cancel          func(windows.Handle) error
}

func startNativeNetworkMonitor(notify func()) (func(), error) {
	return startNativeNetworkWindowsMonitor(notify, nativeNetworkWindowsAPI{
		interfaceChange: windows.NotifyIpInterfaceChange,
		addressChange:   windows.NotifyUnicastIpAddressChange,
		routeChange:     windows.NotifyRouteChange2,
		cancel:          windows.CancelMibChangeNotify2,
	})
}

func startNativeNetworkWindowsMonitor(notify func(), api nativeNetworkWindowsAPI) (func(), error) {
	if notify == nil {
		return nil, fmt.Errorf("NATIVE_NETWORK_MONITOR_FAILED: callback is required")
	}
	var active atomic.Bool
	active.Store(true)
	// All three APIs use the same (context, row, notification type) callback.
	// Only enqueue into the bounded application event pump here. Cancel waits
	// for callbacks to return, so subscription shutdown must run outside them.
	callback := syscall.NewCallback(func(_ uintptr, _ uintptr, _ uintptr) uintptr {
		if active.Load() {
			notify()
		}
		return 0
	})
	handles := make([]windows.Handle, 0, 3)
	cleanup := func() error {
		active.Store(false)
		var errs []error
		for i := len(handles) - 1; i >= 0; i-- {
			if err := api.cancel(handles[i]); err != nil {
				errs = append(errs, fmt.Errorf("NATIVE_NETWORK_MONITOR_FAILED: CancelMibChangeNotify2: %w", err))
			}
		}
		runtime.KeepAlive(callback)
		return errors.Join(errs...)
	}
	for _, registration := range []struct {
		name string
		call nativeNetworkWindowsRegister
	}{
		{"NotifyIpInterfaceChange", api.interfaceChange},
		{"NotifyUnicastIpAddressChange", api.addressChange},
		{"NotifyRouteChange2", api.routeChange},
	} {
		var handle windows.Handle
		if err := registration.call(windows.AF_UNSPEC, callback, nil, false, &handle); err != nil {
			return nil, errors.Join(fmt.Errorf("NATIVE_NETWORK_MONITOR_FAILED: %s: %w", registration.name, err), cleanup())
		}
		handles = append(handles, handle)
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			if err := cleanup(); err != nil {
				slog.Warn("native network monitor cleanup failed", "error", err)
			}
		})
	}, nil
}
