//go:build !darwin

package main

import (
	"errors"
	"os"
)

func nativeExistingDirectoryPath(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", errors.New("PROFILE_PATH_NOT_DIRECTORY")
	}
	return path, nil
}
