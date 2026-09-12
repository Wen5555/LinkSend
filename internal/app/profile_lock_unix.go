//go:build darwin || linux || freebsd || openbsd || netbsd || dragonfly

package app

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

func lockProfileFile(file *os.File) error {
	err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
		return ErrProfileInUse
	}
	if err != nil {
		return fmt.Errorf("lock profile: %w", err)
	}
	return nil
}
