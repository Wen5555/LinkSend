package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Wen5555/LinkSend/internal/protocol"
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
	if err := f.a.StartInbox(filepath.Join(t.TempDir(), "sender-inbox"), cfg); err != nil {
		t.Fatal(err)
	}
	waitFor(t, 5*time.Second, func() bool { return f.b.InboxStatus().Listening }, "persistent inbox did not start")
	waitFor(t, 5*time.Second, func() bool { return f.a.InboxStatus().Listening }, "sender inbox did not start")
	if status := f.b.InboxStatus(); !status.SignalingConnected || status.ConnectionCount != 1 || status.ConnectedAt == "" {
		t.Fatalf("inbox did not expose its persistent signaling state: %+v", status)
	}

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
	if status := f.b.InboxStatus(); status.ConnectionCount != 1 {
		t.Fatalf("inbox reauthenticated after one transfer: %+v", status)
	}

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
	// One receiver WSS plus one fresh sender WSS per transfer. A reconnecting
	// receiver would make this four and recreate the former offline gap.
	if connections := f.wsConnections.Load(); connections != 2 {
		t.Fatalf("persistent inbox WSS was not reused bidirectionally: connections=%d, want 2", connections)
	}
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

func TestStaleInboxBindReportsNoCandidatesWithoutSignalingTimeout(t *testing.T) {
	f := newDirectFixtureServices(t)
	t.Cleanup(f.a.Shutdown)
	t.Cleanup(f.b.Shutdown)
	receiverCfg := DirectConfig{BindAddress: "192.0.2.1:0", AllowLoopback: true, CheckTimeout: 5 * time.Second, WaitTimeout: 10 * time.Second}
	if err := f.b.StartInbox(filepath.Join(t.TempDir(), "received"), receiverCfg); err != nil {
		t.Fatal(err)
	}
	waitFor(t, 5*time.Second, func() bool { return f.b.InboxStatus().Listening }, "stale-bind inbox did not enter signaling wait")

	source := filepath.Join(t.TempDir(), "source.txt")
	if err := os.WriteFile(source, []byte("stale receiver bind"), 0600); err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	if _, err := f.a.StartSend(f.bID.ID(), []string{source}, DirectConfig{AllowLoopback: true, CheckTimeout: 5 * time.Second}); err != nil {
		t.Fatal(err)
	}
	var failed TaskSnapshot
	waitFor(t, 4*time.Second, func() bool {
		for _, task := range f.a.Tasks() {
			if task.Direction == "send" && task.State == "failed" {
				failed = task
				return true
			}
		}
		return false
	}, "sender did not receive the responder setup failure")
	if failed.ErrorCode != string(protocol.NoCandidates) {
		t.Fatalf("sender error = %s %s, want %s", failed.ErrorCode, failed.ErrorMessage, protocol.NoCandidates)
	}
	if time.Since(started) >= 5*time.Second {
		t.Fatalf("sender waited for its signaling deadline instead of receiving the peer failure: %v", time.Since(started))
	}
	if len(f.b.Tasks()) != 0 {
		t.Fatalf("endpoint setup failure created a misleading incoming task: %+v", f.b.Tasks())
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
