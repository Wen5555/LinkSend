package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/Wen5555/LinkSend/internal/identity"
	"github.com/Wen5555/LinkSend/internal/protocol"
	"github.com/Wen5555/LinkSend/internal/sharegrant"
	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

const maxShareSignalScopes = 32
const maxShareSignalConnections = 64
const maxShareSignalActorConnections = 8

type shareSignalHooks struct {
	afterAuthenticate func(context.Context, sharegrant.Key)
	beforeWrite       func(context.Context, sharegrant.SignalFrame)
	beforeCleanup     func(context.Context, sharegrant.Key)
}

// gate serializes share storage transitions, admission, queueing and dequeue
// checks. Network writes never hold it or the ordinary connection mutex.
type shareSignalHub struct {
	gate   sync.Mutex
	closed bool
	owners map[sharegrant.SignalScope]*shareSignalOwner
	slots  chan struct{}
	hooks  shareSignalHooks
}

type shareSignalOwner struct {
	scope          sharegrant.SignalScope
	sender         sharegrant.Key
	receiver       sharegrant.Key
	peers          map[sharegrant.Key]*shareSignalPeer
	negotiation    *negotiation
	currentSession string
	timer          *time.Timer
}

type shareSignalPeer struct {
	owner    *shareSignalOwner
	actor    sharegrant.Key
	conn     *websocket.Conn
	ctx      context.Context
	cancel   context.CancelFunc
	send     chan shareSignalQueued
	done     chan struct{}
	terminal chan protocol.Code
}

type shareSignalQueued struct {
	frame  sharegrant.SignalFrame
	source *shareSignalPeer
}

func newShareSignalHub() *shareSignalHub {
	return &shareSignalHub{owners: make(map[sharegrant.SignalScope]*shareSignalOwner), slots: make(chan struct{}, maxShareSignalConnections)}
}

func (s *Server) beginShareWork() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return false
	}
	s.wg.Add(1)
	return true
}

func (s *Server) closeShareSignals() {
	hub := s.shareSignals
	if hub == nil {
		return
	}
	hub.gate.Lock()
	defer hub.gate.Unlock()
	hub.closed = true
	for _, owner := range hub.owners {
		s.retireShareSignalLocked(owner)
	}
}

// Caller holds the share gate. Old callbacks retain only the retired pointer;
// they cannot remove a replacement owner or a replacement actor connection.
func (s *Server) retireShareSignalLocked(owner *shareSignalOwner, reason ...protocol.Code) {
	hub := s.shareSignals
	if hub.owners[owner.scope] != owner {
		return
	}
	delete(hub.owners, owner.scope)
	if owner.timer != nil && owner.timer.Stop() {
		s.wg.Done()
	}
	owner.timer = nil
	for _, peer := range owner.peers {
		if len(reason) != 0 {
			select {
			case peer.terminal <- reason[0]:
			default:
			}
		} else {
			peer.cancel()
			_ = peer.conn.CloseNow()
		}
	}
	owner.negotiation = nil
}

func (s *Server) refreshShareSignalsLocked(record sharegrant.Record, now time.Time) {
	for _, owner := range s.shareSignals.owners {
		if owner.scope.GrantID == record.ID && owner.scope.Authorize(record, owner.sender, now) != nil {
			s.retireShareSignalLocked(owner, shareSignalRetirementCode(owner, record, now))
		}
	}
}

func (s *Server) reconcileShareSignalsLocked(ctx context.Context, now time.Time) {
	for _, owner := range s.shareSignals.owners {
		record, err := s.store.ReadShare(ctx, owner.scope.GrantID, owner.sender)
		if err != nil || owner.scope.Authorize(record, owner.sender, now) != nil {
			if err != nil {
				s.retireShareSignalLocked(owner)
			} else {
				s.retireShareSignalLocked(owner, shareSignalRetirementCode(owner, record, now))
			}
		}
	}
}

