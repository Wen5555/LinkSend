package app

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestQuickSendConcurrentReceiptSurvivesClearAndRestart(t *testing.T) {
	f := newDirectFixtureServices(t)
	if _, err := f.a.Devices(t.Context()); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "quick.txt")
	if err := os.WriteFile(file, []byte("quick fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	main, err := f.a.SaveDraft(SendDraft{ID: "main", Paths: []string{file}}, "")
	if err != nil {
		t.Fatal(err)
	}
	draft, err := f.a.SaveDraft(SendDraft{ID: QuickSendDraftID, PeerID: f.bID.ID(), Paths: []string{file}}, "")
	if err != nil {
		t.Fatal(err)
	}
	var group sync.WaitGroup
	ids := make(chan string, 8)
	errs := make(chan error, 8)
	for range 8 {
		group.Go(func() { item, err := f.a.EnqueueQuickSend(draft.Revision, true); ids <- item.ID; errs <- err })
	}
	group.Wait()
	close(ids)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var id string
	for got := range ids {
		if id != "" && id != got {
			t.Fatal("duplicate quick work")
		}
		id = got
	}
	if _, err = f.a.EnqueueQuickSend(draft.Revision, false); err == nil {
		t.Fatal("changed wait option duplicated a saved intent")
	}
	draft.Paths = nil
	if _, err = f.a.SaveDraft(draft, ""); err != nil {
		t.Fatal(err)
	}
	if receipt, err := f.a.EnqueueQuickSend(draft.Revision, true); err != nil || receipt.ID != id {
		t.Fatal("clear lost receipt", receipt, err)
	}
	actual, _ := f.a.SendDraft("main")
	if actual.Revision != main.Revision || len(actual.Paths) != 1 {
		t.Fatal("quick enqueue mutated main")
	}
	f.a.Shutdown()
	reopened, err := New(Config{DataDir: f.a.cfg.DataDir, ServerURL: f.http.URL, AllowInsecureLoopback: true})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Shutdown()
	if receipt, err := reopened.EnqueueQuickSend(draft.Revision, true); err != nil || receipt.ID != id {
		t.Fatal("restart lost receipt", receipt, err)
	}
	items, err := reopened.queueItems()
	if err != nil || len(items) != 1 || items[0].State != "needs_attention" {
		t.Fatal("restart auto dispatched or duplicated", items, err)
	}
}

func TestQuickSendSavedDraftActuallyTransfersThroughExistingQueue(t *testing.T) {
	f := newDirectFixtureServices(t)
	dest, cfg := fileBudgetReceiver(t, f)
	file := filepath.Join(t.TempDir(), "快速发送.txt")
	payload := []byte("quick transfer integrity")
	if err := os.WriteFile(file, payload, 0600); err != nil {
		t.Fatal(err)
	}
	main, err := f.a.SaveDraft(SendDraft{ID: "main", Paths: []string{file}}, "")
	if err != nil {
		t.Fatal(err)
	}
	draft, err := f.a.SaveDraft(SendDraft{ID: QuickSendDraftID, PeerID: f.bID.ID(), Paths: []string{file}}, "")
	if err != nil {
		t.Fatal(err)
	}
	item, err := f.a.EnqueueQuickSend(draft.Revision, false)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.a.StartQueue(cfg); err != nil {
		t.Fatal(err)
	}
	waitQueue(t, f.a, item.ID, func(item QueueItem) bool { return item.State == "completed" })
	got, err := os.ReadFile(filepath.Join(dest, filepath.Base(file)))
	if err != nil || string(got) != string(payload) {
		t.Fatal("quick receive integrity", err)
	}
	actual, _ := f.a.SendDraft("main")
	if actual.Revision != main.Revision || len(actual.Paths) != 1 {
		t.Fatal("queue changed main draft")
	}
}

func TestQuickSendRevokedTargetAndStaleRevisionDoNotQueue(t *testing.T) {
	f := newDirectFixtureServices(t)
	_, _ = f.a.Devices(t.Context())
	file := filepath.Join(t.TempDir(), "quick.txt")
	_ = os.WriteFile(file, []byte("quick"), 0600)
	draft, err := f.a.SaveDraft(SendDraft{ID: QuickSendDraftID, PeerID: f.bID.ID(), Paths: []string{file}}, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.a.EnqueueQuickSend(draft.Revision+1, true); err == nil {
		t.Fatal("stale revision queued")
	}
	if _, err = f.a.EnqueueQuickSend(draft.Revision, false); err == nil {
		t.Fatal("unconfirmed offline target queued")
	}
	if err = f.a.BlockPeer(f.bID.ID()); err != nil {
		t.Fatal(err)
	}
	if _, err = f.a.EnqueueQuickSend(draft.Revision, true); err == nil {
		t.Fatal("revoked target queued")
	}
	items, err := f.a.queueItems()
	if err != nil || len(items) != 0 {
		t.Fatal("negative case partially queued", items, err)
	}
}
