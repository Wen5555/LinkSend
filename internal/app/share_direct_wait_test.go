package app

import (
	"context"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wen5555/LinkSend/internal/protocol"
	"github.com/Wen5555/LinkSend/internal/sharegrant"
	"github.com/Wen5555/LinkSend/internal/signaling"
	"github.com/Wen5555/LinkSend/internal/store"
)

func TestShareDirectPeerMayEnterAfterAuthenticatedCheckDeadline(t *testing.T) {
	f := newShareDirectFixture(t)
	type barrier struct{ check, wait context.Context }
	entered := make(chan barrier, 1)
	f.a.temporaryShares.beforeSignalWait = func(check, wait context.Context) {
		entered <- barrier{check, wait}
	}
	done := make(chan error, 1)
	go func() {
		peer, err := f.a.connectSharePeer(f.ctx, f.scope, f.approved, DirectConfig{BindAddress: "127.0.0.1:0", AllowLoopback: true, CheckTimeout: time.Second, WaitTimeout: 6 * time.Second}, func() error { return nil })
		if peer != nil {
			_ = peer.Close()
		}
		done <- err
	}()
	var gate barrier
	select {
	case gate = <-entered:
	case <-f.ctx.Done():
		t.Fatal("did not reach authenticated peer wait")
	}
	// Observe the actual short handshake context expiring after real TLS/WSS
	// admission. No wall-clock guess decides when the second actor may enter.
	select {
	case <-gate.check.Done():
	case <-f.ctx.Done():
		t.Fatal("short handshake context never ended")
	}
	if gate.wait.Err() != nil {
		if err := <-done; protocol.ErrorCode(err) != protocol.SignalingTimeout {
			t.Fatal("unexpected old peer-wait result", protocol.ErrorCode(err))
		}
		t.Fatal("authenticated peer wait expired with the short handshake budget before the peer entered")
	}
	remote, err := f.b.connectSharePeer(f.ctx, f.scope, f.approved, DirectConfig{BindAddress: "127.0.0.1:0", AllowLoopback: true, CheckTimeout: 3 * time.Second, WaitTimeout: 6 * time.Second}, func() error { return nil })
	if remote != nil {
		t.Cleanup(func() { _ = remote.Close() })
	}
	if localErr := <-done; localErr != nil || err != nil {
		t.Fatal("late peer did not establish fresh authenticated ICE/QUIC", localErr, err)
	}
	if evidence := remote.Evidence(); evidence.TLSVersion != 772 || evidence.Relay || evidence.SessionID == "" {
		t.Fatal("late peer has no pinned direct TLS evidence", evidence)
	}
}

func TestShareDirectAuthenticatedPeerWaitHonorsPhaseAndRunLimits(t *testing.T) {
	for _, phase := range []sharegrant.SignalPhase{sharegrant.SignalPreview, sharegrant.SignalTransfer} {
		for _, parentLimited := range []bool{false, true} {
			name := string(phase) + "/grant"
			if parentLimited {
				name = string(phase) + "/original_run"
			}
			t.Run(name, func(t *testing.T) {
				f := newShareDirectFixture(t)
				status, scope := f.approved, f.scope
				if phase == sharegrant.SignalTransfer {
					var err error
					status, err = f.cb.ShareControl(f.ctx, signaling.ShareBind, signaling.ShareRequest{ID: status.ID, TaskID: status.TaskID, Generation: status.Generation, Manifest: status.Manifest, Plan: strings.Repeat("b", 64)})
					if err != nil {
						t.Fatal(err)
					}
					scope.Phase, scope.Plan = phase, status.Plan
				}
				want := time.Unix(status.ExpiresAt, 0)
				if phase == sharegrant.SignalTransfer {
					want = time.Unix(status.ResumeUntil, 0)
				}
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				if parentLimited {
					var stop context.CancelFunc
					want = time.Now().Add(15 * time.Second)
					ctx, stop = context.WithDeadline(ctx, want)
					defer stop()
				}
				var got time.Time
				f.a.temporaryShares.beforeSignalWait = func(_ context.Context, wait context.Context) {
					got, _ = wait.Deadline()
					cancel()
				}
				peer, err := f.a.connectSharePeer(ctx, scope, status, DirectConfig{AllowLoopback: true, CheckTimeout: 2 * time.Second, WaitTimeout: 48 * time.Hour}, func() error { return nil })
				if peer != nil || !got.Equal(want) || protocol.ErrorCode(err) != protocol.Cancelled || !errors.Is(err, context.Canceled) {
					t.Fatal("authenticated wait extended or chose the wrong absolute limit", got, want, protocol.ErrorCode(err))
				}
			})
		}
	}
}

