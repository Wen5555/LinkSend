package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Wen5555/LinkSend/internal/transfer"
)

func inboxTestService(t *testing.T) *Service {
	t.Helper()
	s, err := New(Config{DataDir: t.TempDir(), AllowInsecureLoopback: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Shutdown)
	return s
}

func inboxIndexedFixture(t *testing.T, s *Service, paths []string, direction string) (*taskRecord, *transfer.Prepared) {
	t.Helper()
	prepared, err := transfer.Prepare(t.Context(), paths, 64<<10)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = prepared.Close() })
	task, err := s.tasks.create(TaskSnapshot{Direction: direction, PeerID: strings.Repeat("a", 64)}, func() {})
	if err != nil {
		t.Fatal(err)
	}
	task.updateRecordAttempt("", func(snap *TaskSnapshot, recovery *taskRecovery) {
		snap.TransferID = prepared.Manifest.TransferID
		snap.ManifestDigest = prepared.Manifest.Digest()
		snap.SourceSummary = sourceSummary(paths)
		snap.ManifestSummary = manifestSummary(prepared.Manifest)
		snap.FileCount = len(prepared.Manifest.Files)
		recovery.SourcePaths = paths
		recovery.TransferID = prepared.Manifest.TransferID
		recovery.ManifestDigest = prepared.Manifest.Digest()
		recovery.ChunkSize = prepared.Manifest.ChunkSize
		recovery.TotalBytes = prepared.Manifest.TotalBytes()
		recovery.FileCount = len(prepared.Manifest.Files)
	})
	if err = s.IndexInboxManifest(t.Context(), task.snap.ID, prepared.Manifest); err != nil {
		t.Fatal(err)
	}
	return task, prepared
}

func TestInboxTenThousandRowsKeysetFiltersAndIndex(t *testing.T) {
	s := inboxTestService(t)
	tx, err := s.store.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	statement, err := tx.Prepare(`INSERT INTO tasks(id,revision,snapshot,recovery,inbox_peer,inbox_direction,inbox_state,inbox_started,inbox_summary) VALUES(?,1,?,'{}',?,?,?,?,?)`)
	if err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 10000; i++ {
		id := fmt.Sprintf("history-%05d", i)
		peer := strings.Repeat("a", 64)
		direction, state := "receive", "completed"
		if i%2 != 0 {
			peer = strings.Repeat("b", 64)
			direction = "send"
		}
		if i%7 == 0 {
			state = "cancelled"
		}
		started := base.Add(time.Duration(i) * time.Second)
		snap := TaskSnapshot{ID: id, TaskID: id, Revision: 1, PeerID: peer, Direction: direction, State: state, StartedAt: started.Format(time.RFC3339Nano), ManifestSummary: fmt.Sprintf("file-%05d.txt", i), BilateralConfirmed: state == "completed"}
		data, _ := json.Marshal(snap)
		if _, err = statement.Exec(id, data, peer, direction, state, started.UnixNano(), inboxFold(snap.ManifestSummary)); err != nil {
			t.Fatal(err)
		}
	}
	_ = statement.Close()
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	query := InboxQuery{PeerID: strings.Repeat("a", 64), Direction: "receive", States: []string{"completed"}, After: base.Add(1000 * time.Second).Format(time.RFC3339), Before: base.Add(1100 * time.Second).Format(time.RFC3339), Limit: 7}
	seen := make(map[string]bool)
	started := time.Now()
	pages := 0
	for {
		page, err := s.Inbox(t.Context(), query)
		if err != nil {
			t.Fatal(err)
		}
		if len(page.Items) > 7 {
			t.Fatal("unbounded page")
		}
		for _, item := range page.Items {
			if seen[item.TaskID] || item.PeerID != query.PeerID || item.Direction != "receive" || item.State != "completed" {
				t.Fatal(item)
			}
			seen[item.TaskID] = true
		}
		pages++
		if page.NextCursor == "" {
			break
		}
		query.Cursor = page.NextCursor
	}
	want := 0
	for i := 1000; i < 1100; i++ {
		if i%2 == 0 && i%7 != 0 {
			want++
		}
	}
	if len(seen) != want {
		t.Fatalf("lost rows: got=%d want=%d", len(seen), want)
	}
	t.Logf("10,000 rows: %d filtered rows in %d keyset pages, %s", len(seen), pages, time.Since(started))
	rows, err := s.store.db.Query(`EXPLAIN QUERY PLAN SELECT id FROM tasks WHERE inbox_peer=? ORDER BY inbox_started DESC,id DESC LIMIT 8`, strings.Repeat("a", 64))
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	used := false
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err = rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(detail, "inbox_task_peer") {
			used = true
		}
	}
	if !used {
		t.Fatal("peer/date query did not use index")
	}
	if _, err = s.Inbox(t.Context(), InboxQuery{Search: "different", Cursor: query.Cursor}); !errors.Is(err, ErrInboxQuery) {
		t.Fatal("cursor could be replayed across filters", err)
	}
}

