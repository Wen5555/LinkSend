package app

import (
	"os"
	"path/filepath"
	"testing"
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
	history := filepath.Join(dir, "tasks-history.json")
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
	if got.State != "recovering" || !got.RestartRecoverySupported || !got.ByteResumeSupported {
		t.Fatalf("unexpected recovery snapshot: %+v", got)
	}
	cancel()
}

func TestCorruptTaskHistoryDoesNotPreventStartup(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "tasks-history.json"), []byte("not-json"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := New(Config{DataDir: dir}); err != nil {
		t.Fatal(err)
	}
}