func (s *Server) shareWebsocket(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.ExperimentalShareControl || r.Header.Get("X-LinkSend-Share-Grants") != "1" {
		reject(w, http.StatusUpgradeRequired, protocol.ShareUnsupported, "temporary share signaling not enabled")
		return
	}
	if !s.beginShareWork() {
		reject(w, http.StatusServiceUnavailable, protocol.ShareUnavailable, "share signaling unavailable")
		return
	}
	defer s.wg.Done()
	hub := s.shareSignals
	select {
	case hub.slots <- struct{}{}:
		defer func() { <-hub.slots }()
	default:
		reject(w, http.StatusTooManyRequests, protocol.RateLimited, "share connection limit")
		return
	}
	scope, err := sharegrant.ParseSignalScope(r.PathValue("id"), r.URL.RawQuery)
	if err != nil || r.ContentLength != 0 || len(r.TransferEncoding) != 0 {
		shareFailure(w, sharegrant.ErrInvalid, false)
		return
	}
	actor, ok := s.authenticateShare(w, r, nil, time.Now())
	if !ok {
		return
	}
	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()
	hub.gate.Lock()
	_, err = s.authorizeShareSignalLocked(ctx, scope, actor)
	hub.gate.Unlock()
	if err != nil {
		shareFailure(w, err, false)
		return
	}
	if hook := hub.hooks.afterAuthenticate; hook != nil {
		hook(ctx, actor)
	}
	if ctx.Err() != nil {
		return
	}
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{Subprotocols: []string{sharegrant.SignalSubprotocol}})
	if err != nil {
		return
	}
	defer c.CloseNow()
	c.SetReadLimit(protocol.MaxMessageBytes)
	if c.Subprotocol() != sharegrant.SignalSubprotocol {
		return
	}
	p := &shareSignalPeer{actor: actor, conn: c, ctx: ctx, cancel: cancel, send: make(chan shareSignalQueued, 64), done: make(chan struct{}), terminal: make(chan protocol.Code, 1)}
	hub.gate.Lock()
	record, err := s.authorizeShareSignalLocked(ctx, scope, actor)
	if err == nil {
		err = s.registerShareSignalLocked(p, scope, record)
	}
	hub.gate.Unlock()
	if err != nil {
		return
	}
	defer close(p.done)
	defer s.unregisterShareSignal(p)
	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		s.writeShareSignals(p)
	}()
	defer func() { cancel(); c.CloseNow(); <-writerDone }()
	budget, window := 60, time.Now()
	for {
		readCtx, readCancel := context.WithTimeout(ctx, 65*time.Second)
		typ, raw, readErr := c.Read(readCtx)
		readCancel()
		if readErr != nil || typ != websocket.MessageText {
			return
		}
		now := time.Now()
		if now.Sub(window) >= time.Second {
			window, budget = now, 60
		}
		budget--
		if budget < 0 {
			return
		}
		frame, decodeErr := sharegrant.DecodeSignalFrame(raw)
		if decodeErr != nil || frame.Verify(actor[:], scope, s.shareEpoch, now, s.cfg.AllowLoopbackCandidates) != nil {
			return
		}
		if frame.Type == "signal" {
			s.receivedMessages.Add(1)
			s.receivedBytes.Add(uint64(len(raw)))
		}
		if err = s.forwardShareSignal(p, frame); err != nil {
			s.queueShareSignalError(p, shareSignalCode(err))
		}
	}
}

func (s *Server) authorizeShareSignalLocked(ctx context.Context, scope sharegrant.SignalScope, actor sharegrant.Key) (sharegrant.Record, error) {
	if s.shareSignals.closed || ctx.Err() != nil {
		return sharegrant.Record{}, protocol.Fail(protocol.ShareUnavailable, "share signaling unavailable")
	}
	readCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	record, err := s.store.ReadShare(readCtx, scope.GrantID, actor)
	if err != nil {
		return record, err
	}
	return record, scope.Authorize(record, actor, time.Now())
}

func (s *Server) registerShareSignalLocked(p *shareSignalPeer, scope sharegrant.SignalScope, record sharegrant.Record) error {
	hub := s.shareSignals
	if hub.closed || p.ctx.Err() != nil {
		return protocol.Fail(protocol.ShareUnavailable, "share signaling unavailable")
	}
	owner := hub.owners[scope]
	actorCount := 0
	for _, existing := range hub.owners {
		if existing.peers[p.actor] != nil {
			actorCount++
		}
	}
	if (owner == nil || owner.peers[p.actor] == nil) && actorCount >= maxShareSignalActorConnections {
		return protocol.Fail(protocol.RateLimited, "share actor connection limit")
	}
	if owner == nil {
		if len(hub.owners) >= maxShareSignalScopes {
			return protocol.Fail(protocol.RateLimited, "share scope limit")
		}
		owner = &shareSignalOwner{scope: scope, sender: record.Sender, receiver: record.Receiver, peers: make(map[sharegrant.Key]*shareSignalPeer)}
		hub.owners[scope] = owner
		deadline := record.ExpiresAt
		if scope.Phase == sharegrant.SignalTransfer {
			deadline = record.ResumeUntil
		}
		s.wg.Add(1)
		owner.timer = time.AfterFunc(time.Until(time.Unix(deadline, 0)), func() {
			defer s.wg.Done()
			hub.gate.Lock()
			defer hub.gate.Unlock()
			owner.timer = nil
			s.retireShareSignalLocked(owner, protocol.ShareExpired)
		})
	}
	p.owner = owner
	old := owner.peers[p.actor]
	owner.peers[p.actor] = p
	if old != nil {
		owner.negotiation = nil
		old.cancel()
		_ = old.conn.CloseNow()
	}
	for _, current := range owner.peers {
		s.queueShareReadyLocked(current)
	}
	return nil
}

