//go:build darwin

package main

/*
#cgo CFLAGS: -mmacosx-version-min=13.0 -x objective-c
#cgo LDFLAGS: -mmacosx-version-min=13.0 -framework Foundation
#include <stdlib.h>
char *linksendShareContainerPath(void);
char *linksendResolveShareBookmark(const char *requestID, const char *bookmark);
void linksendReleaseShareAccess(void);
*/
import "C"

import (
	"errors"
	"sync"
	"unsafe"
)

var shareContainer struct {
	sync.Once
	path string
}

func nativeShareRoots(profile string) []string {
	roots := []string{profile}
	shareContainer.Do(func() {
		if value := C.linksendShareContainerPath(); value != nil {
			defer C.free(unsafe.Pointer(value))
			shareContainer.path = C.GoString(value)
		}
	})
	if shareContainer.path != "" && shareContainer.path != profile {
		roots = append(roots, shareContainer.path)
	}
	return roots
}

func resolveNativeShareActivation(activation fileActivation) (fileActivation, error) {
	if len(activation.Bookmarks) == 0 {
		return activation, nil
	}
	paths := append([]string(nil), activation.Paths...)
	for index, encoded := range activation.Bookmarks {
		if encoded == "" {
			continue
		}
		request, bookmark := C.CString(activation.RequestID), C.CString(encoded)
		resolved := C.linksendResolveShareBookmark(request, bookmark)
		C.free(unsafe.Pointer(request))
		C.free(unsafe.Pointer(bookmark))
		if resolved == nil {
			return fileActivation{}, errors.New("SYSTEM_SHARE_AUTHORIZATION: security-scoped bookmark is unavailable")
		}
		paths[index] = C.GoString(resolved)
		C.free(unsafe.Pointer(resolved))
	}
	activation.Paths = paths
	return activation, nil
}

func closeNativeShareAccess() { C.linksendReleaseShareAccess() }
