package app

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wen5555/LinkSend/internal/clipboardsync"
	"github.com/Wen5555/LinkSend/internal/transfer"
)

func TestClipboardBootstrapsAuthenticatedSessionWithoutFileAndCoexists(t *testing.T) {
	f := newDirectFixtureServices(t)
	var err error
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	if _, err := f.a.Devices(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := f.b.Devices(ctx); err != nil {
		t.Fatal(err)
	}
	var aGeneration atomic.Uint64
	aGeneration.Store(1)
	var bGeneration atomic.Uint64
	bGeneration.Store(1)
	written := make(chan []byte, 1)
	reverseWritten := make(chan []byte, 1)
	f.a.ConfigureClipboard(ClipboardAdapter{Generation: aGeneration.Load, Read: func(_ context.Context, kind clipboardsync.Kind, expected uint64) ([]byte, uint64, error) {
		if kind != clipboardsync.Text {
			t.Fatalf("read kind=%s", kind)
		}
		return []byte("fresh clipboard text"), aGeneration.Load(), nil
	}, Write: func(kind clipboardsync.Kind, payload []byte, expected uint64, _ time.Time) (uint64, error) {
		if kind != clipboardsync.Text || expected != aGeneration.Load() {
			return 0, clipboardsync.ErrConflict
		}
		next := expected + 1
		aGeneration.Store(next)
		reverseWritten <- append([]byte(nil), payload...)
		return next, nil
	}})
	f.b.ConfigureClipboard(ClipboardAdapter{Generation: bGeneration.Load, Read: func(_ context.Context, kind clipboardsync.Kind, expected uint64) ([]byte, uint64, error) {
		return []byte("reverse clipboard text"), bGeneration.Load(), nil
	}, Write: func(kind clipboardsync.Kind, payload []byte, expected uint64, _ time.Time) (uint64, error) {
		if kind != clipboardsync.Text || expected != bGeneration.Load() {
			return 0, clipboardsync.ErrConflict
		}
		next := expected + 1
		bGeneration.Store(next)
		written <- append([]byte(nil), payload...)
		return next, nil
	}})
	if _, err := f.a.SetClipboardGrant(t.Context(), ClipboardGrantPatch{PeerID: f.bID.ID(), Direction: "send", Kind: "text", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.b.SetClipboardGrant(t.Context(), ClipboardGrantPatch{PeerID: f.aID.ID(), Direction: "receive", Kind: "text", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if err := f.b.SetAlwaysAccept(f.aID.ID(), true); err != nil {
		t.Fatal(err)
	}
	cfg := DirectConfig{AllowLoopback: true, CheckTimeout: 5 * time.Second, WaitTimeout: 10 * time.Second}
	if err := f.b.StartInbox(filepath.Join(t.TempDir(), "received"), cfg); err != nil {
		t.Fatal(err)
	}
	if err := f.a.EnsureClipboardSessions(t.Context(), cfg); err != nil {
		t.Fatal(err)
	}
	if len(f.a.Tasks()) != 0 || len(f.b.Tasks()) != 0 {
		t.Fatal("clipboard session bootstrap created file tasks")
	}
	var peer *PeerSession
	f.a.directPoolMu.Lock()
	for _, pooled := range f.a.directPool {
		peer = pooled.peer
	}
	f.a.directPoolMu.Unlock()
	if peer == nil {
		t.Fatal("pooled peer missing")
	}
	waitFor(t, 3*time.Second, func() bool {
		_, ok := f.a.clipboardSync.Outbound(f.bID.ID(), peer.SessionID, peer.AuthorizationGeneration, clipboardsync.Text)
		return ok
	}, "receiver lease did not arrive")
	waitFor(t, time.Second, func() bool {
		f.b.directPoolMu.Lock()
		defer f.b.directPoolMu.Unlock()
		for _, p := range f.b.directPool {
			if p.clipboardReceiveReady && !p.clipboardSendReady {
				return true
			}
		}
		return false
	}, "single-direction receiver readiness was cleared")
	f.a.directPoolMu.Lock()
	aPool := f.a.directPool[directPoolKey(f.bID.ID(), peer.AuthorizationGeneration)]
	sendReady, receiveReady := false, false
	if aPool != nil {
		sendReady, receiveReady = aPool.clipboardSendReady, aPool.clipboardReceiveReady
	}
	f.a.directPoolMu.Unlock()
	if aPool == nil || !sendReady || receiveReady {
		t.Fatalf("single-direction sender readiness=%v/%v", sendReady, receiveReady)
	}
	time.Sleep(directSessionIdleTTL + 500*time.Millisecond)
	if peer.Data.Conn.Context().Err() != nil {
		t.Fatal("clipboard-ready session was reclaimed by file idle timer")
	}
	aGeneration.Store(2)
	if err = f.a.ClipboardChanged(t.Context(), ClipboardChange{Generation: 2, Kinds: []clipboardsync.Kind{clipboardsync.Text}}); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-written:
		if string(got) != "fresh clipboard text" {
			t.Fatalf("payload=%q", got)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("clipboard event did not reach native commit owner")
	}
	if _, err := f.a.SetClipboardGrant(t.Context(), ClipboardGrantPatch{PeerID: f.bID.ID(), Direction: "receive", Kind: "text", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.b.SetClipboardGrant(t.Context(), ClipboardGrantPatch{PeerID: f.aID.ID(), Direction: "send", Kind: "text", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	f.a.sendClipboardLease(t.Context(), peer)
	var reversePeer *PeerSession
	f.b.directPoolMu.Lock()
	for _, pooled := range f.b.directPool {
		reversePeer = pooled.peer
	}
	f.b.directPoolMu.Unlock()
	if reversePeer == nil || reversePeer.SessionID != peer.SessionID {
		t.Fatal("reverse clipboard lost authenticated session")
	}
	waitFor(t, 3*time.Second, func() bool {
		_, ok := f.b.clipboardSync.Outbound(f.aID.ID(), reversePeer.SessionID, reversePeer.AuthorizationGeneration, clipboardsync.Text)
		return ok
	}, "reverse lease did not arrive")
	bGeneration.Store(3)
	if err = f.b.ClipboardChanged(t.Context(), ClipboardChange{Generation: 3, Kinds: []clipboardsync.Kind{clipboardsync.Text}}); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-reverseWritten:
		if string(got) != "reverse clipboard text" {
			t.Fatalf("reverse payload=%q", got)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("reverse clipboard did not commit")
	}
	chunkSent, releaseChunk := make(chan struct{}), make(chan struct{})
	var once sync.Once
	fileCfg := cfg
	fileCfg.onChunkSent = func(transfer.ChunkTransmission) { once.Do(func() { close(chunkSent) }); <-releaseChunk }
	large := filepath.Join(t.TempDir(), "large.bin")
	if err = os.WriteFile(large, make([]byte, 2*transfer.DefaultChunkSize+1), 0600); err != nil {
		t.Fatal(err)
	}
	largeTask, err := f.a.StartSend(f.bID.ID(), []string{large}, fileCfg)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-chunkSent:
	case <-time.After(10 * time.Second):
		t.Fatal("large file did not reach active chunk")
	}
	aGeneration.Store(4)
	if err = f.a.ClipboardChanged(t.Context(), ClipboardChange{Generation: 4, Kinds: []clipboardsync.Kind{clipboardsync.Text}}); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-written:
		if string(got) != "fresh clipboard text" {
			t.Fatal(string(got))
		}
	case <-time.After(3 * time.Second):
		t.Fatal("active file starved clipboard stream")
	}
	close(releaseChunk)
	waitFor(t, 10*time.Second, func() bool { got, ok := f.a.Task(largeTask.ID); return ok && got.State == "completed" }, "file did not resume after clipboard event")
	f.a.ResetClipboard(true, aGeneration.Load())
	f.b.ResetClipboard(true, bGeneration.Load())
	waitFor(t, 5*time.Second, func() bool { f.a.directPoolMu.Lock(); defer f.a.directPoolMu.Unlock(); return len(f.a.directPool) == 0 }, "paused clipboard kept idle sender session")
}
