package server

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wen5555/LinkSend/internal/identity"
	"github.com/Wen5555/LinkSend/internal/protocol"
	"github.com/Wen5555/LinkSend/internal/sharegrant"
	"github.com/Wen5555/LinkSend/internal/signaling"
	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

type shareSignalFixture struct {
	s      *Server
	h      *httptest.Server
	a, b   *identity.Identity
	scope  sharegrant.SignalScope
	status ShareStatus
	path   string
}

func newShareSignalFixture(t *testing.T, phase sharegrant.SignalPhase) *shareSignalFixture {
	t.Helper()
	path := filepath.Join(t.TempDir(), "control.sqlite")
	s := shareServer(t, path, true)
	h := httptest.NewServer(s.Handler())
	t.Cleanup(h.Close)
	a, b := shareIdentity(t), shareIdentity(t)
	_, code := createShareHTTP(t, s, a)
	claim := shareCall(t, s, b, "claim", ShareRequest{Code: code, ClaimID: protocol.RandomID()})
	status := shareCall(t, s, a, "approve", ShareRequest{ID: claim.ID, ClaimID: claim.ClaimID, ApprovalID: protocol.RandomID(), TaskID: protocol.RandomID(), Generation: claim.Generation, Manifest: claim.Manifest, ReceiverKey: base64.RawStdEncoding.EncodeToString(b.PublicKey())})
	if phase == sharegrant.SignalTransfer {
		status = shareCall(t, s, b, "bind", ShareRequest{ID: status.ID, TaskID: status.TaskID, Generation: status.Generation, Manifest: status.Manifest, Plan: strings.Repeat("b", 64)})
	}
	scope := sharegrant.SignalScope{GrantID: status.ID, ClaimID: status.ClaimID, TaskID: status.TaskID, GrantGeneration: status.Generation, Phase: phase, Manifest: status.Manifest, Plan: status.Plan}
	return &shareSignalFixture{s: s, h: h, a: a, b: b, scope: scope, status: status, path: path}
}

func signalHeaders(id *identity.Identity, epoch, uri string) http.Header {
	nonce, at := protocol.RandomID(), time.Now().Unix()
	headers := http.Header{}
	headers.Set("X-LinkSend-Membership", "2")
	headers.Set("X-LinkSend-Share-Grants", "1")
	headers.Set("X-LinkSend-Device", id.ID())
	headers.Set("X-LinkSend-Public-Key", base64.RawStdEncoding.EncodeToString(id.PublicKey()))
	headers.Set("X-LinkSend-Share-Epoch", epoch)
	headers.Set("X-LinkSend-Nonce", nonce)
	headers.Set("X-LinkSend-Time", strconv.FormatInt(at, 10))
	headers.Set("X-LinkSend-Signature", base64.RawStdEncoding.EncodeToString(id.Sign(protocol.AuthBytes("GET", uri, id.ID(), epoch+":"+nonce, at, nil))))
	return headers
}

func (f *shareSignalFixture) open(ctx context.Context, id *identity.Identity, scope sharegrant.SignalScope) (*websocket.Conn, error) {
	uri := scope.RequestURI()
	c, response, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(f.h.URL, "http")+uri, &websocket.DialOptions{HTTPHeader: signalHeaders(id, f.s.shareEpoch, uri), Subprotocols: []string{sharegrant.SignalSubprotocol}})
	if response != nil && response.Body != nil {
		response.Body.Close()
	}
	if err != nil {
		return nil, err
	}
	c.SetReadLimit(protocol.MaxMessageBytes)
	var ready sharegrant.SignalFrame
	if err = wsjson.Read(ctx, c, &ready); err != nil {
		c.CloseNow()
		return nil, err
	}
	if ready.Type != "ready" || ready.Version != 1 || ready.Scope != scope || ready.ActorID != id.ID() || ready.Epoch != f.s.shareEpoch || ready.Message != nil || len(ready.Signature) != 0 || ready.Code != "" || ready.Nonce != "" || ready.IssuedAt != 0 || ready.ExpiresAt != 0 {
		c.CloseNow()
		return nil, errors.New("invalid share ready")
	}
	return c, nil
}

