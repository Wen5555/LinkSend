//go:build darwin && cgo

package nativeclipboard

/*
#cgo CFLAGS: -mmacosx-version-min=13.0 -x objective-c
#cgo LDFLAGS: -mmacosx-version-min=13.0 -framework Cocoa
#include <stdlib.h>
#include <stdint.h>
uint64_t linksendClipboardChangeCount(const char *name);
int linksendClipboardReadText(const char *name, int link, uint64_t expected, void **output, size_t *size, uint64_t *current);
int linksendClipboardWritePayload(const char *name, int kind, const void *data, size_t size, uint64_t expected, uint64_t *current);
*/
import "C"

import (
	"context"
	"time"
	"unsafe"

	"github.com/Wen5555/LinkSend/internal/clipboardsync"
)

func clipboardGeneration() uint64 { return uint64(C.linksendClipboardChangeCount(nil)) }
func readClipboardText(ctx context.Context, kind clipboardsync.Kind, expected uint64) ([]byte, uint64, error) {
	if err := ctx.Err(); err != nil {
		return nil, clipboardGeneration(), err
	}
	var output unsafe.Pointer
	var size C.size_t
	var current C.uint64_t
	link := C.int(0)
	if kind == clipboardsync.Link {
		link = 1
	}
	code := C.linksendClipboardReadText(nil, link, C.uint64_t(expected), &output, &size, &current)
	if output != nil {
		defer C.free(output)
	}
	if code == 1 {
		return nil, uint64(current), ErrChanged
	}
	if code != 0 {
		return nil, uint64(current), ErrUnavailable
	}
	if size == 0 || uint64(size) > clipboardsync.MaxTextBytes {
		return nil, uint64(current), clipboardsync.ErrLimit
	}
	return C.GoBytes(output, C.int(size)), uint64(current), nil
}
func writeClipboardPayload(kind clipboardsync.Kind, payload []byte, expected uint64, deadline time.Time) (uint64, error) {
	if !deadline.IsZero() && !time.Now().Before(deadline) {
		return clipboardGeneration(), clipboardsync.ErrExpired
	}
	if len(payload) == 0 {
		return clipboardGeneration(), clipboardsync.ErrStale
	}
	nativeKind := C.int(0)
	if kind == clipboardsync.Link {
		nativeKind = 1
	} else if kind == clipboardsync.Image {
		nativeKind = 2
	}
	var current C.uint64_t
	code := C.linksendClipboardWritePayload(nil, nativeKind, unsafe.Pointer(&payload[0]), C.size_t(len(payload)), C.uint64_t(expected), &current)
	if code == 1 {
		return uint64(current), ErrChanged
	}
	if code != 0 {
		return uint64(current), ErrUnavailable
	}
	return uint64(current), nil
}

func namedPasteboardGeneration(name string) uint64 {
	native := C.CString(name)
	defer C.free(unsafe.Pointer(native))
	return uint64(C.linksendClipboardChangeCount(native))
}

func writeNamedPasteboardText(name string, payload []byte, expected uint64) (uint64, error) {
	native := C.CString(name)
	defer C.free(unsafe.Pointer(native))
	var current C.uint64_t
	code := C.linksendClipboardWritePayload(native, 0, unsafe.Pointer(&payload[0]), C.size_t(len(payload)), C.uint64_t(expected), &current)
	if code == 1 {
		return uint64(current), ErrChanged
	}
	if code != 0 {
		return uint64(current), ErrUnavailable
	}
	return uint64(current), nil
}

func readNamedPasteboardText(name string, expected uint64) ([]byte, uint64, error) {
	native := C.CString(name)
	defer C.free(unsafe.Pointer(native))
	var output unsafe.Pointer
	var size C.size_t
	var current C.uint64_t
	code := C.linksendClipboardReadText(native, 0, C.uint64_t(expected), &output, &size, &current)
	if output != nil {
		defer C.free(output)
	}
	if code == 1 {
		return nil, uint64(current), ErrChanged
	}
	if code != 0 {
		return nil, uint64(current), ErrUnavailable
	}
	return C.GoBytes(output, C.int(size)), uint64(current), nil
}
