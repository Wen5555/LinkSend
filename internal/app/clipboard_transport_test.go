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

func TestClipboardEnsureIsSingleflight(t *testing.T) {
	service, err := New(Config{DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(service.Shutdown)
	service.clipboardEnsureMu.Lock()
	done := make(chan error, 1)
	go func() { done <- service.EnsureClipboardSessions(t.Context(), DirectConfig{}) }()
	select {
	case err = <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("overlapping clipboard Ensure call queued behind the active pass")
	}
	service.clipboardEnsureMu.Unlock()
}

func TestClipboardPeerSenderKeepsOnlyLatestPendingEvent(t *testing.T) {
	sender := &clipboardPeerSender{wake: make(chan struct{}, 1)}
	activeCtx, cancel := context.WithCancel(t.Context())
	sender.current = &clipboardSendJob{event: clipboardsync.Event{OSGeneration: 1}}
	sender.cancel = cancel
	payload := make([]byte, clipboardsync.MaxImageBytes)
	for generation := uint64(2); generation <= 100; generation++ {
		sender.enqueue(&clipboardSendJob{event: clipboardsync.Event{OSGeneration: generation, Payload: payload}})
	}
	select {
	case <-activeCtx.Done():
	default:
		t.Fatal("new clipboard event did not cancel the active event")
	}
	sender.mu.Lock()
	pending := sender.pending
	sender.mu.Unlock()
	pendingGeneration := uint64(0)
	if pending != nil {
		pendingGeneration = pending.event.OSGeneration
	}
	if pendingGeneration != 100 || len(sender.wake) != 1 {
		t.Fatalf("latest-only pending generation=%d wake=%d", pendingGeneration, len(sender.wake))
	}
}

func TestClipboardRevocationActivelyCancelsMatchingSender(t *testing.T) {
	service := &Service{clipboardSenders: make(map[string]*clipboardPeerSender), clipboardPeerRuntime: make(map[string]clipboardPeerRuntimeStatus)}
	activeCtx, cancel := context.WithCancel(t.Context())
	sender := &clipboardPeerSender{wake: make(chan struct{}, 1), cancel: cancel}
	sender.current = &clipboardSendJob{peer: &PeerSession{PeerID: "peer"}, event: clipboardsync.Event{Kind: clipboardsync.Text}}
	sender.pending = &clipboardSendJob{peer: &PeerSession{PeerID: "peer"}, event: clipboardsync.Event{Kind: clipboardsync.Text}}
	service.clipboardSenders["peer/session"] = sender
	service.cancelClipboardSends("peer", clipboardsync.Text)
	select {
	case <-activeCtx.Done():
	default:
		t.Fatal("revocation did not cancel active clipboard send")
	}
	sender.mu.Lock()
	pending := sender.pending
	sender.mu.Unlock()
	if pending != nil {
		t.Fatal("revocation retained pending clipboard payload")
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
	var failNextWrite atomic.Bool
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
		if failNextWrite.CompareAndSwap(true, false) {
			return 0, errors.New("injected native clipboard write failure")
		}
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
	failNextWrite.Store(true)
	aGeneration.Store(2)
	if err = f.a.ClipboardChanged(t.Context(), ClipboardChange{Generation: 2, Kinds: []clipboardsync.Kind{clipboardsync.Text}}); err != nil {
		t.Fatal(err)
	}
	failureDeadline := time.Now().Add(3 * time.Second)
	for failNextWrite.Load() && time.Now().Before(failureDeadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if failNextWrite.Load() {
		t.Fatalf("injected clipboard write failure was not reached: sender=%+v receiver=%+v", f.a.ClipboardPeerStatuses(), f.b.ClipboardPeerStatuses())
	}
	waitFor(t, 3*time.Second, func() bool {
		for _, status := range f.b.ClipboardPeerStatuses() {
			if status.PeerID == f.aID.ID() && status.Error == "write_failed" {
				return true
			}
		}
		return false
	}, "clipboard write failure was not visible in peer status")
	aGeneration.Store(3)
	if err = f.a.ClipboardChanged(t.Context(), ClipboardChange{Generation: 3, Kinds: []clipboardsync.Kind{clipboardsync.Text}}); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-written:
		if string(got) != "fresh clipboard text" {
			t.Fatalf("payload=%q", got)
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("clipboard event did not reach native commit owner: sender=%+v receiver=%+v", f.a.ClipboardPeerStatuses(), f.b.ClipboardPeerStatuses())
	}
	waitFor(t, 3*time.Second, func() bool {
		for _, status := range f.b.ClipboardPeerStatuses() {
			if status.PeerID == f.aID.ID() {
				return status.Error == "" && status.Waiting == ""
			}
		}
		return false
	}, "successful clipboard transfer did not clear peer error")
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
		t.Fatalf("reverse clipboard did not commit: sender=%+v receiver=%+v", f.b.ClipboardPeerStatuses(), f.a.ClipboardPeerStatuses())
	}
	chunkSent, releaseChunk := make(chan struct{}), make(chan struct{})
	var once sync.Once
	var releaseOnce sync.Once
	releaseFile := func() { releaseOnce.Do(func() { close(releaseChunk) }) }
	defer releaseFile()
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
	aGeneration.Store(5)
	if err = f.a.ClipboardChanged(t.Context(), ClipboardChange{Generation: 5, Kinds: []clipboardsync.Kind{clipboardsync.Text}}); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-written:
		if string(got) != "fresh clipboard text" {
			t.Fatal(string(got))
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("active file starved clipboard stream: sender=%+v receiver=%+v", f.a.ClipboardPeerStatuses(), f.b.ClipboardPeerStatuses())
	}
	clipboardLoadDone := make(chan error, 1)
	go func() {
		for generation := uint64(6); generation < 26; generation++ {
			aGeneration.Store(generation)
			if changeErr := f.a.ClipboardChanged(t.Context(), ClipboardChange{Generation: generation, Kinds: []clipboardsync.Kind{clipboardsync.Text}}); changeErr != nil {
				clipboardLoadDone <- changeErr
				return
			}
			time.Sleep(5 * time.Millisecond)
		}
		clipboardLoadDone <- nil
	}()
	releaseFile()
	waitFor(t, 10*time.Second, func() bool { got, ok := f.a.Task(largeTask.ID); return ok && got.State == "completed" }, "file did not resume after clipboard event")
	if err = <-clipboardLoadDone; err != nil {
		t.Fatal("sustained clipboard load failed", err)
	}
	if _, err = f.a.SetClipboardGrant(t.Context(), ClipboardGrantPatch{PeerID: f.bID.ID(), Direction: "send", Kind: "image", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	// Recreate the precise renewal window from the Windows CI failure. Hold
	// the same grant boundary as the renewal worker while clearing and filling
	// the window, so the worker cannot insert a third lease between either
	// fixture operation. The normal image-policy commit below must replace the
	// two text leases with a complete text+image policy.
	f.b.clipboardGrantMu.Lock()
	f.b.clipboardSync.InvalidatePeerDirection(f.aID.ID(), false)
	f.a.clipboardSync.InvalidatePeerDirection(f.bID.ID(), true)
	textGrants := f.b.clipboardGrantsLocked(f.aID.ID(), "receive", reversePeer.AuthorizationGeneration)
	if len(textGrants) != 1 || textGrants[0].Kind != clipboardsync.Text {
		f.b.clipboardGrantMu.Unlock()
		t.Fatalf("unexpected pre-image receive grants: %+v", textGrants)
	}
	for index, id := range []string{"pre-image-text-a", "pre-image-text-b"} {
		if _, err = f.b.clipboardSync.IssueScoped(f.aID.ID(), reversePeer.SessionID, reversePeer.AuthorizationGeneration, textGrants, clipboardLeaseTTL); err != nil {
			f.b.clipboardGrantMu.Unlock()
			t.Fatalf("fill issued text lease %d: %v", index, err)
		}
		if err = f.a.clipboardSync.InstallScoped(id, f.bID.ID(), peer.SessionID, peer.RemoteAuthorizationGeneration, textGrants, clipboardLeaseTTL, aGeneration.Load()); err != nil {
			f.b.clipboardGrantMu.Unlock()
			t.Fatalf("fill installed text lease %d: %v", index, err)
		}
	}
	if _, err = f.b.clipboardSync.IssueScoped(f.aID.ID(), reversePeer.SessionID, reversePeer.AuthorizationGeneration, textGrants, clipboardLeaseTTL); !errors.Is(err, clipboardsync.ErrLimit) {
		f.b.clipboardGrantMu.Unlock()
		t.Fatalf("full text lease window accepted another lease: %v", err)
	}
	f.b.clipboardGrantMu.Unlock()
	if _, err = f.b.SetClipboardGrant(t.Context(), ClipboardGrantPatch{PeerID: f.aID.ID(), Direction: "receive", Kind: "image", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, 3*time.Second, func() bool {
		_, ok := f.a.clipboardSync.Outbound(f.bID.ID(), peer.SessionID, peer.RemoteAuthorizationGeneration, clipboardsync.Image)
		return ok
	}, "image lease did not arrive after full-window policy refresh")

	// Exercise the stale-write serialization separately with a free lease slot.
	// The previous full-window scenario is complete; retaining it here would
	// make sendClipboardLease fail before its before-write hook can run.
	if _, err = f.a.SetClipboardGrant(t.Context(), ClipboardGrantPatch{PeerID: f.bID.ID(), Direction: "send", Kind: "link", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	oldLeaseEntered, releaseOldLease := make(chan struct{}), make(chan struct{})
	var releaseOldLeaseOnce sync.Once
	unblockOldLease := func() { releaseOldLeaseOnce.Do(func() { close(releaseOldLease) }) }
	defer unblockOldLease()
	var oldLeaseOnce sync.Once
	f.b.clipboardGrantMu.Lock()
	f.b.clipboardSync.InvalidatePeerDirection(f.aID.ID(), false)
	f.a.clipboardSync.InvalidatePeerDirection(f.bID.ID(), true)
	f.b.clipboardLeaseBeforeWrite = func() {
		oldLeaseOnce.Do(func() {
			close(oldLeaseEntered)
			<-releaseOldLease
		})
	}
	f.b.clipboardGrantMu.Unlock()
	oldLeaseDone := make(chan struct{})
	go func() {
		f.b.sendClipboardLease(t.Context(), reversePeer)
		close(oldLeaseDone)
	}()
	select {
	case <-oldLeaseEntered:
	case <-time.After(3 * time.Second):
		t.Fatal("stale lease sender did not reach before-write gate")
	}
	linkGrantDone := make(chan error, 1)
	go func() {
		_, grantErr := f.b.SetClipboardGrant(t.Context(), ClipboardGrantPatch{PeerID: f.aID.ID(), Direction: "receive", Kind: "link", Enabled: true})
		linkGrantDone <- grantErr
	}()
	select {
	case grantErr := <-linkGrantDone:
		t.Fatalf("link grant committed before stale lease send finished: %v", grantErr)
	case <-time.After(100 * time.Millisecond):
	}
	unblockOldLease()
	select {
	case <-oldLeaseDone:
	case <-time.After(time.Second):
		t.Fatal("stale lease sender did not finish after release")
	}
	if err = <-linkGrantDone; err != nil {
		t.Fatal(err)
	}
	waitFor(t, 3*time.Second, func() bool {
		_, ok := f.a.clipboardSync.Outbound(f.bID.ID(), peer.SessionID, peer.RemoteAuthorizationGeneration, clipboardsync.Link)
		return ok
	}, "new link lease did not arrive after stale write completed")
	f.b.clipboardReceiveSlots <- struct{}{}
	f.b.clipboardReceiveSlots <- struct{}{}
	sendLargeImage.Store(true)
	aGeneration.Store(26)
	if err = f.a.ClipboardChanged(t.Context(), ClipboardChange{Generation: 26, Kinds: []clipboardsync.Kind{clipboardsync.Image}}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, 3*time.Second, func() bool { return len(f.a.clipboardSendSlots) == 1 }, "large clipboard send did not start")
	time.Sleep(100 * time.Millisecond)
	for generation := uint64(27); generation < 47; generation++ {
		aGeneration.Store(generation)
		if err = f.a.ClipboardChanged(t.Context(), ClipboardChange{Generation: generation, Kinds: []clipboardsync.Kind{clipboardsync.Text}}); err != nil {
			t.Fatal(err)
		}
	}
	f.a.clipboardDispatchMu.Lock()
	senderCount := len(f.a.clipboardSenders)
	f.a.clipboardDispatchMu.Unlock()
	if senderCount != 1 {
		t.Fatalf("clipboard dispatcher count=%d", senderCount)
	}
	shutdownCtx, stopShutdown := context.WithTimeout(context.Background(), 3*time.Second)
	defer stopShutdown()
	if err = f.a.ShutdownContext(shutdownCtx); err != nil {
		t.Fatal("blocked clipboard send delayed shutdown", err)
	}
	f.b.ResetClipboard(true, bGeneration.Load())
	waitFor(t, 5*time.Second, func() bool { f.a.directPoolMu.Lock(); defer f.a.directPoolMu.Unlock(); return len(f.a.directPool) == 0 }, "paused clipboard kept idle sender session")
}

func TestCancelledClipboardLeaseSendReleasesGrantLock(t *testing.T) {
	f := newDirectFixtureServices(t)
	setupCtx, cancelSetup := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancelSetup()
	if _, err := f.a.Devices(setupCtx); err != nil {
		t.Fatal(err)
	}
	if _, err := f.b.Devices(setupCtx); err != nil {
		t.Fatal(err)
	}
	if _, err := f.a.SetClipboardGrant(t.Context(), ClipboardGrantPatch{PeerID: f.bID.ID(), Direction: "send", Kind: "text", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.b.SetClipboardGrant(t.Context(), ClipboardGrantPatch{PeerID: f.aID.ID(), Direction: "receive", Kind: "text", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.a.SetClipboardGrant(t.Context(), ClipboardGrantPatch{PeerID: f.bID.ID(), Direction: "receive", Kind: "text", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.b.SetClipboardGrant(t.Context(), ClipboardGrantPatch{PeerID: f.aID.ID(), Direction: "send", Kind: "text", Enabled: true}); err != nil {
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
	var peer *PeerSession
	f.a.directPoolMu.Lock()
	for _, pooled := range f.a.directPool {
		peer = pooled.peer
	}
	f.a.directPoolMu.Unlock()
	if peer == nil {
		t.Fatal("pooled sender peer missing")
	}
	f.a.sendClipboardLease(t.Context(), peer)
	var reversePeer *PeerSession
	waitFor(t, 3*time.Second, func() bool {
		f.b.directPoolMu.Lock()
		defer f.b.directPoolMu.Unlock()
		for _, pooled := range f.b.directPool {
			reversePeer = pooled.peer
		}
		return reversePeer != nil
	}, "pooled reverse peer missing")
	if reversePeer == nil {
		t.Fatal("pooled reverse peer missing")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	sendDone := make(chan struct{})
	go func() {
		f.b.sendClipboardLease(ctx, reversePeer)
		close(sendDone)
	}()
	select {
	case <-sendDone:
	case <-time.After(time.Second):
		t.Fatal("cancelled lease send held the grant lock")
	}
	updateDone := make(chan error, 1)
	go func() {
		_, err := f.b.SetClipboardGrant(t.Context(), ClipboardGrantPatch{PeerID: f.aID.ID(), Direction: "receive", Kind: "link", Enabled: true})
		updateDone <- err
	}()
	select {
	case err := <-updateDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("grant update remained blocked after cancelled lease send")
	}
}

func TestClipboardConcurrentEnsureSessionsRemainReady(t *testing.T) {
	f := newDirectFixtureServices(t)
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	if _, err := f.a.Devices(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := f.b.Devices(ctx); err != nil {
		t.Fatal(err)
	}
	var aGeneration, bGeneration atomic.Uint64
	aGeneration.Store(1)
	bGeneration.Store(1)
	written := make(chan []byte, 1)
	f.a.ConfigureClipboard(ClipboardAdapter{
		Generation: aGeneration.Load,
		Read: func(_ context.Context, kind clipboardsync.Kind, expected uint64) ([]byte, uint64, error) {
			if kind != clipboardsync.Text || expected != aGeneration.Load() {
				return nil, aGeneration.Load(), clipboardsync.ErrStale
			}
			return []byte("simultaneous-session"), expected, nil
		},
		Write: func(_ clipboardsync.Kind, _ []byte, expected uint64, _ time.Time) (uint64, error) {
			aGeneration.Store(expected + 1)
			return expected + 1, nil
		},
	})
	f.b.ConfigureClipboard(ClipboardAdapter{
		Generation: bGeneration.Load,
		Read: func(_ context.Context, _ clipboardsync.Kind, expected uint64) ([]byte, uint64, error) {
			return []byte("reverse"), expected, nil
		},
		Write: func(kind clipboardsync.Kind, payload []byte, expected uint64, _ time.Time) (uint64, error) {
			if kind != clipboardsync.Text || expected != bGeneration.Load() {
				return 0, clipboardsync.ErrStale
			}
			bGeneration.Store(expected + 1)
			written <- append([]byte(nil), payload...)
			return expected + 1, nil
		},
	})
	for _, item := range []struct {
		service *Service
		peerID  string
	}{
		{f.a, f.bID.ID()},
		{f.b, f.aID.ID()},
	} {
		for _, direction := range []string{"send", "receive"} {
			if _, err := item.service.SetClipboardGrant(ctx, ClipboardGrantPatch{PeerID: item.peerID, Direction: direction, Kind: "text", Enabled: true}); err != nil {
				t.Fatal(err)
			}
		}
		if err := item.service.SetAlwaysAccept(item.peerID, true); err != nil {
			t.Fatal(err)
		}
	}
	cfg := DirectConfig{AllowLoopback: true, CheckTimeout: 5 * time.Second, WaitTimeout: 10 * time.Second}
	if err := f.a.StartInbox(filepath.Join(t.TempDir(), "a-received"), cfg); err != nil {
		t.Fatal(err)
	}
	if err := f.b.StartInbox(filepath.Join(t.TempDir(), "b-received"), cfg); err != nil {
		t.Fatal(err)
	}
	waitFor(t, 5*time.Second, func() bool { return f.a.InboxStatus().Listening && f.b.InboxStatus().Listening }, "both clipboard inboxes did not become ready")
	ensured := make(chan error, 2)
	go func() { ensured <- f.a.EnsureClipboardSessions(ctx, cfg) }()
	go func() { ensured <- f.b.EnsureClipboardSessions(ctx, cfg) }()
	for range 2 {
		if err := <-ensured; err != nil {
			t.Fatalf("concurrent clipboard ensure: %v", err)
		}
	}
	ready := func(service *Service, peerID string) bool {
		for _, status := range service.ClipboardPeerStatuses() {
			if status.PeerID == peerID && status.SendReady && status.ReceiveReady && status.Error == "" {
				return true
			}
		}
		return false
	}
	waitFor(t, 5*time.Second, func() bool { return ready(f.a, f.bID.ID()) && ready(f.b, f.aID.ID()) }, "concurrent clipboard peers did not exchange leases")
	aGeneration.Store(2)
	if err := f.a.ClipboardChanged(ctx, ClipboardChange{Generation: 2, Kinds: []clipboardsync.Kind{clipboardsync.Text}}); err != nil {
		t.Fatal(err)
	}
	select {
	case payload := <-written:
		if string(payload) != "simultaneous-session" {
			t.Fatalf("clipboard payload=%q", payload)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("concurrent clipboard session did not deliver: sender=%+v receiver=%+v", f.a.ClipboardPeerStatuses(), f.b.ClipboardPeerStatuses())
	}
}
