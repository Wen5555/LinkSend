package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPairingCodePersistentInboxAndAlwaysAccept(t *testing.T) {
	f := newDirectFixtureServices(t)
	t.Cleanup(f.a.Shutdown)
	t.Cleanup(f.b.Shutdown)
	destination := filepath.Join(t.TempDir(), "received")
	cfg := DirectConfig{AllowLoopback: true, CheckTimeout: 5 * time.Second, WaitTimeout: 10 * time.Second}
	if err := f.b.StartInbox(destination, cfg); err != nil {
		t.Fatal(err)
	}
	waitFor(t, 5*time.Second, func() bool { return f.b.InboxStatus().Listening }, "persistent inbox did not start")

	first := filepath.Join(t.TempDir(), "first.txt")
	if err := os.WriteFile(first, []byte("first automatic inbox transfer"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := f.a.StartSend(f.bID.ID(), []string{first}, cfg); err != nil {
		t.Fatal(err)
	}
	var incomingID string
	waitFor(t, 10*time.Second, func() bool {
		for _, task := range f.b.Tasks() {
			if task.Direction == "receive" && task.State == "awaiting_acceptance" {
				incomingID = task.ID
				return true
			}
		}
		return false
	}, "incoming confirmation was not created")
	if err := f.b.AcceptTaskAlways(incomingID); err != nil {
		t.Fatal(err)
	}
	waitFor(t, 10*time.Second, func() bool {
		return latestState(f.a.Tasks(), "send") == "completed" && latestState(f.b.Tasks(), "receive") == "completed"
	}, "confirmed transfer did not complete")
	waitFor(t, 5*time.Second, func() bool { return f.b.InboxStatus().Listening }, "inbox did not reopen after transfer")

	second := filepath.Join(t.TempDir(), "second.txt")
	if err := os.WriteFile(second, []byte("no second prompt"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := f.a.StartSend(f.bID.ID(), []string{second}, cfg); err != nil {
		t.Fatal(err)
	}
	waitFor(t, 10*time.Second, func() bool {
		return latestState(f.a.Tasks(), "send") == "completed" && latestState(f.b.Tasks(), "receive") == "completed" && len(f.b.Tasks()) == 2
	}, "always-accept transfer did not complete without a second decision")
	if _, err := os.Stat(filepath.Join(destination, "first.txt")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(destination, "second.txt")); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	devices, err := f.a.Devices(ctx)
	if err != nil || len(devices) != 2 {
		t.Fatalf("paired devices were not automatically pinned: %v %+v", err, devices)
	}
}

func waitFor(t *testing.T, timeout time.Duration, ready func() bool, message string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if ready() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal(message)
}

func latestState(tasks []TaskSnapshot, direction string) string {
	state := ""
	for _, task := range tasks {
		if task.Direction == direction {
			state = task.State
		}
	}
	return state
}