func TestInboxSearchFindsBeyondSummaryAndRejectsUnboundedInput(t *testing.T) {
	s := inboxTestService(t)
	source := t.TempDir()
	var paths []string
	for _, name := range []string{"alpha.txt", "beta.txt", "gamma.txt", "最末的中文报告FINAL.txt"} {
		p := filepath.Join(source, name)
		if err := os.WriteFile(p, []byte(name), 0600); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, p)
	}
	task, _ := inboxIndexedFixture(t, s, paths, "send")
	task.finish("completed", nil)
	for _, search := range []string{"final", "中文报告", "中文", "txt"} {
		page, err := s.Inbox(t.Context(), InboxQuery{Search: search})
		if err != nil || len(page.Items) != 1 {
			t.Fatalf("full filename search %q failed: %+v %v", search, page, err)
		}
	}
	page, err := s.Inbox(t.Context(), InboxQuery{Search: `" OR 1=1 --`})
	if err != nil || len(page.Items) != 0 {
		t.Fatal("search injection", page, err)
	}
	for _, query := range []InboxQuery{{Limit: 101}, {Limit: -1}, {Search: strings.Repeat("x", 257)}, {Before: "invalid"}, {States: []string{"' OR 1=1"}}, {Cursor: strings.Repeat("a", 1025)}} {
		if _, err = s.Inbox(t.Context(), query); !errors.Is(err, ErrInboxQuery) {
			t.Fatal(query, err)
		}
	}
	files, err := s.InboxFiles(t.Context(), task.snap.ID, "", 2)
	if err != nil || len(files.Files) != 2 || files.NextCursor == "" {
		t.Fatal(files, err)
	}
	next, err := s.InboxFiles(t.Context(), task.snap.ID, files.NextCursor, 2)
	if err != nil || len(next.Files) != 2 || next.Files[0].FileID == files.Files[0].FileID {
		t.Fatal(next, err)
	}
	encoded, _ := json.Marshal(page)
	if strings.Contains(string(encoded), source) {
		t.Fatal("private recovery path crossed Inbox DTO")
	}
}

