package app

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Wen5555/LinkSend/internal/protocol"
	"github.com/Wen5555/LinkSend/internal/signaling"
)

func TestShareDirectSignalWaitRetainsTypedContextCause(t *testing.T) {
	for _, cancelled := range []bool{false, true} {
		name := "deadline"
		want, cause := protocol.SignalingTimeout, context.DeadlineExceeded
		if cancelled {
			name, want, cause = "cancel", protocol.Cancelled, context.Canceled
		}
		t.Run(name, func(t *testing.T) {
			f := newShareDirectFixture(t)
			entered := make(chan struct{})
			f.a.temporaryShares.beforeSignalWait = func(context.Context, context.Context) { close(entered) }
			ctx, cancel := context.WithCancel(f.ctx)
			defer cancel()
			var phase string
			done := make(chan error, 1)
			go func() {
				peer, err := f.a.connectSharePeer(ctx, f.scope, f.approved, DirectConfig{AllowLoopback: true, CheckTimeout: time.Second, onPhase: func(value string) { phase = value }}, func() error { return nil })
				if peer != nil {
					_ = peer.Close()
				}
				done <- err
			}()
			select {
			case <-entered:
				// Real TLS/WSS has authenticated this SQLite-backed scope. The
				// second actor never connects; only the peer-presence wait remains.
			case err := <-done:
				t.Fatal("failed before the authenticated WaitPeer barrier", protocol.ErrorCode(err))
			case <-f.ctx.Done():
				t.Fatal("did not reach authenticated WaitPeer barrier")
			}
			if cancelled {
				cancel()
			}
			err := <-done
			if !errors.Is(err, cause) || protocol.ErrorCode(err) != want || phase != "waiting_share_peer" {
				t.Fatalf("typed cause lost after authenticated WaitPeer: code=%s cause=%t phase=%s", protocol.ErrorCode(err), errors.Is(err, cause), phase)
			}
		})
	}
}

func TestShareDirectSignalWaitKeepsAuthoritativeGrantTerminal(t *testing.T) {
	f := newShareDirectFixture(t)
	entered := make(chan struct{})
	f.a.temporaryShares.beforeSignalWait = func(context.Context, context.Context) { close(entered) }
	done := make(chan error, 1)
	go func() {
		peer, err := f.a.connectSharePeer(f.ctx, f.scope, f.approved, DirectConfig{AllowLoopback: true, CheckTimeout: 5 * time.Second}, func() error { return nil })
		if peer != nil {
			_ = peer.Close()
		}
		done <- err
	}()
	select {
	case <-entered:
	case <-f.ctx.Done():
		t.Fatal("did not reach authenticated WaitPeer barrier")
	}
	if _, err := f.ca.ShareControl(f.ctx, signaling.ShareRevoke, signaling.ShareRequest{ID: f.approved.ID}); err != nil {
		t.Fatal(err)
	}
	if err := <-done; protocol.ErrorCode(err) != protocol.ShareTerminal {
		t.Fatal("terminal grant became a transport timeout", protocol.ErrorCode(err))
	}
}

func TestShareDirectSignalingClassificationPreservesProtocolAuthority(t *testing.T) {
	for _, cancelled := range []bool{false, true} {
		ctx, cancel := context.WithCancel(t.Context())
		if cancelled {
			cancel()
		}
		for _, code := range []protocol.Code{protocol.ShareRejected, protocol.ShareExpired, protocol.ShareConflict, protocol.ShareTerminal, protocol.AuthenticationFailed, protocol.InvalidMessage, protocol.Replay} {
			cause := errors.New("PRIVATE_SERVER_DETAIL")
			original := protocol.Wrap(code, "private fixed fixture", cause)
			got := classifyShareSignalingError(ctx, original)
			if got != original || protocol.ErrorCode(got) != code || !errors.Is(got, cause) {
				t.Fatal("authoritative protocol result was reclassified", code)
			}
		}
		cancel()
	}
	for _, test := range []struct {
		err  error
		code protocol.Code
	}{{context.DeadlineExceeded, protocol.SignalingTimeout}, {context.Canceled, protocol.Cancelled}} {
		got := classifyShareSignalingError(t.Context(), test.err)
		if protocol.ErrorCode(got) != test.code || !errors.Is(got, test.err) {
			t.Fatal("bare typed signaling cause was lost", test.code)
		}
	}
	unknown := errors.New("private deadline exceeded; not a typed cause")
	if got := classifyShareSignalingError(t.Context(), unknown); got != unknown {
		t.Fatal("untyped text acquired a signaling classification")
	}
	if got := classifyTaskError(context.DeadlineExceeded); protocol.ErrorCode(got) != protocol.DirectFailed {
		t.Fatal("unrelated task deadline was globally guessed to be signaling")
	}
}

