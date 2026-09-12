//go:build !windows

package main

func installSendTo(string) error   { return errNativeEntryUnsupported }
func uninstallSendTo(string) error { return errNativeEntryUnsupported }