func (f *shareSignalFixture) connect(t *testing.T, id *identity.Identity) *websocket.Conn {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	c, err := f.open(ctx, id, f.scope)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.CloseNow() })
	return c
}

func (f *shareSignalFixture) envelope(t *testing.T, id *identity.Identity, kind, session string, payload any) sharegrant.SignalFrame {
	t.Helper()
	peer := f.a
	if id.ID() == f.a.ID() {
		peer = f.b
	}
	e, err := protocol.NewEnvelope(kind, id.ID(), peer.ID(), session, 1, payload)
	if err != nil {
		t.Fatal(err)
	}
	e.Signature = id.Sign(e.SigningBytes())
	frame, err := sharegrant.NewSignalFrame(f.scope, f.s.shareEpoch, id.ID(), "signal", &e, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	frame.Signature = id.Sign(frame.SigningBytes())
	return frame
}

func writeShareFrame(t *testing.T, c *websocket.Conn, frame sharegrant.SignalFrame) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	if err := wsjson.Write(ctx, c, frame); err != nil {
		t.Fatal(err)
	}
}

func readShareFrame(t *testing.T, c *websocket.Conn, kind string) sharegrant.SignalFrame {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	for {
		var frame sharegrant.SignalFrame
		if err := wsjson.Read(ctx, c, &frame); err != nil {
			t.Fatal(err)
		}
		if frame.Type == "ready" && kind != "ready" {
			continue
		}
		if frame.Type != kind {
			t.Fatalf("unexpected frame type %q", frame.Type)
		}
		return frame
	}
}

func requireShareClosed(t *testing.T, c *websocket.Conn, expected ...protocol.Code) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 4*time.Second)
	defer cancel()
	for {
		var frame sharegrant.SignalFrame
		err := wsjson.Read(ctx, c, &frame)
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				t.Fatal("share connection remained open")
			}
			if len(expected) != 0 {
				var closed websocket.CloseError
				if !errors.As(err, &closed) || closed.Code != websocket.StatusPolicyViolation || closed.Reason != string(expected[0]) {
					t.Fatalf("missing safe terminal close reason: %v", err)
				}
			}
			return
		}
		if frame.Type == "signal" {
			t.Fatal("retired share delivered a signal")
		}
	}
}

func (f *shareSignalFixture) requireNoPermanentActors(t *testing.T) {
	t.Helper()
	for _, id := range []*identity.Identity{f.a, f.b} {
		if _, err := f.s.store.Device(t.Context(), id.ID()); err == nil {
			t.Fatal("temporary actor became a member")
		}
	}
	db, err := sql.Open("sqlite", f.path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Exec("PRAGMA query_only=ON"); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"devices", "groups"} {
		var count int
		if err = db.QueryRow("SELECT count(*) FROM " + table).Scan(&count); err != nil || count != 0 {
			t.Fatalf("permanent %s changed: count=%d err=%v", table, count, err)
		}
	}
}

