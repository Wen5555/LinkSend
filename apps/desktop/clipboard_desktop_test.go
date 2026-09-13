package main

import (
	"context"
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

func TestClipboardPauseReasonsDoNotResumeEachOther(t *testing.T) {
	app := NewApp()
	app.ctx = context.Background()
	app.setClipboardSuspended("lock", true)
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
