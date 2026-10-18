package app

import (
	"bytes"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/Wen5555/LinkSend/internal/transfer"
)

func TestSourceSelectionPauseRestartResumeKeepsExcludedSet(t *testing.T) {
	for _, changed := range []bool{false, true} {
		t.Run(map[bool]string{false: "unchanged", true: "included_source_changed"}[changed], func(t *testing.T) {
			f := newDirectFixtureServices(t)
			project := filepath.Join(t.TempDir(), "project")
			if err := os.MkdirAll(filepath.Join(project, "node_modules"), 0700); err != nil {
				t.Fatal(err)
			}
			payload := bytes.Repeat([]byte("filtered resume\x00"), 900000)
			file, excluded := filepath.Join(project, "payload.bin"), filepath.Join(project, "node_modules", "private.js")
			if err := os.WriteFile(file, payload, 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(excluded, []byte("excluded before"), 0600); err != nil {
				t.Fatal(err)
			}
			destination := t.TempDir()
			cfg := DirectConfig{AllowLoopback: true, CheckTimeout: 5 * time.Second, WaitTimeout: 10 * time.Second}
			if err := f.b.StartInbox(destination, cfg); err != nil {
				t.Fatal(err)
			}
			if err := f.b.SetAlwaysAccept(f.aID.ID(), true); err != nil {
				t.Fatal(err)
			}
			waitFor(t, 5*time.Second, func() bool { return f.b.InboxStatus().Listening }, "receiver not listening")
			item, err := f.a.Enqueue(EnqueueRequest{RequestID: "pause-filtered", PeerID: f.bID.ID(), Paths: []string{project}, SourceSelection: &transfer.SourceSelection{Version: 1, Preset: "source"}})
			if err != nil {
				t.Fatal(err)
			}
			var once sync.Once
			paused := make(chan error, 1)
			cfg.onChunkSent = func(transfer.ChunkTransmission) {
				once.Do(func() {
					tasks := f.a.Tasks()
					if len(tasks) == 0 {
						paused <- errors.New("missing send task")
						return
					}
					paused <- f.a.PauseTask(tasks[0].ID)
				})
			}
			if err := f.a.StartQueue(cfg); err != nil {
				t.Fatal(err)
			}
			select {
			case err = <-paused:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(10 * time.Second):
				t.Fatal("no real first chunk")
			}
			stopped := waitQueue(t, f.a, item.ID, func(item QueueItem) bool { return item.State == "needs_attention" })
			original, ok := f.a.Task(stopped.TaskID)
			if !ok || !original.CanResume {
				t.Fatal("filtered task was not resumable")
			}
			profile := f.a.cfg
			f.a.Shutdown()
			if err := os.WriteFile(excluded, []byte("excluded changed while app stopped"), 0600); err != nil {
				t.Fatal(err)
			}
			if changed {
				if err := os.WriteFile(file, []byte("included source changed"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			reopened, err := New(profile)
			if err != nil {
				t.Fatal(err)
			}
			defer reopened.Shutdown()
			restarted := waitQueue(t, reopened, item.ID, func(item QueueItem) bool { return item.State == "needs_attention" })
			cfg.onChunkSent = nil
			if err := reopened.StartQueue(cfg); err != nil {
				t.Fatal(err)
			}
			waitFor(t, 5*time.Second, func() bool { return f.b.InboxStatus().Listening }, "receiver did not return after pause")
			if err := reopened.ConfirmQueue(restarted.ID, restarted.Revision); err != nil {
				t.Fatal(err)
			}
			terminal := waitQueue(t, reopened, item.ID, func(item QueueItem) bool { return item.State == "completed" || item.State == "needs_attention" })
			if changed {
				if terminal.State != "needs_attention" || terminal.LastError != "SOURCE_CHANGED" {
					t.Fatalf("changed source was reused: %+v", terminal)
				}
			} else {
				if terminal.State != "completed" {
					t.Fatalf("resume failed: %+v", terminal)
				}
				current, _ := reopened.Task(terminal.TaskID)
				if current.ID != original.ID || current.TransferID != original.TransferID || current.AttemptID == original.AttemptID {
					t.Fatal("resume lost original content/task identity")
				}
				got, err := os.ReadFile(filepath.Join(destination, "project", "payload.bin"))
				if err != nil || transfer.Sum(got) != transfer.Sum(payload) {
					t.Fatal("resumed target hash differs")
				}
			}
			if _, err := os.Stat(filepath.Join(destination, "project", "node_modules")); !os.IsNotExist(err) {
				t.Fatal("resume reintroduced excluded files")
			}
		})
	}
}

func TestSourceSelectionDraftQueueRestartAndIdempotency(t *testing.T) {
	f := newDirectFixtureServices(t)
	source := filepath.Join(t.TempDir(), "工程")
	if err := os.MkdirAll(filepath.Join(source, ".git"), 0700); err != nil {
		t.Fatal(err)
	}
	for path, body := range map[string]string{"main.go": "selected", ".git/config": "excluded"} {
		if err := os.WriteFile(filepath.Join(source, filepath.FromSlash(path)), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	selection := &transfer.SourceSelection{Version: 1, Preset: "source", Exclude: []string{"*.tmp"}}
	draft, err := f.a.SaveDraft(SendDraft{Paths: []string{source}, SourceSelection: selection, PeerID: f.bID.ID()}, "")
	if err != nil {
		t.Fatal(err)
	}
	preview, err := f.a.PreviewDraft()
	if err != nil || !preview.Complete || preview.Files != 1 || preview.ExcludedFiles != 1 || preview.Revision != draft.Revision {
		t.Fatalf("persisted draft preview = %+v / %v", preview, err)
	}
	request := EnqueueRequest{RequestID: "filtered", PeerID: f.bID.ID(), Paths: []string{source}, SourceSelection: selection, WaitForPeer: true, DraftID: draft.ID, DraftRevision: draft.Revision}
	item, err := f.a.Enqueue(request)
	if err != nil {
		t.Fatal(err)
	}
	if item.SourceSelection == nil {
		t.Fatal("queue lost selection")
	}
	conflict := request
	conflict.SourceSelection = nil
	if _, err := f.a.Enqueue(conflict); err == nil || err.Error() != "IDEMPOTENCY_CONFLICT" {
		t.Fatalf("same request changed selection: %v", err)
	}
	profile := f.a.cfg.DataDir
	f.a.Shutdown()
	reopened, err := New(Config{DataDir: profile, ServerURL: f.http.URL, AllowInsecureLoopback: true})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Shutdown()
	saved, err := reopened.store.Draft("main")
	if err != nil || !reflect.DeepEqual(saved.SourceSelection, selection) {
		t.Fatalf("draft selection lost on restart: %+v %v", saved, err)
	}
	items, err := reopened.queueItems()
	if err != nil || len(items) != 1 || !reflect.DeepEqual(items[0].SourceSelection, selection) || items[0].State != "needs_attention" {
		t.Fatalf("restart queue = %+v %v", items, err)
	}
	if _, err := reopened.store.db.Exec("UPDATE send_queue SET source_selection=? WHERE id=?", `{"version":99,"preset":"source"}`, item.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := reopened.queueItems(); err == nil {
		t.Fatal("future selection silently became full sending")
	}
}

func TestSourceSelectionQueueActualQUICAndResendKeepsPolicy(t *testing.T) {
	f := newDirectFixtureServices(t)
	source := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(filepath.Join(source, "node_modules"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(source, "空目录"), 0700); err != nil {
		t.Fatal(err)
	}
	for path, body := range map[string]string{"中文.txt": "selected over pinned QUIC", "node_modules/dependency.js": "excluded"} {
		if err := os.WriteFile(filepath.Join(source, filepath.FromSlash(path)), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	destination := t.TempDir()
	cfg := DirectConfig{AllowLoopback: true, CheckTimeout: 5 * time.Second, WaitTimeout: 10 * time.Second}
	if err := f.b.StartInbox(destination, cfg); err != nil {
		t.Fatal(err)
	}
	if err := f.b.SetAlwaysAccept(f.aID.ID(), true); err != nil {
		t.Fatal(err)
	}
	waitFor(t, 5*time.Second, func() bool { return f.b.InboxStatus().Listening }, "receiver unavailable")
	selection := &transfer.SourceSelection{Version: 1, Preset: "source"}
	item, err := f.a.Enqueue(EnqueueRequest{RequestID: "actual-filtered", PeerID: f.bID.ID(), Paths: []string{source}, SourceSelection: selection})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.a.StartQueue(cfg); err != nil {
		t.Fatal(err)
	}
	completed := waitQueue(t, f.a, item.ID, func(item QueueItem) bool { return item.State == "completed" || item.State == "needs_attention" })
	if completed.State != "completed" {
		t.Fatalf("filtered send failed: %+v", completed)
	}
	got, err := os.ReadFile(filepath.Join(destination, "project", "中文.txt"))
	if err != nil || transfer.Sum(got) != transfer.Sum([]byte("selected over pinned QUIC")) {
		t.Fatalf("actual received hash differs: %v", err)
	}
	if _, err := os.Stat(filepath.Join(destination, "project", "node_modules")); !os.IsNotExist(err) {
		t.Fatal("excluded subtree reached receiver")
	}
	if info, err := os.Stat(filepath.Join(destination, "project", "空目录")); err != nil || !info.IsDir() {
		t.Fatal("empty directory lost")
	}
	f.a.tasks.mu.RLock()
	record := f.a.tasks.tasks[completed.TaskID]
	f.a.tasks.mu.RUnlock()
	recovery := record.recoverySnapshot()
	if recovery.Version != 2 || !reflect.DeepEqual(recovery.SourceSelection, selection) || !recoveryUsable(recovery) {
		t.Fatalf("task recovery lost policy: %+v", recovery.SourceSelection)
	}
	if err := f.a.SetQueuePaused(true); err != nil {
		t.Fatal(err)
	}
	resend, err := f.a.ResendInbox(t.Context(), ResendInboxRequest{TaskID: completed.TaskID, RequestID: "filtered-resend", WaitForPeer: true})
	if err != nil || !reflect.DeepEqual(resend.SourceSelection, selection) {
		t.Fatalf("resend lost policy: %+v %v", resend, err)
	}
}

func TestSourceSelectionSchemaNineMigrationBacksUpAndPreserves(t *testing.T) {
	store, file := newTestDesktopStore(t)
	if _, err := store.SaveDraft(SendDraft{Paths: []string{}}, ""); err != nil {
		t.Fatal(err)
	}
	store.Close()
	db, err := sql.Open("sqlite", file)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("ALTER TABLE send_drafts DROP COLUMN source_selection; ALTER TABLE send_queue DROP COLUMN source_selection; PRAGMA user_version=9; UPDATE metadata SET value='9' WHERE key='schema_version'"); err != nil {
		db.Close()
		t.Fatal(err)
	}
	db.Close()
	store, err = openDesktopStore(file)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	draft, err := store.Draft("main")
	if err != nil || draft.SourceSelection != nil || draft.Revision != 1 {
		t.Fatalf("migration changed draft: %+v %v", draft, err)
	}
	backups, err := filepath.Glob(file + ".schema-v9-*.bak")
	if err != nil || len(backups) != 1 {
		t.Fatalf("schema9 backup missing: %v %v", backups, err)
	}
	if err := verifyHistoryBackup(backups[0], 9); err != nil {
		t.Fatal(err)
	}
}

func TestSourceSelectionDraftBindingRejectsStaleOrSubstitutedPolicy(t *testing.T) {
	f := newDirectFixtureServices(t)
	source := filepath.Join(t.TempDir(), "source.txt")
	if err := os.WriteFile(source, []byte("bound draft contents"), 0600); err != nil {
		t.Fatal(err)
	}
	draft, err := f.a.SaveDraft(SendDraft{Paths: []string{source}, PeerID: f.bID.ID(), SourceSelection: &transfer.SourceSelection{Version: 1, Preset: "source"}}, "")
	if err != nil {
		t.Fatal(err)
	}
	request := EnqueueRequest{RequestID: "bound-draft", DraftID: draft.ID, DraftRevision: draft.Revision, PeerID: draft.PeerID, Paths: draft.Paths, SourceSelection: draft.SourceSelection, WaitForPeer: true}
	wrong := request
	wrong.SourceSelection = nil
	if _, err := f.a.Enqueue(wrong); !errors.Is(err, ErrMetadataConflict) {
		t.Fatalf("substituted policy = %v", err)
	}
	draft.SourceSelection = &transfer.SourceSelection{Version: 1, Preset: "all", Exclude: []string{"*.tmp"}}
	draft, err = f.a.SaveDraft(draft, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.a.Enqueue(request); !errors.Is(err, ErrMetadataConflict) {
		t.Fatalf("stale revision = %v", err)
	}
	request.SourceSelection, request.DraftRevision = draft.SourceSelection, draft.Revision
	item, err := f.a.Enqueue(request)
	if err != nil {
		t.Fatal(err)
	}
	if item.DraftID != draft.ID || item.DraftRevision != draft.Revision {
		t.Fatal("content identity lost its saved draft origin")
	}
	draft.Paths = []string{}
	if _, err = f.a.SaveDraft(draft, ""); err != nil {
		t.Fatal(err)
	}
	receipt, err := f.a.Enqueue(request)
	if err != nil || receipt.ID != item.ID {
		t.Fatalf("cleared draft replay lost its receipt: %+v %v", receipt, err)
	}
}
