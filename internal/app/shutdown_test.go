package app

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Wen5555/LinkSend/internal/transfer"
)

func TestSaveExitPreservesRealQUICCheckpointAndProfileOwnership(t *testing.T) {
	f := newDirectFixtureServices(t)
	t.Cleanup(f.a.Shutdown)
	t.Cleanup(f.b.Shutdown)
	source := filepath.Join(t.TempDir(), "save-exit.bin")
	if err := os.WriteFile(source, bytes.Repeat([]byte{0x52}, 3*transfer.DefaultChunkSize), 0600); err != nil {
		t.Fatal(err)
	}
	cfg := DirectConfig{AllowLoopback: true, CheckTimeout: 5 * time.Second, WaitTimeout: 10 * time.Second}
	receiver, err := f.b.StartReceive(f.aID.ID(), t.TempDir(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	waitTask(t, f.b, receiver.ID, func(v TaskSnapshot) bool { return v.Phase == "waiting" })
	checkpoint, release := make(chan struct{}), make(chan struct{})
	cfg.onChunkSent = func(sent transfer.ChunkTransmission) {
		if sent.Index == 0 {
			close(checkpoint)
			<-release
		}
	}
	sender, err := f.a.StartSend(f.bID.ID(), []string{source}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	waitTask(t, f.b, receiver.ID, func(v TaskSnapshot) bool { return v.State == "awaiting_acceptance" })
	if err = f.b.AcceptTask(receiver.ID); err != nil {
		t.Fatal(err)
	}
	select {
	case <-checkpoint:
	case <-time.After(10 * time.Second):
		t.Fatal("missing actual QUIC checkpoint")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err = f.a.ShutdownContext(ctx); err == nil {
		t.Fatal("blocked worker must retain ownership")
	}
	waitTask(t, f.a, sender.ID, func(v TaskSnapshot) bool { return v.State == "shutdown_requested" })
	if _, err = New(Config{DataDir: f.a.cfg.DataDir}); !errors.Is(err, ErrProfileInUse) {
		t.Fatalf("profile released before cleanup: %v", err)
	}
	close(release)
	f.a.Shutdown()
	f.b.Shutdown()
	restored, err := New(Config{DataDir: f.a.cfg.DataDir})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(restored.Shutdown)
	task, ok := restored.Task(sender.ID)
	if !ok || task.State != "recovering" || !task.CanResume || task.SentBytes != transfer.DefaultChunkSize || task.AttemptID != sender.AttemptID {
		t.Fatalf("saved task did not retain checkpoint/attempt: %+v", task)
	}
}

func TestFailedConstructionReleasesProfileLock(t *testing.T) {
	dir := t.TempDir()
	if _, err := New(Config{DataDir: dir, ServerURL: "https://example.invalid/?token=x"}); err == nil {
		t.Fatal("invalid configuration accepted")
	}
	svc, err := New(Config{DataDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	svc.Shutdown()
	if err = svc.StartInbox(t.TempDir(), DirectConfig{}); err == nil {
		t.Fatal("closed service reopened receiver")
	}
}