func TestShareDirectPeerArrivalRechecksLocalAuthorization(t *testing.T) {
	for _, blocked := range []bool{false, true} {
		name := "owner_scope"
		want := protocol.ShareRejected
		if blocked {
			name, want = "blacklist", protocol.AuthenticationFailed
		}
		t.Run(name, func(t *testing.T) {
			f := newShareDirectFixture(t)
			entered := make(chan struct{})
			f.a.temporaryShares.beforeSignalWait = func(context.Context, context.Context) { close(entered) }
			var deny atomic.Bool
			var phase string
			done := make(chan error, 1)
			go func() {
				peer, err := f.a.connectSharePeer(f.ctx, f.scope, f.approved, DirectConfig{AllowLoopback: true, CheckTimeout: 2 * time.Second, WaitTimeout: time.Minute, onPhase: func(value string) { phase = value }}, func() error {
					if deny.Load() {
						return temporaryShareFailure(protocol.ShareRejected)
					}
					return nil
				})
				if peer != nil {
					_ = peer.Close()
				}
				done <- err
			}()
			select {
			case <-entered:
			case <-f.ctx.Done():
				t.Fatal("authenticated wait was not reached")
			}
			if blocked {
				if err := f.a.BlockPeer(f.b.identity.ID()); err != nil {
					t.Fatal(err)
				}
			} else {
				deny.Store(true)
			}
			remote, err := f.cb.ConnectShare(f.ctx, f.scope, f.approved, signaling.ShareSignalOptions{AllowLoopbackCandidates: true})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = remote.Close() })
			if err := <-done; protocol.ErrorCode(err) != want || phase != "waiting_share_peer" {
				t.Fatal("stale local authorization reached endpoint setup", protocol.ErrorCode(err), phase)
			}
		})
	}
}

func TestShareDirectLongPeerWaitDoesNotExtendHTTPHandshake(t *testing.T) {
	f := newTemporaryShareFixture(t)
	created, _ := createApprovedTemporaryShare(t, f)
	e, err := f.a.temporaryEntry(created.Share.ID)
	if err != nil {
		t.Fatal(err)
	}
	r := temporaryRecordCopy(e)
	blocked := &temporaryBlockedHTTP{entered: make(chan struct{}), cancelled: make(chan struct{}), release: make(chan struct{})}
	client, err := signaling.New(signaling.Config{ServerURL: f.origin, Identity: f.a.identity, HTTPClient: &http.Client{Transport: blocked}})
	if err != nil {
		t.Fatal(err)
	}
	f.a.signal = client
	enteredWait := false
	f.a.temporaryShares.beforeSignalWait = func(context.Context, context.Context) { enteredWait = true }
	peer, err := f.a.connectSharePeer(t.Context(), temporaryScope(r, sharegrant.SignalPreview), r.Status, DirectConfig{CheckTimeout: 200 * time.Millisecond, WaitTimeout: time.Hour}, func() error { return nil })
	if peer != nil || enteredWait || protocol.ErrorCode(err) != protocol.SignalingTimeout {
		t.Fatal("human wait budget entered the HTTP/WSS handshake", protocol.ErrorCode(err))
	}
	select {
	case <-blocked.cancelled:
	default:
		t.Fatal("short handshake did not cancel its actual HTTP request")
	}
}

