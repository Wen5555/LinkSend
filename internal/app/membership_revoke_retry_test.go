package app

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wen5555/LinkSend/internal/identity"
	"github.com/Wen5555/LinkSend/internal/protocol"
	"github.com/Wen5555/LinkSend/internal/signaling"
)

func TestMembershipRevocationsRefreshAdvancedGroupRevision(t *testing.T) {
	for _, scenario := range []string{"consecutive", "offline_then_member_added", "offline_then_member_removed"} {
		t.Run(scenario, func(t *testing.T) {
			f := newDirectFixtureServices(t)
			ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
			defer cancel()
			addMember := func() *Service {
				t.Helper()
				peer, err := New(Config{DataDir: filepath.Join(t.TempDir(), "peer"), ServerURL: f.http.URL, AllowInsecureLoopback: true})
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(peer.Shutdown)
				invite, err := f.a.CreateInvitation(ctx)
				if err != nil {
					t.Fatal(err)
				}
				if _, err = peer.Join(ctx, invite.Token, "peer"); err != nil {
					t.Fatal(err)
				}
				return peer
			}
			second := addMember()
			var unrelated *Service
			if scenario == "offline_then_member_removed" {
				unrelated = addMember()
			}
			if _, err := f.a.Devices(ctx); err != nil {
				t.Fatal(err)
			}
			address := f.http.Listener.Addr().String()
			if scenario != "consecutive" {
				f.http.Close()
			}
			for _, peerID := range []string{f.bID.ID(), second.identity.ID()} {
				err := f.a.Revoke(ctx, peerID)
				if scenario == "consecutive" && err != nil {
					t.Fatalf("another member's deletion stranded the next revocation: %v", err)
				}
				if scenario != "consecutive" && protocol.ErrorCode(err) != protocol.SignalingUnreachable {
					t.Fatalf("offline removal should persist pending synchronization: %v", err)
				}
			}
			original, err := identity.LoadDeniedPeers(f.a.cfg.DataDir)
			if err != nil || len(original) != 2 {
				t.Fatalf("expected two durable revocations: %+v %v", original, err)
			}
			if scenario != "consecutive" {
				cfg := f.a.cfg
				f.a.Shutdown()
				f.a, err = New(cfg)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(f.a.Shutdown)
				listener, err := net.Listen("tcp", address)
				if err != nil {
					t.Fatal(err)
				}
				restored := httptest.NewUnstartedServer(f.server.Handler())
				_ = restored.Listener.Close()
				restored.Listener = listener
				restored.Start()
				t.Cleanup(restored.Close)
				if scenario == "offline_then_member_added" {
					unrelated = addMember()
				} else if err = f.b.signal.Revoke(ctx, unrelated.identity.ID()); err != nil {
					t.Fatal(err)
				}
				if _, err = f.a.Devices(ctx); err != nil {
					t.Fatal(err)
				}
			}
			denied, err := identity.LoadDeniedPeers(f.a.cfg.DataDir)
			wantDenied := 2
			if scenario == "offline_then_member_removed" {
				wantDenied++ // The complete snapshot also records the unrelated removal.
			}
			if err != nil || len(denied) != wantDenied {
				t.Fatalf("revocation records changed: %+v %v", denied, err)
			}
			for _, pending := range denied {
				if pending.PendingSync {
					t.Fatalf("advanced group revision left a revocation permanently pending: %+v", pending)
				}
				for _, before := range original {
					if before.ID == pending.ID && (before.RequestID != pending.RequestID || before.TargetIncarnation != pending.TargetIncarnation) {
						t.Fatal("revision refresh replaced the request or target incarnation")
					}
				}
			}
			remote, err := f.a.signal.Devices(ctx)
			if err != nil {
				t.Fatal(err)
			}
			wantMembers := 1
			if scenario == "offline_then_member_added" {
				wantMembers++
			}
			if len(remote) != wantMembers {
				t.Fatalf("unexpected remaining members: %+v", remote)
			}
			for _, member := range remote {
				if member.ID != f.aID.ID() && (unrelated == nil || member.ID != unrelated.identity.ID()) {
					t.Fatalf("revoked target is still an active member: %s", member.ID)
				}
			}
		})
	}
}

