package app

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestTaskHistoryPersistsAndMarksInterruptedRecovery(t *testing.T) {
	dir := t.TempDir()
	s1, err := New(Config{DataDir: dir, AllowInsecureLoopback: true})
	if err != nil {
		t.Fatal(err)
	}
	cancel := func() {}
	task, err := s1.tasks.create(TaskSnapshot{Direction: "send", PeerID: "peer", SourceSummary: "中文.txt"}, cancel)
	if err != nil {
		t.Fatal(err)
	}
	task.update(func(v *TaskSnapshot) { v.State = "transferring"; v.Phase = "transferring" })
	history := filepath.Join(dir, "task-history.sqlite")
	if _, err := os.Stat(history); err != nil {
		t.Fatalf("history not written: %v", err)
	}
	s2, err := New(Config{DataDir: dir, AllowInsecureLoopback: true})
	if err != nil {
		t.Fatal(err)
	}
	got, ok := s2.Task(task.snap.ID)
	if !ok {
		t.Fatal("persisted task missing")
	}
	if got.State != "failed" || got.ErrorCode != "TASK_INTERRUPTED" || got.RestartRecoverySupported || got.ByteResumeSupported || got.CanResume {
		t.Fatalf("unexpected recovery snapshot: %+v", got)
	}
	cancel()
}

func TestCorruptTaskHistoryDoesNotPreventStartup(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "task-history.sqlite"), []byte("not-sqlite"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := New(Config{DataDir: dir}); err != nil {
		t.Fatal(err)
	}
}

