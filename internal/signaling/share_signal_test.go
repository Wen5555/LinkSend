package signaling_test

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wen5555/LinkSend/internal/identity"
	"github.com/Wen5555/LinkSend/internal/protocol"
	"github.com/Wen5555/LinkSend/internal/sharegrant"
	"github.com/Wen5555/LinkSend/internal/signaling"
	"github.com/coder/websocket"
)

var _ interface {
	SendEnvelope(context.Context, protocol.Envelope) error
	SendHeartbeat(context.Context) error
	Read(context.Context) (signaling.Wire, error)
	Stats() signaling.SessionStats
	Close() error
} = (*signaling.ShareSession)(nil)

type shareSignalFixture struct {
	local, peer *identity.Identity
	status      signaling.ShareStatus
	scope       sharegrant.SignalScope
	epoch       string
	wsCalls     atomic.Int32
	requests    atomic.Int32
	server      *httptest.Server
	ctx         context.Context
}

func newSignalFixture(t *testing.T, response func(signaling.ShareStatus) signaling.ShareStatus, handler func(*shareSignalFixture, http.ResponseWriter, *http.Request)) (*shareSignalFixture, *signaling.Client) {
	t.Helper()
	peer, local := shareClientIdentity(t), shareClientIdentity(t)
	status := boundClientStatus(peer, local)
	status.State, status.Plan, status.ResumeUntil = sharegrant.Approved, "", 0
	ctx, cancel := context.WithCancel(t.Context())
	f := &shareSignalFixture{local: local, peer: peer, status: status, epoch: protocol.RandomID(), ctx: ctx}
	f.scope = sharegrant.SignalScope{GrantID: status.ID, ClaimID: status.ClaimID, TaskID: status.TaskID, GrantGeneration: status.Generation, Phase: sharegrant.SignalPreview, Manifest: status.Manifest}
	f.server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.requests.Add(1)
		if r.URL.Path == "/healthz" {
			controlHealth(w, f.epoch, nil)
			return
		}
		if r.URL.Path == "/v1/shares/status" {
			current := f.status
			if response != nil {
				current = response(current)
			}
			_ = json.NewEncoder(w).Encode(current)
			return
		}
		f.wsCalls.Add(1)
		signature, err := base64.RawStdEncoding.DecodeString(r.Header.Get("X-LinkSend-Signature"))
		at, timeErr := strconv.ParseInt(r.Header.Get("X-LinkSend-Time"), 10, 64)
		nonce := r.Header.Get("X-LinkSend-Nonce")
		if err != nil || timeErr != nil || r.Method != http.MethodGet || r.URL.RequestURI() != f.scope.RequestURI() || r.Header.Get("X-LinkSend-Device") != f.local.ID() || r.Header.Get("X-LinkSend-Public-Key") != base64.RawStdEncoding.EncodeToString(f.local.PublicKey()) || r.Header.Get("X-LinkSend-Share-Epoch") != f.epoch || len(nonce) != 32 || !ed25519.Verify(f.local.PublicKey(), protocol.AuthBytes(http.MethodGet, r.URL.RequestURI(), f.local.ID(), f.epoch+":"+nonce, at, nil), signature) {
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(protocol.Error{Code: protocol.AuthenticationFailed})
			return
		}
		handler(f, w, r)
	}))
	t.Cleanup(f.server.Close)
	t.Cleanup(cancel)
	return f, newShareClient(t, f.server, local, nil)
}

func (f *shareSignalFixture) accept(t *testing.T, w http.ResponseWriter, r *http.Request, ready bool) *websocket.Conn {
	t.Helper()
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{Subprotocols: []string{sharegrant.SignalSubprotocol}})
	if err != nil {
		t.Errorf("test websocket accept failed: %v", err)
		return nil
	}
	conn.SetReadLimit(protocol.MaxMessageBytes)
	if err := f.write(conn, f.projection("ready", ready)); err != nil {
		conn.CloseNow()
		return nil
	}
	return conn
}

func (f *shareSignalFixture) projection(kind string, ready bool) sharegrant.SignalFrame {
	return sharegrant.SignalFrame{Version: 1, Type: kind, Scope: f.scope, ActorID: f.local.ID(), Epoch: f.epoch, PeerReady: ready}
}

