package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDesktopPreferencesAtomicRoundTrip(t *testing.T) {
	dir := t.TempDir()
	a := &App{dataDir: dir}
	got := DesktopPreferences{ServerURL: "https://example.test", BindAddress: "192.168.1.10:0", STUNURLs: []string{"stun:example.test:3478"}, ReceiveDirectory: filepath.Join(dir, "downloads"), DeviceName: "测试设备"}
	if err := a.SavePreferences(got); err != nil {
		t.Fatal(err)
	}
	loaded := loadPreferences(dir)
	if loaded.ServerURL != got.ServerURL || loaded.BindAddress != got.BindAddress || loaded.DeviceName != got.DeviceName {
		t.Fatalf("round trip mismatch: %#v", loaded)
	}
	if _, err := os.Stat(filepath.Join(dir, "desktop-preferences.json")); err != nil {
		t.Fatal(err)
	}
}

func TestSavePreferencesRejectsInvalidURL(t *testing.T) {
	a := &App{dataDir: t.TempDir()}
	if err := a.SavePreferences(DesktopPreferences{ServerURL: "file:///unsafe"}); err == nil {
		t.Fatal("expected invalid URL error")
	}
}
