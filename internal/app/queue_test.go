package app

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Wen5555/LinkSend/internal/protocol"
)

func waitQueue(t *testing.T, s *Service, id string, condition func(QueueItem) bool) QueueItem {
	t.Helper()
	var found QueueItem
	waitFor(t, 15*time.Second, func() bool {
		items, err := s.queueItems()
		if err != nil {
			return false
		}
		for _, item := range items {
			if item.ID == id {
				found = item
				return condition(item)
			}
		}
		return false
	}, "queue state not reached")
	return found
}

func TestQueueIdempotentConcurrentEnqueueAndRestartConfirmation(t *testing.T) {
	f := newDirectFixtureServices(t)
	source := filepath.Join(t.TempDir(), "queued.txt")
	if err := os.WriteFile(source, []byte("queued content"), 0600); err != nil {
		t.Fatal(err)
	}
	request := EnqueueRequest{RequestID: "same-command", PeerID: f.bID.ID(), Paths: []string{source}, WaitForPeer: true}
	var wg sync.WaitGroup
	results := make(chan QueueItem, 8)
	errs := make(chan error, 8)
	for range 8 {
		wg.Go(func() {
			item, err := f.a.Enqueue(request)
			if err != nil {
				errs <- err
			} else {
				results <- item
			}
		})
	}
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	id := ""
	for result := range results {
		if id != "" && id != result.ID {
			t.Fatal("duplicate logical queue IDs")
		}
		id = result.ID
	}
	items, err := f.a.queueItems()
	if err != nil || len(items) != 1 {
		t.Fatalf("queue=%+v err=%v", items, err)
	}
	if len(f.a.Tasks()) != 0 {
		t.Fatal("offline enqueue opened a transfer")
	}
	other := request
	other.PeerID = f.aID.ID()
	if _, err = f.a.Enqueue(other); err == nil {
		t.Fatal("idempotency ID accepted different payload")
	}
	f.a.Shutdown()
	reopened, err := New(Config{DataDir: f.a.cfg.DataDir})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(reopened.Shutdown)
	item := waitQueue(t, reopened, id, func(v QueueItem) bool { return v.State == "needs_attention" })
	if item.LastError != "RESTART_CONFIRMATION_REQUIRED" {
		t.Fatalf("missing restart gate: %+v", item)
	}
}

func TestQueueOfflinePeerDoesNotBlockOnlinePeerOverQUIC(t *testing.T) {
	f := newDirectFixtureServices(t)
	c, err := New(Config{DataDir: t.TempDir(), ServerURL: f.http.URL, AllowInsecureLoopback: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Shutdown)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	invite, err := f.a.CreateInvitation(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = c.Join(ctx, invite.Token, "c"); err != nil {
		t.Fatal(err)
	}
	destination := t.TempDir()
	cfg := DirectConfig{AllowLoopback: true, CheckTimeout: 5 * time.Second, WaitTimeout: 10 * time.Second}
	if err = c.StartInbox(destination, cfg); err != nil {
		t.Fatal(err)
	}
	if err = c.SetAlwaysAccept(f.aID.ID(), true); err != nil {
		t.Fatal(err)
	}
	waitFor(t, 5*time.Second, func() bool { return c.InboxStatus().Listening }, "third peer not online")
	source := filepath.Join(t.TempDir(), "online.txt")
	body := []byte("bypass offline peer over authenticated QUIC")
	if err = os.WriteFile(source, body, 0600); err != nil {
		t.Fatal(err)
	}
	offline, err := f.a.Enqueue(EnqueueRequest{RequestID: "offline-first", PeerID: f.bID.ID(), Paths: []string{source}, WaitForPeer: true})
	if err != nil {
		t.Fatal(err)
	}
	online, err := f.a.Enqueue(EnqueueRequest{RequestID: "online-second", PeerID: c.Identity().ID, Paths: []string{source}})
	if err != nil {
		t.Fatal(err)
	}
	if err = f.a.StartQueue(cfg); err != nil {
		t.Fatal(err)
	}
	completed := waitQueue(t, f.a, online.ID, func(v QueueItem) bool { return v.State == "completed" })
	if completed.TaskID == "" {
		t.Fatal("queue missing durable task association")
	}
	waitQueue(t, f.a, offline.ID, func(v QueueItem) bool { return v.State == "waiting_peer" })
	got, err := os.ReadFile(filepath.Join(destination, "online.txt"))
	if err != nil || string(got) != string(body) {
		t.Fatalf("QUIC output mismatch: %v", err)
	}
	if len(f.a.Tasks()) != 1 {
		t.Fatal("offline queue consumed a task/connection")
	}
}

func TestQueueSourceChangeRequiresNewPreparationAndExplicitContinue(t *testing.T) {
	f := newDirectFixtureServices(t)
	cfg := DirectConfig{AllowLoopback: true, CheckTimeout: 5 * time.Second, WaitTimeout: 10 * time.Second}
	destination := t.TempDir()
	if err := f.b.StartInbox(destination, cfg); err != nil {
		t.Fatal(err)
	}
	if err := f.b.SetAlwaysAccept(f.aID.ID(), true); err != nil {
		t.Fatal(err)
	}
	waitFor(t, 5*time.Second, func() bool { return f.b.InboxStatus().Listening }, "receiver not online")
	source := filepath.Join(t.TempDir(), "changed.txt")
	if err := os.WriteFile(source, []byte("before"), 0600); err != nil {
		t.Fatal(err)
	}
	item, err := f.a.Enqueue(EnqueueRequest{RequestID: "changed", PeerID: f.bID.ID(), Paths: []string{source}})
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(source, []byte("after-explicit-confirmation"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = f.a.StartQueue(cfg); err != nil {
		t.Fatal(err)
	}
	attention := waitQueue(t, f.a, item.ID, func(v QueueItem) bool { return v.State == "needs_attention" })
	if attention.LastError != string(protocol.SourceChanged) {
		t.Fatalf("change not classified: %+v", attention)
	}
	if _, err = os.Stat(filepath.Join(destination, "changed.txt")); !os.IsNotExist(err) {
		t.Fatal("changed source was sent without confirmation")
	}
	if err = f.a.ConfirmQueue(item.ID, attention.Revision); err != nil {
		t.Fatal(err)
	}
	waitQueue(t, f.a, item.ID, func(v QueueItem) bool { return v.State == "completed" })
	got, err := os.ReadFile(filepath.Join(destination, "changed.txt"))
	if err != nil || string(got) != "after-explicit-confirmation" {
		t.Fatalf("confirmed output: %v", err)
	}
}

func TestWorkspaceEventsCoalesceAndUnsubscribe(t *testing.T) {
	s, err := New(Config{DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Shutdown)
	ch, unsubscribe := s.SubscribeChanges()
	for range 100 {
		s.notifyChange()
	}
	if revision := <-ch; revision != 100 {
		t.Fatalf("last event revision=%d", revision)
	}
	unsubscribe()
	unsubscribe()
	s.notifyChange()
	if _, ok := <-ch; ok {
		t.Fatal("subscription not closed")
	}
}
