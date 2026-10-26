package signaling

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Wen5555/LinkSend/internal/identity"
	"github.com/Wen5555/LinkSend/internal/protocol"
	"github.com/Wen5555/LinkSend/internal/sharegrant"
	"github.com/coder/websocket"
)

type ShareSignalOptions struct {
	// Test/development admission only; production never needs loopback ICE.
	AllowLoopbackCandidates bool
}

// ShareSession coordinates one immutable temporary scope. It grants neither
// membership nor file access. A ready projection means only that the other
// scoped signaling socket is present; ICE/QUIC still authenticate the peer.
type ShareSession struct {
	conn          *websocket.Conn
	identity      *identity.Identity
	peerKey       ed25519.PublicKey
	peerID        string
	scope         sharegrant.SignalScope
	epoch         string
	localSender   bool
	allowLoopback bool
	writes        chan struct{}
	reads         chan protocol.Envelope
	done          chan struct{}
	readerDone    chan struct{}
	closeOnce     sync.Once
	mu            sync.Mutex
	terminal      error
	peerReady     bool
	readyChanged  chan struct{}
	bytesSent     atomic.Uint64
	bytesReceived atomic.Uint64
	replays       *protocol.ReplayWindow
}

// ConnectShare rechecks the caller's original key/scope pins against the
// current grant, then authenticates a separate scoped WSS connection. It does
// not enroll devices, reuse ordinary signaling, or upgrade preview to transfer.
func (c *Client) ConnectShare(ctx context.Context, scope sharegrant.SignalScope, pinned ShareStatus, options ShareSignalOptions) (*ShareSession, error) {
	actorKey := base64.RawStdEncoding.EncodeToString(c.identity.PublicKey())
	if !shareSignalStatusMatches(scope, pinned, actorKey) {
		return nil, shareInvalid()
	}
	current, err := c.ShareControl(ctx, ShareRead, ShareRequest{ID: scope.GrantID, ClaimID: scope.ClaimID, TaskID: scope.TaskID, Generation: scope.GrantGeneration, Manifest: scope.Manifest, Plan: scope.Plan, ReceiverKey: pinned.ReceiverKey})
	if err != nil {
		return nil, err
	}
	if !shareSignalStatusMatches(scope, current, actorKey) || current.SenderKey != pinned.SenderKey || current.ReceiverKey != pinned.ReceiverKey || current.ExpiresAt != pinned.ExpiresAt || current.ResumeUntil != pinned.ResumeUntil {
		return nil, shareSignalFailure(protocol.ShareRejected)
	}
	info, err := c.ShareControlInfo(ctx)
	if err != nil {
		return nil, err
	}
	requestURI := scope.RequestURI()
	relative, err := url.ParseRequestURI(requestURI)
	if err != nil {
		return nil, shareInvalid()
	}
	endpoint := *c.base
	endpoint.Path, endpoint.RawPath, endpoint.RawQuery = relative.Path, "", relative.RawQuery
	if endpoint.Scheme == "https" {
		endpoint.Scheme = "wss"
	} else {
		endpoint.Scheme = "ws"
	}
	headers := http.Header{}
	nonce, at := protocol.RandomID(), time.Now().Unix()
	headers.Set("X-LinkSend-Membership", "2")
	headers.Set("X-LinkSend-Share-Grants", "1")
	headers.Set("X-LinkSend-Device", c.identity.ID())
	headers.Set("X-LinkSend-Public-Key", actorKey)
	headers.Set("X-LinkSend-Share-Epoch", info.Epoch)
	headers.Set("X-LinkSend-Nonce", nonce)
	headers.Set("X-LinkSend-Time", strconv.FormatInt(at, 10))
	headers.Set("X-LinkSend-Signature", base64.RawStdEncoding.EncodeToString(c.identity.Sign(protocol.AuthBytes(http.MethodGet, requestURI, c.identity.ID(), info.Epoch+":"+nonce, at, nil))))
	dialCtx, cancelDial := context.WithTimeout(ctx, 5*time.Second)
	defer cancelDial()
	conn, response, err := websocket.Dial(dialCtx, endpoint.String(), &websocket.DialOptions{HTTPClient: c.http, HTTPHeader: headers, Subprotocols: []string{sharegrant.SignalSubprotocol}, CompressionMode: websocket.CompressionDisabled})
	if err != nil {
		if response != nil {
			defer response.Body.Close()
			return nil, decodeShareFailure(response)
		}
		return nil, shareTransportFailure(dialCtx)
	}
	closeConnection := true
	defer func() {
		if closeConnection {
			_ = conn.CloseNow()
		}
	}()
	conn.SetReadLimit(protocol.MaxMessageBytes)
	if conn.Subprotocol() != sharegrant.SignalSubprotocol {
		return nil, shareSignalFailure(protocol.ShareUnsupported)
	}
	kind, raw, err := conn.Read(dialCtx)
	if err != nil {
		return nil, shareSignalReadFailure(err, dialCtx)
	}
	frame, decodeErr := sharegrant.DecodeSignalFrame(raw)
	if kind != websocket.MessageText || decodeErr != nil || !shareServerProjection(frame, scope, info.Epoch, c.identity.ID()) {
		return nil, shareSignalFailure(protocol.InvalidMessage)
	}
	if frame.Type == "error" {
		return nil, shareSignalFailure(frame.Code)
	}
	peer := current.SenderKey
	localSender := actorKey == current.SenderKey
	if localSender {
		peer = current.ReceiverKey
	}
	peerKey, _ := base64.RawStdEncoding.DecodeString(peer) // validated by status
	s := &ShareSession{conn: conn, identity: c.identity, peerKey: ed25519.PublicKey(peerKey), peerID: identity.DeviceID(peerKey), scope: scope, epoch: info.Epoch, localSender: localSender, allowLoopback: options.AllowLoopbackCandidates, writes: make(chan struct{}, 1), reads: make(chan protocol.Envelope, 64), done: make(chan struct{}), readerDone: make(chan struct{}), peerReady: frame.PeerReady, readyChanged: make(chan struct{}), replays: protocol.NewReplayWindow(256)}
	s.bytesReceived.Store(uint64(len(raw)))
	closeConnection = false
	go s.readLoop()
	return s, nil
}

