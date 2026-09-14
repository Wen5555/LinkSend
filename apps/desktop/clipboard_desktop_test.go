package main

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	coreapp "github.com/Wen5555/LinkSend/internal/app"
	"github.com/Wen5555/LinkSend/internal/identity"
)

func TestClipboardWatcherFollowsScopedGrantAndRevocation(t *testing.T) {
	dataDir := t.TempDir()
	core, err := coreapp.New(coreapp.Config{DataDir: dataDir})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(core.Shutdown)
	peer, err := identity.Generate()
	if err != nil {
		t.Fatal(err)
	}
	if err = identity.TrustPairedPeer(dataDir, identity.TrustedPeer{ID: peer.ID(), Name: "peer", PublicKey: peer.PublicKey()}); err != nil {
		t.Fatal(err)
	}
	app := NewApp()
	app.ctx = context.Background()
	app.core = core
	if status := app.ClipboardWatcher(); status.Enabled || status.Active {
		t.Fatalf("watcher inherited permission: %+v", status)
	}
	grant, err := app.SetClipboardGrant(coreapp.ClipboardGrantPatch{PeerID: peer.ID(), Direction: "send", Kind: "text", Enabled: true})
	if err != nil || !grant.Enabled {
		t.Fatal(grant, err)
	}
	if status := app.ClipboardWatcher(); !status.Enabled || status.Active {
		t.Fatalf("headless watcher status mismatch: %+v", status)
	}
	app.pauseClipboardWatch()
	if status := app.ClipboardWatcher(); !status.Paused || status.Last.Sequence != 0 {
		t.Fatalf("sleep did not clear watcher generation: %+v", status)
	}
	if err = app.BlockPeer(peer.ID()); err != nil {
		t.Fatal(err)
	}
	if status := app.ClipboardWatcher(); status.Enabled || status.Active {
		t.Fatalf("revocation retained watcher: %+v", status)
	}
}

func TestClipboardWatcherSingleOwnerClosesRegistrationGate(t *testing.T) {
	dir := t.TempDir()
	core, err := coreapp.New(coreapp.Config{DataDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer core.Shutdown()
	peer, _ := identity.Generate()
	if err = identity.TrustPairedPeer(dir, identity.TrustedPeer{ID: peer.ID(), Name: "peer", PublicKey: peer.PublicKey()}); err != nil {
		t.Fatal(err)
	}
	if _, err = core.SetClipboardGrant(t.Context(), coreapp.ClipboardGrantPatch{PeerID: peer.ID(), Direction: "send", Kind: "text", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	app := NewApp()
	app.ctx = context.Background()
	app.core = core
	var starts, stops atomic.Int32
	app.clipboardWatchStart = func() (func(), error) { starts.Add(1); return func() { stops.Add(1) }, nil }
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); app.refreshClipboardWatch() }()
	}
	wg.Wait()
	if starts.Load() != 1 || !app.ClipboardWatcher().Active {
		t.Fatalf("registrations=%d status=%+v", starts.Load(), app.ClipboardWatcher())
	}
	app.closeClipboardOwner()
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); app.refreshClipboardWatch() }()
	}
	wg.Wait()
	if starts.Load() != 1 || stops.Load() != 1 || app.ClipboardWatcher().Active {
		t.Fatalf("closed owner restarted: starts=%d stops=%d status=%+v", starts.Load(), stops.Load(), app.ClipboardWatcher())
	}
}

func TestClipboardPauseReasonsDoNotResumeEachOther(t *testing.T) {
	app := NewApp()
	app.ctx = context.Background()
	app.setClipboardSuspended("lock", true)
	if status := app.ClipboardWatcher(); status.PauseReason != "screen_locked" {
		t.Fatalf("pause reason=%q", status.PauseReason)
	}
	app.setClipboardSuspended("sleep", true)
	app.setClipboardSuspended("sleep", false)
	if !app.ClipboardWatcher().Paused {
		t.Fatal("wake resumed watcher while screen remained locked")
	}
	app.setClipboardSuspended("user", true)
	app.setClipboardSuspended("lock", false)
	if !app.ClipboardWatcher().Paused {
		t.Fatal("unlock cleared user pause")
	}
	app.setClipboardSuspended("user", false)
	if app.ClipboardWatcher().Paused {
		t.Fatal("all pause reasons cleared but watcher remained paused")
	}
}
