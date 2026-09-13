//go:build !windows && !darwin

package main

func startNativeNetworkMonitor(func()) (func(), error) {
	return func() {}, nil
}
