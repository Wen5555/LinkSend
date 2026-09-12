//go:build windows

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

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
	tempDirectory, err := os.MkdirTemp(directory, ".linksend-sendto-")
	if err != nil {
		return err
	}
	// Reserve a directory, not an empty .lnk that COM may try to load as a
	// corrupt existing shortcut. Publish only the complete, released file.
	defer os.Remove(tempDirectory)
	tempPath := filepath.Join(tempDirectory, "entry.lnk")
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
	if value.description != sendToMarker || value.arguments != sendToArgs {
		return false
	}
	if canonicalNativePathKey(value.target) == canonicalNativePathKey(executable) {
		return true
	}
	// Shell Link expands existing DOS 8.3 names when reading its target. For
	// example, CI's TEMP uses RUNNER~1 while GetPath returns runneradmin.
	// Expand only those path components: resolving links or comparing file IDs
	// would also accept a different installation that hard-links the same EXE.
	target, err := sendToLongPathKey(value.target)
	if err != nil {
		return false
	}
	expected, err := sendToLongPathKey(executable)
	return err == nil && target == expected
}

func sendToLongPathKey(path string) (string, error) {
	encoded, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return "", err
	}
	buffer := make([]uint16, 32768)
	n, err := windows.GetLongPathName(encoded, &buffer[0], uint32(len(buffer)))
	if err != nil {
		return "", err
	}
	if n == 0 || n >= uint32(len(buffer)) {
		return "", errors.New("invalid expanded Shell Link target length")
	}
	return canonicalNativePathKey(windows.UTF16ToString(buffer[:n])), nil
}
