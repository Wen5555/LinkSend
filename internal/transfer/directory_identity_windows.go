//go:build windows

package transfer

import (
	"os"
	"syscall"
)

func directoryFileIdentity(f *os.File) (DirectoryIdentity, error) {
	var info syscall.ByHandleFileInformation
	if err := syscall.GetFileInformationByHandle(syscall.Handle(f.Fd()), &info); err != nil {
		return DirectoryIdentity{}, err
	}
	return DirectoryIdentity{Device: uint64(info.VolumeSerialNumber), File: uint64(info.FileIndexHigh)<<32 | uint64(info.FileIndexLow)}, nil
}
