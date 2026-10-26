//go:build windows

package app

import (
	"golang.org/x/sys/windows"
	"os"
)

func clipboardOpenedPath(file *os.File) (string, error) {
	return cacheNativeFinalPath(windows.Handle(file.Fd()))
}
