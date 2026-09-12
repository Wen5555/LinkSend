//go:build windows

package main

import (
	"errors"
	"golang.org/x/sys/windows"
	"os"
)

func lockActivationFile(file *os.File) error {
	var overlapped windows.Overlapped
	err := windows.LockFileEx(windows.Handle(file.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &overlapped)
	if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
		return errActivationBusy
	}
	return err
}

// Windows does not expose directory fsync through os.File.Sync. The activation
// payload is FlushFileBuffers'd before rename; this covers process crashes, not
// an unqualified power-loss guarantee for directory metadata.
func syncActivationDirectory(_ *os.Root) error { return nil }
