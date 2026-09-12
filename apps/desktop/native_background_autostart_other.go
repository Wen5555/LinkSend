//go:build !windows && !darwin

package main

func installNativeAutostart(string) error         { return errNativeEntryUnsupported }
func uninstallNativeAutostart(string) error       { return errNativeEntryUnsupported }
func nativeAutostartEnabled(string) (bool, error) { return false, errNativeEntryUnsupported }