func TestShareDirectPeerPresentNetworkStillUsesShortBudget(t *testing.T) {
	f := newShareDirectFixture(t)
	remote, err := f.cb.ConnectShare(f.ctx, f.scope, f.approved, signaling.ShareSignalOptions{AllowLoopbackCandidates: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = remote.Close() })
	var wait context.Context
	f.a.temporaryShares.beforeSignalWait = func(_ context.Context, value context.Context) { wait = value }
	peer, err := f.a.connectSharePeer(f.ctx, f.scope, f.approved, DirectConfig{BindAddress: "127.0.0.1:0", AllowLoopback: true, CheckTimeout: 500 * time.Millisecond, WaitTimeout: time.Hour}, func() error { return nil })
	if peer != nil || protocol.ErrorCode(err) != protocol.SignalingTimeout || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("a present peer's missing connect_response did not end in the short network window", protocol.ErrorCode(err))
	}
	if wait == nil || !errors.Is(wait.Err(), context.Canceled) {
		t.Fatal("peer-presence wait context survived peer arrival")
	}
}

func TestShareDirectAlreadyReadyPeerCannotOutliveWaitDeadline(t *testing.T) {
	f := newShareDirectFixture(t)
	remote, err := f.cb.ConnectShare(f.ctx, f.scope, f.approved, signaling.ShareSignalOptions{AllowLoopbackCandidates: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = remote.Close() })
	f.a.temporaryShares.beforeSignalWait = func(_ context.Context, wait context.Context) { <-wait.Done() }
	var phase string
	peer, err := f.a.connectSharePeer(f.ctx, f.scope, f.approved, DirectConfig{AllowLoopback: true, CheckTimeout: 2 * time.Second, WaitTimeout: 100 * time.Millisecond, onPhase: func(value string) { phase = value }}, func() error { return nil })
	if peer != nil || protocol.ErrorCode(err) != protocol.SignalingTimeout || !errors.Is(err, context.DeadlineExceeded) || phase != "waiting_share_peer" {
		t.Fatal("ready projection bypassed the expired local wait budget", protocol.ErrorCode(err), phase)
	}
}

func TestShareDirectPeerWaitCancellationReleasesScopedSockets(t *testing.T) {
	f := newShareDirectFixture(t)
	// Repeated cancellation must join the local reader and allow the same
	// immutable scope to connect again. Reconnection and fixture shutdown check
	// teardown without runtime goroutine-count guesses.
	for range 9 {
		ctx, cancel := context.WithCancel(f.ctx)
		f.a.temporaryShares.beforeSignalWait = func(context.Context, context.Context) { cancel() }
		peer, err := f.a.connectSharePeer(ctx, f.scope, f.approved, DirectConfig{AllowLoopback: true, CheckTimeout: 2 * time.Second, WaitTimeout: time.Minute}, func() error { return nil })
		cancel()
		if peer != nil || protocol.ErrorCode(err) != protocol.Cancelled || !errors.Is(err, context.Canceled) {
			t.Fatal("cancelled scoped wait retained resources or lost its cause", protocol.ErrorCode(err))
		}
	}
	f.a.temporaryShares.beforeSignalWait = nil
	a, b := f.connect(t)
	if a.Evidence().TLSVersion != 772 || b.Evidence().TLSVersion != 772 {
		t.Fatal("cancelled waits prevented the next pinned direct session")
	}
}

func TestShareDirectExpiredPhaseCannotEnterSignaling(t *testing.T) {
	for _, phase := range []sharegrant.SignalPhase{sharegrant.SignalPreview, sharegrant.SignalTransfer} {
		t.Run(string(phase), func(t *testing.T) {
			f := newShareDirectFixture(t)
			status, scope := f.approved, f.scope
			if phase == sharegrant.SignalTransfer {
				status.ResumeUntil = time.Now().Unix()
				scope.Phase, scope.Plan = phase, strings.Repeat("b", 64)
			} else {
				status.ExpiresAt = time.Now().Unix()
			}
			entered := false
			peer, err := f.a.connectSharePeer(f.ctx, scope, status, DirectConfig{CheckTimeout: time.Second, WaitTimeout: time.Hour, onPhase: func(string) { entered = true }}, func() error { return nil })
			if peer != nil || entered || protocol.ErrorCode(err) != protocol.ShareExpired {
				t.Fatal("expired phase entered signaling or acquired a new wait deadline", protocol.ErrorCode(err))
			}
		})
	}
}