func (f *shareSignalFixture) write(conn *websocket.Conn, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(f.ctx, 3*time.Second)
	defer cancel()
	return conn.Write(ctx, websocket.MessageText, raw)
}

func (f *shareSignalFixture) signed(t *testing.T, kind string) sharegrant.SignalFrame {
	t.Helper()
	payload := any(map[string]any{"ufrag": "abcdefgh", "password": "abcdefghijklmnopqrstuvwx"})
	if kind == "candidate" {
		payload = protocol.Candidate{Candidate: "candidate:test 1 udp 2130706431 198.51.100.10 54321 typ host"}
	}
	message, err := protocol.NewEnvelope(kind, f.peer.ID(), f.local.ID(), strings.Repeat("7", 32), 1, payload)
	if err != nil {
		t.Fatal(err)
	}
	message.Signature = f.peer.Sign(message.SigningBytes())
	frame, err := sharegrant.NewSignalFrame(f.scope, f.epoch, f.peer.ID(), "signal", &message, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	frame.Signature = f.peer.Sign(frame.SigningBytes())
	return frame
}

func connectSignalFixture(t *testing.T, f *shareSignalFixture, c *signaling.Client) *signaling.ShareSession {
	t.Helper()
	session, err := c.ConnectShare(t.Context(), f.scope, f.status, signaling.ShareSignalOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

func TestShareSignalClientPreservesSignaturesAndFiltersPresence(t *testing.T) {
	start := make(chan struct{})
	sent := make(chan sharegrant.SignalFrame, 2)
	peerProof := make(chan sharegrant.SignalFrame, 1)
	f, c := newSignalFixture(t, nil, func(f *shareSignalFixture, w http.ResponseWriter, r *http.Request) {
		conn := f.accept(t, w, r, false)
		if conn == nil {
			return
		}
		defer conn.CloseNow()
		select {
		case <-start:
		case <-f.ctx.Done():
			return
		}
		if f.write(conn, f.projection("ready", true)) != nil {
			return
		}
		proof := f.signed(t, "connect_request")
		peerProof <- proof
		if f.write(conn, proof) != nil {
			return
		}
		for range 2 {
			_, raw, err := conn.Read(f.ctx)
			if err != nil {
				return
			}
			frame, err := sharegrant.DecodeSignalFrame(raw)
			if err != nil || frame.Verify(f.local.PublicKey(), f.scope, f.epoch, time.Now(), false) != nil {
				t.Error("client sent an invalid inner/outer proof")
				return
			}
			sent <- frame
			if f.write(conn, f.projection("ready", true)) != nil {
				return
			}
		}
		_, _, _ = conn.Read(f.ctx)
	})
	s := connectSignalFixture(t, f, c)
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	if err := s.WaitPeer(cancelled); !errors.Is(err, context.Canceled) {
		t.Fatalf("presence wait ignored cancellation: %v", err)
	}
	if _, err := s.Read(cancelled); !errors.Is(err, context.Canceled) {
		t.Fatalf("read wait ignored cancellation: %v", err)
	}
	close(start)
	ctx, cancelRead := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancelRead()
	if err := s.WaitPeer(ctx); err != nil {
		t.Fatal(err)
	}
	wire, err := s.Read(ctx)
	if err != nil || wire.Message == nil || wire.Type != "signal" {
		t.Fatalf("read returned presence or failed: %v", err)
	}
	original := <-peerProof
	got, _ := json.Marshal(wire.Message)
	want, _ := json.Marshal(original.Message)
	if string(got) != string(want) || wire.Message.Verify(f.peer.PublicKey(), time.Now()) != nil {
		t.Fatal("peer Envelope was modified or no longer verifies")
	}
	response, _ := protocol.NewEnvelope("connect_response", f.local.ID(), f.peer.ID(), wire.Message.SessionID, 1, map[string]string{"ufrag": "abcdefgh", "password": "abcdefghijklmnopqrstuvwx"})
	if err := s.SendEnvelope(ctx, response); err != nil {
		t.Fatal(err)
	}
	if err := s.SendHeartbeat(ctx); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"signal", "heartbeat"} {
		select {
		case frame := <-sent:
			if frame.Type != kind || kind == "signal" && string(frame.Message.Payload) != string(response.Payload) {
				t.Fatal("client rewrote metadata or heartbeat scope")
			}
		case <-ctx.Done():
			t.Fatal("server did not receive signed frame")
		}
	}
	if stats := s.Stats(); stats.BytesSent == 0 || stats.BytesReceived == 0 {
		t.Fatal("actual scoped metadata counters were empty")
	}
}

func TestShareSignalClientRejectsForgedScopeKeysAndFrames(t *testing.T) {
	const private = "PRIVATE_SHARE_SIGNAL_ERROR"
	for _, name := range []string{"scope", "outer signature", "inner signature", "epoch", "actor", "recipient", "role", "projection scope", "signed ready", "private error", "unknown JSON", "oversized", "binary"} {
		t.Run(name, func(t *testing.T) {
			f, c := newSignalFixture(t, nil, func(f *shareSignalFixture, w http.ResponseWriter, r *http.Request) {
				conn := f.accept(t, w, r, true)
				if conn == nil {
					return
				}
				defer conn.CloseNow()
				frame := f.signed(t, "connect_request")
				switch name {
				case "scope":
					frame.Scope.TaskID = protocol.RandomID()
				case "outer signature":
					frame.Signature = make([]byte, ed25519.SignatureSize)
				case "inner signature":
					frame.Message.Signature = make([]byte, ed25519.SignatureSize)
				case "epoch":
					frame.Epoch = protocol.RandomID()
				case "actor":
					frame.ActorID = f.local.ID()
				case "recipient":
					frame.Message.Recipient = f.peer.ID()
					frame.Message.Signature = f.peer.Sign(frame.Message.SigningBytes())
				case "role":
					frame.Message.Type = "connect_response"
					frame.Message.Signature = f.peer.Sign(frame.Message.SigningBytes())
				case "projection scope":
					frame = f.projection("ready", true)
					frame.Scope.TaskID = protocol.RandomID()
				case "signed ready":
					frame = f.projection("ready", true)
					frame.Signature = f.peer.Sign(frame.SigningBytes())
				case "private error":
					frame = f.projection("error", false)
					frame.Code = protocol.Code(private)
				}
				if name != "outer signature" && frame.Type == "signal" {
					frame.Signature = f.peer.Sign(frame.SigningBytes())
				}
				raw, _ := json.Marshal(frame)
				kind := websocket.MessageText
				switch name {
				case "unknown JSON":
					raw = append(raw[:len(raw)-1], []byte(`,"detail":"`+private+`"}`)...)
				case "oversized":
					raw = []byte(`{"detail":"` + strings.Repeat(private, 2000) + `"}`)
				case "binary":
					kind = websocket.MessageBinary
				}
				_ = conn.Write(f.ctx, kind, raw)
				_, _, _ = conn.Read(f.ctx)
			})
			s := connectSignalFixture(t, f, c)
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			wire, err := s.Read(ctx)
			if err == nil || wire.Message != nil || strings.Contains(err.Error(), private) || strings.Contains(err.Error(), f.scope.GrantID) || strings.Contains(err.Error(), f.scope.Manifest) {
				t.Fatalf("forged frame accepted or secret echoed: %v", err)
			}
		})
	}
}

func TestShareSignalClientRejectsOutboundOtherRolesAndBodyMetadata(t *testing.T) {
	read := make(chan sharegrant.SignalFrame, 1)
	f, c := newSignalFixture(t, nil, func(f *shareSignalFixture, w http.ResponseWriter, r *http.Request) {
		conn := f.accept(t, w, r, true)
		if conn == nil {
			return
		}
		defer conn.CloseNow()
		_, raw, err := conn.Read(f.ctx)
		if err == nil {
			frame, _ := sharegrant.DecodeSignalFrame(raw)
			read <- frame
		}
		_, _, _ = conn.Read(f.ctx)
	})
	s := connectSignalFixture(t, f, c)
	for _, name := range []string{"sender", "recipient", "signed input", "wrong role", "file body", "clipboard permission", "loopback candidate"} {
		t.Run(name, func(t *testing.T) {
			envelope, _ := protocol.NewEnvelope("connect_response", f.local.ID(), f.peer.ID(), strings.Repeat("7", 32), 1, map[string]string{"ufrag": "abcdefgh", "password": "abcdefghijklmnopqrstuvwx"})
			switch name {
			case "sender":
				envelope.Sender = f.peer.ID()
			case "recipient":
				envelope.Recipient = f.local.ID()
			case "signed input":
				envelope.Signature = f.local.Sign(envelope.SigningBytes())
			case "wrong role":
				envelope.Type = "connect_request"
			case "file body":
				envelope.Payload = json.RawMessage(`{"ufrag":"abcdefgh","password":"abcdefghijklmnopqrstuvwx","body":"PRIVATE_FILE_BYTES"}`)
			case "clipboard permission":
				envelope.Payload = json.RawMessage(`{"ufrag":"abcdefgh","password":"abcdefghijklmnopqrstuvwx","clipboard_sync":true}`)
			case "loopback candidate":
				envelope.Type = "candidate"
				envelope.Payload = json.RawMessage(`{"candidate":"candidate:test 1 udp 2130706431 127.0.0.1 54321 typ host"}`)
			}
			if err := s.SendEnvelope(t.Context(), envelope); err == nil {
				t.Fatal("unscoped role, permission or body was sent")
			}
		})
	}
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	if err := s.SendHeartbeat(cancelled); !errors.Is(err, context.Canceled) {
		t.Fatalf("already-cancelled write lost its cause: %v", err)
	}
	if err := s.SendHeartbeat(t.Context()); err != nil {
		t.Fatal(err)
	}
	select {
	case frame := <-read:
		if frame.Type != "heartbeat" {
			t.Fatal("invalid envelope reached wire before heartbeat barrier")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("heartbeat barrier did not arrive")
	}
}

func TestShareSignalClientPinsCurrentActorsAndScopeBeforeUpgrade(t *testing.T) {
	for _, name := range []string{"peer key", "phase", "deadline"} {
		t.Run(name, func(t *testing.T) {
			third := shareClientIdentity(t)
			f, c := newSignalFixture(t, func(status signaling.ShareStatus) signaling.ShareStatus {
				switch name {
				case "peer key":
					status.SenderKey = base64.RawStdEncoding.EncodeToString(third.PublicKey())
				case "phase":
					status.State, status.Plan, status.ResumeUntil = sharegrant.Bound, strings.Repeat("b", 64), time.Now().Add(24*time.Hour).Unix()
				case "deadline":
					status.ExpiresAt++
				}
				return status
			}, func(*shareSignalFixture, http.ResponseWriter, *http.Request) {
				t.Error("upgrade occurred despite changed authorization")
			})
			if session, err := c.ConnectShare(t.Context(), f.scope, f.status, signaling.ShareSignalOptions{}); err == nil || session != nil || f.wsCalls.Load() != 0 {
				t.Fatalf("changed binding reached websocket: %v", err)
			}
		})
	}
}

func TestShareSignalClientConcurrentCloseReadAndWaitDoNotRevive(t *testing.T) {
	f, c := newSignalFixture(t, nil, func(f *shareSignalFixture, w http.ResponseWriter, r *http.Request) {
		conn := f.accept(t, w, r, false)
		if conn == nil {
			return
		}
		defer conn.CloseNow()
		_, _, _ = conn.Read(f.ctx)
	})
	s := connectSignalFixture(t, f, c)
	var wg sync.WaitGroup
	for range 6 {
		wg.Go(func() {
			if err := s.WaitPeer(t.Context()); err == nil {
				t.Error("closed scope became peer-ready")
			}
		})
		wg.Go(func() {
			if _, err := s.Read(t.Context()); err == nil {
				t.Error("closed scope returned a signal")
			}
		})
		wg.Go(func() { _ = s.Close() })
	}
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Close/Read/WaitPeer left a blocked owner")
	}
	if err := s.SendHeartbeat(t.Context()); err == nil {
		t.Fatal("closed session sent a new heartbeat")
	}
}

func TestShareSignalClientCancelledHandshakeAndRedirectStayBounded(t *testing.T) {
	t.Run("cancel", func(t *testing.T) {
		started := make(chan struct{})
		f, c := newSignalFixture(t, nil, func(f *shareSignalFixture, w http.ResponseWriter, r *http.Request) {
			conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{Subprotocols: []string{sharegrant.SignalSubprotocol}})
			if err != nil {
				return
			}
			defer conn.CloseNow()
			close(started)
			_, _, _ = conn.Read(f.ctx)
		})
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		done := make(chan error, 1)
		go func() { _, err := c.ConnectShare(ctx, f.scope, f.status, signaling.ShareSignalOptions{}); done <- err }()
		select {
		case <-started:
		case <-time.After(3 * time.Second):
			t.Fatal("handshake did not reach controlled boundary")
		}
		cancel()
		select {
		case err := <-done:
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("cancelled handshake lost its cause: %v", err)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("cancelled handshake did not close")
		}
	})
	t.Run("redirect", func(t *testing.T) {
		var redirected atomic.Int32
		target := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { redirected.Add(1) }))
		defer target.Close()
		f, c := newSignalFixture(t, nil, func(f *shareSignalFixture, w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Location", target.URL)
			w.WriteHeader(http.StatusTemporaryRedirect)
			_, _ = io.WriteString(w, `{"detail":"PRIVATE_SIGNAL_REDIRECT"}`)
		})
		if session, err := c.ConnectShare(t.Context(), f.scope, f.status, signaling.ShareSignalOptions{}); err == nil || session != nil || redirected.Load() != 0 || strings.Contains(err.Error(), "PRIVATE_SIGNAL_REDIRECT") {
			t.Fatalf("redirect followed or private detail escaped: %v", err)
		}
	})
}