func TestMembershipRevocationRefreshPreservesAuthorizationBoundaries(t *testing.T) {
	for _, scenario := range []string{
		"duplicate_local", "duplicate_target", "missing_local", "missing_target",
		"different_group", "inconsistent_revision", "unadvanced_revision", "new_incarnation", "revoked_target",
		"authentication_401", "snapshot_unauthorized", "retry_forbidden", "retry_success",
		"repaired_before_retry", "repaired_before_writeback",
	} {
		t.Run(scenario, func(t *testing.T) {
			dir := t.TempDir()
			localID, err := identity.Generate()
			if err != nil {
				t.Fatal(err)
			}
			peerID, err := identity.Generate()
			if err != nil {
				t.Fatal(err)
			}
			grant := identity.TrustedPeer{ID: peerID.ID(), Name: "peer", PublicKey: peerID.PublicKey(), GroupID: "group", LocalIncarnation: strings.Repeat("a", 32), PeerIncarnation: strings.Repeat("b", 32), MembershipRevision: 1}
			if err = identity.TrustMembershipPeer(dir, grant); err != nil {
				t.Fatal(err)
			}
			snapshot := []signaling.Device{
				{ID: localID.ID(), Name: "local", PublicKey: localID.PublicKey(), GroupID: grant.GroupID, Incarnation: grant.LocalIncarnation, MembershipRevision: 2},
				{ID: peerID.ID(), Name: "peer", PublicKey: peerID.PublicKey(), GroupID: grant.GroupID, Incarnation: grant.PeerIncarnation, MembershipRevision: 2},
			}
			switch scenario {
			case "duplicate_local":
				snapshot = append(snapshot, snapshot[0])
			case "duplicate_target":
				snapshot = append(snapshot, snapshot[1])
			case "missing_local":
				snapshot = snapshot[1:]
			case "missing_target":
				snapshot = snapshot[:1]
			case "different_group":
				snapshot[1].GroupID = "other-group"
			case "inconsistent_revision":
				snapshot[1].MembershipRevision = 3
			case "unadvanced_revision":
				snapshot[0].MembershipRevision, snapshot[1].MembershipRevision = 1, 1
			case "new_incarnation":
				snapshot[1].Incarnation = strings.Repeat("c", 32)
			case "revoked_target":
				snapshot[1].Revoked = true
			}
			newGrant := grant
			newGrant.PeerIncarnation, newGrant.MembershipRevision = strings.Repeat("c", 32), 3
			type request struct {
				RequestID         string `json:"request_id"`
				TargetIncarnation string `json:"target_incarnation"`
				ExpectedRevision  uint64 `json:"expected_revision"`
			}
			requests := make(chan request, 4)
			var deletes, snapshots atomic.Int32
			h := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				reject := func(status int) {
					w.WriteHeader(status)
					_ = json.NewEncoder(w).Encode(protocol.Error{Code: protocol.AuthenticationFailed, Detail: "revocation not authorized"})
				}
				if r.Method == http.MethodGet && r.URL.Path == "/v1/devices" {
					snapshots.Add(1)
					if scenario == "snapshot_unauthorized" {
						reject(http.StatusUnauthorized)
						return
					}
					if scenario == "repaired_before_retry" {
						if err := identity.TrustMembershipPeer(dir, newGrant); err != nil {
							t.Error(err)
							reject(http.StatusInternalServerError)
							return
						}
					}
					_ = json.NewEncoder(w).Encode(snapshot)
					return
				}
				if r.Method != http.MethodDelete || r.URL.Path != "/v1/devices/"+peerID.ID() {
					t.Error("unexpected membership endpoint")
					reject(http.StatusBadRequest)
					return
				}
				var payload request
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Error(err)
				}
				requests <- payload
				attempt := deletes.Add(1)
				if scenario == "authentication_401" {
					reject(http.StatusUnauthorized)
				} else if attempt == 1 || scenario == "retry_forbidden" {
					reject(http.StatusForbidden)
				} else {
					if scenario == "repaired_before_writeback" {
						if err := identity.TrustMembershipPeer(dir, newGrant); err != nil {
							t.Error(err)
							reject(http.StatusInternalServerError)
							return
						}
					}
					_ = json.NewEncoder(w).Encode(map[string]uint64{"membership_revision": 3})
				}
			}))
			defer h.Close()
			owner, err := New(Config{DataDir: dir, Identity: localID, ServerURL: h.URL, AllowInsecureLoopback: true})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(owner.Shutdown)
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			err = owner.Revoke(ctx, peerID.ID())
			if (err == nil) != (scenario == "retry_success") {
				t.Fatalf("unexpected revocation result: %v", err)
			}
			wantDeletes, wantSnapshots := int32(1), int32(1)
			if scenario == "retry_forbidden" || scenario == "retry_success" || scenario == "repaired_before_writeback" {
				wantDeletes = 2
			}
			if scenario == "authentication_401" {
				wantSnapshots = 0
			}
			if deletes.Load() != wantDeletes || snapshots.Load() != wantSnapshots {
				t.Fatalf("unsafe or unbounded retry: DELETE=%d GET=%d", deletes.Load(), snapshots.Load())
			}
			first := <-requests
			if first.RequestID == "" || first.TargetIncarnation != grant.PeerIncarnation || first.ExpectedRevision != 1 {
				t.Fatalf("initial request changed authorization binding: %+v", first)
			}
			if wantDeletes == 2 {
				second := <-requests
				if second.RequestID != first.RequestID || second.TargetIncarnation != first.TargetIncarnation || second.ExpectedRevision != 2 {
					t.Fatalf("retry changed its identity or used the wrong revision: %+v", second)
				}
			}
			denied, loadErr := identity.LoadDeniedPeers(dir)
			if loadErr != nil {
				t.Fatal(loadErr)
			}
			if scenario == "repaired_before_retry" || scenario == "repaired_before_writeback" {
				peers, loadErr := identity.LoadTrust(dir)
				if loadErr != nil || len(denied) != 0 || len(peers) != 1 || peers[0].PeerIncarnation != newGrant.PeerIncarnation {
					t.Fatalf("old request overwrote the new relationship: denied=%+v peers=%+v err=%v", denied, peers, loadErr)
				}
			} else if len(denied) != 1 || denied[0].RequestID != first.RequestID || denied[0].PendingSync != (scenario != "retry_success") {
				t.Fatalf("unexpected outbox state: %+v", denied)
			}
		})
	}
}