func TestShareDirectSignalConnectKeepsCurrentGrantRejection(t *testing.T) {
	f := newShareDirectFixture(t)
	if _, err := f.ca.ShareControl(f.ctx, signaling.ShareRevoke, signaling.ShareRequest{ID: f.approved.ID}); err != nil {
		t.Fatal(err)
	}
	baseline, original := f.ca.ConnectShare(f.ctx, f.scope, f.approved, signaling.ShareSignalOptions{AllowLoopbackCandidates: true})
	if baseline != nil {
		_ = baseline.Close()
	}
	var typed *protocol.Error
	if original == nil || !errors.As(original, &typed) {
		t.Fatal("revoked scope did not retain a typed client rejection")
	}
	entered := false
	f.a.temporaryShares.beforeSignalWait = func(context.Context, context.Context) { entered = true }
	peer, err := f.a.connectSharePeer(f.ctx, f.scope, f.approved, DirectConfig{AllowLoopback: true, CheckTimeout: time.Second}, func() error { return nil })
	if peer != nil || entered || protocol.ErrorCode(err) != typed.Code {
		t.Fatal("stale grant crossed signaling admission or changed authority", protocol.ErrorCode(err))
	}
}

func TestTemporaryShareMissingSignalPeerPersistsTypedFailure(t *testing.T) {
	f := newTemporaryShareFixture(t)
	created, _ := createApprovedTemporaryShare(t, f)
	entered := make(chan struct{})
	f.a.temporaryShares.beforeSignalWait = func(context.Context, context.Context) { close(entered) }
	task, err := f.a.StartTemporaryShare(t.Context(), created.Share.ID, DirectConfig{AllowLoopback: true, CheckTimeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("task did not reach authenticated WaitPeer barrier")
	}
	waitFor(t, 3*time.Second, func() bool { task, _ := f.a.Task(task.ID); return task.State == "failed" }, "isolated peer wait did not end")
	got, _ := f.a.Task(task.ID)
	owner := onlyTemporary(t, f.a)
	if got.ErrorCode != string(protocol.SignalingTimeout) || got.Phase != "waiting_share_peer" || owner.ErrorCode != protocol.SignalingTimeout || owner.State != "needs_attention" || got.VerifiedBytes != 0 || got.CommittedBytes != 0 || got.TLSVersion != 0 {
		t.Fatal("durable task/share lost the known signaling failure", got.ErrorCode, got.Phase, owner.ErrorCode)
	}
}

func TestDiagnosticLegacyDirectFailureUsesOnlyExactSignalingPhase(t *testing.T) {
	const secret = "PRIVATE_CAUSE_AND_SHARE_CODE_3ab4"
	for _, test := range []struct{ phase, code, state, wantStage, wantCode string }{
		{"signaling_connect", "DIRECT_FAILED", "failed", "signaling", "DIRECT_FAILED"},
		{"", "DIRECT_FAILED", "failed", "unknown", "DIRECT_FAILED"},
		{"ice_checking", "DIRECT_FAILED", "failed", "unknown", "DIRECT_FAILED"},
		{"signaling_connect:" + secret, "DIRECT_FAILED", "failed", "unknown", "DIRECT_FAILED"},
		{"signaling_connect", "DIRECT_FAILED:" + secret, "failed", "unknown", "UNKNOWN"},
		{"signaling_connect", "DIRECT_FAILED", "completed", "completed", "NONE"},
	} {
		snapshot := TaskSnapshot{State: test.state, Phase: test.phase, ErrorCode: test.code, ErrorMessage: secret, SourceSummary: secret, TemporaryShareID: secret}
		got := taskDiagnosticGuidance(snapshot)
		if got.Stage != test.wantStage || got.Code != test.wantCode {
			t.Errorf("exact phase guidance missing: phase=%q stage=%s code=%s", test.phase, got.Stage, got.Code)
		}
		if test.wantStage == "signaling" && got.Action != "open_network" {
			t.Error("known signaling stage has no actionable network entry")
		}
		body, err := json.Marshal(makeDiagnosticTask(snapshot, "task-1"))
		if err != nil || strings.Contains(string(body), secret) {
			t.Fatal("legacy fallback leaked private input")
		}
	}
}
