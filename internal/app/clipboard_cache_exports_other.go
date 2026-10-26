//go:build !windows

package app

import (
	"errors"
	"os"
	"path/filepath"
)

// Re-check the opened object, not just a possibly stale string. Failure to
// prove the same object is a rejection, never an authority to read cache data.
func clipboardOpenedPath(file *os.File) (string, error) {
	resolved, err := filepath.EvalSymlinks(file.Name())
	if err != nil {
		return "", err
	}
	byName, err := os.Stat(resolved)
	if err != nil {
		return "", err
	}
	byHandle, err := file.Stat()
	if err != nil || !os.SameFile(byName, byHandle) {
		return "", errors.Join(errClipboardCacheOwnership, err)
	}
	return filepath.Abs(resolved)
}