func TestInboxSchema3BackupAndBoundedStartup(t *testing.T) {
	s := inboxTestService(t)
	profile := s.cfg.DataDir
	dbpath := s.tasks.historyPath
	tx, err := s.store.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 450; i++ {
		snap := TaskSnapshot{ID: fmt.Sprintf("old-%04d", i), TaskID: fmt.Sprintf("old-%04d", i), Revision: 1, State: "completed", Direction: "send", StartedAt: time.Now().Add(time.Duration(i) * time.Second).Format(time.RFC3339Nano)}
		data, _ := json.Marshal(snap)
		if _, err = tx.Exec(`INSERT INTO tasks(id,revision,snapshot,recovery,inbox_state,inbox_started) VALUES(?,1,?,'{}','completed',?)`, snap.ID, data, inboxStarted(snap.StartedAt)); err != nil {
			t.Fatal(err)
		}
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	s.Shutdown()
	reopened, err := New(Config{DataDir: profile})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Shutdown()
	if len(reopened.tasks.tasks) != 200 {
		t.Fatalf("startup loaded all terminal history: %d", len(reopened.tasks.tasks))
	}
	page, err := reopened.Inbox(t.Context(), InboxQuery{Limit: 100})
	if err != nil || len(page.Items) != 100 || page.NextCursor == "" {
		t.Fatal(page, err)
	}
	if workspace, err := reopened.Workspace(); err != nil || len(workspace.Tasks) > 200 {
		t.Fatal("workspace exported all history", err)
	}
	_ = dbpath
}

func TestInboxForgetTombstonePreventsLateRewriteAndProtectsRecovery(t *testing.T) {
	s := inboxTestService(t)
	task, err := s.tasks.create(TaskSnapshot{Direction: "send"}, func() {})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.ForgetInboxRecords([]InboxRecordRef{{TaskID: task.snap.ID, Revision: task.snap.Revision}}); !errors.Is(err, ErrInboxProtected) {
		t.Fatal("active record deleted", err)
	}
	task.finish("completed", nil)
	snap := task.snapshot()
	recovery := task.recoverySnapshot()
	if err = s.ForgetInboxRecords([]InboxRecordRef{{TaskID: snap.ID, Revision: snap.Revision - 1}}); !errors.Is(err, ErrMetadataConflict) {
		t.Fatal(err)
	}
	if err = s.ForgetInboxRecords([]InboxRecordRef{{TaskID: snap.ID, Revision: snap.Revision}}); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Task(snap.ID); ok {
		t.Fatal("forgotten task remained in live workspace")
	}
	snap.Revision += 100
	if err = s.tasks.persistRecord(snap, recovery); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = s.store.db.QueryRow(`SELECT count(*) FROM tasks WHERE id=?`, snap.ID).Scan(&count); err != nil || count != 0 {
		t.Fatal("late observer resurrected task", count, err)
	}
	protected := TaskSnapshot{ID: "protected", TaskID: "protected", Revision: 1, Direction: "send", State: "failed"}
	r := taskRecovery{Version: 1, Direction: "send", PeerID: strings.Repeat("a", 64), PeerFingerprint: strings.Repeat("a", 64), SourcePaths: []string{"source"}, TransferID: strings.Repeat("a", 32), ManifestDigest: strings.Repeat("b", 64), ChunkSize: 64 << 10}
	if err = upsertTask(s.store.db, protected, r); err != nil {
		t.Fatal(err)
	}
	if err = s.ForgetInboxRecords([]InboxRecordRef{{TaskID: protected.ID, Revision: 1}}); !errors.Is(err, ErrInboxProtected) {
		t.Fatal("recoverable failure deleted", err)
	}
}

func inboxReceivedFixture(t *testing.T, s *Service) (*taskRecord, *transfer.Receiver, transfer.Manifest, string) {
	t.Helper()
	source := filepath.Join(t.TempDir(), "中文 received.txt")
	if err := os.WriteFile(source, []byte("received file must survive staging cleanup"), 0600); err != nil {
		t.Fatal(err)
	}
	task, p := inboxIndexedFixture(t, s, []string{source}, "receive")
	dest := t.TempDir()
	plan, err := transfer.BuildReceivePlan(t.Context(), dest, p.Manifest, transfer.PlanRequest{ConflictPolicy: transfer.ConflictKeepBoth})
	if err != nil {
		t.Fatal(err)
	}
	task.updateRecordAttempt("", func(snap *TaskSnapshot, recovery *taskRecovery) {
		snap.TargetDirectory = dest
		recovery.TargetDirectory = dest
	})
	r, err := transfer.OpenReceiverWithPlan(t.Context(), dest, task.snap.PeerID, p.Manifest, plan)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = r.Close() })
	if err = s.RegisterInboxReceivePlan(task.snap.ID, p.Manifest, plan); err != nil {
		t.Fatal(err)
	}
	buffer := make([]byte, p.Manifest.ChunkSize)
	for _, file := range p.Manifest.Files {
		for i := range file.Chunks {
			data, err := p.ReadChunk(t.Context(), file.ID, i, buffer)
			if err != nil {
				t.Fatal(err)
			}
			if err = r.WriteChunk(t.Context(), file.ID, i, data); err != nil {
				t.Fatal(err)
			}
		}
	}
	return task, r, p.Manifest, dest
}

