//go:build darwin || linux || freebsd || openbsd || netbsd || dragonfly

package app

import (
	"fmt"
	"golang.org/x/sys/unix"
	"os"
)

func inboxFileIdentity(file *os.File) (string, error) {
	var info unix.Stat_t
	if err := unix.Fstat(int(file.Fd()), &info); err != nil {
		return "", err
	}
	return fmt.Sprintf("unix:%x:%x", info.Dev, info.Ino), nil
}