func TestShareSignalUnenrolledPairHasScopedSignedCoordinationOnly(t *testing.T) {
	f := newShareSignalFixture(t, sharegrant.SignalPreview)
	a, b := f.connect(t, f.a), f.connect(t, f.b)
	session := protocol.RandomID()
	request := f.envelope(t, f.a, "connect_request", session, map[string]string{"ufrag": "a-fixture", "password": "fixture-only-password"})
	writeShareFrame(t, a, request)
	got := readShareFrame(t, b, "signal")
	if got.Message.MessageID != request.Message.MessageID || got.Verify(f.a.PublicKey(), f.scope, f.s.shareEpoch, time.Now(), false) != nil {
		t.Fatal("original signed request changed")
	}
	response := f.envelope(t, f.b, "connect_response", session, map[string]string{"ufrag": "b-fixture", "password": "fixture-only-password"})
	writeShareFrame(t, b, response)
	got = readShareFrame(t, a, "signal")
	if got.Verify(f.b.PublicKey(), f.scope, f.s.shareEpoch, time.Now(), false) != nil {
		t.Fatal("original responder proof changed")
	}
	heartbeat, _ := sharegrant.NewSignalFrame(f.scope, f.s.shareEpoch, f.a.ID(), "heartbeat", nil, time.Now())
	heartbeat.Signature = f.a.Sign(heartbeat.SigningBytes())
	writeShareFrame(t, a, heartbeat)
	if !readShareFrame(t, a, "ready").PeerReady {
		t.Fatal("peer readiness lost")
	}
	f.requireNoPermanentActors(t)
	client, err := signaling.New(signaling.Config{ServerURL: f.h.URL, Identity: f.a, AllowInsecureLoopback: true})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	if ordinary, err := client.Connect(ctx); err == nil {
		ordinary.Close()
		t.Fatal("temporary actor entered ordinary membership WS")
	}
}

func TestShareSignalHandshakeRejectsForeignScopeKeyEpochAndDisabledService(t *testing.T) {
	for _, name := range []string{"task", "claim", "generation", "phase", "third identity", "epoch", "public key", "signature", "query tamper", "unknown query"} {
		t.Run(name, func(t *testing.T) {
			f := newShareSignalFixture(t, sharegrant.SignalPreview)
			scope := f.scope
			id := f.a
			switch name {
			case "task":
				scope.TaskID = protocol.RandomID()
			case "claim":
				scope.ClaimID = protocol.RandomID()
			case "generation":
				scope.GrantGeneration++
			case "phase":
				scope.Phase = sharegrant.SignalTransfer
				scope.Plan = strings.Repeat("b", 64)
			case "third identity":
				id = shareIdentity(t)
			}
			uri := scope.RequestURI()
			headers := signalHeaders(id, f.s.shareEpoch, uri)
			switch name {
			case "epoch":
				headers.Set("X-LinkSend-Share-Epoch", protocol.RandomID())
			case "public key":
				headers.Set("X-LinkSend-Public-Key", base64.RawStdEncoding.EncodeToString(f.b.PublicKey()))
			case "signature":
				headers.Set("X-LinkSend-Signature", base64.RawStdEncoding.EncodeToString(make([]byte, 64)))
			case "query tamper":
				uri = strings.Replace(uri, scope.TaskID, protocol.RandomID(), 1)
			case "unknown query":
				uri += "&file_path=private"
			}
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			c, response, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(f.h.URL, "http")+uri, &websocket.DialOptions{HTTPHeader: headers, Subprotocols: []string{sharegrant.SignalSubprotocol}})
			if response != nil && response.Body != nil {
				response.Body.Close()
			}
			if err == nil {
				c.CloseNow()
				t.Fatal("foreign handshake accepted")
			}
			f.requireNoPermanentActors(t)
		})
	}
	t.Run("disabled", func(t *testing.T) {
		s := shareServer(t, filepath.Join(t.TempDir(), "off.sqlite"), false)
		h := httptest.NewServer(s.Handler())
		defer h.Close()
		req := httptest.NewRequest("GET", "/v1/shares/"+protocol.RandomID()+"/ws", nil)
		req.RemoteAddr = "127.0.0.1:1234"
		req.Header.Set("X-LinkSend-Membership", "2")
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, req)
		if w.Code != 426 {
			t.Fatalf("disabled status %d", w.Code)
		}
	})
}

