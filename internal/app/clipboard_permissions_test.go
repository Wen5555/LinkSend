package app

import (
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wen5555/LinkSend/internal/clipboardsync"
	"github.com/Wen5555/LinkSend/internal/identity"
)

func TestClipboardGrantsDefaultOffCASAndRevocation(t *testing.T) {
	dir := t.TempDir()
	service, err := New(Config{DataDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(service.Shutdown)
	peer, err := identity.Generate()
	if err != nil {
		t.Fatal(err)
	}
	if err = identity.TrustPairedPeer(dir, identity.TrustedPeer{ID: peer.ID(), Name: "peer", PublicKey: peer.PublicKey()}); err != nil {
		t.Fatal(err)
	}
	grants, err := service.ClipboardGrants(t.Context(), peer.ID())
	if err != nil || len(grants) != 0 {
		t.Fatalf("new peer inherited clipboard permission: %+v %v", grants, err)
	}
	send, err := service.SetClipboardGrant(t.Context(), ClipboardGrantPatch{PeerID: peer.ID(), Direction: "send", Kind: "text", Enabled: true})
	if err != nil || !send.Enabled || send.Revision != 1 {
		t.Fatalf("enable send: %+v %v", send, err)
	}
	if _, err = service.SetClipboardGrant(t.Context(), ClipboardGrantPatch{PeerID: peer.ID(), Direction: "send", Kind: "text", Enabled: false}); !errors.Is(err, ErrMetadataConflict) {
		t.Fatal("stale clipboard grant update accepted:", err)
	}
	grants, err = service.ClipboardGrants(t.Context(), peer.ID())
	if err != nil || len(grants) != 1 || grants[0].Direction != "send" || grants[0].Kind != "text" {
		t.Fatalf("direction/type permissions were conflated: %+v %v", grants, err)
	}
	if err = service.BlockPeer(peer.ID()); err != nil {
		t.Fatal(err)
	}
	grants, err = service.ClipboardGrants(t.Context(), peer.ID())
	if err != nil || len(grants) != 1 || grants[0].Enabled || grants[0].Revision != 2 {
		t.Fatalf("revocation retained clipboard grant: %+v %v", grants, err)
	}
	if _, err = service.SetClipboardGrant(t.Context(), ClipboardGrantPatch{PeerID: peer.ID(), Direction: "send", Kind: "text", Enabled: true}); err == nil {
		t.Fatal("blocked peer regained clipboard permission")
	}
}

func TestClipboardPermissionRevisionRevokesOldLeaseWithoutBreakingOtherDirection(t *testing.T) {
	dir := t.TempDir()
	service, err := New(Config{DataDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(service.Shutdown)
	peer, _ := identity.Generate()
	if err = identity.TrustPairedPeer(dir, identity.TrustedPeer{ID: peer.ID(), Name: "peer", PublicKey: peer.PublicKey()}); err != nil {
		t.Fatal(err)
	}
	receive, err := service.SetClipboardGrant(t.Context(), ClipboardGrantPatch{PeerID: peer.ID(), Direction: "receive", Kind: "text", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	lease, err := service.clipboardSync.IssueScoped(peer.ID(), "session", receive.AuthorizationGeneration, []clipboardsync.Grant{{Kind: clipboardsync.Text, Revision: receive.Revision}}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.SetClipboardGrant(t.Context(), ClipboardGrantPatch{PeerID: peer.ID(), Direction: "send", Kind: "image", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	header := clipboardsync.Event{LeaseID: lease.ID, OriginID: peer.ID(), Boot: "boot", OriginSeq: 1, Lamport: 1, Kind: clipboardsync.Text, Digest: strings.Repeat("0", 64), SenderGrantRevision: 1, ReceiverGrantRevision: receive.Revision}
	if _, err = service.clipboardSync.BeginHeader(peer.ID(), "session", receive.AuthorizationGeneration, header); err != nil {
		t.Fatal("unrelated send permission invalidated receive lease", err)
	}
	if _, err = service.SetClipboardGrant(t.Context(), ClipboardGrantPatch{PeerID: peer.ID(), Direction: "receive", Kind: "text", Enabled: false, ExpectedRevision: receive.Revision}); err != nil {
		t.Fatal(err)
	}
	if _, err = service.clipboardSync.BeginHeader(peer.ID(), "session", receive.AuthorizationGeneration, header); !errors.Is(err, clipboardsync.ErrExpired) {
		t.Fatal("disabled permission retained old lease", err)
	}
	reopened, err := service.SetClipboardGrant(t.Context(), ClipboardGrantPatch{PeerID: peer.ID(), Direction: "receive", Kind: "text", Enabled: true, ExpectedRevision: receive.Revision + 1})
	if err != nil {
		t.Fatal(err)
	}
	newLease, err := service.clipboardSync.IssueScoped(peer.ID(), "session", reopened.AuthorizationGeneration, []clipboardsync.Grant{{Kind: clipboardsync.Text, Revision: reopened.Revision}}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	header.LeaseID = newLease.ID
	if _, err = service.clipboardSync.BeginHeader(peer.ID(), "session", reopened.AuthorizationGeneration, header); !errors.Is(err, clipboardsync.ErrStale) {
		t.Fatal("old permission revision entered reopened lease", err)
	}
}

func TestClipboardValidationDoesNotHoldCommitLockAndRevocationWinsFinalGate(t *testing.T) {
	dir := t.TempDir()
	service, err := New(Config{DataDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(service.Shutdown)
	peerIdentity, _ := identity.Generate()
	if err = identity.TrustPairedPeer(dir, identity.TrustedPeer{ID: peerIdentity.ID(), Name: "peer", PublicKey: peerIdentity.PublicKey()}); err != nil {
		t.Fatal(err)
	}
	grant, err := service.SetClipboardGrant(t.Context(), ClipboardGrantPatch{PeerID: peerIdentity.ID(), Direction: "receive", Kind: "image", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	const sessionID = "session"
	lease, err := service.clipboardSync.IssueScoped(peerIdentity.ID(), sessionID, grant.AuthorizationGeneration, []clipboardsync.Grant{{Kind: clipboardsync.Image, Revision: grant.Revision}}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	sender := clipboardsync.New(peerIdentity.ID(), nil)
	sender.Reset(false, 1)
	if err = sender.InstallScoped(lease.ID, service.identity.ID(), sessionID, grant.AuthorizationGeneration, []clipboardsync.Grant{{Kind: clipboardsync.Image, Revision: grant.Revision}}, time.Minute, 1); err != nil {
		t.Fatal(err)
	}
	base, err := sender.ObserveLocalEvent(2)
	if err != nil {
		t.Fatal(err)
	}
	base, err = sender.PrepareObserved(base, clipboardsync.Image, []byte("validated outside state lock"))
	if err != nil {
		t.Fatal(err)
	}
	event, err := sender.BindScoped(base, lease.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := service.clipboardSync.Begin(peerIdentity.ID(), sessionID, grant.AuthorizationGeneration, event)
	if err != nil {
		t.Fatal(err)
	}
	peer := &PeerSession{PeerID: peerIdentity.ID(), AuthorizationGeneration: grant.AuthorizationGeneration}
	validated, release := make(chan struct{}), make(chan struct{})
	var writes atomic.Int32
	commitDone := make(chan error, 1)
	go func() {
		commitDone <- service.commitClipboardCandidate(peer, candidate, ClipboardAdapter{
			Validate: func(clipboardsync.Kind, []byte) error { close(validated); <-release; return nil },
			Write:    func(clipboardsync.Kind, []byte, uint64, time.Time) (uint64, error) { writes.Add(1); return 2, nil },
		})
	}()
	<-validated
	if _, err = service.SetClipboardGrant(t.Context(), ClipboardGrantPatch{PeerID: peerIdentity.ID(), Direction: "receive", Kind: "image", Enabled: false, ExpectedRevision: grant.Revision}); err != nil {
		t.Fatal("revocation blocked behind image validation", err)
	}
	close(release)
	if err = <-commitDone; err == nil {
		t.Fatal("revoked candidate crossed the final mutation gate")
	}
	if writes.Load() != 0 {
		t.Fatal("native mutation ran after revocation")
	}
}

func TestClipboardGrantRejectsUnknownPeer(t *testing.T) {
	service, err := New(Config{DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(service.Shutdown)
	peer, _ := identity.Generate()
	if _, err = service.SetClipboardGrant(t.Context(), ClipboardGrantPatch{PeerID: peer.ID(), Direction: "send", Kind: "text", Enabled: true}); err == nil {
		t.Fatal("unknown peer received clipboard permission")
	}
}

func TestClipboardEnableAndRevokeShareCommitGate(t *testing.T) {
	dir := t.TempDir()
	service, err := New(Config{DataDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(service.Shutdown)
	peer, _ := identity.Generate()
	if err = identity.TrustPairedPeer(dir, identity.TrustedPeer{ID: peer.ID(), Name: "peer", PublicKey: peer.PublicKey()}); err != nil {
		t.Fatal(err)
	}
	reached, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	service.clipboardGrantBeforeCommit = func() { once.Do(func() { close(reached); <-release }) }
	enableDone := make(chan error, 1)
	go func() {
		_, setErr := service.SetClipboardGrant(t.Context(), ClipboardGrantPatch{PeerID: peer.ID(), Direction: "send", Kind: "text", Enabled: true})
		enableDone <- setErr
	}()
	<-reached
	revokeDone := make(chan error, 1)
	go func() { revokeDone <- service.BlockPeer(peer.ID()) }()
	select {
	case err := <-revokeDone:
		t.Fatal("revoke crossed enable commit gate", err)
	case <-time.After(25 * time.Millisecond):
	}
	close(release)
	if err = <-enableDone; err != nil {
		t.Fatal(err)
	}
	if err = <-revokeDone; err != nil {
		t.Fatal(err)
	}
	if service.ClipboardSyncEnabled() {
		t.Fatal("late enable survived ordered revocation")
	}
}

func TestClipboardClearFailureCannotReactivateAfterRestart(t *testing.T) {
	dir := t.TempDir()
	service, err := New(Config{DataDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	peer, _ := identity.Generate()
	if err = identity.TrustPairedPeer(dir, identity.TrustedPeer{ID: peer.ID(), Name: "peer", PublicKey: peer.PublicKey()}); err != nil {
		t.Fatal(err)
	}
	if _, err = service.SetClipboardGrant(t.Context(), ClipboardGrantPatch{PeerID: peer.ID(), Direction: "send", Kind: "text", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if _, err = service.store.db.Exec(`CREATE TRIGGER fail_clipboard_clear BEFORE UPDATE ON clipboard_grants BEGIN SELECT RAISE(ABORT,'test clear failure'); END`); err != nil {
		t.Fatal(err)
	}
	if err = service.BlockPeer(peer.ID()); err == nil {
		t.Fatal("injected clear failure was hidden")
	}
	if service.ClipboardSyncEnabled() {
		t.Fatal("invalid grant remained effective after trust revocation")
	}
	service.Shutdown()
	restarted, err := New(Config{DataDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Shutdown()
	if restarted.ClipboardSyncEnabled() {
		t.Fatal("failed clear reactivated after restart")
	}
	if err = restarted.UnblockPeer(peer.ID()); err != nil {
		t.Fatal(err)
	}
	if err = identity.TrustPairedPeer(dir, identity.TrustedPeer{ID: peer.ID(), Name: "paired-again", PublicKey: peer.PublicKey()}); err != nil {
		t.Fatal(err)
	}
	if restarted.ClipboardSyncEnabled() {
		t.Fatal("new pairing inherited stale enabled clipboard generation")
	}
}

func TestClipboardGrantValidation(t *testing.T) {
	service, err := New(Config{DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(service.Shutdown)
	for _, patch := range []ClipboardGrantPatch{
		{}, {PeerID: "peer", Direction: "both", Kind: "text"}, {PeerID: "peer", Direction: "send", Kind: "file"},
	} {
		if _, err := service.SetClipboardGrant(t.Context(), patch); err == nil {
			t.Fatalf("invalid clipboard grant accepted: %+v", patch)
		}
	}
}
