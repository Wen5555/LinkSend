package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

const nativeAutostartOwner = "com.linksend.desktop.autostart/v1"

func nativeAutostartExecutable(executable string, mustExist bool) (string, error) {
	if strings.TrimSpace(executable) == "" || strings.ContainsRune(executable, 0) {
		return "", errors.New("AUTOSTART_EXECUTABLE_REQUIRED")
	}
	absolute, err := filepath.Abs(executable)
	if err != nil {
		return "", err
	}
	if mustExist {
		info, err := os.Stat(absolute)
		if err != nil || !info.Mode().IsRegular() {
			return "", errors.New("AUTOSTART_EXECUTABLE_INVALID")
		}
	}
	return absolute, nil
}
