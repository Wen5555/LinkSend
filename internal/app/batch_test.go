package app

import (
	"bytes"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wen5555/LinkSend/internal/protocol"
	"github.com/Wen5555/LinkSend/internal/transfer"
)

// Persistence fault fixture, independent from the real QUIC tests below:
// commit the cancel intent, then simulate exit before cancelling the task.
func TestBatchCancelledIntentSurvivesCrashGapAndKeepsDurableCompletion(t *testing.T) {
	sender, targets := batchFixture(t)
	draft, root := batchDraft(t, sender, targets)
	batch, err := sender.EnqueueBatch(EnqueueBatchRequest{RequestID: "cancel-crash-gap", DraftID: draft.ID, DraftRevision: draft.Revision, WaitForPeer: true})
	if err != nil {
		t.Fatal(err)
	}
	children := batchChildren(t, sender, batch.ID)
	prepared, err := transfer.PrepareSelected(t.Context(), draft.Paths, 0, draft.SourceSelection)
	if err != nil {
		t.Fatal(err)
	}
	defer prepared.Close()
	makeRecord := func(peer string, completed bool) *taskRecord {
		generation, err := sender.currentAuthorizationGeneration(peer)
		if err != nil {
			t.Fatal(err)
		}
		record, err := sender.tasks.create(TaskSnapshot{Direction: "send", PeerID: peer, AuthorizationGeneration: generation}, func() {})
		if err != nil {
			t.Fatal(err)
		}
		record.mu.Lock()
		record.cancel = nil
		record.mu.Unlock()
		record.updateRecordAttempt("", func(snap *TaskSnapshot, recovery *taskRecovery) {
			snap.TransferID, snap.ManifestDigest, snap.ChunkSize = prepared.Manifest.TransferID, prepared.Manifest.Digest(), prepared.Manifest.ChunkSize
			snap.State, snap.CanResume = "paused", true
			recovery.Version = 2
			recovery.SourceSelection = cloneSourceSelection(draft.SourceSelection)
			recovery.SourcePaths = append([]string{}, draft.Paths...)
			recovery.TransferID, recovery.ManifestDigest, recovery.ChunkSize = prepared.Manifest.TransferID, prepared.Manifest.Digest(), prepared.Manifest.ChunkSize
			recovery.TotalBytes = prepared.Manifest.TotalBytes()
		})
		if completed {
			record.finish("completed", nil)
		}
		return record
	}
	first := makeRecord(targets[0].Identity().ID, true)
	second := makeRecord(targets[1].Identity().ID, false)
	if _, err := sender.store.db.Exec("UPDATE send_queue SET task_id=?,state='running' WHERE id=?", first.snapshot().ID, children[0].ID); err != nil {
		t.Fatal(err)
	}
	if _, err := sender.store.db.Exec("UPDATE send_queue SET task_id=?,state='running' WHERE id=?", second.snapshot().ID, children[1].ID); err != nil {
		t.Fatal(err)
	}
	if _, err := sender.store.db.Exec("UPDATE send_batches SET cancelled=1,revision=revision+1 WHERE id=?", batch.ID); err != nil {
		t.Fatal(err)
	}
	profile := sender.cfg
	sender.Shutdown()
	reopened, err := New(profile)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Shutdown()
	savedDraft, err := reopened.store.Draft(draft.ID)
	if err != nil || !reflect.DeepEqual(savedDraft.PeerIDs, draft.PeerIDs) {
		t.Fatal("restart lost saved target list", err)
	}
	current, err := reopened.GetBatch(batch.ID)
	if err != nil || current.Completed != 1 || current.Stopped != 2 || current.State != "cancelled" {
		t.Fatalf("cancel recovery = %+v / %v", current, err)
	}
	if _, err := reopened.ResumeTask(second.snapshot().ID, DirectConfig{}); !errors.Is(err, ErrBatchCancelled) {
		t.Fatalf("direct resume bypassed cancellation: %v", err)
	}
	if _, err := reopened.RetryTask(second.snapshot().ID); !errors.Is(err, ErrBatchCancelled) {
		t.Fatalf("retry bypassed cancellation: %v", err)
	}
	for _, child := range batchChildren(t, reopened, batch.ID) {
		if child.PeerID == targets[1].Identity().ID {
			if err := reopened.ConfirmQueue(child.ID, child.Revision); !errors.Is(err, ErrBatchCancelled) {
				t.Fatalf("queue confirmation bypassed cancellation: %v", err)
			}
		}
	}
	got, err := os.ReadFile(filepath.Join(root, "中文.txt"))
	if err != nil || string(got) != "original batch contents" {
		t.Fatal("recovery changed source")
	}
}

