//go:build darwin && cgo

package nativeclipboard

/*
#cgo CFLAGS: -mmacosx-version-min=13.0 -x objective-c
#cgo LDFLAGS: -mmacosx-version-min=13.0 -framework Cocoa
#include <stdlib.h>
#include <stdint.h>
uint64_t linksendClipboardChangeCount(const char *name);
uint32_t linksendClipboardTypes(const char *name);
int linksendTestPasteboardString(const char *name, const char *value);
int linksendTestPasteboardMarker(const char *name, const char *marker);
*/
import "C"

import (
	"context"
	"sync"
	"time"
	"unsafe"
)

func watchChanges(ctx context.Context, _ uintptr, notify func(Change)) (func(), error) {
	return watchPasteboardChanges(ctx, "", notify)
}

func watchPasteboardChanges(ctx context.Context, name string, notify func(Change)) (func(), error) {
	watchCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	var once sync.Once
	var nativeName *C.char
	if name != "" {
		nativeName = C.CString(name)
	}
	initial := uint64(C.linksendClipboardChangeCount(nativeName))
	go func() {
		if nativeName != nil {
			defer C.free(unsafe.Pointer(nativeName))
		}
		defer close(done)
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()
		last := initial
		for {
			select {
			case <-watchCtx.Done():
				return
			case <-ticker.C:
				sequence := uint64(C.linksendClipboardChangeCount(nativeName))
				if sequence == last {
					continue
				}
				last = sequence
				kinds := uint32(C.linksendClipboardTypes(nativeName))
				notify(Change{Sequence: sequence, Text: kinds&1 != 0, Link: kinds&2 != 0, Image: kinds&4 != 0})
			}
		}
	}()
	return func() { once.Do(func() { cancel(); <-done }) }, nil
}

func setTestPasteboardString(name, value string) bool {
	nativeName, nativeValue := C.CString(name), C.CString(value)
	defer C.free(unsafe.Pointer(nativeName))
	defer C.free(unsafe.Pointer(nativeValue))
	return C.linksendTestPasteboardString(nativeName, nativeValue) == 1
}

func setTestPasteboardMarker(name, marker string) bool {
	nativeName, nativeMarker := C.CString(name), C.CString(marker)
	defer C.free(unsafe.Pointer(nativeName))
	defer C.free(unsafe.Pointer(nativeMarker))
	return C.linksendTestPasteboardMarker(nativeName, nativeMarker) == 1
}