func (s *Server) unregisterShareSignal(p *shareSignalPeer) {
	hub := s.shareSignals
	if hook := hub.hooks.beforeCleanup; hook != nil {
		hook(p.ctx, p.actor)
	}
	hub.gate.Lock()
	defer hub.gate.Unlock()
	owner := p.owner
	if hub.owners[owner.scope] != owner || owner.peers[p.actor] != p {
		return
	}
	delete(owner.peers, p.actor)
	owner.negotiation = nil
	if len(owner.peers) == 0 {
		s.retireShareSignalLocked(owner)
		return
	}
	for _, current := range owner.peers {
		s.queueShareReadyLocked(current)
	}
}

func (s *Server) queueShareReadyLocked(p *shareSignalPeer) {
	frame := sharegrant.SignalFrame{Version: 1, Type: "ready", Scope: p.owner.scope, Epoch: s.shareEpoch, ActorID: identity.DeviceID(p.actor[:]), PeerReady: len(p.owner.peers) == 2}
	select {
	case p.send <- shareSignalQueued{frame: frame}:
	default:
		p.cancel()
		_ = p.conn.CloseNow()
	}
}

func (s *Server) queueShareSignalError(p *shareSignalPeer, code protocol.Code) {
	hub := s.shareSignals
	hub.gate.Lock()
	defer hub.gate.Unlock()
	if hub.owners[p.owner.scope] != p.owner || p.owner.peers[p.actor] != p {
		return
	}
	frame := sharegrant.SignalFrame{Version: 1, Type: "error", Scope: p.owner.scope, Epoch: s.shareEpoch, ActorID: identity.DeviceID(p.actor[:]), Code: code}
	select {
	case p.send <- shareSignalQueued{frame: frame}:
	default:
		p.cancel()
		_ = p.conn.CloseNow()
	}
}

func (s *Server) writeShareSignals(p *shareSignalPeer) {
	hub := s.shareSignals
	closeTerminal := func(code protocol.Code) {
		_ = p.conn.Close(websocket.StatusPolicyViolation, string(code))
		p.cancel()
		_ = p.conn.CloseNow()
	}
	for {
		select {
		case code := <-p.terminal:
			closeTerminal(code)
			return
		default:
		}
		select {
		case <-p.ctx.Done():
			return
		case code := <-p.terminal:
			closeTerminal(code)
			return
		case item := <-p.send:
			if hook := hub.hooks.beforeWrite; hook != nil {
				hook(p.ctx, item.frame)
			}
			hub.gate.Lock()
			owner := p.owner
			valid := hub.owners[owner.scope] == owner && owner.peers[p.actor] == p && p.ctx.Err() == nil
			if valid {
				record, err := s.authorizeShareSignalLocked(p.ctx, owner.scope, p.actor)
				if err != nil {
					if record.ID != "" {
						s.retireShareSignalLocked(owner, shareSignalRetirementCode(owner, record, time.Now()))
					} else {
						s.retireShareSignalLocked(owner)
					}
					valid = false
				}
			}
			if valid && item.source != nil {
				valid = owner.peers[item.source.actor] == item.source && owner.negotiation != nil && item.frame.Message.SessionID == owner.currentSession && item.frame.Message.Generation == owner.negotiation.generation
			}
			hub.gate.Unlock()
			if !valid {
				continue
			}
			writeCtx, cancel := context.WithTimeout(p.ctx, 5*time.Second)
			err := wsjson.Write(writeCtx, p.conn, item.frame)
			cancel()
			if err != nil {
				p.cancel()
				_ = p.conn.CloseNow()
				return
			}
		}
	}
}

func shareSignalRetirementCode(owner *shareSignalOwner, record sharegrant.Record, now time.Time) protocol.Code {
	if record.State == sharegrant.Expired || (owner.scope.Phase == sharegrant.SignalPreview && now.Unix() >= record.ExpiresAt) || (owner.scope.Phase == sharegrant.SignalTransfer && now.Unix() >= record.ResumeUntil) {
		return protocol.ShareExpired
	}
	if owner.scope.Phase == sharegrant.SignalPreview && record.State == sharegrant.Bound {
		return protocol.ShareConflict
	}
	return protocol.ShareTerminal
}

