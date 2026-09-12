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
