package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// ErrProfileInUse means another live application service owns this profile.
// Callers must not create a second writer or remove the lock file to bypass it.
var ErrProfileInUse = errors.New("PROFILE_IN_USE: this profile is already open in another application service")

// profileLock keeps a kernel lock for the complete service lifetime. The file
// contains no data and is deliberately retained on close: unlinking a lock file
// lets contenders lock different inodes and defeats exclusive ownership.
type profileLock struct {
	file *os.File
	once sync.Once
	err  error
}

func acquireProfileLock(dataDir string) (*profileLock, error) {
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(filepath.Join(dataDir, ".profile.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, fmt.Errorf("open profile lock: %w", err)
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		_ = file.Close()
		if err != nil {
			return nil, fmt.Errorf("inspect profile lock: %w", err)
		}
		return nil, errors.New("PROFILE_LOCK_INVALID: profile lock is not a regular file")
	}
	if err := lockProfileFile(file); err != nil {
		_ = file.Close()
		return nil, err
	}
	return &profileLock{file: file}, nil
}

// Close releases ownership. Kernel locks are also released if the process
// exits or is killed, so neither PID polling nor stale-file deletion is needed.
func (l *profileLock) Close() error {
	if l == nil {
		return nil
	}
	l.once.Do(func() { l.err = l.file.Close() })
	return l.err
}
