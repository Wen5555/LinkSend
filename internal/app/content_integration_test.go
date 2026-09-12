package app

import (
	"context"
	"errors"
	"image"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wen5555/LinkSend/internal/content"
	"github.com/Wen5555/LinkSend/internal/protocol"
	"github.com/Wen5555/LinkSend/internal/transfer"
)

func startContentInbox(t *testing.T, f directFixtureServices, native bool) DirectConfig {
	t.Helper()
	if native {
		f.b.EnableNativeContentActions()
	}
	cfg := DirectConfig{AllowLoopback: true, CheckTimeout: 5 * time.Second, WaitTimeout: 10 * time.Second}
	if err := f.b.StartInbox(t.TempDir(), cfg); err != nil {
		t.Fatal(err)
	}
	if err := f.b.SetAlwaysAccept(f.aID.ID(), true); err != nil {
		t.Fatal(err)
	}
	waitFor(t, 5*time.Second, func() bool { return f.b.InboxStatus().Listening }, "content receiver not listening")
	return cfg
}

func TestContentQueueRealQUICPersistsNativeAndActions(t *testing.T) {
	for _, kind := range []content.Kind{content.Text, content.URL, content.Image} {
		t.Run(string(kind), func(t *testing.T) {
			f := newDirectFixtureServices(t)
			cfg := startContentInbox(t, f, true)
			var draft ContentDraft
			var err error
			value := "中文完整内容\n保持原样"
			if kind == content.URL {
				value = "https://example.test/safe"
			}
			if kind == content.Image {
				draft, err = f.a.CreateClipboardImage(t.Context(), "image-command", func(context.Context) (image.Image, error) { return image.NewNRGBA(image.Rect(0, 0, 2, 3)), nil })
			} else {
				draft, err = f.a.CreateContentText(t.Context(), ContentTextRequest{RequestID: "text-command", Kind: kind, Text: value})
			}
			if err != nil {
				t.Fatal(err)
			}
			if err = f.a.SetContentSettings(ContentSettings{RetainSentSnapshots: true}); err != nil {
				t.Fatal(err)
			}
			item, err := f.a.EnqueueContent(t.Context(), EnqueueContentRequest{RequestID: "send-native", DraftID: draft.ID, DraftRevision: draft.Revision, PeerID: f.bID.ID(), WaitForPeer: true})
			if err != nil {
				t.Fatal(err)
			}
			if err = f.a.StartQueue(cfg); err != nil {
				t.Fatal(err)
			}
			completed := waitQueue(t, f.a, item.ID, func(item QueueItem) bool { return item.State == "completed" || item.State == "needs_attention" })
			if completed.State != "completed" {
				t.Fatalf("content queue %+v tasks %+v receiver %+v", completed, f.a.Tasks(), f.b.Tasks())
			}
			sent, ok := f.a.Task(completed.TaskID)
			if !ok || !sent.BilateralConfirmed || sent.TLSVersion != 0x304 || sent.Relay {
				t.Fatal("no authenticated actual content transfer", sent)
			}
			var received TaskSnapshot
			waitFor(t, 5*time.Second, func() bool {
				for _, task := range f.b.Tasks() {
					if task.Direction == "receive" && task.State == "completed" {
						received = task
						return true
					}
				}
				return false
			}, "content receive not completed")
			senderRecord, err := f.a.loadContentTask(t.Context(), sent.ID)
			if err != nil {
				t.Fatal(err)
			}
			receiverRecord, err := f.b.loadContentTask(t.Context(), received.ID)
			if err != nil {
				t.Fatal(err)
			}
			if senderRecord.mode != "native" || receiverRecord.mode != "native" || senderRecord.binding == "" || senderRecord.binding != receiverRecord.binding || senderRecord.descriptor.Kind != kind {
				t.Fatal("actual native metadata not bilateral")
			}
			info, err := f.b.ContentTask(t.Context(), received.ID)
			if err != nil || !info.Available || info.Kind != kind {
				t.Fatal("received actions unavailable", info, err)
			}
			if kind != content.Image {
				copied := ""
				if _, err = f.b.CopyReceivedText(t.Context(), received.ID, func(text string) error { copied = text; return nil }); err != nil || copied != value {
					t.Fatal("actual received text cannot be copied", err)
				}
			}
			if result, err := f.a.CleanupContentSnapshots(t.Context()); err != nil || result.Removed != 0 {
				t.Fatal("retained history body lost", result, err)
			}
			// History resend remains native content, with a new logical queue/task.
			resend, err := f.a.ResendInbox(t.Context(), ResendInboxRequest{TaskID: sent.ID, RequestID: "resend", WaitForPeer: true})
			if err != nil || resend.ID == item.ID || resend.Content == nil {
				t.Fatal("resend silently became ordinary file", resend, err)
			}
			if err = f.a.CancelQueue(resend.ID, resend.Revision); err != nil {
				t.Fatal(err)
			}
			if err = f.a.SetContentSettings(ContentSettings{}); err != nil {
				t.Fatal(err)
			}
			if result, err := f.a.CleanupContentSnapshots(t.Context()); err != nil || result.Removed != 1 {
				t.Fatal("terminal owned source did not clean", result, err)
			}
			if _, err = f.a.ResendInbox(t.Context(), ResendInboxRequest{TaskID: sent.ID, RequestID: "after-cleanup", WaitForPeer: true}); err == nil {
				t.Fatal("cleaned content recreated via file fallback")
			}
		})
	}
}