func TestBatchSchemaTenMigrationPreservesLegacySingleDraft(t *testing.T) {
	store, file := newTestDesktopStore(t)
	draft, err := store.SaveDraft(SendDraft{Paths: []string{}, PeerID: metadataTestPeer, SourceSelection: &transfer.SourceSelection{Version: 1, Preset: "source"}}, "")
	if err != nil {
		t.Fatal(err)
	}
	store.Close()
	db, err := sql.Open("sqlite", file)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`DROP INDEX send_queue_batch_peer; DROP INDEX send_queue_batch_position; ALTER TABLE send_drafts DROP COLUMN peer_ids; ALTER TABLE send_queue DROP COLUMN batch_id; ALTER TABLE send_queue DROP COLUMN batch_position; DROP TABLE send_batches; PRAGMA user_version=10; UPDATE metadata SET value='10' WHERE key='schema_version'`)
	db.Close()
	if err != nil {
		t.Fatal(err)
	}
	store, err = openDesktopStore(file)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	got, err := store.Draft(draft.ID)
	if err != nil || got.PeerID != draft.PeerID || len(got.PeerIDs) != 0 || !reflect.DeepEqual(got.SourceSelection, draft.SourceSelection) {
		t.Fatalf("legacy draft changed: %+v / %v", got, err)
	}
	backups, err := filepath.Glob(file + ".schema-v10-*.bak")
	if err != nil || len(backups) != 1 {
		t.Fatal("schema10 backup missing", err)
	}
	if err := verifyHistoryBackup(backups[0], 10); err != nil {
		t.Fatal(err)
	}
}

func TestBatchRestartNeedsPerTargetConfirmationAndRevocationDoesNotUndoSuccess(t *testing.T) {
	sender, targets := batchFixture(t)
	draft, _ := batchDraft(t, sender, targets)
	batch, err := sender.EnqueueBatch(EnqueueBatchRequest{RequestID: "restart-and-revoke", DraftID: draft.ID, DraftRevision: draft.Revision, WaitForPeer: true})
	if err != nil {
		t.Fatal(err)
	}
	profile := sender.cfg
	sender.Shutdown()
	reopened, err := New(profile)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Shutdown()
	children := batchChildren(t, reopened, batch.ID)
	if len(children) != 3 {
		t.Fatal("restart lost children")
	}
	for _, child := range children {
		if child.State != "needs_attention" || child.LastError != "RESTART_CONFIRMATION_REQUIRED" {
			t.Fatalf("old batch auto-started: %+v", child)
		}
	}
	destination, cfg := startBatchReceiver(t, reopened, targets[0], true)
	if err := reopened.StartQueue(cfg); err != nil {
		t.Fatal(err)
	}
	if len(reopened.Tasks()) != 0 {
		t.Fatal("restart dispatched without per-target confirmation")
	}
	if err := reopened.ConfirmQueue(children[0].ID, children[0].Revision); err != nil {
		t.Fatal(err)
	}
	waitQueue(t, reopened, children[0].ID, func(item QueueItem) bool { return item.State == "completed" })
	if err := reopened.Revoke(t.Context(), targets[1].Identity().ID); err != nil {
		t.Fatal(err)
	}
	if err := reopened.ConfirmQueue(children[1].ID, children[1].Revision); err == nil {
		t.Fatal("revoked child dispatched")
	}
	current, err := reopened.GetBatch(batch.ID)
	if err != nil || current.Completed != 1 {
		t.Fatalf("revocation undid completed target: %+v / %v", current, err)
	}
	if _, err := reopened.CancelBatch(batch.ID, current.Revision); err != nil {
		t.Fatal(err)
	}
	current, err = reopened.GetBatch(batch.ID)
	if err != nil || current.Completed != 1 || current.Stopped != 2 {
		t.Fatalf("batch cancellation lost success: %+v / %v", current, err)
	}
	body, err := os.ReadFile(filepath.Join(destination, "工程", "中文.txt"))
	if err != nil || string(body) != "original batch contents" {
		t.Fatal("successful receive was changed")
	}
}

