//go:build darwin

package main

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
)

func TestNativeFinderPersistsBeforeAcknowledgement(t *testing.T) {
	paths := []string{filepath.Join(t.TempDir(), "中文 文件.txt")}
	data, err := json.Marshal(paths)
	if err != nil {
		t.Fatal(err)
	}
	intakeErr := errors.New("ENOSPC: activation journal could not be saved")
	finderServicesState.Lock()
	previous := finderServicesState.onPaths
	finderServicesState.onPaths = func(got []string, workingDir string) error {
		if len(got) != 1 || got[0] != paths[0] || workingDir == "" {
			t.Error("Finder intake lost path/working directory metadata")
		}
		return intakeErr
	}
	finderServicesState.Unlock()
	t.Cleanup(func() {
		finderServicesState.Lock()
		finderServicesState.onPaths = previous
		finderServicesState.Unlock()
	})
	if err := deliverNativeFinderPaths(string(data)); !errors.Is(err, intakeErr) {
		t.Fatalf("intake failure was acknowledged as success: %v", err)
	}
	finderServicesState.Lock()
	finderServicesState.onPaths = nil
	finderServicesState.Unlock()
	if err := deliverNativeFinderPaths(string(data)); err == nil {
		t.Fatal("unready app acknowledged a Finder activation")
	}
}