func TestTaskHistoryCompletedAndRevisionGuard(t *testing.T) {
	dir := t.TempDir()
	s, err := New(Config{DataDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	r, err := s.tasks.create(TaskSnapshot{Direction: "send"}, func() {})
	if err != nil {
		t.Fatal(err)
	}
	stale := r.snapshot()
	r.finish("completed", nil)
	if err = s.tasks.persistSnapshot(stale); err != nil {
		t.Fatal(err)
	}
	s2, err := New(Config{DataDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := s2.Task(stale.ID)
	if got.State != "completed" || got.Revision <= stale.Revision {
		t.Fatalf("stale row replaced completion: %+v", got)
	}
}

func TestTaskHistoryProcessHelper(t *testing.T) {
	dir := os.Getenv("LINKSEND_HISTORY_TEST_DIR")
	if dir == "" {
		return
	}
	s, err := New(Config{DataDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	r, err := s.tasks.create(TaskSnapshot{Direction: "receive"}, func() {})
	if err != nil {
		t.Fatal(err)
	}
	r.update(func(v *TaskSnapshot) { v.State = "transferring"; v.Phase = "transferring"; v.ProcessedBytes = 4096 })
	if os.Getenv("LINKSEND_HISTORY_TEST_KILL") == "1" {
		fmt.Println("READY")
		time.Sleep(time.Minute)
		return
	}
	r.finish("completed", nil)
}

// This is a real process lifecycle test of history, not a P2P resume test.
func TestTaskHistoryAcrossNormalExitAndKilledProcess(t *testing.T) {
	for _, kill := range []bool{false, true} {
		t.Run(fmt.Sprint(kill), func(t *testing.T) {
			dir := t.TempDir()
			cmd := exec.Command(os.Args[0], "-test.run=^TestTaskHistoryProcessHelper$")
			cmd.Env = append(os.Environ(), "LINKSEND_HISTORY_TEST_DIR="+dir)
			if kill {
				cmd.Env = append(cmd.Env, "LINKSEND_HISTORY_TEST_KILL=1")
				stdout, err := cmd.StdoutPipe()
				if err != nil {
					t.Fatal(err)
				}
				if err = cmd.Start(); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = cmd.Process.Kill() })
				ready := make(chan bool, 1)
				go func() {
					s := bufio.NewScanner(stdout)
					for s.Scan() {
						if s.Text() == "READY" {
							ready <- true
							return
						}
					}
					ready <- false
				}()
				select {
				case ok := <-ready:
					if !ok {
						t.Fatal("helper exited before checkpoint")
					}
				case <-time.After(15 * time.Second):
					t.Fatal("helper not ready")
				}
				if err = cmd.Process.Kill(); err != nil {
					t.Fatal(err)
				}
				if err = cmd.Wait(); err == nil {
					t.Fatal("killed helper returned success")
				}
			} else if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("helper: %v %s", err, output)
			}
			s, err := New(Config{DataDir: dir})
			if err != nil {
				t.Fatal(err)
			}
			tasks := s.Tasks()
			if len(tasks) != 1 {
				t.Fatalf("tasks=%d", len(tasks))
			}
			if kill {
				if tasks[0].ErrorCode != "TASK_INTERRUPTED" || tasks[0].ProcessedBytes != 4096 {
					t.Fatalf("lost interruption: %+v", tasks)
				}
			} else if tasks[0].State != "completed" {
				t.Fatalf("lost completion: %+v", tasks)
			}
		})
	}
}

func TestTaskHistoryMigratesV1AndQuarantinesCorruptRows(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "task-history.sqlite")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("CREATE TABLE tasks (id TEXT PRIMARY KEY, revision INTEGER NOT NULL, snapshot BLOB NOT NULL); CREATE TABLE metadata (key TEXT PRIMARY KEY, value TEXT NOT NULL); PRAGMA user_version=1"); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	snap := TaskSnapshot{ID: "legacy", Direction: "send", State: "completed", Revision: 3, StartedAt: time.Now().UTC().Format(time.RFC3339Nano), UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	data, _ := json.Marshal(snap)
	if _, err = db.Exec("INSERT INTO tasks(id,revision,snapshot) VALUES(?,?,?)", snap.ID, snap.Revision, data); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	_ = db.Close()
	service, err := New(Config{DataDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if migrated, ok := service.Task("legacy"); !ok || migrated.TaskID != "legacy" || migrated.AttemptID == "" {
		t.Fatalf("legacy task was not migrated compatibly: %+v", migrated)
	}
	backups, err := filepath.Glob(path + ".schema-v1-*.bak")
	if err != nil || len(backups) != 1 {
		t.Fatalf("migration backup count=%d err=%v", len(backups), err)
	}
	backupDB, err := sql.Open("sqlite", backups[0])
	if err != nil {
		t.Fatal(err)
	}
	var backupVersion, backupTasks int
	if err = backupDB.QueryRow("PRAGMA user_version").Scan(&backupVersion); err == nil {
		err = backupDB.QueryRow("SELECT count(*) FROM tasks WHERE id='legacy'").Scan(&backupTasks)
	}
	_ = backupDB.Close()
	if err != nil || backupVersion != 1 || backupTasks != 1 {
		t.Fatalf("migration backup is not a readable v1 rollback point: version=%d tasks=%d err=%v", backupVersion, backupTasks, err)
	}
	db, err = sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var version int
	if err = db.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != taskStoreSchema {
		t.Fatalf("schema version=%d err=%v", version, err)
	}
	if _, err = db.Exec("INSERT INTO tasks(id,revision,snapshot,recovery) VALUES('corrupt',1,?,NULL)", []byte("{not-json")); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	if _, err = New(Config{DataDir: dir}); err != nil {
		t.Fatal(err)
	}
	db, err = sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var tasks, quarantined int
	if err = db.QueryRow("SELECT count(*) FROM tasks WHERE id='corrupt'").Scan(&tasks); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow("SELECT count(*) FROM task_quarantine WHERE id='corrupt' AND reason='INVALID_SNAPSHOT'").Scan(&quarantined); err != nil {
		t.Fatal(err)
	}
	if tasks != 0 || quarantined != 1 {
		t.Fatalf("corrupt row was not isolated: tasks=%d quarantine=%d", tasks, quarantined)
	}
}

func TestTaskHistoryWriteFailureRevokesPersistenceClaim(t *testing.T) {
	service, err := New(Config{DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	record, err := service.tasks.create(TaskSnapshot{Direction: "send"}, func() {})
	if err != nil {
		t.Fatal(err)
	}
	service.tasks.historyPath = filepath.Join(t.TempDir(), "missing-parent", "task-history.sqlite")
	record.update(func(v *TaskSnapshot) { v.Phase = "forced-persistence-failure" })
	if got := record.snapshot(); got.HistoryPersisted {
		t.Fatalf("task still claimed durable history after write failure: %+v", got)
	}
	diagnostics := service.Diagnostics(t.Context())
	if service.tasks.historyError() == nil || diagnostics.HistoryPersisted {
		t.Fatal("manager diagnostics did not retain the task-store failure")
	}
	if diagnostics.RestartRecoverySupported || !diagnostics.ByteResumeSupported {
		t.Fatalf("persistence failure conflated independent recovery capabilities: %+v", diagnostics)
	}
}
