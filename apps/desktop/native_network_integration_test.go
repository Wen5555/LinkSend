//go:build windows || darwin

package main

import "testing"

func TestNativeNetworkMonitorRegistersAndStops(t *testing.T) {
	stop, err := startNativeNetworkMonitor(func() {})
	if err != nil {
		t.Fatal(err)
	}
	stop()
	stop()
}
