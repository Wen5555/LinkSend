//go:build windows

package content

import (
	"errors"
	"golang.org/x/sys/windows"
	"os"
)

func lockStoreFile(file *os.File) error {
	var overlapped windows.Overlapped
	err := windows.LockFileEx(windows.Handle(file.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &overlapped)
	if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
		return ErrInUse
	}
	return err
}
