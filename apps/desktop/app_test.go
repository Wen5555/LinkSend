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
	got.DeviceName = "第二个名称"
	if err := a.SavePreferences(got); err != nil {
		t.Fatalf("second atomic save failed: %v", err)
	}
	if loaded := loadPreferences(dir); loaded.DeviceName != got.DeviceName {
		t.Fatalf("second save did not replace preferences: %#v", loaded)
	}
}

func TestSavePreferencesRejectsInvalidURL(t *testing.T) {
	a := &App{dataDir: t.TempDir()}
	if err := a.SavePreferences(DesktopPreferences{ServerURL: "file:///unsafe"}); err == nil {
		t.Fatal("expected invalid URL error")
	}
}

func TestEffectiveConfigReportsTrimmedSources(t *testing.T) {
	a := &App{prefs: DesktopPreferences{FormatVersion: 1, ServerURL: "https://saved.example", BindAddress: "10.0.0.2:0", STUNURLs: []string{"stun:saved.example:3478"}}, prefsStatus: PreferencesStatus{State: "valid"}}
	t.Setenv("LINKSEND_SERVER_URL", " https://env.example ")
	t.Setenv("LINKSEND_BIND", " 192.168.1.2:0 ")
	t.Setenv("LINKSEND_STUN", " stun:one.example:3478, ,stun:two.example:3478 ")
	got := a.EffectiveConfig()
	if got.ServerURL != "https://env.example" || got.ServerSource != "环境变量 LINKSEND_SERVER_URL" {
		t.Fatalf("unexpected server source: %+v", got)
	}
	if got.BindAddress != "192.168.1.2:0" || got.BindSource != "环境变量 LINKSEND_BIND" {
		t.Fatalf("unexpected bind source: %+v", got)
	}
	if len(got.STUNURLs) != 2 || got.STUNURLs[0] != "stun:one.example:3478" || got.STUNURLs[1] != "stun:two.example:3478" {
		t.Fatalf("unexpected STUN values: %+v", got)
	}
}

func TestLoadPreferencesDetailedPreservesRecoveryState(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "desktop-preferences.json")
	if err := os.WriteFile(path, []byte("{broken"), 0600); err != nil {
		t.Fatal(err)
	}
	got, state := loadPreferencesDetailed(dir)
	if state.State != "corrupt" || got.FormatVersion != 1 {
		t.Fatalf("unexpected state: %#v %#v", got, state)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("corrupt preference was not preserved")
	}
	if err := os.WriteFile(path, []byte(`{"format_version":99}`), 0600); err != nil {
		t.Fatal(err)
	}
	_, state = loadPreferencesDetailed(dir)
	if state.State != "unsupported" {
		t.Fatalf("expected unsupported, got %#v", state)
	}
}

func TestCorruptPreferencesBlockRemoteOperationsUntilSaved(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "desktop-preferences.json"), []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	a := NewApp()
	a.dataDir = dir
	a.prefs, a.prefsStatus = loadPreferencesDetailed(dir)
	a.configBlocked = true
	if _, err := a.StartReceive("", dir); err == nil {
		t.Fatal("expected config block")
	}
	if err := a.SavePreferences(DesktopPreferences{ServerURL: "https://example.test"}); err != nil {
		t.Fatal(err)
	}
	if a.configBlocked || a.prefsStatus.State != "valid" {
		t.Fatal("save did not recover config state")
	}
}
