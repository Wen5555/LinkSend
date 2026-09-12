//go:build !windows

package main

func prepareNativeNotificationHarness(string) (func(), error) { return func() {}, nil }
