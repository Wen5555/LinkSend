package main

import (
	"testing"
	"time"

	coreapp "github.com/Wen5555/LinkSend/internal/app"
)

func TestBackgroundNotificationsHistoryRevisionAndBilateralBarrier(t *testing.T) {
	now := time.Now().UTC()
	stamp := func(offset time.Duration) string { return now.Add(offset).Format(time.RFC3339Nano) }
	tracker := backgroundTaskTracker{started: now, last: now}
	old := coreapp.TaskSnapshot{ID: "old", State: "completed", Revision: 40, BilateralConfirmed: true, StartedAt: stamp(-2 * time.Hour), EndedAt: stamp(-time.Hour)}
	if notes, active := tracker.observe([]coreapp.TaskSnapshot{old}, now); len(notes) != 0 || active {
		t.Fatal("old history alerted")
	}
	incoming := coreapp.TaskSnapshot{ID: "new", AttemptID: "attempt", Direction: "receive", State: "awaiting_acceptance", Revision: 2, StartedAt: stamp(time.Second)}
	if notes, _ := tracker.observe([]coreapp.TaskSnapshot{old, incoming}, now.Add(2*time.Second)); len(notes) != 1 || notes[0].TaskID != "new" {
		t.Fatal(notes)
	}
	incoming.Revision = 3
	if notes, _ := tracker.observe([]coreapp.TaskSnapshot{old, incoming}, now.Add(3*time.Second)); len(notes) != 0 {
		t.Fatal("progress revision repeated request", notes)
	}
	incoming.State = "transferring"
	if _, active := tracker.observe([]coreapp.TaskSnapshot{incoming}, now.Add(4*time.Second)); !active {
		t.Fatal("active body not detected")
	}
	incoming.State = "completed"
	incoming.EndedAt = stamp(5 * time.Second)
	incoming.Revision = 5
	if notes, active := tracker.observe([]coreapp.TaskSnapshot{incoming}, now.Add(6*time.Second)); len(notes) != 0 || active {
		t.Fatal("completion before bilateral ack", notes)
	}
	incoming.BilateralConfirmed = true
	incoming.Revision = 6
	if notes, _ := tracker.observe([]coreapp.TaskSnapshot{incoming}, now.Add(7*time.Second)); len(notes) != 1 {
		t.Fatal("confirmed transition lost", notes)
	}
	if notes, _ := tracker.observe([]coreapp.TaskSnapshot{incoming}, now.Add(8*time.Second)); len(notes) != 0 {
		t.Fatal("terminal replay", notes)
	}
	restarted := backgroundTaskTracker{started: now.Add(9 * time.Second), last: now.Add(9 * time.Second)}
	if notes, _ := restarted.observe([]coreapp.TaskSnapshot{old, incoming}, now.Add(10*time.Second)); len(notes) != 0 {
		t.Fatal("restart replay", notes)
	}
}

func TestSettingsFormCannotUndoNewBackgroundPreference(t *testing.T) {
	a := NewApp()
	a.dataDir = t.TempDir()
	stale := a.Preferences()
	options := BackgroundOptions{CloseMode: "exit", PreventSleep: true}
	if err := a.SetBackgroundOptions(options); err != nil {
		t.Fatal(err)
	}
	stale.DeviceName = "changed name"
	if err := a.SavePreferences(stale); err != nil {
		t.Fatal(err)
	}
	if got := loadPreferences(a.dataDir); got.Background != options || got.DeviceName != "changed name" {
		t.Fatalf("lost newer behavior %+v", got)
	}
	if err := a.SetBackgroundOptions(BackgroundOptions{CloseMode: "background"}); err == nil {
		t.Fatal("background without tray accepted")
	}
	if a.Preferences().Background != options {
		t.Fatal("failed preference mutated")
	}
}