func TestBatchLateDurableCompletionUpdatesCancelledReceiptAndInvalidatesView(t *testing.T) {
	sender, targets := batchFixture(t)
	draft, _ := batchDraft(t, sender, targets)
	batch, err := sender.EnqueueBatch(EnqueueBatchRequest{RequestID: "late-durable-receipt", DraftID: draft.ID, DraftRevision: draft.Revision, WaitForPeer: true})
	if err != nil {
		t.Fatal(err)
	}
	children := batchChildren(t, sender, batch.ID)
	record, err := sender.tasks.create(TaskSnapshot{Direction: "send", PeerID: targets[0].Identity().ID, BatchID: batch.ID}, func() {})
	if err != nil {
		t.Fatal(err)
	}
	record.mu.Lock()
	record.cancel = nil
	record.mu.Unlock()
	if _, err := sender.store.db.Exec("UPDATE send_queue SET task_id=?,state='running' WHERE id=?", record.snapshot().ID, children[0].ID); err != nil {
		t.Fatal(err)
	}
	if _, err := sender.CancelBatch(batch.ID, batch.Revision); err != nil {
		t.Fatal(err)
	}
	// Persistence fault fixture: the final completed revision becomes durable
	// after the cancellation transaction. No received file is fabricated here.
	if _, err := sender.store.db.Exec("UPDATE tasks SET snapshot=json_set(snapshot,'$.state','completed') WHERE id=?", record.snapshot().ID); err != nil {
		t.Fatal(err)
	}
	before := sender.WorkspaceChange().Revision
	if err := sender.reconcileCancelledBatchReceipts(); err != nil {
		t.Fatal(err)
	}
	current, err := sender.GetBatch(batch.ID)
	if err != nil || current.Completed != 1 || sender.WorkspaceChange().Revision <= before {
		t.Fatalf("late receipt was hidden: %+v / %v", current, err)
	}
}

func batchFixture(t *testing.T) (*Service, []*Service) {
	t.Helper()
	f := newDirectFixtureServices(t)
	targets := []*Service{f.b}
	for range 2 {
		peer, err := New(Config{DataDir: t.TempDir(), ServerURL: f.http.URL, AllowInsecureLoopback: true})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(peer.Shutdown)
		invite, err := f.a.CreateInvitation(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if _, err = peer.Join(t.Context(), invite.Token, "batch target"); err != nil {
			t.Fatal(err)
		}
		targets = append(targets, peer)
	}
	if _, err := f.a.Devices(t.Context()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if t.Failed() {
			items, _ := f.a.queueItems()
			for _, item := range items {
				t.Logf("child=%d peer=%s state=%s error=%s task=%s", item.BatchPosition, item.PeerID[:8], item.State, item.LastError, item.TaskID)
			}
			for _, task := range f.a.Tasks() {
				t.Logf("sender task=%s state=%s phase=%s error=%s resumable=%t", task.ID, task.State, task.Phase, task.ErrorCode, task.CanResume)
			}
			for index, peer := range targets {
				for _, task := range peer.Tasks() {
					t.Logf("target=%d task=%s state=%s phase=%s error=%s", index, task.ID, task.State, task.Phase, task.ErrorCode)
				}
			}
		}
	})
	return f.a, targets
}

func batchDraft(t *testing.T, sender *Service, targets []*Service) (SendDraft, string) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "工程")
	if err := os.MkdirAll(filepath.Join(root, "node_modules"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "空目录"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "中文.txt"), []byte("original batch contents"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "node_modules", "excluded.js"), []byte("excluded"), 0600); err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, peer := range targets {
		ids = append(ids, peer.Identity().ID)
	}
	draft, err := sender.SaveDraft(SendDraft{Paths: []string{root}, PeerIDs: ids, SourceSelection: &transfer.SourceSelection{Version: 1, Preset: "source"}}, "")
	if err != nil {
		t.Fatal(err)
	}
	return draft, root
}

func batchChildren(t *testing.T, sender *Service, id string) []QueueItem {
	t.Helper()
	items, err := sender.queueItems()
	if err != nil {
		t.Fatal(err)
	}
	var result []QueueItem
	for _, item := range items {
		if item.BatchID == id {
			result = append(result, item)
		}
	}
	return result
}

func waitBatchCompleted(t *testing.T, sender *Service, id string) QueueItem {
	t.Helper()
	item := waitQueue(t, sender, id, func(item QueueItem) bool { return item.State == "completed" || item.State == "needs_attention" })
	if item.State != "completed" {
		t.Fatalf("batch target stopped instead of completing: position=%d error=%s task=%s", item.BatchPosition, item.LastError, item.TaskID)
	}
	return item
}

