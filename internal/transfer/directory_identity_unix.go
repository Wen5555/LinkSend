//go:build !windows

package transfer

import (
	"errors"
	"os"
	"syscall"
)

func directoryFileIdentity(f *os.File) (DirectoryIdentity, error) {
	info, err := f.Stat()
	if err != nil {
		return DirectoryIdentity{}, err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return DirectoryIdentity{}, errors.New("DIRECTORY_IDENTITY_UNAVAILABLE")
	}
	return DirectoryIdentity{Device: uint64(stat.Dev), File: uint64(stat.Ino)}, nil
}