func TestShareSignalClientDistinguishesExplicitGrantCloseFromTransportLoss(t *testing.T) {
	for name, tc := range map[string]struct {
		close  websocket.StatusCode
		reason string
		want   protocol.Code
	}{
		"terminal":            {websocket.StatusPolicyViolation, string(protocol.ShareTerminal), protocol.ShareTerminal},
		"expired":             {websocket.StatusPolicyViolation, string(protocol.ShareExpired), protocol.ShareExpired},
		"rejected":            {websocket.StatusPolicyViolation, string(protocol.ShareRejected), protocol.ShareRejected},
		"preview retired":     {websocket.StatusPolicyViolation, string(protocol.ShareConflict), protocol.ShareConflict},
		"private policy text": {websocket.StatusPolicyViolation, "PRIVATE_CLOSE_DETAIL", protocol.InvalidMessage},
		"ordinary close":      {websocket.StatusNormalClosure, string(protocol.ShareTerminal), protocol.SignalingUnreachable},
		"abrupt EOF":          {0, "", protocol.SignalingUnreachable},
	} {
		t.Run(name, func(t *testing.T) {
			f, c := newSignalFixture(t, nil, func(f *shareSignalFixture, w http.ResponseWriter, r *http.Request) {
				conn := f.accept(t, w, r, true)
				if conn == nil {
					return
				}
				defer conn.CloseNow()
				if tc.close != 0 {
					_ = conn.Close(tc.close, tc.reason)
				}
			})
			s := connectSignalFixture(t, f, c)
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			if _, err := s.Read(ctx); protocol.ErrorCode(err) != tc.want || strings.Contains(err.Error(), "PRIVATE_CLOSE_DETAIL") {
				t.Fatalf("grant outcome conflated with transport loss: code=%s err=%v", protocol.ErrorCode(err), err)
			}
		})
	}
}