func shareSignalCode(err error) protocol.Code {
	var pe *protocol.Error
	if errors.As(err, &pe) {
		return pe.Code
	}
	switch {
	case errors.Is(err, sharegrant.ErrExpired):
		return protocol.ShareExpired
	case errors.Is(err, sharegrant.ErrTerminal):
		return protocol.ShareTerminal
	case errors.Is(err, sharegrant.ErrConflict):
		return protocol.ShareConflict
	case errors.Is(err, sharegrant.ErrInvalid):
		return protocol.ShareInvalid
	default:
		return protocol.ShareRejected
	}
}

func (s *Server) forwardShareSignal(from *shareSignalPeer, frame sharegrant.SignalFrame) error {
	if frame.Verify(from.actor[:], from.owner.scope, s.shareEpoch, time.Now(), s.cfg.AllowLoopbackCandidates) != nil {
		return sharegrant.ErrRejected
	}
	hub := s.shareSignals
	hub.gate.Lock()
	defer hub.gate.Unlock()
	owner := from.owner
	if hub.owners[owner.scope] != owner || owner.peers[from.actor] != from {
		return sharegrant.ErrRejected
	}
	record, err := s.authorizeShareSignalLocked(from.ctx, owner.scope, from.actor)
	if err != nil {
		if record.ID != "" {
			s.retireShareSignalLocked(owner, shareSignalRetirementCode(owner, record, time.Now()))
		} else {
			s.retireShareSignalLocked(owner)
		}
		return err
	}
	now := time.Now()
	if err = s.replays.Use("share-ws:"+s.shareEpoch+":"+frame.ActorID+":"+frame.Nonce, time.Unix(frame.ExpiresAt, 0).Add(time.Second), now); err != nil {
		return err
	}
	if frame.Type == "heartbeat" {
		s.queueShareReadyLocked(from)
		return nil
	}
	envelope := frame.Message
	other := record.Sender
	if from.actor == record.Sender {
		other = record.Receiver
	}
	if envelope.Sender != identity.DeviceID(from.actor[:]) || envelope.Recipient != identity.DeviceID(other[:]) {
		return sharegrant.ErrRejected
	}
	to := owner.peers[other]
	if to == nil {
		return protocol.Fail(protocol.PeerOffline, "share peer not connected")
	}
	n := owner.negotiation
	if n != nil && !n.expires.After(now) {
		owner.negotiation = nil
		n = nil
	}
	if envelope.Type == "connect_request" {
		if from.actor != record.Sender {
			return sharegrant.ErrRejected
		}
		if n != nil && !(n.ended[n.from] && n.ended[n.to]) {
			return sharegrant.ErrConflict
		}
		// Keep old ICE identifiers unrepeatable throughout envelope freshness,
		// including a WSS reconnect, in the existing bounded replay owner.
		// A phase transition must not make the preview identifier fresh again.
		// Keep this reservation tied to the grant, not its phase-specific URL.
		if err = s.replays.Use("share-ice:"+s.shareEpoch+":"+owner.scope.GrantID+":"+envelope.SessionID, now.Add(2*time.Minute), now); err != nil {
			return err
		}
		n = &negotiation{from: envelope.Sender, to: envelope.Recipient, generation: envelope.Generation, expires: now.Add(2 * time.Minute), counts: make(map[string]int), ended: make(map[string]bool)}
		owner.negotiation, owner.currentSession = n, envelope.SessionID
	} else {
		if n == nil || envelope.SessionID != owner.currentSession || envelope.Generation != n.generation {
			return sharegrant.ErrRejected
		}
		switch envelope.Type {
		case "connect_response":
			if from.actor != record.Receiver || n.accepted {
				return sharegrant.ErrRejected
			}
			n.accepted = true
		case "candidate":
			n.counts[envelope.Sender]++
			if n.counts[envelope.Sender] > protocol.MaxCandidates {
				return protocol.Fail(protocol.RateLimited, "share candidate limit")
			}
		case "end_of_candidates":
			n.ended[envelope.Sender] = true
		}
	}
	select {
	case to.send <- shareSignalQueued{frame: frame, source: from}:
		encoded, _ := json.Marshal(frame)
		s.forwardedMessages.Add(1)
		s.forwardedBytes.Add(uint64(len(encoded)))
	default:
		return protocol.Fail(protocol.RateLimited, "share recipient queue full")
	}
	return nil
}
