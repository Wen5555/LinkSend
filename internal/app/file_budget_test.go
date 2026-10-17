package app

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wen5555/LinkSend/internal/clipboardsync"
	"github.com/Wen5555/LinkSend/internal/transfer"
)

func fileBudgetReceiver(t *testing.T, f directFixtureServices) (string, DirectConfig) {
	t.Helper()
	if _, err := f.a.Devices(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := f.b.Devices(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := f.b.SetAlwaysAccept(f.aID.ID(), true); err != nil {
		t.Fatal(err)
	}
	dest := t.TempDir()
	cfg := DirectConfig{AllowLoopback: true, CheckTimeout: 5 * time.Second, WaitTimeout: 10 * time.Second}
	if err := f.b.StartInbox(dest, cfg); err != nil {
		t.Fatal(err)
	}
	waitFor(t, 5*time.Second, func() bool { return f.b.InboxStatus().Listening }, "budget receiver not listening")
	return dest, cfg
}

func TestFileBodyLimitRealQUICRateHashAndDynamicUpdate(t *testing.T) {
	f := newDirectFixtureServices(t)
	dest, cfg := fileBudgetReceiver(t, f)
	t.Cleanup(func() { _ = ConfigureFileSendLimit(0) })
	path := filepath.Join(t.TempDir(), "预算.bin")
	payload := bytes.Repeat([]byte("budget-content\x00\x01"), 1<<16)
	if err := os.WriteFile(path, payload, 0600); err != nil {
		t.Fatal(err)
	}
	prepared, err := transfer.Prepare(t.Context(), []string{path}, 64<<10)
	if err != nil {
		t.Fatal(err)
	}
	defer prepared.Close()
	if err = ConfigureFileSendLimit(256 << 10); err != nil {
		t.Fatal(err)
	}
	var first, last time.Time
	var chunks int
	var body int64
	result, err := f.a.SendPreparedWithHooksDetailed(t.Context(), f.bID.ID(), prepared, cfg, transfer.SendHooks{ChunkSent: func(sent transfer.ChunkTransmission) {
		now := time.Now()
		if chunks == 0 {
			first = now
		}
		last = now
		chunks++
		body += sent.Bytes
	}})
	if err != nil {
		t.Fatal(err)
	}
	elapsed := last.Sub(first)
	expected := time.Duration(float64(body-(64<<10)) / float64(256<<10) * float64(time.Second))
	if elapsed < expected-150*time.Millisecond || elapsed > expected+10*time.Second {
		t.Fatalf("body rate outside bounded coarse sample: %s expected~%s", elapsed, expected)
	}
	got, err := os.ReadFile(filepath.Join(dest, filepath.Base(path)))
	if err != nil || transfer.Sum(got) != transfer.Sum(payload) || result.Transfer.Bytes != int64(len(payload)) {
		t.Fatal("limited receive hash", err, result.Transfer)
	}
	t.Logf("real QUIC body=%d chunks=%d first-to-last=%s cap=%dB/s (excludes first chunk)", body, chunks, elapsed, FileSendLimit())
	// Change a live low-rate wait to unlimited from the first completed chunk.
	_ = ConfigureFileSendLimit(64 << 10)
	second, err := transfer.Prepare(t.Context(), []string{path}, 64<<10)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	var updated atomic.Bool
	started := time.Now()
	_, err = f.a.SendPreparedWithHooksDetailed(t.Context(), f.bID.ID(), second, cfg, transfer.SendHooks{ChunkSent: func(transfer.ChunkTransmission) {
		if updated.CompareAndSwap(false, true) {
			_ = ConfigureFileSendLimit(0)
		}
	}})
	if err != nil || !updated.Load() || time.Since(started) > 8*time.Second {
		t.Fatal("active rate update did not release remaining body", err)
	}
}

func TestFileBodyLimitWaitKeepsClipboardAndPauseCancelResponsive(t *testing.T) {
	f := newDirectFixtureServices(t)
	_, cfg := fileBudgetReceiver(t, f)
	t.Cleanup(func() { _ = ConfigureFileSendLimit(0) })
	var generation atomic.Uint64
	generation.Store(1)
	var receiverGeneration atomic.Uint64
	receiverGeneration.Store(1)
	written := make(chan string, 1)
	f.a.ConfigureClipboard(ClipboardAdapter{Generation: generation.Load, Read: func(context.Context, clipboardsync.Kind, uint64) ([]byte, uint64, error) {
		return []byte("fixture clipboard while file waits"), generation.Load(), nil
	}})
	f.b.ConfigureClipboard(ClipboardAdapter{Generation: receiverGeneration.Load, Write: func(_ clipboardsync.Kind, payload []byte, expected uint64, _ time.Time) (uint64, error) {
		receiverGeneration.Store(expected + 1)
		written <- string(payload)
		return expected + 1, nil
	}})
	if _, err := f.a.SetClipboardGrant(t.Context(), ClipboardGrantPatch{PeerID: f.bID.ID(), Direction: "send", Kind: "text", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.b.SetClipboardGrant(t.Context(), ClipboardGrantPatch{PeerID: f.aID.ID(), Direction: "receive", Kind: "text", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if err := f.a.EnsureClipboardSessions(t.Context(), cfg); err != nil {
		t.Fatal(err)
	}
	waitFor(t, 5*time.Second, func() bool {
		f.a.directPoolMu.Lock()
		defer f.a.directPoolMu.Unlock()
		for _, pooled := range f.a.directPool {
			_, ok := f.a.clipboardSync.Outbound(f.bID.ID(), pooled.peer.SessionID, pooled.peer.RemoteAuthorizationGeneration, clipboardsync.Text)
			if ok {
				return true
			}
		}
		return false
	}, "clipboard lease missing")
	file := filepath.Join(t.TempDir(), "slow.bin")
	payload := bytes.Repeat([]byte("slow"), 256<<10)
	if err := os.WriteFile(file, payload, 0600); err != nil {
		t.Fatal(err)
	}
	_ = ConfigureFileSendLimit(1024)
	task, err := f.a.StartSend(f.bID.ID(), []string{file}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	waitTask(t, f.a, task.ID, func(s TaskSnapshot) bool { return s.State == "transferring" })
	generation.Store(2)
	started := time.Now()
	if err = f.a.ClipboardChanged(t.Context(), ClipboardChange{Generation: 2, Kinds: []clipboardsync.Kind{clipboardsync.Text}}); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-written:
		if got != "fixture clipboard while file waits" {
			t.Fatal("clipboard fixture mismatch")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("file budget blocked clipboard")
	}
	t.Logf("clipboard committed while file limited to 1KiB/s: %s", time.Since(started))
	started = time.Now()
	if err = f.a.PauseTask(task.ID); err != nil {
		t.Fatal(err)
	}
	waitTask(t, f.a, task.ID, func(s TaskSnapshot) bool { return s.State == "paused" })
	if time.Since(started) > 2*time.Second {
		t.Fatal("pause blocked on budget")
	}
	if _, err = f.a.ResumeTask(task.ID, cfg); err != nil {
		t.Fatal(err)
	}
	waitTask(t, f.a, task.ID, func(s TaskSnapshot) bool { return s.State == "transferring" })
	started = time.Now()
	if err = f.a.CancelTask(task.ID); err != nil {
		t.Fatal(err)
	}
	waitTask(t, f.a, task.ID, func(s TaskSnapshot) bool { return s.State == "cancelled" })
	if time.Since(started) > 2*time.Second {
		t.Fatal("cancel blocked on budget")
	}
	got, err := os.ReadFile(file)
	if err != nil || !bytes.Equal(got, payload) {
		t.Fatal("control mutated source", err)
	}
}
