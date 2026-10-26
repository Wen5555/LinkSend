package server

import (
	"context"
	"encoding/hex"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wen5555/LinkSend/internal/identity"
	"github.com/Wen5555/LinkSend/internal/protocol"
	"github.com/Wen5555/LinkSend/internal/sharegrant"
	"github.com/coder/websocket"
)

func TestShareSignalReplayStaleICEAndCandidateLimit(t *testing.T) {
	f := newShareSignalFixture(t, sharegrant.SignalPreview)
	a, b := f.connect(t, f.a), f.connect(t, f.b)
	session := protocol.RandomID()
	request := f.envelope(t, f.a, "connect_request", session, map[string]string{"ufrag": "fixture", "password": "fixture"})
	writeShareFrame(t, a, request)
	readShareFrame(t, b, "signal")
	before := f.s.Counters().ForwardedMessages
	writeShareFrame(t, a, request)
	if readShareFrame(t, a, "error").Code != protocol.Replay || f.s.Counters().ForwardedMessages != before {
		t.Fatal("replayed frame admitted")
	}
	for _, wrong := range []string{"session", "generation"} {
		candidate := f.envelope(t, f.a, "candidate", session, protocol.Candidate{Candidate: "1 1 udp 2130706431 10.0.0.1 3478 typ host"})
		if wrong == "session" {
			candidate.Message.SessionID = protocol.RandomID()
		} else {
			candidate.Message.Generation = 2
		}
		candidate.Message.Signature = f.a.Sign(candidate.Message.SigningBytes())
		candidate.Signature = f.a.Sign(candidate.SigningBytes())
		writeShareFrame(t, a, candidate)
		if readShareFrame(t, a, "error").Code != protocol.ShareRejected || f.s.Counters().ForwardedMessages != before {
			t.Fatal("stale ICE binding admitted")
		}
	}
	for range protocol.MaxCandidates {
		writeShareFrame(t, a, f.envelope(t, f.a, "candidate", session, protocol.Candidate{Candidate: "1 1 udp 2130706431 10.0.0.1 3478 typ host"}))
		readShareFrame(t, b, "signal")
	}
	before = f.s.Counters().ForwardedMessages
	writeShareFrame(t, a, f.envelope(t, f.a, "candidate", session, protocol.Candidate{Candidate: "1 1 udp 2130706431 10.0.0.1 3478 typ host"}))
	if readShareFrame(t, a, "error").Code != protocol.RateLimited || f.s.Counters().ForwardedMessages != before {
		t.Fatal("candidate bound exceeded")
	}
	writeShareFrame(t, a, f.envelope(t, f.a, "end_of_candidates", session, map[string]any{}))
	readShareFrame(t, b, "signal")
	writeShareFrame(t, b, f.envelope(t, f.b, "end_of_candidates", session, map[string]any{}))
	readShareFrame(t, a, "signal")
	fresh := protocol.RandomID()
	writeShareFrame(t, a, f.envelope(t, f.a, "connect_request", fresh, map[string]string{"ufrag": "fresh", "password": "fresh"}))
	readShareFrame(t, b, "signal")
	writeShareFrame(t, a, f.envelope(t, f.a, "candidate", session, protocol.Candidate{Candidate: "1 1 udp 2130706431 10.0.0.1 3478 typ host"}))
	if readShareFrame(t, a, "error").Code != protocol.ShareRejected {
		t.Fatal("old candidate joined replacement negotiation")
	}
}

func TestShareSignalTransferCannotReusePreviewICEIdentifier(t *testing.T) {
	f := newShareSignalFixture(t, sharegrant.SignalPreview)
	a, b := f.connect(t, f.a), f.connect(t, f.b)
	previewSession := protocol.RandomID()
	writeShareFrame(t, a, f.envelope(t, f.a, "connect_request", previewSession, map[string]string{"ufrag": "preview", "password": "fixture"}))
	readShareFrame(t, b, "signal")
	bound := shareCall(t, f.s, f.b, "bind", ShareRequest{ID: f.status.ID, TaskID: f.status.TaskID, Generation: f.status.Generation, Manifest: f.status.Manifest, Plan: strings.Repeat("b", 64)})
	requireShareClosed(t, a, protocol.ShareConflict)
	requireShareClosed(t, b, protocol.ShareConflict)
	f.scope.Phase, f.scope.Plan, f.status = sharegrant.SignalTransfer, bound.Plan, bound
	a, b = f.connect(t, f.a), f.connect(t, f.b)
	// Both the outer proof and the original Envelope are freshly signed for
	// transfer. The old ICE identifier must remain reserved across phases.
	reused := f.envelope(t, f.a, "connect_request", previewSession, map[string]string{"ufrag": "transfer", "password": "fixture"})
	f.s.shareSignals.gate.Lock()
	from := f.s.shareSignals.owners[f.scope].peers[sharegrant.Key(f.a.PublicKey())]
	f.s.shareSignals.gate.Unlock()
	before := f.s.Counters().ForwardedMessages
	if err := f.s.forwardShareSignal(from, reused); protocol.ErrorCode(err) != protocol.Replay {
		t.Fatalf("fresh transfer proof reused the preview ICE identifier: %v", err)
	}
	if f.s.Counters().ForwardedMessages != before {
		t.Fatal("cross-phase replay was forwarded")
	}
	fresh := protocol.RandomID()
	writeShareFrame(t, a, f.envelope(t, f.a, "connect_request", fresh, map[string]string{"ufrag": "transfer", "password": "fixture"}))
	if got := readShareFrame(t, b, "signal"); got.Message.SessionID != fresh {
		t.Fatal("replay refusal damaged the original bound task")
	}
	f.requireNoPermanentActors(t)
}

