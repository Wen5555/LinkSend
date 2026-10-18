package app

import (
	"context"
	"encoding/json"
	"errors"
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
	"github.com/Wen5555/LinkSend/internal/server"
	"github.com/Wen5555/LinkSend/internal/signaling"
)

type recoveryHandler struct{ handler http.Handler }
type recoveryInitBarrier struct{ entered, release chan struct{} }

type recoveryFixture struct {
	a, b        *Service
	http        *httptest.Server
	handler     atomic.Pointer[recoveryHandler]
	dropInit    atomic.Bool
	initBarrier atomic.Pointer[recoveryInitBarrier]
	getBarrier  atomic.Pointer[recoveryInitBarrier]
	oldHandler  *recoveryHandler
	initializes atomic.Int32
	joins       atomic.Int32
	deletes     atomic.Int32
	oldSnapshot []signaling.Device
	ctx         context.Context
}

func newRecoveryFixture(t *testing.T) *recoveryFixture {
	t.Helper()
	f := &recoveryFixture{ctx: t.Context()}
	root := t.TempDir()
	makeServer := func(path string) http.Handler {
		srv, err := server.New(server.Config{Listen: "127.0.0.1:0", Database: filepath.Join(root, path), AllowInsecureLoopback: true})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = srv.Close() })
		return srv.Handler()
	}
	f.oldHandler = &recoveryHandler{makeServer("old.db")}
	f.handler.Store(f.oldHandler)
	f.http = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/v1/devices" {
			if barrier := f.getBarrier.Swap(nil); barrier != nil {
				close(barrier.entered)
				select {
				case <-barrier.release:
				case <-r.Context().Done():
					return
				}
			}
		}
		if r.Method == http.MethodDelete {
			f.deletes.Add(1)
		}
		if r.URL.Path == "/v1/pairing/join" {
			f.joins.Add(1)
		}
		if r.URL.Path == "/v1/membership/init" {
			f.initializes.Add(1)
			if barrier := f.initBarrier.Swap(nil); barrier != nil {
				close(barrier.entered)
				select {
				case <-barrier.release:
				case <-r.Context().Done():
					return
				}
			}
			if f.dropInit.Swap(false) {
				recorder := httptest.NewRecorder()
				f.handler.Load().handler.ServeHTTP(recorder, r)
				if recorder.Code != http.StatusCreated {
					t.Errorf("lost response fixture did not actually initialize: %d", recorder.Code)
				}
				conn, _, err := w.(http.Hijacker).Hijack()
				if err != nil {
					t.Error(err)
					return
				}
				_ = conn.Close()
				return
			}
		}
		f.handler.Load().handler.ServeHTTP(w, r)
	}))
	t.Cleanup(f.http.Close)
	var err error
	f.a, err = New(Config{DataDir: filepath.Join(root, "a"), ServerURL: f.http.URL, AllowInsecureLoopback: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(f.a.Shutdown)
	f.b, err = New(Config{DataDir: filepath.Join(root, "b"), ServerURL: f.http.URL, AllowInsecureLoopback: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(f.b.Shutdown)
	if _, err = f.a.InitializeMembership(f.ctx, "a"); err != nil {
		t.Fatal(err)
	}
	invitation, err := f.a.CreateInvitation(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.b.Join(f.ctx, invitation.Token, "b"); err != nil {
		t.Fatal(err)
	}
	third, err := New(Config{DataDir: filepath.Join(root, "old-third"), ServerURL: f.http.URL, AllowInsecureLoopback: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(third.Shutdown)
	invitation, err = f.a.CreateInvitation(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	thirdMember, err := third.Join(f.ctx, invitation.Token, "offline removed member")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.a.Devices(f.ctx); err != nil {
		t.Fatal(err)
	}
	c, _ := f.a.client()
	f.oldSnapshot, err = c.Devices(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	// Persist an offline outbox item under a real old actor/target incarnation.
	// Do not send DELETE: the test proves recovery never sends this old intent
	// through the same URL after the control database has been replaced.
	if err = identity.RevokeMembershipPeer(f.a.cfg.DataDir, third.identity.ID(), thirdMember.Incarnation, "old-offline-revoke", thirdMember.MembershipRevision); err != nil {
		t.Fatal(err)
	}
	// Replace the database behind exactly the same URL. No identity/profile is
	// removed, no cached group is uploaded, and the normal server APIs remain.
	f.handler.Store(&recoveryHandler{makeServer("empty.db")})
	f.initializes.Store(0)
	f.joins.Store(0)
	return f
}

func TestExplicitMembershipRecoverySameURLInitializeJoinAndOldQueue(t *testing.T) {
	f := newRecoveryFixture(t)
	oldID := f.a.identity.ID()
	oldGeneration, err := identity.AuthorizationGeneration(f.a.cfg.DataDir, f.b.identity.ID())
	if err != nil {
		t.Fatal(err)
	}
	if err = identity.CommitLANPeer(f.a.cfg.DataDir, identity.TrustedPeer{ID: f.b.identity.ID(), PublicKey: f.b.identity.PublicKey(), LastLANAddress: "192.168.10.5"}, oldGeneration); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(t.TempDir(), "not-autosent.txt")
	if err = os.WriteFile(source, []byte("retain source and isolate old queue"), 0600); err != nil {
		t.Fatal(err)
	}
	queued, err := f.a.Enqueue(EnqueueRequest{RequestID: "old-group-queue", PeerID: f.b.identity.ID(), Paths: []string{source}, WaitForPeer: true})
	if err != nil {
		t.Fatal(err)
	}
	if f.a.Membership(f.ctx).State != "auth_failed" {
		t.Fatal("empty same-URL service was not reported as authentication failure")
	}
	if status, e := f.a.MembershipRecoveryStatus(); e != nil || status.State != "idle" {
		t.Fatal("401 automatically created a recovery intent", status, e)
	}
	if _, err = f.a.InitializeMembership(f.ctx, "unconfirmed reset"); err == nil || f.initializes.Load() != 0 {
		t.Fatal("ordinary initialize bypassed explicit old-profile recovery", err)
	}
	if _, err = f.a.PrepareMembershipRecovery("initialize", false); err == nil {
		t.Fatal("unconfirmed recovery changed the profile")
	}
	prepared, err := f.a.PrepareMembershipRecovery("initialize", true)
	if err != nil {
		t.Fatal(err)
	}
	deletesAtFence := f.deletes.Load() // Ordinary pre-recovery queue preflight may retry the old offline outbox.
	if f.a.Membership(f.ctx).State != "recovery_pending" {
		t.Fatal("pending intent was hidden by a generic authentication failure")
	}
	if _, err = f.a.Join(f.ctx, "not-a-code", "a"); err == nil {
		t.Fatal("normal join bypassed pending recovery")
	}
	if _, err = f.a.Devices(f.ctx); err != nil {
		t.Fatal("local/LAN directory unavailable during recovery", err)
	}
	owner, err := f.a.ContinueMembershipRecovery(f.ctx, prepared.ID, "", "a recovered")
	if err != nil || owner.Admin || owner.ID != oldID || owner.GroupID == f.oldSnapshot[0].GroupID {
		t.Fatalf("normal new initialization failed: %+v %v", owner, err)
	}
	if _, err = f.a.ContinueMembershipRecovery(f.ctx, prepared.ID, "", "a recovered"); err != nil || f.initializes.Load() != 1 {
		t.Fatal("completion retry initialized a second group", err, f.initializes.Load())
	}
	if err = f.a.syncPairedDevices(f.oldSnapshot); err == nil {
		t.Fatal("late old snapshot was applied after explicit recovery")
	}
	if err = f.a.setQueueState(queued.ID, "needs_attention", "RESTART_CONFIRMATION_REQUIRED"); err != nil {
		t.Fatal(err)
	}
	items, err := f.a.queueItems()
	if err != nil || len(items) != 1 {
		t.Fatal("queue history removed", items, err)
	}
	if err = f.a.ConfirmQueue(items[0].ID, items[0].Revision); protocol.ErrorCode(err) != protocol.AuthenticationFailed {
		t.Fatal("old queue rebound to retained independent LAN", err)
	}
	invitation, err := f.a.CreateInvitation(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	joining, err := f.b.PrepareMembershipRecovery("join", true)
	if err != nil {
		t.Fatal(err)
	}
	joined, err := f.b.ContinueMembershipRecovery(f.ctx, joining.ID, invitation.Token, "b recovered")
	if err != nil || joined.Admin || joined.GroupID != owner.GroupID || f.joins.Load() != 1 {
		t.Fatalf("normal fresh invitation failed: %+v %v", joined, err)
	}
	if _, err = f.b.ContinueMembershipRecovery(f.ctx, joining.ID, "", "b recovered"); err != nil || f.joins.Load() != 1 {
		t.Fatal("completion retry replayed a consumed code", err)
	}
	if _, err = f.a.Devices(f.ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = identity.AuthorizationGeneration(f.a.cfg.DataDir, f.b.identity.ID()); err != nil {
		t.Fatal("new peer was not actually trusted on recovered first device", err)
	}
	if _, err = identity.AuthorizationGeneration(f.b.cfg.DataDir, f.a.identity.ID()); err != nil {
		t.Fatal("new peer was not actually trusted on recovered joining device", err)
	}
	denied, err := identity.LoadDeniedPeers(f.a.cfg.DataDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, old := range denied {
		if old.RequestID == "old-offline-revoke" && (!old.ContextRetired || old.PendingSync || old.ActorGroupID == "") {
			t.Fatal("old offline request lost its context isolation", old)
		}
	}
	if f.deletes.Load() != deletesAtFence || f.a.identity.ID() != oldID {
		t.Fatal("recovery sent an old revocation or changed identity", f.deletes.Load(), deletesAtFence)
	}
	if actual, e := os.ReadFile(source); e != nil || !strings.Contains(string(actual), "retain source") {
		t.Fatal("source changed during membership recovery", e)
	}
}

func TestMembershipRecoveryLostResponsePauseAndRestartReconciles(t *testing.T) {
	f := newRecoveryFixture(t)
	oldID := f.a.identity.ID()
	prepared, err := f.a.PrepareMembershipRecovery("initialize", true)
	if err != nil {
		t.Fatal(err)
	}
	f.dropInit.Store(true)
	if _, err = f.a.ContinueMembershipRecovery(f.ctx, prepared.ID, "", "a"); err == nil {
		t.Fatal("lost response unexpectedly completed locally")
	}
	paused, err := f.a.CancelMembershipRecovery(prepared.ID)
	if err != nil || paused.State != "paused" {
		t.Fatal("uncertain request cancellation discarded recovery", paused, err)
	}
	dataDir := f.a.cfg.DataDir
	f.a.Shutdown()
	reopened, err := New(Config{DataDir: dataDir, ServerURL: f.http.URL, AllowInsecureLoopback: true})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Shutdown()
	if reopened.identity.ID() != oldID {
		t.Fatal("recovery restart changed persisted identity")
	}
	status, err := reopened.MembershipRecoveryStatus()
	if err != nil || status.State != "paused" || status.ID != prepared.ID {
		t.Fatal("recovery intent lost after restart", status, err)
	}
	ctx, cancel := context.WithTimeout(f.ctx, 5*time.Second)
	defer cancel()
	owner, err := reopened.ContinueMembershipRecovery(ctx, prepared.ID, "", "a")
	if err != nil || owner.ID != oldID || f.initializes.Load() != 1 {
		t.Fatal("restart did not reconcile the actual initialized membership", owner, err)
	}
	if intent, _, e := identity.LoadMembershipRecovery(dataDir); e != nil || intent != nil {
		t.Fatal("successful reconciliation retained pending state", intent, e)
	}
	if err = reopened.syncPairedDevices(f.oldSnapshot); !errors.Is(err, identity.ErrAuthentication) {
		t.Fatal("old snapshot did not remain retired after restart", err)
	}
}

func TestRecoverySerializesOldOutboxBeforeAnyDeleteInNewContext(t *testing.T) {
	f := newRecoveryFixture(t)
	denied, err := identity.LoadDeniedPeers(f.a.cfg.DataDir)
	if err != nil || len(denied) != 1 {
		t.Fatal("old outbox fixture missing", denied, err)
	}
	oldIntent := denied[0]
	prepared, err := f.a.PrepareMembershipRecovery("initialize", true)
	if err != nil {
		t.Fatal(err)
	}
	barrier := &recoveryInitBarrier{entered: make(chan struct{}), release: make(chan struct{})}
	f.initBarrier.Store(barrier)
	ctx, cancel := context.WithTimeout(f.ctx, 5*time.Second)
	defer cancel()
	recovered := make(chan error, 1)
	go func() { _, e := f.a.ContinueMembershipRecovery(ctx, prepared.ID, "", "a"); recovered <- e }()
	select {
	case <-barrier.entered:
	case <-ctx.Done():
		t.Fatal("recovery did not reach real initialize request")
	}
	// Dispatch the original durable tuple while recovery owns the membership
	// operation lock. It must revalidate after that lock is released.
	polled := make(chan error, 1)
	go func() {
		c, e := f.a.client()
		if e == nil {
			_, _, e = f.a.revokeMembershipWithRefresh(ctx, c, oldIntent, false)
		}
		polled <- e
	}()
	close(barrier.release)
	for index, result := range []chan error{recovered, polled} {
		select {
		case e := <-result:
			if index == 0 && e != nil || index == 1 && protocol.ErrorCode(e) != protocol.AuthenticationFailed {
				t.Fatal(e)
			}
		case <-ctx.Done():
			t.Fatal("recovery/outbox operation deadlocked")
		}
	}
	if f.deletes.Load() != 0 {
		t.Fatal("selected old outbox reached the new service context")
	}
}

func TestRecoveryCapturesLegacyContextBeforeDelayedReadCanApply(t *testing.T) {
	f := newRecoveryFixture(t)
	dataDir := t.TempDir()
	legacy := identity.TrustFile{Peers: []identity.TrustedPeer{{ID: f.b.identity.ID(), Name: "legacy cached key", PublicKey: f.b.identity.PublicKey()}}}
	data, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dataDir, "trust.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	svc, err := New(Config{DataDir: dataDir, Identity: f.a.identity, ServerURL: f.http.URL, AllowInsecureLoopback: true})
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Shutdown()
	newHandler := f.handler.Load()
	f.handler.Store(f.oldHandler)
	barrier := &recoveryInitBarrier{entered: make(chan struct{}), release: make(chan struct{})}
	f.getBarrier.Store(barrier)
	ctx, cancel := context.WithTimeout(f.ctx, 5*time.Second)
	defer cancel()
	read := make(chan error, 1)
	go func() { _, e := svc.Devices(ctx); read <- e }()
	select {
	case <-barrier.entered:
	case <-ctx.Done():
		t.Fatal("legacy read did not reach real old service")
	}
	type preparedResult struct {
		status MembershipRecoveryStatus
		err    error
	}
	prepared := make(chan preparedResult, 1)
	go func() {
		status, e := svc.PrepareMembershipRecovery("initialize", true)
		prepared <- preparedResult{status, e}
	}()
	close(barrier.release)
	select {
	case err = <-read:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("legacy membership read did not finish")
	}
	var intent MembershipRecoveryStatus
	select {
	case result := <-prepared:
		if result.err != nil {
			t.Fatal(result.err)
		}
		intent = result.status
	case <-ctx.Done():
		t.Fatal("preparation did not serialize after old membership read")
	}
	f.handler.Store(newHandler)
	if _, err = svc.ContinueMembershipRecovery(ctx, intent.ID, "", "legacy recovered"); err != nil {
		t.Fatal(err)
	}
	late := append([]signaling.Device(nil), f.oldSnapshot...)
	for index := range late {
		late[index].MembershipRevision = 9999
	}
	if err = svc.syncPairedDevices(late); err == nil {
		t.Fatal("late context absent from the initial legacy cache was not retired")
	}
}

func TestPendingRecoveryDoesNotAutomaticallyIssueOrJoinLANMembership(t *testing.T) {
	f := newRecoveryFixture(t)
	if _, err := f.a.PrepareMembershipRecovery("initialize", true); err != nil {
		t.Fatal(err)
	}
	if state := f.a.joinLANMembershipCredential(f.ctx, "unconsumed-fixture-code"); state != "pending" || f.joins.Load() != 0 {
		t.Fatal("LAN consent automatically joined a group during explicit recovery", state)
	}
	if _, err := f.a.issueLANMembershipCredential(f.ctx, f.b.identity.PublicKey(), signaling.LANPairFrame{}, signaling.LANPairFrame{}); err == nil {
		t.Fatal("LAN membership credential was issued during recovery")
	}
	if f.initializes.Load() != 0 {
		t.Fatal("LAN path initialized a group during explicit recovery")
	}
}