func TestShareSignalWrongProofPayloadAndScopeCloseBeforeForward(t *testing.T) {
	for _, name := range []string{"outer signature", "inner signature", "epoch", "scope", "recipient", "file payload", "clipboard", "oversize"} {
		t.Run(name, func(t *testing.T) {
			f := newShareSignalFixture(t, sharegrant.SignalPreview)
			a := f.connect(t, f.a)
			_ = f.connect(t, f.b)
			frame := f.envelope(t, f.a, "connect_request", protocol.RandomID(), map[string]string{"ufrag": "fixture", "password": "fixture-password"})
			switch name {
			case "outer signature":
				frame.Signature = f.b.Sign(frame.SigningBytes())
			case "inner signature":
				frame.Message.Signature = f.b.Sign(frame.Message.SigningBytes())
				frame.Signature = f.a.Sign(frame.SigningBytes())
			case "epoch":
				frame.Epoch = protocol.RandomID()
				frame.Signature = f.a.Sign(frame.SigningBytes())
			case "scope":
				frame.Scope.GrantGeneration++
				frame.Signature = f.a.Sign(frame.SigningBytes())
			case "recipient":
				frame.Message.Recipient = shareIdentity(t).ID()
				frame.Message.Signature = f.a.Sign(frame.Message.SigningBytes())
				frame.Signature = f.a.Sign(frame.SigningBytes())
			case "file payload":
				frame.Message.Payload = json.RawMessage(`{"ufrag":"fixture","password":"fixture","file_bytes":"private"}`)
				frame.Message.Signature = f.a.Sign(frame.Message.SigningBytes())
				frame.Signature = f.a.Sign(frame.SigningBytes())
			case "clipboard":
				frame.Message.Payload = json.RawMessage(`{"ufrag":"fixture","password":"fixture","clipboard_sync":true}`)
				frame.Message.Signature = f.a.Sign(frame.Message.SigningBytes())
				frame.Signature = f.a.Sign(frame.SigningBytes())
			case "oversize":
				frame.Message.Payload = json.RawMessage(`{"ufrag":"` + strings.Repeat("a", protocol.MaxMessageBytes) + `","password":"fixture"}`)
				frame.Message.Signature = f.a.Sign(frame.Message.SigningBytes())
				frame.Signature = f.a.Sign(frame.SigningBytes())
			}
			before := f.s.Counters().ForwardedMessages
			writeShareFrame(t, a, frame)
			if name == "recipient" {
				if got := readShareFrame(t, a, "error"); got.Code != protocol.ShareRejected {
					t.Fatal("recipient mismatch was not rejected")
				}
			} else {
				requireShareClosed(t, a)
			}
			if f.s.Counters().ForwardedMessages != before {
				t.Fatal("rejected frame forwarded")
			}
		})
	}
}

