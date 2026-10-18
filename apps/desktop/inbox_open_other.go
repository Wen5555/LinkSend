//go:build !windows

package main

import (
	"errors"
	"os/exec"
	"runtime"
)

func openNativeInboxFile(filename string) error {
	if runtime.GOOS != "darwin" {
		return errors.New("NATIVE_REVEAL_UNSUPPORTED")
	}
	command := exec.Command("/usr/bin/open", filename)
	if err := command.Start(); err != nil {
		return err
	}
	go func() { _ = command.Wait() }()
	return nil
}
