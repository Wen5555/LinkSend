package app

import (
	"bytes"
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wen5555/LinkSend/internal/identity"
	"github.com/Wen5555/LinkSend/internal/protocol"
	"github.com/Wen5555/LinkSend/internal/server"
)

func TestOfflineMembershipRevokeSurvivesRestartAndSync(t *testing.T) {
	for _, tc := range []struct {
		name   string
		lan    bool
		rejoin bool
	}{
		{name: "group"},
		{name: "group_and_lan", lan: true},
		{name: "new_incarnation_is_not_revoked", lan: true, rejoin: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			token := "offline-revoke-test-token-012345678901234567890"
			control, err := server.New(server.Config{Listen: "127.0.0.1:0", Database: filepath.Join(root, "control.db"), BootstrapToken: token, AllowInsecureLoopback: true})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = control.Close() })
			httpServer := httptest.NewServer(control.Handler())
			t.Cleanup(httpServer.Close)
			address := httpServer.Listener.Addr().String()
			newService := func(name string) *Service {
				t.Helper()
				svc, createErr := New(Config{DataDir: filepath.Join(root, name), ServerURL: httpServer.URL, AllowInsecureLoopback: true})
				if createErr != nil {
					t.Fatal(createErr)
				}
				t.Cleanup(svc.Shutdown)
				return svc
			}
			owner, target, other := newService("owner"), newService("target"), newService("other")
			ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
			defer cancel()
			if _, err = owner.Bootstrap(ctx, token, "owner"); err != nil {
				t.Fatal(err)
			}
			for _, peer := range []*Service{target, other} {
				invite, inviteErr := owner.CreateInvitation(ctx)
				if inviteErr != nil {
					t.Fatal(inviteErr)
				}
				if _, err = peer.Join(ctx, invite.Token, "peer"); err != nil {
					t.Fatal(err)
				}
			}
			if _, err = owner.Devices(ctx); err != nil {
				t.Fatal(err)
			}
			originalSnapshot, err := owner.signal.Devices(ctx)
			if err != nil {
				t.Fatal(err)
			}
			peers, err := identity.LoadTrust(owner.cfg.DataDir)
			if err != nil {
				t.Fatal(err)
			}
			var original identity.TrustedPeer
			for _, peer := range peers {
				if peer.ID == target.identity.ID() {
					original = peer
				}
			}
			if original.GrantKind != "group" || original.PeerIncarnation == "" || original.MembershipRevision == 0 {
				t.Fatalf("real server did not establish a membership grant: %+v", original)
			}
			if tc.lan {
				if err = identity.CommitLANPeer(owner.cfg.DataDir, identity.TrustedPeer{ID: original.ID, Name: original.Name, PublicKey: original.PublicKey}, original.GrantGeneration); err != nil {
					t.Fatal(err)
				}
			}
			if err = owner.SetAlwaysAccept(original.ID, true); err != nil {
				t.Fatal(err)
			}

			// Completed history and received files belong to the user, even after
			// the peer's authorization and service membership have been removed.
			receivedDir := filepath.Join(owner.cfg.DataDir, "received")
			if err = os.Mkdir(receivedDir, 0700); err != nil {
				t.Fatal(err)
			}
			receivedPath := filepath.Join(receivedDir, "preserved.txt")
			body := []byte("completed file remains after peer removal")
			if err = os.WriteFile(receivedPath, body, 0600); err != nil {
				t.Fatal(err)
			}
			record, err := owner.tasks.create(TaskSnapshot{Direction: "receive", PeerID: original.ID, SourceSummary: "preserved.txt", TargetDirectory: receivedDir, AuthorizationGeneration: original.GrantGeneration}, func() {})
			if err != nil {
				t.Fatal(err)
			}
			record.finish("completed", nil)
			history := record.snapshot()
			assertRetained := func() {
				t.Helper()
				got, ok := owner.Task(history.ID)
				if !ok || got.State != "completed" || got.PeerID != history.PeerID || got.Revision != history.Revision || got.TargetDirectory != receivedDir {
					t.Fatalf("peer removal changed completed history: %+v", got)
				}
				gotBody, readErr := os.ReadFile(receivedPath)
				if readErr != nil || !bytes.Equal(gotBody, body) {
					t.Fatalf("peer removal changed the received file: %v", readErr)
				}
			}

			// Close the actual listener: this exercises a failed HTTP connection,
			// not an injected client error or a successful HTTP error response.
			httpServer.Close()
			devices, err := owner.Devices(ctx)
			if err != nil {
				t.Fatal(err)
			}
			for _, device := range devices {
				if device.ID == original.ID && device.Relationship != "group_paired" {
					t.Errorf("offline group membership was misclassified as %q", device.Relationship)
				}
			}
			if err = owner.Revoke(ctx, original.ID); protocol.ErrorCode(err) != protocol.SignalingUnreachable {
				t.Fatalf("closed membership listener should fail remotely: %v", err)
			}
			var requestID string
			assertDenied := func(pending bool) identity.DeniedPeer {
				t.Helper()
				denied, loadErr := identity.LoadDeniedPeers(owner.cfg.DataDir)
				if loadErr != nil || len(denied) != 1 {
					t.Fatalf("revocation tombstone missing: %+v %v", denied, loadErr)
				}
				got := denied[0]
				if got.ID != original.ID || got.TargetIncarnation != original.PeerIncarnation || got.RequestID == "" || got.LocalBlock || got.PendingSync != pending || got.SeenRevision < original.MembershipRevision || got.AuthorizationGeneration <= original.GrantGeneration {
					t.Fatalf("membership outbox lost its target or state: %+v", got)
				}
				if requestID != "" && got.RequestID != requestID {
					t.Fatal("retry or restart replaced the pending revocation request")
				}
				if err = owner.checkPeerAllowed(original.ID); !errors.Is(err, identity.ErrPeerDenied) || owner.alwaysAccept(original.ID) {
					t.Fatalf("revoked peer regained permission: %v", err)
				}
				return got
			}
			requestID = assertDenied(true).RequestID
			if err = owner.Revoke(ctx, original.ID); protocol.ErrorCode(err) != protocol.SignalingUnreachable {
				t.Fatalf("offline retry unexpectedly succeeded: %v", err)
			}
			assertDenied(true)
			assertRetained()
			owner.Shutdown()
			owner = newService("owner")
			assertDenied(true)
			if err = owner.syncPairedDevices(originalSnapshot); err != nil {
				t.Fatal(err)
			}
			assertDenied(true)
			devices, err = owner.Devices(ctx)
			if err != nil {
				t.Fatal(err)
			}
			var pendingVisible bool
			for _, device := range devices {
				if device.ID == original.ID {
					pendingVisible = device.Blocked && !device.Trusted && !device.AlwaysAccept && device.Relationship == "removed" && device.ServiceState == "pending_revoke_sync"
				}
			}
			if !pendingVisible {
				t.Fatalf("restarted offline device directory lost pending removal: %+v", devices)
			}
			if _, err = owner.StartSend(original.ID, []string{receivedPath}, DirectConfig{}); !errors.Is(err, identity.ErrPeerDenied) {
				t.Fatalf("restarted profile allowed a revoked peer to send: %v", err)
			}
			assertRetained()

			listener, err := net.Listen("tcp", address)
			if err != nil {
				t.Fatal(err)
			}
			restored := httptest.NewUnstartedServer(control.Handler())
			_ = restored.Listener.Close()
			restored.Listener = listener
			restored.Start()
			t.Cleanup(restored.Close)
			if restored.URL != httpServer.URL {
				t.Fatal("test did not restore the original service endpoint")
			}
			if tc.rejoin {
				// Another authorized member removes and explicitly re-pairs the
				// same identity before the old offline request reaches the server.
				if err = other.Revoke(ctx, original.ID); err != nil {
					t.Fatal(err)
				}
				invite, inviteErr := other.CreateInvitation(ctx)
				if inviteErr != nil {
					t.Fatal(inviteErr)
				}
				if _, err = target.Join(ctx, invite.Token, "rejoined"); err != nil {
					t.Fatal(err)
				}
			}
			if _, err = owner.Devices(ctx); err != nil {
				t.Fatal(err)
			}
			remote, err := other.signal.Devices(ctx)
			if err != nil {
				t.Fatal(err)
			}
			var targetPresent, ownerPresent, otherPresent bool
			for _, device := range remote {
				switch device.ID {
				case original.ID:
					targetPresent = true
					if !tc.rejoin || device.Incarnation == original.PeerIncarnation {
						t.Fatalf("server retained the revoked incarnation: %+v", device)
					}
				case owner.identity.ID():
					ownerPresent = true
				case other.identity.ID():
					otherPresent = true
				}
			}
			if !ownerPresent || !otherPresent || targetPresent != tc.rejoin {
				t.Fatalf("outbox revoked the wrong server membership: target=%v owner=%v other=%v", targetPresent, ownerPresent, otherPresent)
			}
			if !tc.rejoin {
				synced := assertDenied(false)
				if synced.SeenRevision <= original.MembershipRevision {
					t.Fatal("successful synchronization did not retain the server revision")
				}
				owner.Shutdown()
				owner = newService("owner")
				if _, err = owner.Devices(ctx); err != nil {
					t.Fatal(err)
				}
				assertDenied(false)
			} else {
				generation, grantErr := identity.AuthorizationGeneration(owner.cfg.DataDir, original.ID)
				if grantErr != nil || generation <= original.GrantGeneration || owner.alwaysAccept(original.ID) {
					t.Fatalf("explicit new incarnation inherited old consent or generation: %d %v", generation, grantErr)
				}
			}
			assertRetained()
		})
	}
}

