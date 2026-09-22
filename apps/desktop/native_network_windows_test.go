//go:build windows

package main

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

func TestNativeNetworkWindowsRealRegistrationsAndStops(t *testing.T) {
	events := make(chan string, 8)
	var registered, cancelled []windows.Handle
	var cancelErrors []error
	wrap := func(name string, register nativeNetworkWindowsRegister) nativeNetworkWindowsRegister {
		return func(family uint16, callback uintptr, callerContext unsafe.Pointer, initial bool, handle *windows.Handle) error {
			if family != windows.AF_UNSPEC || callerContext != nil || initial {
				t.Fatal("unexpected notification registration arguments")
			}
			// Initial notifications exercise each real OS callback without changing
			// the host's interfaces, addresses or routes.
			observedCallback := syscall.NewCallback(func(context, row, notificationType uintptr) uintptr {
				syscall.SyscallN(callback, context, row, notificationType)
				select {
				case events <- name:
				default:
				}
				return 0
			})
			if err := register(family, observedCallback, callerContext, true, handle); err != nil {
				return err
			}
			registered = append(registered, *handle)
			return nil
		}
	}
	stop, err := startNativeNetworkWindowsMonitor(func() {}, nativeNetworkWindowsAPI{
		interfaceChange: wrap("interface", windows.NotifyIpInterfaceChange),
		addressChange:   wrap("address", windows.NotifyUnicastIpAddressChange),
		routeChange:     wrap("route", windows.NotifyRouteChange2),
		cancel: func(handle windows.Handle) error {
			err := windows.CancelMibChangeNotify2(handle)
			cancelled = append(cancelled, handle)
			cancelErrors = append(cancelErrors, err)
			return err
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(stop)
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	observed := make(map[string]bool)
	for len(observed) < 3 {
		select {
		case name := <-events:
			observed[name] = true
		case <-deadline.C:
			t.Fatal("real Windows notification callbacks did not arrive")
		}
	}
	var stopping sync.WaitGroup
	for range 8 {
		stopping.Go(stop)
	}
	stopping.Wait()
	if len(registered) != 3 {
		t.Fatalf("registered %d notification subscriptions", len(registered))
	}
	slices.Reverse(registered)
	if !slices.Equal(cancelled, registered) || errors.Join(cancelErrors...) != nil {
		t.Fatalf("incomplete cancellation: handles=%v want=%v errors=%v", cancelled, registered, cancelErrors)
	}
}

func TestNativeNetworkWindowsRegistrationFailureCleansSubscriptions(t *testing.T) {
	names := []string{"NotifyIpInterfaceChange", "NotifyUnicastIpAddressChange", "NotifyRouteChange2"}
	for failedIndex, name := range names {
		t.Run(name, func(t *testing.T) {
			registrationError := windows.ERROR_ACCESS_DENIED
			cleanupError := windows.ERROR_INVALID_HANDLE
			var callbacks []uintptr
			var cancelled []windows.Handle
			var delivered atomic.Int32
			register := func(family uint16, callback uintptr, _ unsafe.Pointer, _ bool, handle *windows.Handle) error {
				callbacks = append(callbacks, callback)
				if len(callbacks)-1 == failedIndex {
					return registrationError
				}
				*handle = windows.Handle(len(callbacks))
				return nil
			}
			stop, err := startNativeNetworkWindowsMonitor(func() { delivered.Add(1) }, nativeNetworkWindowsAPI{
				interfaceChange: register,
				addressChange:   register,
				routeChange:     register,
				cancel: func(handle windows.Handle) error {
					cancelled = append(cancelled, handle)
					// Cancellation may overlap an outstanding notification. The
					// disabled callback must return without delivering more work.
					syscall.SyscallN(callbacks[handle-1], 0, 0, 0)
					if len(cancelled) == 1 {
						return cleanupError
					}
					return nil
				},
			})
			if stop != nil || !errors.Is(err, registrationError) || !strings.Contains(err.Error(), name) {
				t.Fatalf("failure lost operation or cause: stop=%v err=%v", stop != nil, err)
			}
			if failedIndex > 0 && !errors.Is(err, cleanupError) {
				t.Fatalf("cleanup failure was discarded: %v", err)
			}
			if len(cancelled) != failedIndex || delivered.Load() != 0 {
				t.Fatalf("rollback handles=%v notifications=%d", cancelled, delivered.Load())
			}
			for index, handle := range cancelled {
				if handle != windows.Handle(failedIndex-index) {
					t.Fatalf("rollback order=%v", cancelled)
				}
			}
		})
	}
}

func TestNativeNetworkWindowsConcurrentCallbacksAndStop(t *testing.T) {
	handlerEntered, releaseHandler := make(chan struct{}), make(chan struct{})
	var enteredOnce sync.Once
	pump := newNativeSystemEventPump(context.Background(), func(string) {
		enteredOnce.Do(func() { close(handlerEntered) })
		<-releaseHandler
	}, nil)
	defer pump.close()
	defer close(releaseHandler)
	var callbacks []uintptr
	var delivered atomic.Int32
	var callbackMu sync.RWMutex
	register := func(_ uint16, callback uintptr, _ unsafe.Pointer, _ bool, handle *windows.Handle) error {
		callbacks = append(callbacks, callback)
		*handle = windows.Handle(len(callbacks))
		return nil
	}
	stop, err := startNativeNetworkWindowsMonitor(func() {
		delivered.Add(1)
		pump.notify("network")
	}, nativeNetworkWindowsAPI{
		interfaceChange: register,
		addressChange:   register,
		routeChange:     register,
		cancel: func(windows.Handle) error {
			// Model CancelMibChangeNotify2 waiting for in-flight callbacks.
			callbackMu.Lock()
			defer callbackMu.Unlock()
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	invoke := func(callback uintptr) {
		callbackMu.RLock()
		defer callbackMu.RUnlock()
		syscall.SyscallN(callback, 0, 0, 0)
	}
	invoke(callbacks[0])
	select {
	case <-handlerEntered:
	case <-time.After(5 * time.Second):
		t.Fatal("network event never reached the shared pump")
	}
	var workers sync.WaitGroup
	for _, callback := range callbacks {
		workers.Go(func() {
			for range 256 {
				invoke(callback)
			}
		})
	}
	for range 8 {
		workers.Go(stop)
	}
	done := make(chan struct{})
	go func() { workers.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("network callback or shutdown blocked behind the recovery handler")
	}
	before := delivered.Load()
	for _, callback := range callbacks {
		invoke(callback)
	}
	if delivered.Load() != before {
		t.Fatal("notification delivered after shutdown")
	}
}
