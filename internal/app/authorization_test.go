package app

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wen5555/LinkSend/internal/identity"
	"github.com/Wen5555/LinkSend/internal/protocol"
	"github.com/Wen5555/LinkSend/internal/signaling"
)

func authorizationPeer(t *testing.T, svc *Service) *identity.Identity {
	t.Helper()
	peer, err := identity.Generate()
	if err != nil {
		t.Fatal(err)
	}
	if err = identity.TrustPairedPeer(svc.cfg.DataDir, identity.TrustedPeer{ID: peer.ID(), Name: "peer", PublicKey: peer.PublicKey()}); err != nil {
		t.Fatal(err)
	}
	return peer
}

func TestRevokeWhileServerOfflineKeepsLocalDenialAndVisibleDevice(t *testing.T) {
	server := httptest.NewServer(nil)
	server.Close()
	svc, err := New(Config{DataDir: t.TempDir(), ServerURL: server.URL, AllowInsecureLoopback: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(svc.Shutdown)
	peer := authorizationPeer(t, svc)
	if err = svc.SetAlwaysAccept(peer.ID(), true); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	if err = svc.Revoke(ctx, peer.ID()); err == nil {
		t.Fatal("unavailable membership server was reported as revoked")
	}
	if err = svc.checkPeerAllowed(peer.ID()); !errors.Is(err, identity.ErrPeerDenied) {
		t.Fatalf("server failure undid local denial: %v", err)
	}
	if svc.alwaysAccept(peer.ID()) {
		t.Fatal("revoked peer retained auto accept")
	}
	devices, err := svc.Devices(ctx)
	if err != nil || len(devices) != 1 || devices[0].ID != peer.ID() || !devices[0].Blocked || devices[0].Trusted || devices[0].AlwaysAccept {
		t.Fatalf("offline blocked device was hidden or privileged: %+v %v", devices, err)
	}
}

func TestMembershipAndLANCompletionCannotRepinRevokedPeer(t *testing.T) {
	svc, err := New(Config{DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(svc.Shutdown)
	peer := authorizationPeer(t, svc)
	if err = svc.BlockPeer(peer.ID()); err != nil {
		t.Fatal(err)
	}
	if err = svc.syncPairedDevices([]signaling.Device{{ID: peer.ID(), PublicKey: peer.PublicKey(), Name: "new display name"}}); err != nil {
		t.Fatalf("blocked membership should be skipped without breaking other members: %v", err)
	}
	lateSession := &PeerSession{PeerID: peer.ID(), peerName: "peer", peerPublicKey: peer.PublicKey(), localPeer: true, lanAddress: "192.168.10.5"}
	if err = svc.rememberLANPeer(lateSession); !errors.Is(err, identity.ErrPeerDenied) {
		t.Fatalf("late LAN completion recreated pin: %v", err)
	}
	peers, err := identity.LoadTrust(svc.cfg.DataDir)
	if err != nil || len(peers) != 0 {
		t.Fatalf("revoked pin reappeared: %+v %v", peers, err)
	}
	if err = svc.UnblockPeer(peer.ID()); err != nil {
		t.Fatal(err)
	}
	peers, err = identity.LoadTrust(svc.cfg.DataDir)
	if err != nil || len(peers) != 0 || svc.alwaysAccept(peer.ID()) {
		t.Fatalf("unblock restored old privileges: %+v %v", peers, err)
	}
}

func TestRevokedPeerCannotStartLANOrWSSNegotiation(t *testing.T) {
	svc, err := New(Config{DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(svc.Shutdown)
	peer := authorizationPeer(t, svc)
	if err = svc.BlockPeer(peer.ID()); err != nil {
		t.Fatal(err)
	}
	phases := 0
	session, err := svc.ConnectDirect(t.Context(), peer.ID(), DirectConfig{onPhase: func(string) { phases++ }})
	if !errors.Is(err, identity.ErrPeerDenied) || protocol.ErrorCode(err) != protocol.AuthenticationFailed || session != nil || phases != 0 {
		t.Fatalf("revoked peer reached discovery/negotiation: session=%v phases=%d err=%v", session, phases, err)
	}
}

func TestPendingConsentCannotBypassNewRevocation(t *testing.T) {
	svc, err := New(Config{DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(svc.Shutdown)
	peer := authorizationPeer(t, svc)
	record, err := svc.tasks.create(TaskSnapshot{Direction: "receive", PeerID: peer.ID()}, func() {})
	if err != nil {
		t.Fatal(err)
	}
	record.decision = make(chan bool, 1)
	record.update(func(s *TaskSnapshot) { s.State = "awaiting_acceptance" })
	// Persisted revocation is authoritative even before the cancellation
	// callback has updated the pending task or the UI has received its event.
	if err = identity.RevokePeer(svc.cfg.DataDir, peer.ID()); err != nil {
		t.Fatal(err)
	}
	for _, accept := range []func(string) error{svc.AcceptTask, svc.AcceptTaskAlways} {
		if err = accept(record.snap.ID); !errors.Is(err, identity.ErrPeerDenied) {
			t.Fatalf("stale consent bypassed denial: %v", err)
		}
	}
	select {
	case accepted := <-record.decision:
		t.Fatalf("revoked consent reached body scheduling: %v", accepted)
	default:
	}
	if err = svc.RejectTask(record.snap.ID); err != nil {
		t.Fatalf("denial should not prevent rejecting pending content: %v", err)
	}
}

func TestBlockPeerCancelsItsCurrentTask(t *testing.T) {
	svc, err := New(Config{DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(svc.Shutdown)
	peer := authorizationPeer(t, svc)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	record, err := svc.tasks.create(TaskSnapshot{Direction: "send", PeerID: peer.ID()}, cancel)
	if err != nil {
		t.Fatal(err)
	}
	if err = svc.BlockPeer(peer.ID()); err != nil {
		t.Fatal(err)
	}
	if ctx.Err() != context.Canceled || record.snapshot().State != "cancel_requested" {
		t.Fatalf("revocation did not stop active transfer: %+v", record.snapshot())
	}
}

func TestResumeRevokedPeerDoesNotStartAnotherAttempt(t *testing.T) {
	f, _, paused := preparePausedTaskForResumeNegativeTest(t)
	if err := identity.RevokePeer(f.a.cfg.DataDir, f.bID.ID()); err != nil {
		t.Fatal(err)
	}
	if _, err := f.a.ResumeTask(paused.ID, DirectConfig{AllowLoopback: true}); !errors.Is(err, identity.ErrPeerDenied) || protocol.ErrorCode(err) != protocol.AuthenticationFailed {
		t.Fatalf("paused task bypassed revocation: %v", err)
	}
	after, ok := f.a.Task(paused.ID)
	if !ok || after.AttemptID != paused.AttemptID || after.Revision != paused.Revision || after.SentBytes != paused.SentBytes || after.State != "paused" {
		t.Fatalf("denied resume modified the task: before=%+v after=%+v", paused, after)
	}
}