func shareSignalStatusMatches(scope sharegrant.SignalScope, status ShareStatus, actorKey string) bool {
	if scope.Validate() != nil || !validShareStatus(status) || status.ID != scope.GrantID || status.ClaimID != scope.ClaimID || status.TaskID != scope.TaskID || status.Generation != scope.GrantGeneration || status.Manifest != scope.Manifest || status.Plan != scope.Plan || (actorKey != status.SenderKey && actorKey != status.ReceiverKey) {
		return false
	}
	return (scope.Phase == sharegrant.SignalPreview && status.State == sharegrant.Approved) || (scope.Phase == sharegrant.SignalTransfer && status.State == sharegrant.Bound)
}

func shareServerProjection(frame sharegrant.SignalFrame, scope sharegrant.SignalScope, epoch, actor string) bool {
	if frame.Version != 1 || frame.Scope != scope || frame.Epoch != epoch || frame.ActorID != actor || frame.Nonce != "" || frame.IssuedAt != 0 || frame.ExpiresAt != 0 || len(frame.Signature) != 0 || frame.Message != nil {
		return false
	}
	return frame.Type == "ready" && frame.Code == "" || frame.Type == "error" && !frame.PeerReady && frame.Code != ""
}

func shareSignalFailure(code protocol.Code) error {
	switch code {
	case protocol.ShareInvalid, protocol.ShareRejected, protocol.ShareExpired, protocol.ShareConflict, protocol.ShareTerminal, protocol.ShareUnsupported, protocol.ShareUnavailable, protocol.AuthenticationFailed, protocol.InvalidMessage, protocol.Replay, protocol.RateLimited, protocol.SignalingUnreachable, protocol.SignalingTimeout, protocol.PeerOffline, protocol.SessionConflict:
	default:
		code = protocol.InvalidMessage
	}
	return protocol.Fail(code, "temporary share signaling unavailable")
}

func shareSignalReadFailure(err error, ctx context.Context) error {
	var closeError websocket.CloseError
	if errors.As(err, &closeError) && closeError.Code == websocket.StatusPolicyViolation {
		// Only exact stable reasons on a verified TLS scoped connection can
		// carry an explicit grant outcome. Arbitrary close text is never echoed
		// or interpreted as revocation; ordinary EOF remains a transport loss.
		switch protocol.Code(closeError.Reason) {
		case protocol.ShareTerminal, protocol.ShareExpired, protocol.ShareRejected, protocol.ShareConflict:
			return shareSignalFailure(protocol.Code(closeError.Reason))
		default:
			return shareSignalFailure(protocol.InvalidMessage)
		}
	}
	if ctx.Err() != nil {
		return shareTransportFailure(ctx)
	}
	return shareSignalFailure(protocol.SignalingUnreachable)
}

func (s *ShareSession) err() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.terminal
}

func (s *ShareSession) stop(err error) {
	s.closeOnce.Do(func() {
		if err == nil {
			err = shareSignalFailure(protocol.SignalingUnreachable)
		}
		s.mu.Lock()
		s.terminal = err
		s.peerReady = false
		close(s.readyChanged)
		close(s.done)
		s.mu.Unlock()
		_ = s.conn.CloseNow()
	})
}

func (s *ShareSession) updatePeer(ready bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.terminal == nil && s.peerReady != ready {
		s.peerReady = ready
		close(s.readyChanged)
		s.readyChanged = make(chan struct{})
	}
}