func TestShareDirectSignalingExpiryCauseKeepsAuthorizationCode(t *testing.T) {
	ctx, cancel := context.WithDeadlineCause(t.Context(), time.Now(), temporaryShareFailure(protocol.ShareExpired))
	defer cancel()
	<-ctx.Done()
	got := classifyShareSignalingError(ctx, ctx.Err())
	if protocol.ErrorCode(got) != protocol.ShareExpired || !errors.Is(got, context.DeadlineExceeded) {
		t.Fatal("grant expiry became a signaling timeout", protocol.ErrorCode(got))
	}
	original := protocol.Fail(protocol.AuthenticationFailed, "fixed fixture")
	if got := classifyShareSignalingError(ctx, original); got != original {
		t.Fatal("expiry overwrote an authoritative security result")
	}
}

func TestShareDirectAuthenticatedWaitEndsAtOriginalGrantExpiry(t *testing.T) {
	for _, phase := range []sharegrant.SignalPhase{sharegrant.SignalPreview, sharegrant.SignalTransfer} {
		t.Run(string(phase), func(t *testing.T) {
			f := newShareDirectFixture(t)
			// Seed a valid, already-aged grant using the real store transitions in
			// this isolated SQLite fixture. Production TTLs and the server clock
			// remain unchanged; real TLS/WSS observes its approaching expiry.
			control, err := store.OpenControl(f.database)
			if err != nil {
				t.Fatal(err)
			}
			defer control.Close()
			expiry := time.Now().Truncate(time.Second).Add(3 * time.Second)
			created := expiry.Add(-sharegrant.OfferTTL)
			if phase == sharegrant.SignalTransfer {
				created = expiry.Add(-sharegrant.ResumeTTL - time.Second)
			}
			code, digest, err := sharegrant.GenerateCode()
			if err != nil {
				t.Fatal(err)
			}
			var sender, receiver sharegrant.Key
			copy(sender[:], f.a.identity.PublicKey())
			copy(receiver[:], f.b.identity.PublicKey())
			var manifest sharegrant.Digest
			raw, _ := hex.DecodeString(f.approved.Manifest)
			copy(manifest[:], raw)
			record, err := sharegrant.New(protocol.RandomID(), sender, manifest, digest, created)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = control.CreateShare(f.ctx, record, created); err != nil {
				t.Fatal(err)
			}
			record, err = control.ClaimShare(f.ctx, receiver, code, protocol.RandomID(), created)
			if err != nil {
				t.Fatal(err)
			}
			record, err = control.ApproveShare(f.ctx, record.ID, sender, record.ClaimID, protocol.RandomID(), f.approved.TaskID, record.Generation, created)
			if err != nil {
				t.Fatal(err)
			}
			if phase == sharegrant.SignalTransfer {
				var plan sharegrant.Digest
				plan[0] = 1
				record, err = control.BindShare(f.ctx, record.ID, receiver, record.TaskID, record.Manifest, plan, record.Generation, expiry.Add(-sharegrant.ResumeTTL))
				if err != nil {
					t.Fatal(err)
				}
			}
			status, err := f.ca.ShareControl(f.ctx, signaling.ShareRead, signaling.ShareRequest{ID: record.ID})
			if err != nil {
				t.Fatal(err)
			}
			scope := sharegrant.SignalScope{GrantID: status.ID, ClaimID: status.ClaimID, TaskID: status.TaskID, GrantGeneration: status.Generation, Phase: phase, Manifest: status.Manifest, Plan: status.Plan}
			ctx := f.ctx
			if phase == sharegrant.SignalPreview {
				// The ordinary run owner uses this exact same deadline; exercise
				// the parent/child timer race as well as the server expiry frame.
				var cancel context.CancelFunc
				ctx, cancel = context.WithDeadline(ctx, expiry)
				defer cancel()
			}
			var observed time.Time
			f.a.temporaryShares.beforeSignalWait = func(_ context.Context, wait context.Context) { observed, _ = wait.Deadline() }
			peer, err := f.a.connectSharePeer(ctx, scope, status, DirectConfig{AllowLoopback: true, CheckTimeout: time.Second, WaitTimeout: time.Minute}, func() error { return nil })
			if peer != nil || !observed.Equal(expiry) || protocol.ErrorCode(err) != protocol.ShareExpired || time.Now().Before(expiry) {
				t.Fatal("authenticated wait extended or misclassified the original grant expiry", observed, expiry, protocol.ErrorCode(err))
			}
		})
	}
}

