//go:build windows

package main

import (
	"errors"
	"sync"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const (
	nativeAutostartRunKey   = `Software\Microsoft\Windows\CurrentVersion\Run`
	nativeAutostartOwnerKey = `Software\LinkSend\AutoStart`
	nativeAutostartRunName  = "LinkSend"
)

var nativeAutostartMu sync.Mutex

func installNativeAutostart(executable string) error {
	return installNativeAutostartAt(executable, nativeAutostartRunKey, nativeAutostartOwnerKey)
}

func uninstallNativeAutostart(executable string) error {
	return uninstallNativeAutostartAt(executable, nativeAutostartRunKey, nativeAutostartOwnerKey)
}

func nativeAutostartEnabled(executable string) (bool, error) {
	executable, err := nativeAutostartExecutable(executable, false)
	if err != nil {
		return false, err
	}
	command := windows.ComposeCommandLine([]string{executable, "--background"})
	return nativeAutostartMatches(command, nativeAutostartRunKey, nativeAutostartOwnerKey)
}

func nativeAutostartReadValue(path, name string) (string, error) {
	key, err := registry.OpenKey(registry.CURRENT_USER, path, registry.QUERY_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	defer key.Close()
	value, _, err := key.GetStringValue(name)
	if errors.Is(err, registry.ErrNotExist) {
		return "", nil
	}
	return value, err
}

func nativeAutostartMatches(command, runPath, ownerPath string) (bool, error) {
	current, err := nativeAutostartReadValue(runPath, nativeAutostartRunName)
	if err != nil || current == "" {
		return false, err
	}
	owner, err := nativeAutostartReadValue(ownerPath, "Owner")
	if err != nil {
		return false, err
	}
	ownedCommand, err := nativeAutostartReadValue(ownerPath, "Command")
	if err != nil {
		return false, err
	}
	if owner != nativeAutostartOwner || current != command || ownedCommand != command {
		return false, errors.New("AUTOSTART_CONFLICT: preserve the existing startup entry")
	}
	return true, nil
}

func installNativeAutostartAt(executable, runPath, ownerPath string) error {
	nativeAutostartMu.Lock()
	defer nativeAutostartMu.Unlock()
	executable, err := nativeAutostartExecutable(executable, true)
	if err != nil {
		return err
	}
	command := windows.ComposeCommandLine([]string{executable, "--background"})
	if enabled, err := nativeAutostartMatches(command, runPath, ownerPath); err != nil || enabled {
		return err
	}
	owner, err := nativeAutostartReadValue(ownerPath, "Owner")
	if err != nil {
		return err
	}
	ownedCommand, err := nativeAutostartReadValue(ownerPath, "Command")
	if err != nil {
		return err
	}
	if (owner != "" || ownedCommand != "") && (owner != nativeAutostartOwner || ownedCommand != command) {
		return errors.New("AUTOSTART_CONFLICT: preserve existing startup ownership metadata")
	}
	metadata, existed, err := registry.CreateKey(registry.CURRENT_USER, ownerPath, registry.SET_VALUE)
	if err != nil {
		return err
	}
	if err = metadata.SetStringValue("Owner", nativeAutostartOwner); err == nil {
		err = metadata.SetStringValue("Command", command)
	}
	_ = metadata.Close()
	if err != nil {
		return err
	}
	installed := false
	defer func() {
		if !installed && !existed {
			_ = removeNativeAutostartMetadata(ownerPath, command)
		}
	}()
	run, _, err := registry.CreateKey(registry.CURRENT_USER, runPath, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		if !existed {
			_ = removeNativeAutostartMetadata(ownerPath, command)
		}
		return err
	}
	defer run.Close()
	if current, _, err := run.GetStringValue(nativeAutostartRunName); err == nil && current != "" && current != command {
		return errors.New("AUTOSTART_CHANGED: startup entry changed during registration")
	} else if err != nil && !errors.Is(err, registry.ErrNotExist) {
		return err
	}
	if err := run.SetStringValue(nativeAutostartRunName, command); err != nil {
		return err
	}
	installed = true
	return nil
}

func uninstallNativeAutostartAt(executable, runPath, ownerPath string) error {
	nativeAutostartMu.Lock()
	defer nativeAutostartMu.Unlock()
	executable, err := nativeAutostartExecutable(executable, false)
	if err != nil {
		return err
	}
	command := windows.ComposeCommandLine([]string{executable, "--background"})
	enabled, err := nativeAutostartMatches(command, runPath, ownerPath)
	if err != nil {
		return err
	}
	if enabled {
		key, err := registry.OpenKey(registry.CURRENT_USER, runPath, registry.QUERY_VALUE|registry.SET_VALUE)
		if err != nil {
			return err
		}
		defer key.Close()
		current, _, err := key.GetStringValue(nativeAutostartRunName)
		if err != nil || current != command {
			return errors.New("AUTOSTART_CHANGED: startup entry changed during removal")
		}
		if err := key.DeleteValue(nativeAutostartRunName); err != nil {
			return err
		}
	}
	return removeNativeAutostartMetadata(ownerPath, command)
}

func removeNativeAutostartMetadata(path, command string) error {
	owner, err := nativeAutostartReadValue(path, "Owner")
	if err != nil || owner == "" {
		return err
	}
	ownedCommand, err := nativeAutostartReadValue(path, "Command")
	if err != nil {
		return err
	}
	if owner != nativeAutostartOwner || ownedCommand != command {
		return errors.New("AUTOSTART_NOT_OWNED: ownership metadata was preserved")
	}
	key, err := registry.OpenKey(registry.CURRENT_USER, path, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return err
	}
	for _, name := range []string{"Owner", "Command"} {
		if err := key.DeleteValue(name); err != nil && !errors.Is(err, registry.ErrNotExist) {
			_ = key.Close()
			return err
		}
	}
	names, err := key.ReadValueNames(-1)
	_ = key.Close()
	if err != nil {
		return err
	}
	if len(names) == 0 {
		// DeleteKey refuses if unknown child keys remain; preserve those.
		_ = registry.DeleteKey(registry.CURRENT_USER, path)
	}
	return nil
}