func TestLANOnlyRevokeDoesNotDeleteUnrelatedMembership(t *testing.T) {
	root := t.TempDir()
	control, err := server.New(server.Config{Listen: "127.0.0.1:0", Database: filepath.Join(root, "control.db"), AllowInsecureLoopback: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = control.Close() })
	handler := control.Handler()
	var deletes atomic.Int32
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			deletes.Add(1)
		}
		handler.ServeHTTP(w, r)
	}))
	t.Cleanup(httpServer.Close)
	newService := func(name string) *Service {
		t.Helper()
		svc, createErr := New(Config{DataDir: filepath.Join(root, name), ServerURL: httpServer.URL, AllowInsecureLoopback: true})
		if createErr != nil {
			t.Fatal(createErr)
		}
		t.Cleanup(svc.Shutdown)
		return svc
	}
	owner, target := newService("owner"), newService("target")
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	for _, svc := range []*Service{owner, target} {
		if _, err = svc.InitializeMembership(ctx, "separate membership"); err != nil {
			t.Fatal(err)
		}
	}
	if err = identity.TrustPairedPeer(owner.cfg.DataDir, identity.TrustedPeer{ID: target.identity.ID(), Name: "LAN peer", PublicKey: target.identity.PublicKey()}); err != nil {
		t.Fatal(err)
	}
	if err = owner.Revoke(ctx, target.identity.ID()); protocol.ErrorCode(err) != protocol.AuthenticationFailed {
		t.Fatalf("unrelated service membership should not be revocable: %v", err)
	}
	if _, err = owner.Devices(ctx); err != nil {
		t.Fatal(err)
	}
	denied, err := identity.LoadDeniedPeers(owner.cfg.DataDir)
	if err != nil || len(denied) != 1 || !denied[0].LocalBlock || denied[0].PendingSync || denied[0].RequestID != "" {
		t.Fatalf("LAN-only removal unexpectedly queued membership revocation: %+v %v", denied, err)
	}
	if deletes.Load() != 0 {
		t.Fatalf("LAN-only removal attempted %d remote membership deletions", deletes.Load())
	}
	remote, err := target.signal.Devices(ctx)
	if err != nil || len(remote) != 1 || remote[0].ID != target.identity.ID() || remote[0].Incarnation == "" {
		t.Fatalf("LAN-only removal changed unrelated membership: %+v %v", remote, err)
	}
}