func TestShareSignalClientRejectsReplayAndQueueOverflow(t *testing.T) {
	t.Run("replay", func(t *testing.T) {
		consumed := make(chan struct{})
		f, c := newSignalFixture(t, nil, func(f *shareSignalFixture, w http.ResponseWriter, r *http.Request) {
			conn := f.accept(t, w, r, true)
			if conn == nil {
				return
			}
			defer conn.CloseNow()
			frame := f.signed(t, "connect_request")
			if f.write(conn, frame) != nil {
				return
			}
			select {
			case <-consumed:
			case <-f.ctx.Done():
				return
			}
			_ = f.write(conn, frame)
			_, _, _ = conn.Read(f.ctx)
		})
		s := connectSignalFixture(t, f, c)
		ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
		defer cancel()
		if _, err := s.Read(ctx); err != nil {
			t.Fatal(err)
		}
		close(consumed)
		if _, err := s.Read(ctx); protocol.ErrorCode(err) != protocol.Replay {
			t.Fatalf("replayed peer proof accepted: %v", err)
		}
	})
	t.Run("queue", func(t *testing.T) {
		f, c := newSignalFixture(t, nil, func(f *shareSignalFixture, w http.ResponseWriter, r *http.Request) {
			conn := f.accept(t, w, r, false)
			if conn == nil {
				return
			}
			defer conn.CloseNow()
			for range 65 {
				if f.write(conn, f.signed(t, "candidate")) != nil {
					return
				}
			}
			_, _, _ = conn.Read(f.ctx)
		})
		s := connectSignalFixture(t, f, c)
		ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
		defer cancel()
		if err := s.WaitPeer(ctx); protocol.ErrorCode(err) != protocol.RateLimited {
			t.Fatalf("unconsumed signal queue did not fail closed: %v", err)
		}
	})
}