func startBatchReceiver(t *testing.T, sender, peer *Service, accept bool) (string, DirectConfig) {
	t.Helper()
	destination := t.TempDir()
	cfg := DirectConfig{AllowLoopback: true, CheckTimeout: 5 * time.Second, WaitTimeout: 10 * time.Second}
	if err := peer.StartInbox(destination, cfg); err != nil {
		t.Fatal(err)
	}
	if accept {
		if err := peer.SetAlwaysAccept(sender.Identity().ID, true); err != nil {
			t.Fatal(err)
		}
	}
	waitFor(t, 5*time.Second, func() bool { return peer.InboxStatus().Listening }, "batch receiver not listening")
	return destination, cfg
}

func TestBatchAtomicRollbackAndConcurrentIdempotency(t *testing.T) {
	sender, targets := batchFixture(t)
	draft, _ := batchDraft(t, sender, targets)
	request := EnqueueBatchRequest{RequestID: "atomic-batch", DraftID: draft.ID, DraftRevision: draft.Revision, WaitForPeer: true}
	if _, err := sender.Enqueue(EnqueueRequest{RequestID: "partial-single", DraftID: draft.ID, DraftRevision: draft.Revision, PeerID: draft.PeerID, Paths: draft.Paths, SourceSelection: draft.SourceSelection, WaitForPeer: true}); !errors.Is(err, ErrMetadataConflict) {
		t.Fatalf("multi-target draft allowed partial single enqueue: %v", err)
	}
	if _, err := sender.store.db.Exec(`CREATE TRIGGER abort_third_batch BEFORE INSERT ON send_queue WHEN NEW.batch_position=3 BEGIN SELECT RAISE(ABORT,'injected third child failure'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := sender.EnqueueBatch(request); err == nil {
		t.Fatal("fault did not abort batch")
	}
	for _, table := range []string{"send_batches", "send_queue"} {
		var count int
		if err := sender.store.db.QueryRow("SELECT count(*) FROM " + table).Scan(&count); err != nil || count != 0 {
			t.Fatalf("partial %s survived: %d / %v", table, count, err)
		}
	}
	if _, err := sender.store.db.Exec("DROP TRIGGER abort_third_batch"); err != nil {
		t.Fatal(err)
	}
	var workers sync.WaitGroup
	results := make(chan BatchSnapshot, 8)
	errs := make(chan error, 8)
	for range 8 {
		workers.Go(func() {
			batch, err := sender.EnqueueBatch(request)
			if err != nil {
				errs <- err
			} else {
				results <- batch
			}
		})
	}
	workers.Wait()
	close(errs)
	close(results)
	for err := range errs {
		t.Fatal(err)
	}
	var batchID string
	for batch := range results {
		if batch.Total != 3 || batchID != "" && batch.ID != batchID {
			t.Fatal("concurrent batch was duplicated")
		}
		batchID = batch.ID
	}
	children := batchChildren(t, sender, batchID)
	if len(children) != 3 {
		t.Fatal("missing independent children")
	}
	for index, child := range children {
		if child.BatchPosition != index+1 || child.PeerID != draft.PeerIDs[index] || child.AuthorizationGeneration == 0 || child.SourceSelection == nil {
			t.Fatalf("wrong child snapshot: %+v", child)
		}
	}
	changed := request
	changed.WaitForPeer = false
	if _, err := sender.EnqueueBatch(changed); err == nil || err.Error() != "IDEMPOTENCY_CONFLICT" {
		t.Fatalf("request changed options: %v", err)
	}
	draft.Paths = []string{}
	if _, err := sender.SaveDraft(draft, ""); err != nil {
		t.Fatal(err)
	}
	receipt, err := sender.EnqueueBatch(request)
	if err != nil || receipt.ID != batchID {
		t.Fatalf("lost receipt after draft cleared: %+v / %v", receipt, err)
	}
}

func TestBatchPreflightRejectsDuplicateRevokedAndUnconfirmedOffline(t *testing.T) {
	sender, targets := batchFixture(t)
	draft, _ := batchDraft(t, sender, targets)
	duplicate := draft
	duplicate.PeerIDs = append(append([]string{}, draft.PeerIDs...), draft.PeerIDs[0])
	if _, err := sender.SaveDraft(duplicate, ""); err == nil {
		t.Fatal("duplicate target accepted")
	}
	request := EnqueueBatchRequest{RequestID: "preflight", DraftID: draft.ID, DraftRevision: draft.Revision}
	if _, err := sender.EnqueueBatch(request); protocol.ErrorCode(err) != protocol.PeerOffline {
		t.Fatalf("offline without consent: %v", err)
	}
	if err := sender.Revoke(t.Context(), targets[1].Identity().ID); err != nil {
		t.Fatal(err)
	}
	request.WaitForPeer = true
	if _, err := sender.EnqueueBatch(request); err == nil {
		t.Fatal("revoked target accepted")
	}
	var count int
	if err := sender.store.db.QueryRow("SELECT count(*) FROM send_queue").Scan(&count); err != nil || count != 0 {
		t.Fatal("preflight failure partially enqueued", err)
	}
	if _, err := sender.store.db.Exec("UPDATE send_drafts SET peer_ids='null' WHERE id=?", draft.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := sender.store.Draft(draft.ID); err == nil {
		t.Fatal("malformed target list became a single send")
	}
}

func TestBatchMixedTargetsAndOnlyRejectedTargetRetriesOverQUIC(t *testing.T) {
	sender, targets := batchFixture(t)
	draft, _ := batchDraft(t, sender, targets)
	firstDest, cfg := startBatchReceiver(t, sender, targets[0], true)
	batch, err := sender.EnqueueBatch(EnqueueBatchRequest{RequestID: "mixed-targets", DraftID: draft.ID, DraftRevision: draft.Revision, WaitForPeer: true})
	if err != nil {
		t.Fatal(err)
	}
	children := batchChildren(t, sender, batch.ID)
	if err := sender.StartQueue(cfg); err != nil {
		t.Fatal(err)
	}
	waitBatchCompleted(t, sender, children[0].ID)
	secondDest, _ := startBatchReceiver(t, sender, targets[1], false)
	var incoming TaskSnapshot
	waitFor(t, 10*time.Second, func() bool {
		for _, task := range targets[1].Tasks() {
			if task.State == "awaiting_acceptance" {
				incoming = task
				return true
			}
		}
		return false
	}, "second target never requested consent")
	if err := targets[1].RejectTask(incoming.ID); err != nil {
		t.Fatal(err)
	}
	failed := waitQueue(t, sender, children[1].ID, func(item QueueItem) bool { return item.State == "needs_attention" })
	if _, err := sender.RetryTask(failed.TaskID); err == nil || err.Error() != "BATCH_TARGET_REQUIRES_CONFIRMATION" {
		t.Fatalf("batch retry escaped its queue owner: %v", err)
	}
	thirdDest, _ := startBatchReceiver(t, sender, targets[2], true)
	waitBatchCompleted(t, sender, children[2].ID)
	if err := targets[1].SetAlwaysAccept(sender.Identity().ID, true); err != nil {
		t.Fatal(err)
	}
	if err := sender.ConfirmQueue(failed.ID, failed.Revision); err != nil {
		t.Fatal(err)
	}
	waitBatchCompleted(t, sender, failed.ID)
	for _, dest := range []string{firstDest, secondDest, thirdDest} {
		got, err := os.ReadFile(filepath.Join(dest, "工程", "中文.txt"))
		if err != nil || transfer.Sum(got) != transfer.Sum([]byte("original batch contents")) {
			t.Fatalf("target hash differs: %v", err)
		}
		if _, err := os.Stat(filepath.Join(dest, "工程", "node_modules")); !os.IsNotExist(err) {
			t.Fatal("batch reintroduced excluded content")
		}
		entries, err := os.ReadDir(dest)
		if err != nil {
			t.Fatal(err)
		}
		var visible []string
		for _, entry := range entries {
			if !strings.HasPrefix(entry.Name(), ".linksend-") {
				visible = append(visible, entry.Name())
			}
		}
		if len(visible) != 1 || visible[0] != "工程" {
			t.Fatalf("unexpected committed copies: %v", visible)
		}
	}
	current, err := sender.GetBatch(batch.ID)
	if err != nil || current.State != "completed" || current.Completed != 3 {
		t.Fatalf("batch summary = %+v / %v", current, err)
	}
}

func TestBatchSourceChangesBetweenTargetsNeedIndependentConfirmation(t *testing.T) {
	sender, targets := batchFixture(t)
	draft, root := batchDraft(t, sender, targets)
	var destinations []string
	var cfg DirectConfig
	for _, peer := range targets {
		dest, config := startBatchReceiver(t, sender, peer, true)
		destinations = append(destinations, dest)
		cfg = config
	}
	var once sync.Once
	pauseErr := make(chan error, 1)
	cfg.onChunkSent = func(transfer.ChunkTransmission) { once.Do(func() { pauseErr <- sender.SetQueuePaused(true) }) }
	batch, err := sender.EnqueueBatch(EnqueueBatchRequest{RequestID: "source-between-targets", DraftID: draft.ID, DraftRevision: draft.Revision})
	if err != nil {
		t.Fatal(err)
	}
	children := batchChildren(t, sender, batch.ID)
	if err := sender.StartQueue(cfg); err != nil {
		t.Fatal(err)
	}
	waitQueue(t, sender, children[0].ID, func(item QueueItem) bool { return item.State == "completed" })
	if err := <-pauseErr; err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "中文.txt"), []byte("explicitly confirmed new contents"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := sender.SetQueuePaused(false); err != nil {
		t.Fatal(err)
	}
	for _, child := range children[1:] {
		stopped := waitQueue(t, sender, child.ID, func(item QueueItem) bool { return item.State == "needs_attention" })
		if stopped.LastError != "SOURCE_CHANGED" {
			t.Fatalf("source drift bypassed: %+v", stopped)
		}
	}
	second := waitQueue(t, sender, children[1].ID, func(item QueueItem) bool { return item.State == "needs_attention" })
	if err := sender.ConfirmQueue(second.ID, second.Revision); err != nil {
		t.Fatal(err)
	}
	waitQueue(t, sender, second.ID, func(item QueueItem) bool { return item.State == "completed" })
	first, err := os.ReadFile(filepath.Join(destinations[0], "工程", "中文.txt"))
	if err != nil || string(first) != "original batch contents" {
		t.Fatal("first completed target changed")
	}
	secondBody, err := os.ReadFile(filepath.Join(destinations[1], "工程", "中文.txt"))
	if err != nil || string(secondBody) != "explicitly confirmed new contents" {
		t.Fatal("explicitly confirmed child not updated")
	}
	if _, err := os.Stat(filepath.Join(destinations[2], "工程", "中文.txt")); !os.IsNotExist(err) {
		t.Fatal("unconfirmed target received changed source")
	}
}

func TestBatchCancelStopsActiveTaskAndPreservesSource(t *testing.T) {
	sender, targets := batchFixture(t)
	draft, root := batchDraft(t, sender, targets)
	large := bytes.Repeat([]byte("batch cancel\x00"), 1000000)
	if err := os.WriteFile(filepath.Join(root, "large.bin"), large, 0600); err != nil {
		t.Fatal(err)
	}
	_, cfg := startBatchReceiver(t, sender, targets[0], true)
	reached, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	defer once.Do(func() { close(release) })
	cfg.onChunkSent = func(transfer.ChunkTransmission) {
		select {
		case <-reached:
		default:
			close(reached)
		}
		<-release
	}
	batch, err := sender.EnqueueBatch(EnqueueBatchRequest{RequestID: "cancel-active", DraftID: draft.ID, DraftRevision: draft.Revision, WaitForPeer: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := sender.StartQueue(cfg); err != nil {
		t.Fatal(err)
	}
	select {
	case <-reached:
	case <-time.After(10 * time.Second):
		t.Fatal("no real batch chunk")
	}
	if _, err := sender.CancelBatch(batch.ID, batch.Revision); err != nil {
		t.Fatal(err)
	}
	once.Do(func() { close(release) })
	waitFor(t, 5*time.Second, func() bool {
		b, err := sender.GetBatch(batch.ID)
		return err == nil && b.State == "cancelled" && b.Running == 0
	}, "batch cancellation did not settle")
	for _, child := range batchChildren(t, sender, batch.ID) {
		if child.State != "cancelled" {
			t.Fatalf("uncancelled child: %+v", child)
		}
		if child.TaskID != "" {
			if _, err := sender.ResumeTask(child.TaskID, cfg); !errors.Is(err, ErrBatchCancelled) {
				t.Fatalf("resume bypassed batch cancellation: %v", err)
			}
		}
	}
	got, err := os.ReadFile(filepath.Join(root, "large.bin"))
	if err != nil || transfer.Sum(got) != transfer.Sum(large) {
		t.Fatal("cancel altered source file")
	}
}