func (s *ShareSession) readLoop() {
	defer close(s.readerDone)
	defer close(s.reads)
	for {
		readCtx, cancel := context.WithTimeout(context.Background(), 65*time.Second)
		kind, raw, err := s.conn.Read(readCtx)
		if err != nil {
			failure := shareSignalReadFailure(err, readCtx)
			cancel()
			s.stop(failure)
			return
		}
		cancel()
		s.bytesReceived.Add(uint64(len(raw)))
		frame, err := sharegrant.DecodeSignalFrame(raw)
		if kind != websocket.MessageText || err != nil {
			s.stop(shareSignalFailure(protocol.InvalidMessage))
			return
		}
		if frame.Type == "ready" || frame.Type == "error" {
			if !shareServerProjection(frame, s.scope, s.epoch, s.identity.ID()) {
				s.stop(shareSignalFailure(protocol.InvalidMessage))
				return
			}
			if frame.Type == "error" {
				s.stop(shareSignalFailure(frame.Code))
				return
			}
			s.updatePeer(frame.PeerReady)
			continue
		}
		now := time.Now()
		if frame.Type != "signal" || frame.ActorID != s.peerID || frame.Verify(s.peerKey, s.scope, s.epoch, now, s.allowLoopback) != nil || frame.Message == nil || frame.Message.Recipient != s.identity.ID() || frame.Message.Type == "connect_request" && s.localSender || frame.Message.Type == "connect_response" && !s.localSender {
			s.stop(shareSignalFailure(protocol.AuthenticationFailed))
			return
		}
		if s.replays.Use("outer:"+frame.Nonce, time.Unix(frame.ExpiresAt, 0), now) != nil || s.replays.Use("inner:"+frame.Message.MessageID, time.Unix(frame.Message.ExpiresAt, 0), now) != nil {
			s.stop(shareSignalFailure(protocol.Replay))
			return
		}
		select {
		case s.reads <- *frame.Message:
		case <-s.done:
			return
		default:
			s.stop(shareSignalFailure(protocol.RateLimited))
			return
		}
	}
}

func (s *ShareSession) write(ctx context.Context, message *protocol.Envelope) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case s.writes <- struct{}{}:
		defer func() { <-s.writes }()
	case <-ctx.Done():
		return ctx.Err()
	case <-s.done:
		return s.err()
	}
	if err := s.err(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	kind := "heartbeat"
	if message != nil {
		kind = "signal"
	}
	now := time.Now()
	frame, err := sharegrant.NewSignalFrame(s.scope, s.epoch, s.identity.ID(), kind, message, now)
	if err != nil {
		return shareSignalFailure(protocol.InvalidMessage)
	}
	frame.Signature = s.identity.Sign(frame.SigningBytes())
	if frame.Verify(s.identity.PublicKey(), s.scope, s.epoch, now, s.allowLoopback) != nil {
		return shareSignalFailure(protocol.InvalidMessage)
	}
	raw, err := json.Marshal(frame)
	if err != nil || len(raw) > protocol.MaxMessageBytes {
		return shareSignalFailure(protocol.InvalidMessage)
	}
	writeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err = s.conn.Write(writeCtx, websocket.MessageText, raw); err != nil {
		failure := shareTransportFailure(writeCtx)
		s.stop(failure)
		return failure
	}
	s.bytesSent.Add(uint64(len(raw)))
	return nil
}

func (s *ShareSession) SendEnvelope(ctx context.Context, envelope protocol.Envelope) error {
	if envelope.Sender != s.identity.ID() || envelope.Recipient != s.peerID || len(envelope.Signature) != 0 || envelope.Type == "connect_request" && !s.localSender || envelope.Type == "connect_response" && s.localSender {
		return shareSignalFailure(protocol.AuthenticationFailed)
	}
	envelope.Payload = append(json.RawMessage(nil), envelope.Payload...)
	envelope.Signature = s.identity.Sign(envelope.SigningBytes())
	return s.write(ctx, &envelope)
}

func (s *ShareSession) SendHeartbeat(ctx context.Context) error { return s.write(ctx, nil) }

func (s *ShareSession) Read(ctx context.Context) (Wire, error) {
	if err := s.err(); err != nil {
		return Wire{}, err
	}
	select {
	case envelope, ok := <-s.reads:
		if err := s.err(); err != nil {
			return Wire{}, err
		}
		if !ok {
			return Wire{}, shareSignalFailure(protocol.SignalingUnreachable)
		}
		return Wire{Type: "signal", Message: &envelope}, nil
	case <-ctx.Done():
		return Wire{}, ctx.Err()
	case <-s.done:
		return Wire{}, s.err()
	}
}

func (s *ShareSession) WaitPeer(ctx context.Context) error {
	for {
		s.mu.Lock()
		err, ready, changed := s.terminal, s.peerReady, s.readyChanged
		s.mu.Unlock()
		if err != nil {
			return err
		}
		if ready {
			return nil
		}
		select {
		case <-changed:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func (s *ShareSession) Stats() SessionStats {
	return SessionStats{BytesSent: s.bytesSent.Load(), BytesReceived: s.bytesReceived.Load()}
}

func (s *ShareSession) Close() error {
	s.stop(nil)
	<-s.readerDone
	return nil
}