func prepareShareSignalRecord(t *testing.T, s *Server, a, b *identity.Identity) sharegrant.SignalScope {
	t.Helper()
	s.shareSignals.gate.Lock()
	defer s.shareSignals.gate.Unlock()
	now := time.Now()
	code, digest, err := sharegrant.GenerateCode()
	if err != nil {
		t.Fatal(err)
	}
	r, err := sharegrant.New(protocol.RandomID(), sharegrant.Key(a.PublicKey()), sharegrant.Digest{1}, digest, now)
	if err != nil {
		t.Fatal(err)
	}
	if r, err = s.store.CreateShare(t.Context(), r, now); err != nil {
		t.Fatal(err)
	}
	if r, err = s.store.ClaimShare(t.Context(), sharegrant.Key(b.PublicKey()), code, protocol.RandomID(), now); err != nil {
		t.Fatal(err)
	}
	if r, err = s.store.ApproveShare(t.Context(), r.ID, r.Sender, r.ClaimID, protocol.RandomID(), protocol.RandomID(), r.Generation, now); err != nil {
		t.Fatal(err)
	}
	return sharegrant.SignalScope{GrantID: r.ID, ClaimID: r.ClaimID, TaskID: r.TaskID, GrantGeneration: r.Generation, Phase: sharegrant.SignalPreview, Manifest: hex.EncodeToString(r.Manifest[:])}
}

func TestShareSignalScopeActorAndConnectionSlotsAreBounded(t *testing.T) {
	t.Run("actor", func(t *testing.T) {
		f := newShareSignalFixture(t, sharegrant.SignalPreview)
		for range maxShareSignalActorConnections {
			scope := prepareShareSignalRecord(t, f.s, shareIdentity(t), f.b)
			c, err := f.open(t.Context(), f.b, scope)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { c.CloseNow() })
		}
		scope := prepareShareSignalRecord(t, f.s, shareIdentity(t), f.b)
		ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
		defer cancel()
		if c, err := f.open(ctx, f.b, scope); err == nil {
			c.CloseNow()
			t.Fatal("identity opened more than eight share connections")
		}
		f.s.shareSignals.gate.Lock()
		count := len(f.s.shareSignals.owners)
		f.s.shareSignals.gate.Unlock()
		if count != 8 {
			t.Fatalf("actor refusal leaked a scope: %d", count)
		}
	})
	t.Run("scope", func(t *testing.T) {
		f := newShareSignalFixture(t, sharegrant.SignalPreview)
		for range maxShareSignalScopes {
			a, b := shareIdentity(t), shareIdentity(t)
			scope := prepareShareSignalRecord(t, f.s, a, b)
			c, err := f.open(t.Context(), a, scope)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { c.CloseNow() })
		}
		a, b := shareIdentity(t), shareIdentity(t)
		scope := prepareShareSignalRecord(t, f.s, a, b)
		ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
		defer cancel()
		if c, err := f.open(ctx, a, scope); err == nil {
			c.CloseNow()
			t.Fatal("share scope limit exceeded")
		}
		f.s.shareSignals.gate.Lock()
		count := len(f.s.shareSignals.owners)
		f.s.shareSignals.gate.Unlock()
		if count != 32 {
			t.Fatalf("scope refusal changed owner bound: %d", count)
		}
	})
	t.Run("global slots", func(t *testing.T) {
		f := newShareSignalFixture(t, sharegrant.SignalPreview)
		// Occupy every explicit admission slot before a signed upgrade. No
		// socket or fake grant is registered to manufacture the refusal.
		for range maxShareSignalConnections {
			f.s.shareSignals.slots <- struct{}{}
		}
		defer func() {
			for range maxShareSignalConnections {
				<-f.s.shareSignals.slots
			}
		}()
		uri := f.scope.RequestURI()
		ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
		defer cancel()
		c, response, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(f.h.URL, "http")+uri, &websocket.DialOptions{HTTPHeader: signalHeaders(f.a, f.s.shareEpoch, uri), Subprotocols: []string{sharegrant.SignalSubprotocol}})
		if response != nil && response.Body != nil {
			response.Body.Close()
		}
		if err == nil {
			c.CloseNow()
			t.Fatal("global share slot overflow admitted")
		}
		if response == nil || response.StatusCode != 429 {
			t.Fatal("slot exhaustion did not return bounded rate limit")
		}
	})
}