func TestContentQueueDefaultUnsupportedAndExplicitFallback(t *testing.T) {
	for _, allow := range []bool{false, true} {
		t.Run(map[bool]string{false: "default_stop", true: "explicit_file"}[allow], func(t *testing.T) {
			f := newDirectFixtureServices(t)
			cfg := startContentInbox(t, f, false)
			draft, err := f.a.CreateContentText(t.Context(), ContentTextRequest{RequestID: "unsupported", Kind: content.Text, Text: "explicit downgrade only"})
			if err != nil {
				t.Fatal(err)
			}
			item, err := f.a.EnqueueContent(t.Context(), EnqueueContentRequest{RequestID: "send", DraftID: draft.ID, DraftRevision: draft.Revision, PeerID: f.bID.ID(), AllowFileFallback: allow, WaitForPeer: true})
			if err != nil {
				t.Fatal(err)
			}
			if err = f.a.StartQueue(cfg); err != nil {
				t.Fatal(err)
			}
			terminal := waitQueue(t, f.a, item.ID, func(item QueueItem) bool { return item.State == "completed" || item.State == "needs_attention" })
			task, _ := f.a.Task(terminal.TaskID)
			if !allow {
				if terminal.State != "needs_attention" || task.ErrorCode != string(protocol.ContentUnsupported) || task.SentBytes != 0 {
					t.Fatal("unsupported content not stopped truthfully", terminal, task)
				}
				return
			}
			if terminal.State != "completed" {
				t.Fatal("explicit fallback failed", terminal, task)
			}
			record, err := f.a.loadContentTask(t.Context(), terminal.TaskID)
			if err != nil || record.mode != "file" || record.descriptor != nil || record.binding != "" {
				t.Fatal("fallback not persisted", record, err)
			}
			for _, received := range f.b.Tasks() {
				if received.State == "completed" {
					info, err := f.b.ContentTask(t.Context(), received.ID)
					if err != nil || info.Mode != "file" || info.CanCopy || info.CanOpen || info.CanSave {
						t.Fatal("file fallback exposed native actions", info, err)
					}
				}
			}
		})
	}
}

