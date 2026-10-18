package main

import "runtime"

// In pinned Wails beta.18 the Windows CFD cancellation sentinel lives in an
// internal package, so callers cannot use errors.Is against it. Match only its
// exact message at this native-dialog boundary, never in general error handling.
// Verified source: internal/go-common-file-dialog/cfd/errors.go and dialogs_windows.go.
func nativePickerCancelled(err error) bool {
	return runtime.GOOS == "windows" && err != nil && err.Error() == "cancelled by user"
}

func selectedFiles(prompt func() ([]string, error)) ([]string, error) {
	paths, err := prompt()
	if nativePickerCancelled(err) {
		return nil, nil
	}
	return paths, err
}

func selectedDirectory(prompt func() (string, error)) (string, error) {
	path, err := prompt()
	if nativePickerCancelled(err) {
		return "", nil
	}
	return path, err
}