func TestOldActorRevocationRequiresNewExplicitIntentAfterRejoin(t *testing.T) {
	f := newDirectFixtureServices(t)
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	if _, err := f.a.Devices(ctx); err != nil {
		t.Fatal(err)
	}
	address := f.http.Listener.Addr().String()
	f.http.Close()
	if err := f.a.Revoke(ctx, f.bID.ID()); protocol.ErrorCode(err) != protocol.SignalingUnreachable {
		t.Fatalf("expected a durable offline revocation: %v", err)
	}
	before, err := identity.LoadDeniedPeers(f.a.cfg.DataDir)
	if err != nil || len(before) != 1 || !before[0].PendingSync {
		t.Fatalf("missing pending revocation: %+v %v", before, err)
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		t.Fatal(err)
	}
	restored := httptest.NewUnstartedServer(f.server.Handler())
	_ = restored.Listener.Close()
	restored.Listener = listener
	restored.Start()
	defer restored.Close()
	if err = f.b.Revoke(ctx, f.aID.ID()); err != nil {
		t.Fatal(err)
	}
	invite, err := f.b.CreateInvitation(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.a.Join(ctx, invite.Token, "explicitly rejoined actor"); err != nil {
		t.Fatal(err)
	}
	if _, err = f.a.Devices(ctx); err != nil {
		t.Fatal(err)
	}
	remote, err := f.a.signal.Devices(ctx)
	if err != nil || len(remote) != 2 {
		t.Fatalf("automatic outbox replay used the actor's new membership to revoke its peer: %+v %v", remote, err)
	}
	pending, err := identity.LoadDeniedPeers(f.a.cfg.DataDir)
	if err != nil || len(pending) != 1 || !pending[0].PendingSync || pending[0].RequestID != before[0].RequestID {
		t.Fatalf("automatic refresh replaced the old actor's intent: %+v %v", pending, err)
	}
	if err = f.a.Revoke(ctx, f.bID.ID()); err != nil {
		t.Fatalf("new explicit deletion could not replace the stale actor intent: %v", err)
	}
	after, err := identity.LoadDeniedPeers(f.a.cfg.DataDir)
	if err != nil || len(after) != 1 || after[0].PendingSync || after[0].RequestID == before[0].RequestID || after[0].TargetIncarnation != before[0].TargetIncarnation {
		t.Fatalf("explicit deletion did not preserve a distinct, target-bound intent: %+v %v", after, err)
	}
	if err = identity.MarkMembershipRevokeSynced(f.a.cfg.DataDir, f.bID.ID(), before[0].RequestID, after[0].SeenRevision+1); err == nil {
		t.Fatal("late old-request completion overwrote the new deletion intent")
	}
}

func TestLegacyMembershipRevocationRequiresExplicitReauthorization(t *testing.T) {
	f := newDirectFixtureServices(t)
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	if _, err := f.a.Devices(ctx); err != nil {
		t.Fatal(err)
	}
	address := f.http.Listener.Addr().String()
	f.http.Close()
	if err := f.a.Revoke(ctx, f.bID.ID()); protocol.ErrorCode(err) != protocol.SignalingUnreachable {
		t.Fatalf("expected offline revocation: %v", err)
	}
	cfg := f.a.cfg
	f.a.Shutdown()
	// Recreate a schema-2 file written by an older binary, before restarting
	// the isolated test service. Legacy records have no actor binding.
	path := filepath.Join(cfg.DataDir, "trust.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var legacy identity.TrustFile
	if err = json.Unmarshal(data, &legacy); err != nil || len(legacy.DeniedPeers) != 1 {
		t.Fatalf("unexpected test policy: %v", err)
	}
	legacy.DeniedPeers[0].ActorGroupID, legacy.DeniedPeers[0].ActorIncarnation = "", ""
	original := legacy.DeniedPeers[0]
	data, err = json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	f.a, err = New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(f.a.Shutdown)
	listener, err := net.Listen("tcp", address)
	if err != nil {
		t.Fatal(err)
	}
	restored := httptest.NewUnstartedServer(f.server.Handler())
	_ = restored.Listener.Close()
	restored.Listener = listener
	restored.Start()
	defer restored.Close()
	third, err := New(Config{DataDir: t.TempDir(), ServerURL: f.http.URL, AllowInsecureLoopback: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(third.Shutdown)
	invite, err := f.b.CreateInvitation(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = third.Join(ctx, invite.Token, "unrelated new member"); err != nil {
		t.Fatal(err)
	}
	if _, err = f.a.Devices(ctx); err != nil {
		t.Fatal(err)
	}
	denied, err := identity.LoadDeniedPeers(cfg.DataDir)
	if err != nil || len(denied) != 1 || !denied[0].PendingSync || denied[0].RequestID != original.RequestID || denied[0].ActorGroupID != "" || denied[0].ActorIncarnation != "" {
		t.Fatalf("background sync inferred an actor for legacy intent: %+v %v", denied, err)
	}
	if err = f.a.Revoke(ctx, f.bID.ID()); err != nil {
		t.Fatalf("explicit legacy removal did not establish a fresh intent: %v", err)
	}
	denied, err = identity.LoadDeniedPeers(cfg.DataDir)
	if err != nil || len(denied) != 1 || denied[0].PendingSync || denied[0].RequestID == original.RequestID || denied[0].TargetIncarnation != original.TargetIncarnation || denied[0].ActorGroupID == "" || len(denied[0].ActorIncarnation) != 32 {
		t.Fatalf("explicit intent lacks its actor binding: %+v %v", denied, err)
	}
	remote, err := f.a.signal.Devices(ctx)
	if err != nil || len(remote) != 2 {
		t.Fatalf("legacy intent removed another member: %+v %v", remote, err)
	}
	for _, peer := range remote {
		if peer.ID != f.aID.ID() && peer.ID != third.identity.ID() {
			t.Fatal("legacy target is still present")
		}
	}
}