func TestShareSignalQueuedMetadataAndWriterOwnershipAreBounded(t *testing.T) {
	f := newShareSignalFixture(t, sharegrant.SignalPreview)
	entered, release := make(chan struct{}), make(chan struct{})
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	var once sync.Once
	f.s.shareSignals.hooks.beforeWrite = func(ctx context.Context, frame sharegrant.SignalFrame) {
		if frame.Message != nil && frame.Message.Type == "status" {
			once.Do(func() { close(entered) })
			select {
			case <-release:
			case <-ctx.Done():
			}
		}
	}
	a, b := f.connect(t, f.a), f.connect(t, f.b)
	session := protocol.RandomID()
	writeShareFrame(t, a, f.envelope(t, f.a, "connect_request", session, map[string]string{"ufrag": "fixture", "password": "fixture"}))
	readShareFrame(t, b, "signal")
	status := map[string]string{"state": "failed", "code": "CHECK_TIMEOUT"}
	writeShareFrame(t, a, f.envelope(t, f.a, "status", session, status))
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("writer was not held")
	}
	f.s.shareSignals.gate.Lock()
	owner := f.s.shareSignals.owners[f.scope]
	from, to := owner.peers[sharegrant.Key(f.a.PublicKey())], owner.peers[sharegrant.Key(f.b.PublicKey())]
	f.s.shareSignals.gate.Unlock()
	for range cap(to.send) {
		if err := f.s.forwardShareSignal(from, f.envelope(t, f.a, "status", session, status)); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.s.forwardShareSignal(from, f.envelope(t, f.a, "status", session, status)); protocol.ErrorCode(err) != protocol.RateLimited {
		t.Fatalf("full recipient queue not bounded: %v", err)
	}
	if len(to.send) != cap(to.send) {
		t.Fatal("queue size changed unexpectedly")
	}
	// A failing recipient releases only its owned actor connection; the other
	// actor and scope remain eligible for an explicit fresh reconnect.
	b.CloseNow()
	select {
	case <-to.done:
	case <-time.After(3 * time.Second):
		t.Fatal("failed writer/reader did not terminate")
	}
	f.s.shareSignals.gate.Lock()
	still := f.s.shareSignals.owners[f.scope]
	valid := still == owner && owner.peers[to.actor] == nil && owner.peers[from.actor] == from
	f.s.shareSignals.gate.Unlock()
	if !valid {
		t.Fatal("failed peer cleanup damaged unrelated owner")
	}
}

func TestShareSignalRestartRejectsCapturedProofButPreservesGrant(t *testing.T) {
	f := newShareSignalFixture(t, sharegrant.SignalTransfer)
	uri := f.scope.RequestURI()
	captured := signalHeaders(f.a, f.s.shareEpoch, uri)
	original, err := f.s.store.ReadShare(t.Context(), f.scope.GrantID, sharegrant.Key(f.a.PublicKey()))
	if err != nil {
		t.Fatal(err)
	}
	oldEpoch := f.s.shareEpoch
	if err = f.s.Close(); err != nil {
		t.Fatal(err)
	}
	f.h.Close()
	f.s = shareServer(t, f.path, true)
	f.h = httptest.NewServer(f.s.Handler())
	t.Cleanup(f.h.Close)
	if f.s.shareEpoch == oldEpoch {
		t.Fatal("server run challenge reused")
	}
	for _, rewrite := range []bool{false, true} {
		headers := captured.Clone()
		if rewrite {
			headers.Set("X-LinkSend-Share-Epoch", f.s.shareEpoch)
		}
		ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
		c, response, dialErr := websocket.Dial(ctx, "ws"+strings.TrimPrefix(f.h.URL, "http")+uri, &websocket.DialOptions{HTTPHeader: headers, Subprotocols: []string{sharegrant.SignalSubprotocol}})
		cancel()
		if response != nil && response.Body != nil {
			response.Body.Close()
		}
		if dialErr == nil {
			c.CloseNow()
			t.Fatal("pre-restart proof accepted")
		}
	}
	a, b := f.connect(t, f.a), f.connect(t, f.b)
	current, err := f.s.store.ReadShare(t.Context(), f.scope.GrantID, sharegrant.Key(f.a.PublicKey()))
	if err != nil || current != original {
		t.Fatal("reconnect changed persistent task or deadlines")
	}
	request := f.envelope(t, f.a, "connect_request", protocol.RandomID(), map[string]string{"ufrag": "fresh", "password": "fixture"})
	writeShareFrame(t, a, request)
	if readShareFrame(t, b, "signal").Message.SessionID != request.Message.SessionID {
		t.Fatal("fresh original task could not coordinate")
	}
	f.requireNoPermanentActors(t)
}
