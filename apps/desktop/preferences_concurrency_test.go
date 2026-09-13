package main

import (
	"sync"
	"testing"
)

func TestPreferencesConcurrentSaveAndSnapshotDoNotAlias(t *testing.T) {
	a := &App{dataDir: t.TempDir()}
	var workers sync.WaitGroup
	for range 3 {
		workers.Go(func() {
			for range 30 {
				prefs := a.Preferences()
				if len(prefs.InterfacePriority) > 0 {
					prefs.InterfacePriority[0] = "local snapshot mutation"
				}
				_ = a.PreferencesStatus()
				_ = a.EffectiveConfig()
				_ = a.directConfig()
				_ = a.Status()
			}
		})
	}
	for range 12 {
		if err := a.SavePreferences(DesktopPreferences{DeviceName: "并发偏好", InterfacePriority: []string{"original"}}); err != nil {
			t.Fatal(err)
		}
	}
	workers.Wait()
	if got := a.Preferences(); len(got.InterfacePriority) != 1 || got.InterfacePriority[0] != "original" {
		t.Fatalf("snapshot mutated persisted preferences: %+v", got)
	}
}

func TestPreferencesSectionSavePreservesUnownedFieldsAndRejectsStaleRevision(t *testing.T) {
	a := &App{dataDir: t.TempDir()}
	initial := DesktopPreferences{DeviceName: "旧名称", ServerURL: "https://old.example", ReceiveDirectory: t.TempDir()}
	if err := a.SavePreferences(initial); err != nil {
		t.Fatal(err)
	}
	opened := a.Preferences()
	general := opened
	general.DeviceName = "新名称"
	saved, err := a.SavePreferencesSection("general", opened.Revision, general)
	if err != nil {
		t.Fatal(err)
	}
	if saved.DeviceName != "新名称" || saved.ServerURL != initial.ServerURL || saved.ReceiveDirectory != initial.ReceiveDirectory || saved.Revision != opened.Revision+1 {
		t.Fatalf("section save replaced an unrelated field: %+v", saved)
	}
	network := opened
	network.ServerURL = "https://stale.example"
	conflict, err := a.SavePreferencesSection("network", opened.Revision, network)
	if err == nil || conflict.Revision != saved.Revision {
		t.Fatalf("stale section save was not rejected with current snapshot: result=%+v err=%v", conflict, err)
	}
	got := a.Preferences()
	if got.DeviceName != "新名称" || got.ServerURL != initial.ServerURL {
		t.Fatalf("stale save changed preferences: %+v", got)
	}
}

func TestPreferencesSectionSaveAllowsRetryWithoutLosingEarlierCategory(t *testing.T) {
	a := &App{dataDir: t.TempDir()}
	if err := a.SavePreferences(DesktopPreferences{DeviceName: "before", ServerURL: "https://before.example"}); err != nil {
		t.Fatal(err)
	}
	first := a.Preferences()
	general := first
	general.DeviceName = "after"
	if _, err := a.SavePreferencesSection("general", first.Revision, general); err != nil {
		t.Fatal(err)
	}
	current := a.Preferences()
	network := current
	network.ServerURL = "https://after.example"
	if _, err := a.SavePreferencesSection("network", current.Revision, network); err != nil {
		t.Fatal(err)
	}
	got := loadPreferences(a.dataDir)
	if got.DeviceName != "after" || got.ServerURL != "https://after.example" {
		t.Fatalf("category retry lost a field: %+v", got)
	}
}
