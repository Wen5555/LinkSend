package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"image"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Wen5555/LinkSend/internal/content"
)

func contentService(t *testing.T) *Service {
	t.Helper()
	s, err := New(Config{DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Shutdown)
	return s
}

func TestContentDraftDurableIdempotencyAndMetadataOnly(t *testing.T) {
	s := contentService(t)
	request := ContentTextRequest{RequestID: "form-save", Kind: content.Text, Text: "private multiline 中文\n<script>never IPC back</script>"}
	draft, err := s.CreateContentText(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	repeated, err := s.CreateContentText(t.Context(), request)
	if err != nil || repeated.Snapshot.ID != draft.Snapshot.ID || repeated.Revision != draft.Revision {
		t.Fatal("duplicate create changed immutable snapshot", err)
	}
	changed := request
	changed.Text = "different"
	if _, err = s.CreateContentText(t.Context(), changed); err == nil {
		t.Fatal("idempotency key silently changed body")
	}
	encoded, _ := json.Marshal(draft)
	if strings.Contains(string(encoded), "private multiline") || strings.Contains(string(encoded), s.cfg.DataDir) {
		t.Fatal("draft DTO leaked body/path")
	}
	bodyPath, err := s.content.store.OwnedPath(t.Context(), draft.Snapshot.ID)
	if err != nil {
		t.Fatal(err)
	}
	if body, err := os.ReadFile(bodyPath); err != nil || string(body) != request.Text {
		t.Fatal("actual snapshot differs", err)
	}
	s.Shutdown()
	restored, err := New(Config{DataDir: s.cfg.DataDir})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(restored.Shutdown)
	drafts, err := restored.ContentDrafts(t.Context())
	if err != nil || len(drafts) != 1 || drafts[0].Snapshot.ID != draft.Snapshot.ID {
		t.Fatal("draft did not survive restart", err)
	}
	result, err := restored.CleanupContentSnapshots(t.Context())
	if err != nil || result.Removed != 0 || result.Protected != 1 {
		t.Fatalf("live draft cleanup %+v %v", result, err)
	}
	if err = restored.DiscardContentDraft(t.Context(), draft.ID, draft.Revision+1); !errors.Is(err, ErrMetadataConflict) {
		t.Fatal("stale revision discarded draft")
	}
	if err = restored.DiscardContentDraft(t.Context(), draft.ID, draft.Revision); err != nil {
		t.Fatal(err)
	}
	result, err = restored.CleanupContentSnapshots(t.Context())
	if err != nil || result.Removed != 1 {
		t.Fatalf("discarded snapshot not cleaned %+v %v", result, err)
	}
	if _, err = os.Stat(bodyPath); !os.IsNotExist(err) {
		t.Fatal("owned discarded body remains")
	}
}

func TestContentClipboardDraftCapturesOnceAcrossConcurrentRetries(t *testing.T) {
	s := contentService(t)
	var captures atomic.Int32
	capture := func(context.Context) (image.Image, error) {
		captures.Add(1)
		return image.NewNRGBA(image.Rect(0, 0, 2, 3)), nil
	}
	var wg sync.WaitGroup
	results := make(chan ContentDraft, 6)
	errs := make(chan error, 6)
	for range 6 {
		wg.Go(func() {
			draft, err := s.CreateClipboardImage(t.Context(), "same clipboard action", capture)
			results <- draft
			errs <- err
		})
	}
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	id := ""
	for draft := range results {
		if id != "" && id != draft.Snapshot.ID {
			t.Fatal("duplicate capture snapshot")
		}
		id = draft.Snapshot.ID
		if draft.Snapshot.Width != 2 || draft.Snapshot.Height != 3 {
			t.Fatal("native dimensions lost")
		}
	}
	if captures.Load() != 1 {
		t.Fatal("clipboard reread during idempotent retry")
	}
	if _, err := s.CreateClipboardImage(t.Context(), "new clipboard action", func(context.Context) (image.Image, error) { return nil, errors.New("CLIPBOARD_UNAVAILABLE") }); err == nil {
		t.Fatal("clipboard failure reported draft success")
	}
	tooLarge := ContentTextRequest{RequestID: "large", Kind: content.Text, Text: strings.Repeat("x", content.MaxTextBytes+1)}
	if _, err := s.CreateContentText(t.Context(), tooLarge); !errors.Is(err, content.ErrLimit) {
		t.Fatal(err)
	}
	drafts, err := s.ContentDrafts(t.Context())
	if err != nil || len(drafts) != 1 {
		t.Fatal("failed capture/limit left visible draft", err)
	}
}

func TestContentEnqueueCommitsSnapshotAndChoiceAtomically(t *testing.T) {
	f := newDirectFixtureServices(t)
	draft, err := f.a.CreateContentText(t.Context(), ContentTextRequest{RequestID: "queue-body", Kind: content.URL, Text: "https://example.test/path"})
	if err != nil {
		t.Fatal(err)
	}
	request := EnqueueContentRequest{RequestID: "queue-action", DraftID: draft.ID, DraftRevision: draft.Revision, PeerID: f.bID.ID(), AllowFileFallback: true, WaitForPeer: true}
	item, err := f.a.EnqueueContent(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	var snapshotID string
	var fallback bool
	if err = f.a.store.db.QueryRow(`SELECT snapshot_id,allow_file_fallback FROM content_queue WHERE queue_id=?`, item.ID).Scan(&snapshotID, &fallback); err != nil || snapshotID != draft.Snapshot.ID || !fallback {
		t.Fatal("queue missing atomically committed content", err)
	}
	if drafts, err := f.a.ContentDrafts(t.Context()); err != nil || len(drafts) != 0 {
		t.Fatal("consumed content draft remained editable", err)
	}
	if again, err := f.a.EnqueueContent(t.Context(), request); err != nil || again.ID != item.ID {
		t.Fatal("same enqueue created another queue", err)
	}
	changed := request
	changed.AllowFileFallback = false
	if _, err = f.a.EnqueueContent(t.Context(), changed); err == nil {
		t.Fatal("idempotent replay changed fallback decision")
	}
	result, err := f.a.CleanupContentSnapshots(t.Context())
	if err != nil || result.Removed != 0 {
		t.Fatalf("queued body cleaned %+v %v", result, err)
	}
	if err = f.a.CancelQueue(item.ID, item.Revision); err != nil {
		t.Fatal(err)
	}
	result, err = f.a.CleanupContentSnapshots(t.Context())
	if err != nil || result.Removed != 1 {
		t.Fatalf("cancelled queue body not cleaned %+v %v", result, err)
	}
}

func TestContentEnqueueFailurePreservesDraftAndBody(t *testing.T) {
	f := newDirectFixtureServices(t)
	draft, err := f.a.CreateContentText(t.Context(), ContentTextRequest{RequestID: "queue-fail-body", Kind: content.Text, Text: "preserve on database rejection"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.a.store.db.Exec(`CREATE TRIGGER content_queue_fail BEFORE INSERT ON content_queue BEGIN SELECT RAISE(FAIL,'injected content mapping failure'); END`); err != nil {
		t.Fatal(err)
	}
	_, err = f.a.EnqueueContent(t.Context(), EnqueueContentRequest{RequestID: "queue-fail", DraftID: draft.ID, DraftRevision: draft.Revision, PeerID: f.bID.ID(), WaitForPeer: true})
	if err == nil {
		t.Fatal("mapping failure returned success")
	}
	var count int
	if err = f.a.store.db.QueryRow(`SELECT count(*) FROM send_queue`).Scan(&count); err != nil || count != 0 {
		t.Fatal("queue escaped rollback without content metadata", err)
	}
	drafts, err := f.a.ContentDrafts(t.Context())
	if err != nil || len(drafts) != 1 || drafts[0].Revision != draft.Revision {
		t.Fatal("rollback consumed draft", err)
	}
	if _, err = f.a.content.store.OwnedPath(t.Context(), draft.Snapshot.ID); err != nil {
		t.Fatal("rollback lost snapshot", err)
	}
}

func TestContentCleanupPreservesForeignRefsChangedBodiesAndFileDrafts(t *testing.T) {
	s := contentService(t)
	draft, err := s.CreateContentText(t.Context(), ContentTextRequest{RequestID: "cleanup", Kind: content.Text, Text: "owned body"})
	if err != nil {
		t.Fatal(err)
	}
	bodyPath, err := s.content.store.OwnedPath(t.Context(), draft.Snapshot.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.DiscardContentDraft(t.Context(), draft.ID, draft.Revision); err != nil {
		t.Fatal(err)
	}
	if err = s.content.store.Retain(draft.Snapshot.ID, "other-owner:keep"); err != nil {
		t.Fatal(err)
	}
	if result, err := s.CleanupContentSnapshots(t.Context()); err != nil || result.Removed != 0 {
		t.Fatal("foreign ref removed", result, err)
	}
	if err = s.content.store.Release(draft.Snapshot.ID, "other-owner:keep"); err != nil {
		t.Fatal(err)
	}
	if err = s.content.store.Retain(draft.Snapshot.ID, "app-content:test"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.SaveDraft(SendDraft{ID: "main", Paths: []string{bodyPath}}, s.cfg.DataDir); err != nil {
		t.Fatal(err)
	}
	if result, err := s.CleanupContentSnapshots(t.Context()); err != nil || result.Removed != 0 || result.Protected != 1 {
		t.Fatal("ordinary file draft ref ignored", result, err)
	}
	saved, err := s.store.Draft("main")
	if err != nil {
		t.Fatal(err)
	}
	saved.Paths = []string{}
	if _, err = s.SaveDraft(saved, s.cfg.DataDir); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(bodyPath, []byte("replaced by foreign edit"), 0600); err != nil {
		t.Fatal(err)
	}
	if result, err := s.CleanupContentSnapshots(t.Context()); err == nil || result.Removed != 0 {
		t.Fatal("changed user bytes removed", result, err)
	}
	if data, err := os.ReadFile(bodyPath); err != nil || string(data) != "replaced by foreign edit" {
		t.Fatal("changed file not preserved")
	}
}

func TestContentSchema4BackupAndTransactionalUpgrade(t *testing.T) {
	dir := t.TempDir()
	history := filepath.Join(dir, "task-history.sqlite")
	db, err := historyDB(history)
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{`DROP TABLE content_tasks`, `DROP TABLE content_queue`, `DROP TABLE content_drafts`, `PRAGMA user_version=4`, `UPDATE metadata SET value='4' WHERE key='schema_version'`} {
		if _, err = db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	_ = db.Close()
	db, err = historyDB(history)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var version int
	if err = db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil || version != 5 {
		t.Fatal("schema5 missing", err)
	}
	backups, err := filepath.Glob(history + ".schema-v4-*.bak")
	if err != nil || len(backups) != 1 {
		t.Fatal("schema4 backup missing", err)
	}
	backup, err := sql.Open("sqlite", backups[0])
	if err != nil {
		t.Fatal(err)
	}
	defer backup.Close()
	if err = backup.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil || version != 4 {
		t.Fatal("backup no longer schema4", err)
	}
	var count int
	if err = backup.QueryRow(`SELECT count(*) FROM sqlite_master WHERE name='content_drafts'`).Scan(&count); err != nil || count != 0 {
		t.Fatal("migration mutated rollback schema", err)
	}
}
