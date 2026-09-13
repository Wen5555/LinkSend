//go:build darwin

package main

/*
#cgo CFLAGS: -mmacosx-version-min=13.0
#cgo LDFLAGS: -mmacosx-version-min=13.0 -framework CoreFoundation -framework SystemConfiguration
#include <stdint.h>
void *linksendStartNetworkMonitor(uintptr_t token);
void linksendStopNetworkMonitor(void *opaque);
*/
import "C"

import (
	"fmt"
	"runtime/cgo"
	"sync"
)

func startNativeNetworkMonitor(notify func()) (func(), error) {
	if notify == nil {
		return nil, fmt.Errorf("NATIVE_NETWORK_MONITOR_FAILED: callback is required")
	}
	handle := cgo.NewHandle(notify)
	monitor := C.linksendStartNetworkMonitor(C.uintptr_t(handle))
	if monitor == nil {
		handle.Delete()
		return nil, fmt.Errorf("NATIVE_NETWORK_MONITOR_FAILED: SCDynamicStore registration failed")
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			C.linksendStopNetworkMonitor(monitor)
			handle.Delete()
		})
	}, nil
}

//export linksendGoNetworkChanged
func linksendGoNetworkChanged(token C.uintptr_t) {
	notify, ok := cgo.Handle(token).Value().(func())
	if ok {
		notify()
	}
}
