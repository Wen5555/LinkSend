//go:build windows

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
	"golang.org/x/sys/windows"
)

const (
	sendToName   = "LinkSend.lnk"
	sendToMarker = "LinkSend SendTo (com.linksend.desktop.sendto/v1)"
	sendToArgs   = "--send-files --"
)

var sendToMu sync.Mutex

type sendToShortcut struct{ target, arguments, description, workingDir string }

func installSendTo(executable string) error {
	directory, err := windows.KnownFolderPath(windows.FOLDERID_SendTo, 0)
	if err != nil {
		return fmt.Errorf("SENDTO_DIRECTORY: %w", err)
	}
	return installSendToAt(executable, directory)
}

func uninstallSendTo(executable string) error {
	directory, err := windows.KnownFolderPath(windows.FOLDERID_SendTo, 0)
	if err != nil {
		return fmt.Errorf("SENDTO_DIRECTORY: %w", err)
	}
	return uninstallSendToAt(executable, directory)
}

func installSendToAt(executable, directory string) error {
	sendToMu.Lock()
	defer sendToMu.Unlock()
	executable, err := filepath.Abs(executable)
	if err != nil {
		return err
	}
	info, err := os.Stat(executable)
	if err != nil || !info.Mode().IsRegular() {
		return errors.New("SENDTO_EXECUTABLE: select an existing application executable")
	}
	if err := os.MkdirAll(directory, 0700); err != nil {
		return err
	}
	link := filepath.Join(directory, sendToName)
	if info, err := os.Lstat(link); err == nil {
		if !info.Mode().IsRegular() {
			return errors.New("SENDTO_CONFLICT: existing entry is not an owned shortcut")
		}
		current, err := readSendToShortcut(link)
		if err != nil || !ownsSendTo(current, executable) {
			return errors.New("SENDTO_CONFLICT: preserve the existing shortcut; it belongs to another entry or installation")
		}
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	temp, err := os.CreateTemp(directory, ".linksend-sendto-*.lnk")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	if err := temp.Close(); err != nil {
		_ = os.Remove(tempPath)
		return err
	}
	defer os.Remove(tempPath)
	value := sendToShortcut{target: executable, arguments: sendToArgs, description: sendToMarker, workingDir: filepath.Dir(executable)}
	if err := writeSendToShortcut(tempPath, value); err != nil {
		return fmt.Errorf("SENDTO_CREATE: %w", err)
	}
	// Link creates the final name without replacement. An entry created by
	// another process after the initial check is never overwritten.
	if err := os.Link(tempPath, link); err != nil {
		return fmt.Errorf("SENDTO_CREATE: could not atomically install a new shortcut: %w", err)
	}
	return nil
}

func uninstallSendToAt(executable, directory string) error {
	sendToMu.Lock()
	defer sendToMu.Unlock()
	executable, err := filepath.Abs(executable)
	if err != nil {
		return err
	}
	link := filepath.Join(directory, sendToName)
	info, err := os.Lstat(link)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("SENDTO_NOT_OWNED: existing entry was preserved")
	}
	current, err := readSendToShortcut(link)
	if err != nil || !ownsSendTo(current, executable) {
		return errors.New("SENDTO_NOT_OWNED: existing shortcut was preserved")
	}
	latest, err := os.Lstat(link)
	if err != nil || !os.SameFile(info, latest) {
		return errors.New("SENDTO_CHANGED: existing entry changed and was preserved")
	}
	return os.Remove(link)
}

func ownsSendTo(value sendToShortcut, executable string) bool {
	return value.description == sendToMarker && value.arguments == sendToArgs &&
		canonicalNativePathKey(value.target) == canonicalNativePathKey(executable)
}

// Windows Script Host is used only as the documented Shell Link COM adapter.
// No Run/Exec method or shell command is called; paths remain property values.
func withSendToShortcut(path string, fn func(*ole.IDispatch) error) error {
	done := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		if err := ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED); err != nil {
			var oleErr *ole.OleError
			if !errors.As(err, &oleErr) || oleErr.Code() != 1 { // S_FALSE is already initialised.
				done <- err
				return
			}
		}
		defer ole.CoUninitialize()
		unknown, err := oleutil.CreateObject("WScript.Shell")
		if err != nil {
			done <- err
			return
		}
		defer unknown.Release()
		shell, err := unknown.QueryInterface(ole.IID_IDispatch)
		if err != nil {
			done <- err
			return
		}
		defer shell.Release()
		variant, err := oleutil.CallMethod(shell, "CreateShortcut", path)
		if err != nil {
			done <- err
			return
		}
		defer variant.Clear()
		shortcut := variant.ToIDispatch()
		if shortcut == nil {
			done <- errors.New("shell did not return a shortcut object")
			return
		}
		done <- fn(shortcut)
	}()
	return <-done
}

func readSendToShortcut(path string) (sendToShortcut, error) {
	var result sendToShortcut
	err := withSendToShortcut(path, func(shortcut *ole.IDispatch) error {
		for _, property := range []struct {
			name string
			to   *string
		}{{"TargetPath", &result.target}, {"Arguments", &result.arguments}, {"Description", &result.description}, {"WorkingDirectory", &result.workingDir}} {
			value, err := oleutil.GetProperty(shortcut, property.name)
			if err != nil {
				return err
			}
			*property.to = value.ToString()
			_ = value.Clear()
		}
		return nil
	})
	return result, err
}

func writeSendToShortcut(path string, value sendToShortcut) error {
	return withSendToShortcut(path, func(shortcut *ole.IDispatch) error {
		for _, property := range []struct{ name, value string }{
			{"TargetPath", value.target}, {"Arguments", value.arguments}, {"Description", value.description}, {"WorkingDirectory", value.workingDir},
		} {
			result, err := oleutil.PutProperty(shortcut, property.name, property.value)
			if err != nil {
				return err
			}
			if result != nil {
				_ = result.Clear()
			}
		}
		result, err := oleutil.CallMethod(shortcut, "Save")
		if result != nil {
			_ = result.Clear()
		}
		return err
	})
}