func TestContentAcceptancePersistenceFailureSendsZeroBody(t *testing.T) {
	f := newDirectFixtureServices(t)
	cfg := startContentInbox(t, f, true)
	draft, err := f.a.CreateContentText(t.Context(), ContentTextRequest{RequestID: "persist-fail", Kind: content.Text, Text: "must not reach receiver body"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.a.store.db.Exec(`CREATE TRIGGER content_accept_fail BEFORE UPDATE ON content_tasks BEGIN SELECT RAISE(FAIL,'injected persistence failure'); END`); err != nil {
		t.Fatal(err)
	}
	item, err := f.a.EnqueueContent(t.Context(), EnqueueContentRequest{RequestID: "send", DraftID: draft.ID, DraftRevision: draft.Revision, PeerID: f.bID.ID(), WaitForPeer: true})
	if err != nil {
		t.Fatal(err)
	}
	if err = f.a.StartQueue(cfg); err != nil {
		t.Fatal(err)
	}
	terminal := waitQueue(t, f.a, item.ID, func(item QueueItem) bool { return item.State == "needs_attention" })
	task, _ := f.a.Task(terminal.TaskID)
	if task.SentBytes != 0 || task.BilateralConfirmed {
		t.Fatal("persistence failure crossed acceptance barrier", task)
	}
	for _, received := range f.b.Tasks() {
		if received.ReceivedBytes != 0 || received.CommittedBytes != 0 {
			t.Fatal("receiver read body before persisted content acceptance", received)
		}
	}
	if result, err := f.a.CleanupContentSnapshots(t.Context()); err != nil || result.Removed != 0 {
		t.Fatal("failed resumable content lost its source", result, err)
	}
	if _, err = f.a.store.db.Exec(`DROP TRIGGER content_accept_fail`); err != nil {
		t.Fatal(err)
	}
	waitFor(t, 5*time.Second, func() bool { return f.b.InboxStatus().Listening }, "receiver did not resume listening")
	if err = f.a.ConfirmQueue(terminal.ID, terminal.Revision); err != nil {
		t.Fatal("failed persistence cannot safely resume", err)
	}
	if resumed := waitQueue(t, f.a, terminal.ID, func(item QueueItem) bool { return item.State == "completed" || item.State == "needs_attention" }); resumed.State != "completed" {
		t.Fatal("content resume after persistence recovery failed", resumed, f.a.Tasks(), f.b.Tasks())
	}
}

func TestContentOrphanReferenceRecoveryAndShutdown(t *testing.T) {
	s := contentService(t)
	s.content.mu.Lock()
	store, err := s.contentStoreLocked()
	if err != nil {
		t.Fatal(err)
	}
	orphan, err := store.CreateText(t.Context(), content.Text, "crash after object before SQL", "app-content:"+strings.Repeat("e", 64))
	s.content.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	s.Shutdown()
	restored, err := New(Config{DataDir: s.cfg.DataDir})
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Shutdown()
	result, err := restored.CleanupContentSnapshots(t.Context())
	if err != nil || result.Removed != 1 {
		t.Fatal("crash orphan not reconciled", result, err)
	}
	if _, err = os.Stat(restored.contentObjectPath(orphan.ID)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("orphan body persists", err)
	}
}

func TestContentQueueRestartResumeRetainsExactSnapshot(t *testing.T) {
	f := newDirectFixtureServices(t)
	cfg := startContentInbox(t, f, true)
	draft, err := f.a.CreateContentText(t.Context(), ContentTextRequest{RequestID: "pause-body", Kind: content.Text, Text: strings.Repeat("可恢复\n", 4000)})
	if err != nil {
		t.Fatal(err)
	}
	item, err := f.a.EnqueueContent(t.Context(), EnqueueContentRequest{RequestID: "pause-send", DraftID: draft.ID, DraftRevision: draft.Revision, PeerID: f.bID.ID(), WaitForPeer: true})
	if err != nil {
		t.Fatal(err)
	}
	var once sync.Once
	paused := make(chan error, 1)
	cfg.onChunkSent = func(_ transfer.ChunkTransmission) {
		once.Do(func() {
			tasks := f.a.Tasks()
			if len(tasks) == 0 {
				paused <- errors.New("task missing")
				return
			}
			paused <- f.a.PauseTask(tasks[0].ID)
		})
	}
	if err = f.a.StartQueue(cfg); err != nil {
		t.Fatal(err)
	}
	select {
	case err = <-paused:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("content did not reach real first chunk")
	}
	stopped := waitQueue(t, f.a, item.ID, func(item QueueItem) bool { return item.State == "needs_attention" })
	original, ok := f.a.Task(stopped.TaskID)
	if !ok || !original.CanResume {
		t.Fatal("content pause not resumable", original)
	}
	record, err := f.a.loadContentTask(t.Context(), original.ID)
	if err != nil || record.mode != "native" {
		t.Fatal("accepted type not persisted before pause", err)
	}
	if result, err := f.a.CleanupContentSnapshots(t.Context()); err != nil || result.Removed != 0 {
		t.Fatal("paused source cleaned", result, err)
	}
	profile := f.a.cfg
	f.a.Shutdown()
	reopened, err := New(profile)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Shutdown()
	restarted := waitQueue(t, reopened, item.ID, func(item QueueItem) bool { return item.State == "needs_attention" })
	cfg.onChunkSent = nil
	if err = reopened.StartQueue(cfg); err != nil {
		t.Fatal(err)
	}
	waitFor(t, 5*time.Second, func() bool { return f.b.InboxStatus().Listening }, "receiver not available for restart resume")
	if err = reopened.ConfirmQueue(restarted.ID, restarted.Revision); err != nil {
		t.Fatal(err)
	}
	completed := waitQueue(t, reopened, item.ID, func(item QueueItem) bool { return item.State == "completed" || item.State == "needs_attention" })
	if completed.State != "completed" {
		t.Fatal("restart content resume failed", completed, reopened.Tasks(), f.b.Tasks())
	}
	current, _ := reopened.Task(completed.TaskID)
	after, err := reopened.loadContentTask(t.Context(), current.ID)
	if err != nil || current.ID != original.ID || current.TransferID != original.TransferID || after.snapshotID != draft.Snapshot.ID || after.binding != record.binding || current.AttemptID == original.AttemptID {
		t.Fatal("restart changed content/transfer identity", err, current)
	}
}