func TestDiagnosticSharePeerWaitProjectsOnlyExactSafePhase(t *testing.T) {
	for _, test := range []struct{ state, phase, code, stage, action string }{
		{"preparing", "waiting_share_peer", "", "presence", "wait"},
		{"connecting", "waiting_share_peer", "", "presence", "wait"},
		{"recovering", "waiting_share_peer", "", "presence", "wait"},
		{"failed", "waiting_share_peer", string(protocol.SignalingTimeout), "presence", "details"},
		{"failed", "waiting_share_peer", string(protocol.AuthenticationFailed), "authentication", "open_devices"},
		{"failed", "waiting_share_peer", string(protocol.ShareExpired), "authorization", "details"},
		{"failed", "waiting_share_peer", string(protocol.ShareTerminal), "authorization", "details"},
		{"failed", "waiting_share_peer", string(protocol.ShareRejected), "authorization", "details"},
		{"failed", "waiting_share_peer", string(protocol.ShareConflict), "authorization", "details"},
		{"failed", "waiting_share_peer:PRIVATE_PHASE", string(protocol.SignalingTimeout), "signaling", "open_network"},
		{"completed", "waiting_share_peer", string(protocol.SignalingTimeout), "completed", "none"},
	} {
		got := taskDiagnosticGuidance(TaskSnapshot{State: test.state, Phase: test.phase, ErrorCode: test.code, ErrorMessage: "PRIVATE_ERROR"})
		if got.Stage != test.stage || got.Action != test.action || strings.Contains(got.Reason+got.NextStep, "PRIVATE") {
			t.Fatal("waiting phase acquired unsafe or misleading diagnostic guidance", got)
		}
		if test.code != "" && test.state != "completed" && got.Code != test.code {
			t.Fatal("stable security or timeout code was lost", got.Code, test.code)
		}
	}
}

func TestTemporaryShareExtendedPeerWaitKeepsOriginalTaskAndCancellation(t *testing.T) {
	f := newTemporaryShareFixture(t)
	_, approved := createApprovedTemporaryShare(t, f)
	entered := make(chan context.Context, 1)
	f.b.temporaryShares.beforeSignalWait = func(check context.Context, _ context.Context) { entered <- check }
	task, err := f.b.StartTemporaryShare(t.Context(), approved.ID, DirectConfig{AllowLoopback: true, CheckTimeout: 500 * time.Millisecond, WaitTimeout: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	var initial context.Context
	select {
	case initial = <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("original task did not reach authenticated peer wait")
	}
	requestCount := f.requests.Load()
	<-initial.Done()
	current, _ := f.b.Task(task.ID)
	owner := onlyTemporary(t, f.b)
	if current.State != "preparing" || current.Phase != "waiting_share_peer" || current.ErrorCode != "" || owner.State != "active" || current.Diagnostic == nil || current.Diagnostic.Guidance.Action != "wait" {
		t.Fatal("human wait prematurely ended the original task", current.State, current.Phase, owner.State)
	}
	if len(f.b.Tasks()) != 1 || owner.ClaimID != approved.ClaimID || owner.Generation != approved.Generation || owner.TaskID != approved.TaskID || f.requests.Load() != requestCount || current.VerifiedBytes != 0 || current.CommittedBytes != 0 {
		t.Fatal("waiting created another claim/task or progressed file bytes")
	}
	if _, err := f.b.CancelTemporaryShare(t.Context(), approved.ID); err != nil {
		t.Fatal(err)
	}
	waitFor(t, 3*time.Second, func() bool { got, _ := f.b.Task(task.ID); return got.State == "cancelled" }, "explicit cancellation did not stop the extended peer wait")
	if owner := onlyTemporary(t, f.b); owner.State != "stopped" || owner.ErrorCode != protocol.Cancelled || owner.ClaimID != approved.ClaimID || len(f.b.Tasks()) != 1 {
		t.Fatal("cancellation reopened or replaced the original share")
	}
}
