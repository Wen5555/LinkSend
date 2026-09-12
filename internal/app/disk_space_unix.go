//go:build darwin || linux

package app

import "golang.org/x/sys/unix"

func availableDiskBytes(directory string) (uint64, error) {
	var stat unix.Statfs_t
	if err := unix.Statfs(directory, &stat); err != nil {
		return 0, err
	}
	return stat.Bavail * uint64(stat.Bsize), nil
}