func TestShareSignalBindAndRevokeRetireOnlyTheirScope(t *testing.T) {
	f := newShareSignalFixture(t, sharegrant.SignalPreview)
	a, b := f.connect(t, f.a), f.connect(t, f.b)
	bound := shareCall(t, f.s, f.b, "bind", ShareRequest{ID: f.status.ID, TaskID: f.status.TaskID, Generation: f.status.Generation, Manifest: f.status.Manifest, Plan: strings.Repeat("b", 64)})
	requireShareClosed(t, a, protocol.ShareConflict)
	requireShareClosed(t, b, protocol.ShareConflict)
	if c, err := f.open(t.Context(), f.a, f.scope); err == nil {
		c.CloseNow()
		t.Fatal("retired preview reopened")
	}
	f.status = bound
	f.scope.Phase = sharegrant.SignalTransfer
	f.scope.Plan = bound.Plan
	a, b = f.connect(t, f.a), f.connect(t, f.b)
	ordinaryID := shareIdentity(t)
	ordinaryClient, err := signaling.New(signaling.Config{ServerURL: f.h.URL, Identity: ordinaryID, AllowInsecureLoopback: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = ordinaryClient.Initialize(t.Context(), "ordinary independent actor"); err != nil {
		t.Fatal(err)
	}
	ordinary, err := ordinaryClient.Connect(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer ordinary.Close()
	f.s.mu.Lock()
	normalPeer := f.s.clients[ordinaryID.ID()]
	f.s.mu.Unlock()
	shareCall(t, f.s, f.a, "revoke", ShareRequest{ID: f.status.ID})
	requireShareClosed(t, a, protocol.ShareTerminal)
	requireShareClosed(t, b, protocol.ShareTerminal)
	select {
	case <-normalPeer.done:
		t.Fatal("share revoke closed unrelated ordinary WS")
	default:
	}
	if err = ordinary.SendHeartbeat(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestShareSignalRegistrationRechecksRevocationAfterAuthentication(t *testing.T) {
	f := newShareSignalFixture(t, sharegrant.SignalPreview)
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	f.s.shareSignals.hooks.afterAuthenticate = func(ctx context.Context, key sharegrant.Key) {
		if key == sharegrant.Key(f.a.PublicKey()) {
			once.Do(func() { close(entered) })
			select {
			case <-release:
			case <-ctx.Done():
			}
		}
	}
	result := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		c, err := f.open(ctx, f.a, f.scope)
		if c != nil {
			c.CloseNow()
		}
		result <- err
	}()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("authentication barrier not reached")
	}
	shareCall(t, f.s, f.a, "revoke", ShareRequest{ID: f.status.ID})
	close(release)
	if err := <-result; err == nil {
		t.Fatal("registration used pre-revoke authorization")
	}
	f.s.shareSignals.gate.Lock()
	count := len(f.s.shareSignals.owners)
	f.s.shareSignals.gate.Unlock()
	if count != 0 {
		t.Fatal("revoked scope registered")
	}
}

func TestShareSignalRevocationDropsDequeuedSignalBeforeWrite(t *testing.T) {
	f := newShareSignalFixture(t, sharegrant.SignalPreview)
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	f.s.shareSignals.hooks.beforeWrite = func(ctx context.Context, frame sharegrant.SignalFrame) {
		if frame.Message != nil && frame.Message.Type == "candidate" {
			once.Do(func() { close(entered) })
			select {
			case <-release:
			case <-ctx.Done():
			}
		}
	}
	a, b := f.connect(t, f.a), f.connect(t, f.b)
	session := protocol.RandomID()
	writeShareFrame(t, a, f.envelope(t, f.a, "connect_request", session, map[string]string{"ufrag": "a", "password": "fixture"}))
	readShareFrame(t, b, "signal")
	writeShareFrame(t, a, f.envelope(t, f.a, "candidate", session, protocol.Candidate{Candidate: "1 1 udp 2130706431 10.0.0.1 3478 typ host"}))
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("writer barrier not reached")
	}
	shareCall(t, f.s, f.a, "revoke", ShareRequest{ID: f.status.ID})
	close(release)
	requireShareClosed(t, b)
	requireShareClosed(t, a)
}

func TestShareSignalOldCleanupCannotRemoveReauthenticatedConnection(t *testing.T) {
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
	f.s.shareSignals.hooks.beforeCleanup = func(_ context.Context, key sharegrant.Key) {
		if key == sharegrant.Key(f.a.PublicKey()) {
			once.Do(func() { close(entered); <-release })
		}
	}
	a, b := f.connect(t, f.a), f.connect(t, f.b)
	f.s.shareSignals.gate.Lock()
	old := f.s.shareSignals.owners[f.scope].peers[sharegrant.Key(f.a.PublicKey())]
	f.s.shareSignals.gate.Unlock()
	replacement := f.connect(t, f.a)
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		close(release)
		t.Fatal("old cleanup barrier not reached")
	}
	close(release)
	select {
	case <-old.done:
	case <-time.After(3 * time.Second):
		t.Fatal("old cleanup did not finish")
	}
	f.s.shareSignals.gate.Lock()
	current := f.s.shareSignals.owners[f.scope].peers[sharegrant.Key(f.a.PublicKey())]
	f.s.shareSignals.gate.Unlock()
	if current == nil || current == old {
		t.Fatal("old actor remained current")
	}
	session := protocol.RandomID()
	writeShareFrame(t, replacement, f.envelope(t, f.a, "connect_request", session, map[string]string{"ufrag": "fixture", "password": "fixture"}))
	got := readShareFrame(t, b, "signal")
	if got.Message.SessionID != session {
		t.Fatal("old cleanup removed replacement negotiation")
	}
	requireShareClosed(t, a)
}

func TestShareSignalActualDeadlineTimerAndCloseRegistrationRace(t *testing.T) {
	t.Run("timer", func(t *testing.T) {
		f := newShareSignalFixture(t, sharegrant.SignalPreview)
		// A valid offer created in the past has its original absolute deadline
		// about two seconds away. No test clock or minute reaper drives closure.
		created := time.Now().Add(-sharegrant.OfferTTL + 2*time.Second)
		code, digest, _ := sharegrant.GenerateCode()
		record, err := sharegrant.New(protocol.RandomID(), sharegrant.Key(f.a.PublicKey()), sharegrant.Digest{9}, digest, created)
		if err != nil {
			t.Fatal(err)
		}
		if record, err = f.s.store.CreateShare(t.Context(), record, created); err != nil {
			t.Fatal(err)
		}
		if record, err = f.s.store.ClaimShare(t.Context(), sharegrant.Key(f.b.PublicKey()), code, protocol.RandomID(), created.Add(time.Second)); err != nil {
			t.Fatal(err)
		}
		if record, err = f.s.store.ApproveShare(t.Context(), record.ID, record.Sender, record.ClaimID, protocol.RandomID(), protocol.RandomID(), record.Generation, created.Add(time.Second)); err != nil {
			t.Fatal(err)
		}
		f.scope = sharegrant.SignalScope{GrantID: record.ID, ClaimID: record.ClaimID, TaskID: record.TaskID, GrantGeneration: record.Generation, Phase: sharegrant.SignalPreview, Manifest: hex.EncodeToString(record.Manifest[:])}
		a, b := f.connect(t, f.a), f.connect(t, f.b)
		requireShareClosed(t, a, protocol.ShareExpired)
		requireShareClosed(t, b, protocol.ShareExpired)
	})
	t.Run("Close versus registration", func(t *testing.T) {
		f := newShareSignalFixture(t, sharegrant.SignalPreview)
		entered := make(chan struct{})
		var once sync.Once
		f.s.shareSignals.hooks.afterAuthenticate = func(ctx context.Context, _ sharegrant.Key) { once.Do(func() { close(entered) }); <-ctx.Done() }
		result := make(chan error, 1)
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			c, err := f.open(ctx, f.a, f.scope)
			if c != nil {
				c.CloseNow()
			}
			result <- err
		}()
		select {
		case <-entered:
		case <-time.After(3 * time.Second):
			t.Fatal("register barrier missing")
		}
		closed := make(chan struct{})
		go func() { f.s.Close(); close(closed) }()
		select {
		case <-closed:
		case <-time.After(3 * time.Second):
			t.Fatal("Close did not join pending share registration")
		}
		if <-result == nil {
			t.Fatal("registered after server Close")
		}
	})
}

func TestShareSignalServerShutdownIsNotGrantTerminal(t *testing.T) {
	f := newShareSignalFixture(t, sharegrant.SignalTransfer)
	f.connect(t, f.a)
	f.connect(t, f.b)
	if err := f.s.Close(); err != nil {
		t.Fatal(err)
	}
	f.s.shareSignals.gate.Lock()
	_, err := f.s.authorizeShareSignalLocked(context.Background(), f.scope, sharegrant.Key(f.a.PublicKey()))
	f.s.shareSignals.gate.Unlock()
	if protocol.ErrorCode(err) != protocol.ShareUnavailable {
		t.Fatalf("server shutdown became a durable grant terminal: %v", err)
	}
	req := signedShare(t, f.s, f.a, "status", ShareRequest{ID: f.scope.GrantID})
	_, status, body := runShare(t, f.s, req)
	if status != 503 || !strings.Contains(body, string(protocol.ShareUnavailable)) || strings.Contains(body, string(protocol.ShareTerminal)) {
		t.Fatalf("shutdown share status=%d %s", status, body)
	}
}