func TestInboxLocateAndSeparateOwnedCleanupPreservesReceivedFile(t *testing.T) {
	s := inboxTestService(t)
	task, r, m, dest := inboxReceivedFixture(t, s)
	if result, err := s.CleanupInboxStaging(t.Context(), 20); err != nil || len(result.Protected) != 1 || len(result.Removed) != 0 {
		t.Fatal("active staging was not protected", result, err)
	}
	if err := r.Finish(t.Context()); err != nil {
		t.Fatal(err)
	}
	_ = r.Close()
	task.update(func(snap *TaskSnapshot) { snap.BilateralConfirmed = true })
	task.finish("completed", nil)
	location, err := s.ResolveInboxFile(t.Context(), task.snap.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(location)
	if strings.Contains(string(encoded), dest) {
		t.Fatal("reveal path exposed to JS")
	}
	before, err := os.ReadFile(location.Path)
	if err != nil {
		t.Fatal(err)
	}
	if result, err := s.CleanupInboxStaging(t.Context(), 20); err != nil || len(result.Removed) != 1 || len(result.Issues) != 0 {
		t.Fatal(result, err)
	}
	if after, err := os.ReadFile(location.Path); err != nil || string(after) != string(before) {
		t.Fatal("received user file deleted/changed", err)
	}
	if _, err := os.Stat(filepath.Join(dest, ".linksend-"+m.TransferID)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("registered staging survived", err)
	}
	if err = os.Rename(location.Path, location.Path+".moved"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ResolveInboxFile(t.Context(), task.snap.ID, 0); !errors.Is(err, ErrInboxFileMissing) {
		t.Fatal("moved file was reported available", err)
	}
	snap := task.snapshot()
	if err = s.ForgetInboxRecords([]InboxRecordRef{{TaskID: snap.ID, Revision: snap.Revision}}); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(location.Path + ".moved"); err != nil {
		t.Fatal("forget removed received file", err)
	}
}

func TestInboxCleanupRejectsForeignStagePartsAndOtherReferences(t *testing.T) {
	for _, scenario := range []string{"foreign_part", "unknown_file", "draft_reference", "paused"} {
		t.Run(scenario, func(t *testing.T) {
			s := inboxTestService(t)
			task, r, m, dest := inboxReceivedFixture(t, s)
			stage := filepath.Join(dest, ".linksend-"+m.TransferID)
			if err := r.Finish(t.Context()); err != nil {
				t.Fatal(err)
			}
			_ = r.Close()
			task.finish("completed", nil)
			switch scenario {
			case "foreign_part":
				part := filepath.Join(stage, "0.part")
				if err := os.Rename(part, filepath.Join(t.TempDir(), "owned.part")); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(part, []byte("foreign user bytes"), 0600); err != nil {
					t.Fatal(err)
				}
			case "unknown_file":
				if err := os.WriteFile(filepath.Join(stage, "user.txt"), []byte("preserve"), 0600); err != nil {
					t.Fatal(err)
				}
			case "draft_reference":
				if _, err := s.store.SaveDraft(SendDraft{ID: "main", Paths: []string{filepath.Join(stage, "0.part")}}, s.cfg.DataDir); err != nil {
					t.Fatal(err)
				}
			case "paused":
				task.mu.Lock()
				task.snap.State = "paused"
				task.snap.CanResume = true
				task.mu.Unlock()
			}
			result, err := s.CleanupInboxStaging(t.Context(), 20)
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Removed) != 0 || len(result.Issues)+len(result.Protected) != 1 {
				t.Fatal("unsafe cleanup", result)
			}
			if _, err = os.Stat(stage); err != nil {
				t.Fatal("protected stage removed", err)
			}
		})
	}
}

func TestInboxResendRejectsChangedSourceBeforeEnqueue(t *testing.T) {
	s := inboxTestService(t)
	source := filepath.Join(t.TempDir(), "source.txt")
	if err := os.WriteFile(source, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	task, _ := inboxIndexedFixture(t, s, []string{source}, "send")
	task.finish("completed", nil)
	if err := os.WriteFile(source, []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ResendInbox(t.Context(), ResendInboxRequest{TaskID: task.snap.ID, RequestID: "request-one", WaitForPeer: true}); !errors.Is(err, transfer.ErrChanged) {
		t.Fatalf("source changed but was requeued: %v", err)
	}
	var count int
	if err := s.store.db.QueryRow(`SELECT count(*) FROM send_queue`).Scan(&count); err != nil || count != 0 {
		t.Fatal(count, err)
	}
}

func TestInboxCleanupProtectsNewReceiverBeforeRegistration(t *testing.T) {
	s := inboxTestService(t)
	old, r, m, directory := inboxReceivedFixture(t, s)
	if err := r.Finish(t.Context()); err != nil {
		t.Fatal(err)
	}
	_ = r.Close()
	old.finish("completed", nil)
	newTask, err := s.tasks.create(TaskSnapshot{Direction: "receive", PeerID: old.snap.PeerID, TargetDirectory: directory}, func() {})
	if err != nil {
		t.Fatal(err)
	}
	newTask.updateRecordAttempt("", func(snap *TaskSnapshot, recovery *taskRecovery) {
		snap.TransferID = m.TransferID
		recovery.TransferID = m.TransferID
		recovery.TargetDirectory = directory
	})
	result, err := s.CleanupInboxStaging(t.Context(), 20)
	if err != nil || len(result.Protected) != 1 || len(result.Removed) != 0 {
		t.Fatal("same transfer's new receiver lost staging", result, err)
	}
	newTask.finish("cancelled", nil)
	result, err = s.CleanupInboxStaging(t.Context(), 20)
	if err != nil || len(result.Removed) != 1 {
		t.Fatal(result, err)
	}
}

func TestInboxForgetProtectsQueueReference(t *testing.T) {
	s := inboxTestService(t)
	task, err := s.tasks.create(TaskSnapshot{Direction: "send"}, func() {})
	if err != nil {
		t.Fatal(err)
	}
	task.finish("completed", nil)
	snap := task.snapshot()
	_, err = s.store.db.Exec(`INSERT INTO send_queue(id,request_id,peer_id,source_paths,source_digest,state,position,task_id,expires_at,last_error,wait_for_peer,revision,created_at,updated_at) VALUES('queue','request',?,'[]','','needs_attention',1,?,'','',1,1,'','')`, strings.Repeat("a", 64), snap.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.ForgetInboxRecords([]InboxRecordRef{{TaskID: snap.ID, Revision: snap.Revision}}); !errors.Is(err, ErrInboxProtected) {
		t.Fatal("referenced task forgotten", err)
	}
	if _, err = s.store.db.Exec(`UPDATE send_queue SET state='completed' WHERE id='queue'`); err != nil {
		t.Fatal(err)
	}
	if err = s.ForgetInboxRecords([]InboxRecordRef{{TaskID: snap.ID, Revision: snap.Revision}}); err != nil {
		t.Fatal(err)
	}
	var taskID string
	if err = s.store.db.QueryRow(`SELECT task_id FROM send_queue WHERE id='queue'`).Scan(&taskID); err != nil || taskID != "" {
		t.Fatal("historical queue retained dangling task", taskID, err)
	}
}

func TestInboxQueriesCancelledContext(t *testing.T) {
	s := inboxTestService(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := s.Inbox(ctx, InboxQuery{}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestInboxResendCreatesNewIdempotentLogicalQueue(t *testing.T) {
	f := newDirectFixtureServices(t)
	source := filepath.Join(t.TempDir(), "resend-source.txt")
	if err := os.WriteFile(source, []byte("immutable resend source"), 0600); err != nil {
		t.Fatal(err)
	}
	task, _ := inboxIndexedFixture(t, f.a, []string{source}, "send")
	task.updateRecordAttempt("", func(snap *TaskSnapshot, recovery *taskRecovery) {
		snap.PeerID = f.bID.ID()
		recovery.PeerID = f.bID.ID()
		recovery.PeerFingerprint = f.bID.ID()
	})
	task.finish("completed", nil)
	request := ResendInboxRequest{TaskID: task.snap.ID, RequestID: "resend-command", WaitForPeer: true}
	queued, err := f.a.ResendInbox(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	if queued.ID == task.snap.ID || queued.TaskID == task.snap.ID || queued.PeerID != f.bID.ID() {
		t.Fatal("resend reused original logical task", queued)
	}
	// An uncertain acknowledgment can be retried after the source changes;
	// it must find the already-created queue entry, not enqueue current bytes.
	if err = os.WriteFile(source, []byte("later change"), 0600); err != nil {
		t.Fatal(err)
	}
	again, err := f.a.ResendInbox(t.Context(), request)
	if err != nil || again.ID != queued.ID {
		t.Fatal("resend acknowledgment was not idempotent", again, err)
	}
	var count int
	if err = f.a.store.db.QueryRow(`SELECT count(*) FROM send_queue`).Scan(&count); err != nil || count != 1 {
		t.Fatal(count, err)
	}
}

func TestInboxMigrationKeepsVerifiedSchema3Backup(t *testing.T) {
	// Build a real v3 database by running the established v2->v3 migration,
	// then let historyDB perform the new migration with its verified backup.
	filename := filepath.Join(t.TempDir(), "history.sqlite")
	db, err := sql.Open("sqlite", filename)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE tasks(id TEXT PRIMARY KEY,revision INTEGER NOT NULL,snapshot BLOB NOT NULL,recovery BLOB);CREATE TABLE metadata(key TEXT PRIMARY KEY,value TEXT NOT NULL);PRAGMA user_version=3`)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err = migrateDesktopMetadata(tx); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	db, err = historyDB(filename)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var schema int
	if err = db.QueryRow(`PRAGMA user_version`).Scan(&schema); err != nil || schema != taskStoreSchema {
		t.Fatal(schema, err)
	}
	backups, err := filepath.Glob(filename + ".schema-v3-*.bak")
	if err != nil || len(backups) != 1 || verifyHistoryBackup(backups[0], 3) != nil {
		t.Fatal("schema3 backup not verified", backups, err)
	}
}

func TestInboxCurrentSchemaOpenDoesNotWriteAndTransactionsWait(t *testing.T) {
	s := inboxTestService(t)
	writer, err := historyDB(s.tasks.historyPath)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	tx, err := writer.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`INSERT INTO metadata(key,value) VALUES('held_write','one')`); err != nil {
		t.Fatal(err)
	}
	opened := make(chan error, 1)
	go func() {
		db, e := historyDB(s.tasks.historyPath)
		if db != nil {
			_ = db.Close()
		}
		opened <- e
	}()
	select {
	case err = <-opened:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("opening current schema attempted a write behind a held transaction")
	}
	second, err := historyDB(s.tasks.historyPath)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	written := make(chan error, 1)
	go func() {
		other, e := second.Begin()
		if e != nil {
			written <- e
			return
		}
		defer other.Rollback()
		var value string
		e = other.QueryRow(`SELECT value FROM metadata WHERE key='held_write'`).Scan(&value)
		if e == nil {
			_, e = other.Exec(`UPDATE metadata SET value='two' WHERE key='held_write'`)
		}
		if e == nil {
			e = other.Commit()
		}
		written <- e
	}()
	select {
	case err = <-written:
		t.Fatalf("write transaction did not wait for reservation: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	select {
	case err = <-written:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("waiting transaction never acquired released lock")
	}
}
