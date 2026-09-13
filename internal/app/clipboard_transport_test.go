package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wen5555/LinkSend/internal/clipboardsync"
	"github.com/Wen5555/LinkSend/internal/identity"
	"github.com/Wen5555/LinkSend/internal/transfer"
)

func TestClipboardChangeAdvancesOwnerWithoutLeaseOrSupportedFormat(t *testing.T) {
	service, err := New(Config{DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(service.Shutdown)
	var reads atomic.Int32
	service.ConfigureClipboard(ClipboardAdapter{Generation: func() uint64 { return 1 }, Read: func(context.Context, clipboardsync.Kind, uint64) ([]byte, uint64, error) {
		reads.Add(1)
		return []byte("must not be read"), 2, nil
	}})
	if err = service.ClipboardChanged(t.Context(), ClipboardChange{Generation: 2, Kinds: []clipboardsync.Kind{clipboardsync.Text}}); err != nil {
		t.Fatal(err)
	}
	if reads.Load() != 0 {
		t.Fatal("clipboard was read without an authorized lease")
	}
	if _, err = service.clipboardSync.ObserveLocalEvent(2); !errors.Is(err, clipboardsync.ErrStale) {
		t.Fatal("no-lease copy did not advance the owner watermark", err)
	}
	if err = service.ClipboardChanged(t.Context(), ClipboardChange{Generation: 3}); err != nil {
		t.Fatal(err)
	}
	if _, err = service.clipboardSync.ObserveLocalEvent(3); !errors.Is(err, clipboardsync.ErrStale) {
		t.Fatal("unsupported-format copy did not advance the owner watermark", err)
	}
}

type clipboardWriterFunc func([]byte) (int, error)

func (f clipboardWriterFunc) Write(payload []byte) (int, error) { return f(payload) }

func TestClipboardGuardedWriterCancelsOldCopyAndEnforcesDeadline(t *testing.T) {
	state := clipboardsync.New("sender", nil)
	state.Reset(false, 1)
	if err := state.InstallScoped("lease", "peer", "session", 5, []clipboardsync.Grant{{Kind: clipboardsync.Image, Revision: 2}}, time.Minute, 1); err != nil {
		t.Fatal(err)
	}
	base, err := state.ObserveLocalEvent(2)
	if err != nil {
		t.Fatal(err)
	}
	base, err = state.PrepareObserved(base, clipboardsync.Image, make([]byte, 3*(64<<10)))
	if err != nil {
		t.Fatal(err)
	}
	event, err := state.BindScoped(base, "lease", 3)
	if err != nil {
		t.Fatal(err)
	}
	var chunks atomic.Int32
	destination := clipboardWriterFunc(func(payload []byte) (int, error) {
		if chunks.Add(1) == 1 {
			_, _ = state.ObserveLocalEvent(3)
		}
		return len(payload), nil
	})
	writer := clipboardGuardedWriter{ctx: t.Context(), dst: destination, state: state, event: event, deadline: time.Now().Add(time.Minute)}
	if _, err = writer.Write(event.Payload); !errors.Is(err, clipboardsync.ErrStale) {
		t.Fatal("new copy did not cancel old clipboard body", err)
	}
	if chunks.Load() != 1 {
		t.Fatalf("stale writer emitted %d chunks", chunks.Load())
	}
	writer.deadline = time.Now().Add(-time.Millisecond)
	if _, err = writer.Write([]byte("late")); !errors.Is(err, clipboardsync.ErrStale) {
		t.Fatal("expired send deadline was ignored", err)
	}
}

func TestClipboardGuardedWriterUsesBoundedChunksAndRate(t *testing.T) {
	state := clipboardsync.New("sender", nil)
	state.Reset(false, 1)
	if err := state.InstallScoped("lease", "peer", "session", 5, []clipboardsync.Grant{{Kind: clipboardsync.Image, Revision: 2}}, time.Minute, 1); err != nil {
		t.Fatal(err)
	}
	base, _ := state.ObserveLocalEvent(2)
	base, _ = state.PrepareObserved(base, clipboardsync.Image, make([]byte, 4*(64<<10)))
	event, _ := state.BindScoped(base, "lease", 3)
	var largest, chunks atomic.Int32
	destination := clipboardWriterFunc(func(payload []byte) (int, error) {
		chunks.Add(1)
		for {
			current := largest.Load()
			if int32(len(payload)) <= current || largest.CompareAndSwap(current, int32(len(payload))) {
				break
			}
		}
		return len(payload), nil
	})
	started := time.Now()
	writer := clipboardGuardedWriter{ctx: t.Context(), dst: destination, state: state, event: event, deadline: time.Now().Add(time.Minute)}
	if _, err := writer.Write(event.Payload); err != nil {
		t.Fatal(err)
	}
	if chunks.Load() != 4 || largest.Load() > 64<<10 {
		t.Fatalf("chunks=%d largest=%d", chunks.Load(), largest.Load())
	}
	if elapsed := time.Since(started); elapsed < 10*time.Millisecond {
		t.Fatalf("clipboard byte rate was unbounded: %s", elapsed)
	}
}

func TestClipboardSendSlotsAreBounded(t *testing.T) {
	service, err := New(Config{DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(service.Shutdown)
	if cap(service.clipboardSendSlots) != 2 || cap(service.clipboardReceiveSlots) != 2 {
		t.Fatalf("clipboard slot budgets send=%d receive=%d", cap(service.clipboardSendSlots), cap(service.clipboardReceiveSlots))
	}
}

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
	// Authorization generations are local trust epochs. They are deliberately
	// asymmetric here so the wire cannot accidentally require numeric equality.
	trustPath := filepath.Join(f.b.cfg.DataDir, "trust.json")
	rawTrust, err := os.ReadFile(trustPath)
	if err != nil {
		t.Fatal(err)
	}
	var trust identity.TrustFile
	if err = json.Unmarshal(rawTrust, &trust); err != nil {
		t.Fatal(err)
	}
	found := false
	for index := range trust.Peers {
		if trust.Peers[index].ID == f.aID.ID() {
			trust.Peers[index].GrantGeneration = 29
			found = true
		}
	}
	if !found {
		t.Fatal("fixture peer trust missing")
	}
	rawTrust, err = json.MarshalIndent(trust, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(trustPath, append(rawTrust, '\n'), 0600); err != nil {
		t.Fatal("failed to create asymmetric authorization fixture", err)
	}
	var aGeneration atomic.Uint64
	aGeneration.Store(1)
	var bGeneration atomic.Uint64
	bGeneration.Store(1)
	var sendLargeImage atomic.Bool
	written := make(chan []byte, 64)
	reverseWritten := make(chan []byte, 1)
	f.a.ConfigureClipboard(ClipboardAdapter{Generation: aGeneration.Load, Read: func(_ context.Context, kind clipboardsync.Kind, expected uint64) ([]byte, uint64, error) {
		if kind == clipboardsync.Image && sendLargeImage.Load() {
			return make([]byte, clipboardsync.MaxImageBytes), aGeneration.Load(), nil
		}
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
	if peer.AuthorizationGeneration == peer.RemoteAuthorizationGeneration || peer.RemoteAuthorizationGeneration != 29 {
		t.Fatalf("authorization generations were not asymmetric: local=%d remote=%d", peer.AuthorizationGeneration, peer.RemoteAuthorizationGeneration)
	}
	waitFor(t, 3*time.Second, func() bool {
		_, ok := f.a.clipboardSync.Outbound(f.bID.ID(), peer.SessionID, peer.RemoteAuthorizationGeneration, clipboardsync.Text)
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
		_, ok := f.b.clipboardSync.Outbound(f.aID.ID(), reversePeer.SessionID, reversePeer.RemoteAuthorizationGeneration, clipboardsync.Text)
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
	clipboardLoadDone := make(chan error, 1)
	go func() {
		for generation := uint64(5); generation < 25; generation++ {
			aGeneration.Store(generation)
			if changeErr := f.a.ClipboardChanged(t.Context(), ClipboardChange{Generation: generation, Kinds: []clipboardsync.Kind{clipboardsync.Text}}); changeErr != nil {
				clipboardLoadDone <- changeErr
				return
			}
			time.Sleep(5 * time.Millisecond)
		}
		clipboardLoadDone <- nil
	}()
	close(releaseChunk)
	waitFor(t, 10*time.Second, func() bool { got, ok := f.a.Task(largeTask.ID); return ok && got.State == "completed" }, "file did not resume after clipboard event")
	if err = <-clipboardLoadDone; err != nil {
		t.Fatal("sustained clipboard load failed", err)
	}
	if _, err = f.a.SetClipboardGrant(t.Context(), ClipboardGrantPatch{PeerID: f.bID.ID(), Direction: "send", Kind: "image", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if _, err = f.b.SetClipboardGrant(t.Context(), ClipboardGrantPatch{PeerID: f.aID.ID(), Direction: "receive", Kind: "image", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	f.b.sendClipboardLease(t.Context(), reversePeer)
	waitFor(t, 3*time.Second, func() bool {
		_, ok := f.a.clipboardSync.Outbound(f.bID.ID(), peer.SessionID, peer.RemoteAuthorizationGeneration, clipboardsync.Image)
		return ok
	}, "image lease did not arrive")
	f.b.clipboardReceiveSlots <- struct{}{}
	f.b.clipboardReceiveSlots <- struct{}{}
	sendLargeImage.Store(true)
	aGeneration.Store(25)
	if err = f.a.ClipboardChanged(t.Context(), ClipboardChange{Generation: 25, Kinds: []clipboardsync.Kind{clipboardsync.Image}}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, 3*time.Second, func() bool { return len(f.a.clipboardSendSlots) == 1 }, "large clipboard send did not start")
	time.Sleep(100 * time.Millisecond)
	shutdownCtx, stopShutdown := context.WithTimeout(context.Background(), 3*time.Second)
	defer stopShutdown()
	if err = f.a.ShutdownContext(shutdownCtx); err != nil {
		t.Fatal("blocked clipboard send delayed shutdown", err)
	}
	f.b.ResetClipboard(true, bGeneration.Load())
	waitFor(t, 5*time.Second, func() bool { f.a.directPoolMu.Lock(); defer f.a.directPoolMu.Unlock(); return len(f.a.directPool) == 0 }, "paused clipboard kept idle sender session")
}
