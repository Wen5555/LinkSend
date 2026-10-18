//go:build darwin && cgo

package nativeclipboard

/*
#cgo CFLAGS: -mmacosx-version-min=13.0 -x objective-c
#cgo LDFLAGS: -mmacosx-version-min=13.0 -framework Cocoa
#include <stdlib.h>
#include <stdint.h>
int linksendClipboardReadFiles(const char*, uint64_t, void**, size_t*, uint64_t*);
int linksendClipboardWriteFiles(const char*, const void*, size_t, uint64_t, uint64_t*);
*/
import "C"

import (
	"context"
	"encoding/json"
	"time"
	"unsafe"
)

func readClipboardFiles(_ context.Context, expected uint64) ([]string, uint64, error) {
	return readNamedPasteboardFiles("", expected)
}

func readNamedPasteboardFiles(name string, expected uint64) ([]string, uint64, error) {
	var board *C.char
	if name != "" {
		board = C.CString(name)
		defer C.free(unsafe.Pointer(board))
	}
	var data unsafe.Pointer
	var size C.size_t
	var current C.uint64_t
	code := C.linksendClipboardReadFiles(board, C.uint64_t(expected), &data, &size, &current)
	if data != nil {
		defer C.free(data)
	}
	if code == 1 {
		return nil, uint64(current), ErrChanged
	}
	if code != 0 || data == nil || size == 0 || size > 64<<10 {
		return nil, uint64(current), ErrUnsupported
	}
	var paths []string
	if err := json.Unmarshal(C.GoBytes(data, C.int(size)), &paths); err != nil {
		return nil, uint64(current), err
	}
	return paths, uint64(current), nil
}

func writeClipboardFiles(paths []string, expected uint64, deadline time.Time) (uint64, error) {
	if !deadline.IsZero() && !time.Now().Before(deadline) {
		return Generation(), ErrChanged
	}
	return writeNamedPasteboardFiles("", paths, expected)
}

func writeNamedPasteboardFiles(name string, paths []string, expected uint64) (uint64, error) {
	if err := validateClipboardPaths(paths); err != nil {
		return expected, err
	}
	var board *C.char
	if name != "" {
		board = C.CString(name)
		defer C.free(unsafe.Pointer(board))
	}
	data, _ := json.Marshal(paths)
	var current C.uint64_t
	code := C.linksendClipboardWriteFiles(board, unsafe.Pointer(&data[0]), C.size_t(len(data)), C.uint64_t(expected), &current)
	if code == 1 {
		return uint64(current), ErrChanged
	}
	if code != 0 {
		return uint64(current), ErrUnavailable
	}
	return uint64(current), nil
}