func TestShareSignalClientRealUnjoinedPreviewAndBoundUseSeparateConnections(t *testing.T) {
	backend := experimentalShareServer(t, filepath.Join(t.TempDir(), "control.sqlite"))
	defer backend.Close()
	h := httptest.NewTLSServer(backend.Handler())
	defer h.Close()
	sender, receiver := shareClientIdentity(t), shareClientIdentity(t)
	sc, rc := newShareClient(t, h, sender, nil), newShareClient(t, h, receiver, nil)
	_, offered, code := createClientShare(t, sc)
	claimed := shareControlCall(t, rc, signaling.ShareClaim, signaling.ShareRequest{Code: code, ClaimID: protocol.RandomID()})
	approved := shareControlCall(t, sc, signaling.ShareApprove, signaling.ShareRequest{ID: offered.ID, ClaimID: claimed.ClaimID, ApprovalID: protocol.RandomID(), TaskID: protocol.RandomID(), Generation: claimed.Generation, Manifest: offered.Manifest, ReceiverKey: claimed.ReceiverKey})
	scope := sharegrant.SignalScope{GrantID: approved.ID, ClaimID: approved.ClaimID, TaskID: approved.TaskID, GrantGeneration: approved.Generation, Phase: sharegrant.SignalPreview, Manifest: approved.Manifest}
	ctx, cancel := context.WithTimeout(t.Context(), 8*time.Second)
	defer cancel()
	receiverPreview, err := rc.ConnectShare(ctx, scope, approved, signaling.ShareSignalOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer receiverPreview.Close()
	senderPreview, err := sc.ConnectShare(ctx, scope, approved, signaling.ShareSignalOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer senderPreview.Close()
	for _, session := range []*signaling.ShareSession{senderPreview, receiverPreview} {
		if err := session.WaitPeer(ctx); err != nil {
			t.Fatal(err)
		}
		if err := session.SendHeartbeat(ctx); err != nil {
			t.Fatal(err)
		}
	}
	exchange := func(from, to *signaling.ShareSession, fromID, toID *identity.Identity, sessionID string, kind string) {
		t.Helper()
		message, err := protocol.NewEnvelope(kind, fromID.ID(), toID.ID(), sessionID, 1, map[string]string{"ufrag": "abcdefgh", "password": "abcdefghijklmnopqrstuvwx"})
		if err != nil || from.SendEnvelope(ctx, message) != nil {
			t.Fatal("real scoped metadata send failed")
		}
		wire, err := to.Read(ctx)
		if err != nil || wire.Type != "signal" || wire.Message == nil || wire.Message.SessionID != sessionID || wire.Message.Type != kind || wire.Message.Verify(fromID.PublicKey(), time.Now()) != nil || string(wire.Message.Payload) != string(message.Payload) {
			t.Fatalf("real scoped metadata verification failed: %v", err)
		}
	}
	previewICE := protocol.RandomID()
	exchange(senderPreview, receiverPreview, sender, receiver, previewICE, "connect_request")
	exchange(receiverPreview, senderPreview, receiver, sender, previewICE, "connect_response")
	binding := signaling.ShareRequest{ID: approved.ID, TaskID: approved.TaskID, Manifest: approved.Manifest, Plan: strings.Repeat("b", 64), Generation: approved.Generation}
	bound := shareControlCall(t, rc, signaling.ShareBind, binding)
	for _, session := range []*signaling.ShareSession{senderPreview, receiverPreview} {
		if wire, err := session.Read(ctx); protocol.ErrorCode(err) != protocol.ShareConflict || wire.Message != nil {
			t.Fatal("binding left preview socket usable")
		}
		if err := session.SendHeartbeat(ctx); err == nil {
			t.Fatal("closed preview was upgraded in place")
		}
	}
	scope.Phase, scope.Plan = sharegrant.SignalTransfer, bound.Plan
	receiverTransfer, err := rc.ConnectShare(ctx, scope, bound, signaling.ShareSignalOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer receiverTransfer.Close()
	senderTransfer, err := sc.ConnectShare(ctx, scope, bound, signaling.ShareSignalOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer senderTransfer.Close()
	for _, session := range []*signaling.ShareSession{senderTransfer, receiverTransfer} {
		if err := session.WaitPeer(ctx); err != nil {
			t.Fatal(err)
		}
	}
	transferICE := protocol.RandomID()
	exchange(senderTransfer, receiverTransfer, sender, receiver, transferICE, "connect_request")
	exchange(receiverTransfer, senderTransfer, receiver, sender, transferICE, "connect_response")
	shareControlCall(t, sc, signaling.ShareComplete, binding)
	for _, session := range []*signaling.ShareSession{senderTransfer, receiverTransfer} {
		if wire, err := session.Read(ctx); protocol.ErrorCode(err) != protocol.ShareTerminal || wire.Message != nil {
			t.Fatal("completion left transfer coordination reusable")
		}
	}
	for _, client := range []*signaling.Client{sc, rc} {
		if _, err := client.Devices(ctx); err == nil {
			t.Fatal("scoped websocket added permanent membership")
		}
		if ordinary, err := client.Connect(ctx); err == nil || ordinary != nil {
			if ordinary != nil {
				ordinary.Close()
			}
			t.Fatal("scoped websocket enabled ordinary member signaling")
		}
		if info, err := client.ShareControlInfo(ctx); err != nil || info.FileTransferReady {
			t.Fatal("control prototype advertised a complete data plane")
		}
	}
}

func TestShareSignalClientRealRevokeIsTerminalAndServerCloseIsTransportLoss(t *testing.T) {
	for _, revoke := range []bool{true, false} {
		name := "server close"
		if revoke {
			name = "revoke"
		}
		t.Run(name, func(t *testing.T) {
			backend := experimentalShareServer(t, filepath.Join(t.TempDir(), "control.sqlite"))
			defer backend.Close()
			h := httptest.NewTLSServer(backend.Handler())
			defer h.Close()
			sender, receiver := shareClientIdentity(t), shareClientIdentity(t)
			sc, rc := newShareClient(t, h, sender, nil), newShareClient(t, h, receiver, nil)
			_, offered, code := createClientShare(t, sc)
			claimed := shareControlCall(t, rc, signaling.ShareClaim, signaling.ShareRequest{Code: code, ClaimID: protocol.RandomID()})
			approved := shareControlCall(t, sc, signaling.ShareApprove, signaling.ShareRequest{ID: offered.ID, ClaimID: claimed.ClaimID, ApprovalID: protocol.RandomID(), TaskID: protocol.RandomID(), Generation: claimed.Generation, Manifest: offered.Manifest, ReceiverKey: claimed.ReceiverKey})
			scope := sharegrant.SignalScope{GrantID: approved.ID, ClaimID: approved.ClaimID, TaskID: approved.TaskID, GrantGeneration: approved.Generation, Phase: sharegrant.SignalPreview, Manifest: approved.Manifest}
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			session, err := sc.ConnectShare(ctx, scope, approved, signaling.ShareSignalOptions{})
			if err != nil {
				t.Fatal(err)
			}
			defer session.Close()
			want := protocol.SignalingUnreachable
			if revoke {
				shareControlCall(t, sc, signaling.ShareRevoke, signaling.ShareRequest{ID: offered.ID})
				want = protocol.ShareTerminal
			} else if err := backend.Close(); err != nil {
				t.Fatal(err)
			}
			if _, err := session.Read(ctx); protocol.ErrorCode(err) != want {
				t.Fatalf("real grant/transport outcome code=%s want=%s err=%v", protocol.ErrorCode(err), want, err)
			}
		})
	}
}
