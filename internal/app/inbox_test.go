package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Wen5555/LinkSend/internal/protocol"
	"github.com/Wen5555/LinkSend/internal/transfer"
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
	senderDestination := filepath.Join(t.TempDir(), "sender-inbox")
	if err := f.a.StartInbox(senderDestination, cfg); err != nil {
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
	firstSend, err := f.a.StartSend(f.bID.ID(), []string{first}, cfg)
	if err != nil {
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
	firstDone, ok := f.a.Task(firstSend.ID)
	if !ok || firstDone.SessionID == "" {
		t.Fatalf("first transfer did not record its authenticated session: %+v", firstDone)
	}
	waitFor(t, 5*time.Second, func() bool { return f.b.InboxStatus().Listening }, "inbox did not reopen after transfer")
	if status := f.b.InboxStatus(); status.ConnectionCount != 1 {
		t.Fatalf("inbox reauthenticated after one transfer: %+v", status)
	}

	second := filepath.Join(t.TempDir(), "second.txt")
	if err := os.WriteFile(second, []byte("no second prompt"), 0600); err != nil {
		t.Fatal(err)
	}
	secondSend, err := f.a.StartSend(f.bID.ID(), []string{second}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, 10*time.Second, func() bool {
		return latestState(f.a.Tasks(), "send") == "completed" && latestState(f.b.Tasks(), "receive") == "completed" && len(f.b.Tasks()) == 2
	}, "always-accept transfer did not complete without a second decision")
	secondDone, ok := f.a.Task(secondSend.ID)
	if !ok || secondDone.SessionID != firstDone.SessionID {
		t.Fatalf("sequential send did not reuse QUIC session: first=%s second=%s", firstDone.SessionID, secondDone.SessionID)
	}
	reverse := filepath.Join(t.TempDir(), "reverse.txt")
	if err := os.WriteFile(reverse, []byte("reverse on the authenticated QUIC session"), 0600); err != nil {
		t.Fatal(err)
	}
	reverseSend, err := f.b.StartSend(f.aID.ID(), []string{reverse}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	var reverseIncoming string
	waitFor(t, 10*time.Second, func() bool {
		for _, task := range f.a.Tasks() {
			if task.Direction == "receive" && task.State == "awaiting_acceptance" {
				reverseIncoming = task.ID
				return true
			}
		}
		return false
	}, "reverse stream did not reach the original QUIC initiator")
	if err := f.a.AcceptTaskAlways(reverseIncoming); err != nil {
		t.Fatal(err)
	}
	waitFor(t, 10*time.Second, func() bool {
		done, ok := f.b.Task(reverseSend.ID)
		return ok && done.State == "completed"
	}, "reverse stream did not complete")
	reverseDone, _ := f.b.Task(reverseSend.ID)
	if reverseDone.SessionID != firstDone.SessionID {
		t.Fatalf("reverse transfer opened a different QUIC session: first=%s reverse=%s", firstDone.SessionID, reverseDone.SessionID)
	}
	if _, err := os.Stat(filepath.Join(senderDestination, "reverse.txt")); err != nil {
		t.Fatal(err)
	}
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
	f.a.directPoolMu.Lock()
	pooledBeforeBlock := len(f.a.directPool)
	f.a.directPoolMu.Unlock()
	if pooledBeforeBlock == 0 {
		t.Fatal("authenticated session was not retained before revocation")
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	devices, err := f.a.Devices(ctx)
	if err != nil || len(devices) != 2 {
		t.Fatalf("paired devices were not automatically pinned: %v %+v", err, devices)
	}
	if err := f.a.BlockPeer(f.bID.ID()); err != nil {
		t.Fatal(err)
	}
	f.a.directPoolMu.Lock()
	pooledAfterBlock := len(f.a.directPool)
	f.a.directPoolMu.Unlock()
	if pooledAfterBlock != 0 {
		t.Fatal("authorization revocation retained a pooled QUIC session")
	}
	waitFor(t, 5*time.Second, func() bool {
		f.b.directPoolMu.Lock()
		defer f.b.directPoolMu.Unlock()
		return len(f.b.directPool) == 0
	}, "idle pooled session was not reclaimed")
}

func TestCancelledStreamKeepsAuthenticatedSessionForNextFile(t *testing.T) {
	f := newDirectFixtureServices(t)
	t.Cleanup(f.a.Shutdown)
	t.Cleanup(f.b.Shutdown)
	cfg := DirectConfig{AllowLoopback: true, CheckTimeout: 5 * time.Second, WaitTimeout: 10 * time.Second}
	if err := f.b.StartInbox(filepath.Join(t.TempDir(), "received"), cfg); err != nil {
		t.Fatal(err)
	}
	if err := f.a.StartInbox(filepath.Join(t.TempDir(), "sender-inbox"), cfg); err != nil {
		t.Fatal(err)
	}
	waitFor(t, 5*time.Second, func() bool {
		return f.a.InboxStatus().Listening && f.b.InboxStatus().Listening
	}, "inboxes not ready")

	chunkSent := make(chan struct{})
	releaseChunk := make(chan struct{})
	var chunkOnce sync.Once
	cfg.onChunkSent = func(transfer.ChunkTransmission) {
		chunkOnce.Do(func() { close(chunkSent) })
		<-releaseChunk
	}
	firstPath := filepath.Join(t.TempDir(), "cancelled.bin")
	if err := os.WriteFile(firstPath, make([]byte, 2*transfer.DefaultChunkSize+1), 0600); err != nil {
		t.Fatal(err)
	}
	first, err := f.a.StartSend(f.bID.ID(), []string{firstPath}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	var firstIncoming string
	waitFor(t, 10*time.Second, func() bool {
		for _, task := range f.b.Tasks() {
			if task.Direction == "receive" && task.State == "awaiting_acceptance" {
				firstIncoming = task.ID
				return true
			}
		}
		return false
	}, "first stream did not reach consent")
	firstConnected, ok := f.a.Task(first.ID)
	if !ok || firstConnected.SessionID == "" {
		t.Fatalf("missing first session: %+v", firstConnected)
	}
	pending, ok := f.b.Task(firstIncoming)
	if !ok {
		t.Fatal("missing incoming task")
	}
	if _, err := f.b.AcceptIncomingDefault(firstIncoming, pending.AttemptID, pending.Revision, false); err != nil {
		t.Fatal(err)
	}
	select {
	case <-chunkSent:
	case <-time.After(10 * time.Second):
		t.Fatal("first data chunk was not sent")
	}
	if err := f.a.CancelTask(first.ID); err != nil {
		t.Fatal(err)
	}
	close(releaseChunk)
	waitFor(t, 5*time.Second, func() bool {
		task, ok := f.a.Task(first.ID)
		return ok && (task.State == "cancelled" || task.State == "failed")
	}, "cancelled stream did not terminate")
	waitFor(t, 5*time.Second, func() bool {
		task, ok := f.b.Task(firstIncoming)
		return ok && task.State != "awaiting_acceptance"
	}, "receiver did not release cancelled stream")

	secondPath := filepath.Join(t.TempDir(), "second.txt")
	if err := os.WriteFile(secondPath, []byte("session survived stream cancellation"), 0600); err != nil {
		t.Fatal(err)
	}
	second, err := f.a.StartSend(f.bID.ID(), []string{secondPath}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	var secondIncoming string
	waitFor(t, 10*time.Second, func() bool {
		for _, task := range f.b.Tasks() {
			if task.ID != firstIncoming && task.Direction == "receive" && task.State == "awaiting_acceptance" {
				secondIncoming = task.ID
				return true
			}
		}
		return false
	}, "second stream did not reach consent")
	pending, ok = f.b.Task(secondIncoming)
	if !ok {
		t.Fatal("second incoming task disappeared")
	}
	if _, err := f.b.AcceptIncomingDefault(secondIncoming, pending.AttemptID, pending.Revision, false); err != nil {
		t.Fatal(err)
	}
	waitFor(t, 10*time.Second, func() bool {
		task, ok := f.a.Task(second.ID)
		return ok && task.State == "completed"
	}, "second stream did not complete; sender="+fmt.Sprint(f.a.Tasks())+" receiver="+fmt.Sprint(f.b.Tasks()))
	secondDone, _ := f.a.Task(second.ID)
	if secondDone.SessionID != firstConnected.SessionID {
		t.Fatalf("stream cancellation closed QUIC session: first=%s second=%s", firstConnected.SessionID, secondDone.SessionID)
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
