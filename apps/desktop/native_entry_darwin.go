//go:build darwin

package main

/*
#cgo CFLAGS: -mmacosx-version-min=13.0 -x objective-c
#cgo LDFLAGS: -mmacosx-version-min=13.0 -framework Cocoa -framework Security
#include <stdlib.h>
int linksendRegisterFinderServices(void);
void linksendUnregisterFinderServices(void);
char *linksendCanonicalProfileDirectory(const char *path);
*/
import "C"

import (
	"encoding/json"
	"errors"
	"os"
	"sync"
	"unsafe"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func nativeExistingDirectoryPath(path string) (string, error) {
	input := C.CString(path)
	defer C.free(unsafe.Pointer(input))
	resolved := C.linksendCanonicalProfileDirectory(input)
	if resolved == nil {
		return "", errors.New("PROFILE_PATH_NOT_DIRECTORY: canonical directory could not be resolved")
	}
	defer C.free(unsafe.Pointer(resolved))
	return C.GoString(resolved), nil
}

var finderServicesState struct {
	sync.Mutex
	onPaths func([]string, string)
}

// Register after the Wails application has a live AppKit main loop and the
// draft callback is ready. Finder can invoke a service immediately on register.
// The current desktop package is unsandboxed. URLs that need security scope or
// point into temporary storage are refused, not treated as durable queue sources.
func registerFinderServices(onPaths func([]string, string)) (func(), error) {
	if onPaths == nil {
		return func() {}, errors.New("FINDER_SERVICE_CALLBACK_REQUIRED")
	}
	finderServicesState.Lock()
	if finderServicesState.onPaths != nil {
		finderServicesState.Unlock()
		return func() {}, errors.New("FINDER_SERVICE_ALREADY_REGISTERED")
	}
	finderServicesState.onPaths = onPaths
	finderServicesState.Unlock()
	code := application.InvokeSyncWithResult(func() int { return int(C.linksendRegisterFinderServices()) })
	if code != 0 {
		finderServicesState.Lock()
		finderServicesState.onPaths = nil
		finderServicesState.Unlock()
		return func() {}, errors.New("FINDER_SERVICE_NOT_READY: AppKit must be running and the services provider must be available")
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			application.InvokeSync(func() { C.linksendUnregisterFinderServices() })
			finderServicesState.Lock()
			finderServicesState.onPaths = nil
			finderServicesState.Unlock()
		})
	}, nil
}

//export linksendReceiveFinderPaths
func linksendReceiveFinderPaths(encoded *C.char) C.int {
	data := C.GoString(encoded)
	if len(data) > maxNativeEntryBytes {
		return 1
	}
	var paths []string
	if err := json.Unmarshal([]byte(data), &paths); err != nil {
		return 1
	}
	workingDir, _ := os.Getwd()
	normalized, err := normalizeNativePaths(paths, workingDir)
	if err != nil || len(normalized) == 0 {
		return 1
	}
	finderServicesState.Lock()
	callback := finderServicesState.onPaths
	finderServicesState.Unlock()
	if callback == nil {
		return 1
	}
	callback(normalized, workingDir)
	return 0
}
