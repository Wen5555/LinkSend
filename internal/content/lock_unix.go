//go:build darwin || linux || freebsd || openbsd || netbsd || dragonfly

package content

import (
	"errors"
	"golang.org/x/sys/unix"
	"os"
)

func lockStoreFile(file *os.File) error {
	err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
		return ErrInUse
	}
	return err
}
