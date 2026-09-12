//go:build !windows

package main

import (
	"errors"
	"golang.org/x/sys/unix"
	"os"
)

func lockActivationFile(file *os.File) error {
	err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
		return errActivationBusy
	}
	return err
}
func syncActivationDirectory(root *os.Root) error {
	directory, err := root.Open(".")
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}
