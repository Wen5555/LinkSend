package app

import (
	"context"
	"errors"
	"io"
	"sort"
	"sync"
	"time"

	"github.com/Wen5555/LinkSend/internal/clipboardsync"
)

const clipboardLeaseTTL = 10 * time.Second

type ClipboardChange struct {
	Generation uint64
	Kinds      []clipboardsync.Kind
}

type ClipboardPeerStatus struct {
	PeerID       string `json:"peer_id"`
	State        string `json:"state"`
	SendReady    bool   `json:"send_ready"`
	ReceiveReady bool   `json:"receive_ready"`
}

func clipboardMessageGenerationValid(peer *PeerSession, message clipboardsync.Message) bool {
	if peer == nil {
		return false
	}
	switch message.Type {
	case "lease":
		return peer.RemoteAuthorizationGeneration > 0 && message.Generation == peer.RemoteAuthorizationGeneration
	case "event":
		return peer.AuthorizationGeneration > 0 && message.Generation == peer.AuthorizationGeneration
	default:
		return false
	}
}

func (s *Service) ClipboardPeerStatuses() []ClipboardPeerStatus {
	s.directPoolMu.Lock()
	defer s.directPoolMu.Unlock()
	statuses := make([]ClipboardPeerStatus, 0, len(s.directPool))
	for _, pooled := range s.directPool {
		if pooled.peer == nil {
			continue
		}
		state := "ready"
		if !pooled.peer.ClipboardSync {
			state = "unsupported"
		} else if !pooled.clipboardSendReady && !pooled.clipboardReceiveReady {
			state = "connecting"
		}
		statuses = append(statuses, ClipboardPeerStatus{PeerID: pooled.peer.PeerID, State: state, SendReady: pooled.clipboardSendReady, ReceiveReady: pooled.clipboardReceiveReady})
	}
	sort.Slice(statuses, func(i, j int) bool { return statuses[i].PeerID < statuses[j].PeerID })
	return statuses
}

type ClipboardAdapter struct {
	Generation func() uint64
	Read       func(context.Context, clipboardsync.Kind, uint64) ([]byte, uint64, error)
	Validate   func(clipboardsync.Kind, []byte) error
	Write      func(clipboardsync.Kind, []byte, uint64, time.Time) (uint64, error)
}

func (s *Service) ConfigureClipboard(adapter ClipboardAdapter) {
	s.clipboardAdapterMu.Lock()
	s.clipboardAdapter = adapter
	s.clipboardAdapterMu.Unlock()
	generation := uint64(0)
	if adapter.Generation != nil {
		generation = adapter.Generation()
	}
	s.clipboardSync.Reset(false, generation)
}

func (s *Service) invalidateClipboardState() {
	s.clipboardAdapterMu.RLock()
	adapter := s.clipboardAdapter
	s.clipboardAdapterMu.RUnlock()
	generation := uint64(0)
	if adapter.Generation != nil {
		generation = adapter.Generation()
	}
	s.ResetClipboard(s.clipboardPaused.Load(), generation)
}

func (s *Service) clipboardKinds(peerID, direction string, generation uint64) []clipboardsync.Kind {
	s.clipboardGrantMu.Lock()
	defer s.clipboardGrantMu.Unlock()
	return s.clipboardKindsLocked(peerID, direction, generation)
}

func (s *Service) clipboardKindsLocked(peerID, direction string, generation uint64) []clipboardsync.Kind {
	rows, err := s.store.db.Query(`SELECT kind FROM clipboard_grants WHERE peer_id=? AND direction=? AND enabled=1 AND authorization_generation=? ORDER BY kind`, peerID, direction, generation)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var kinds []clipboardsync.Kind
	for rows.Next() {
		var kind string
		if rows.Scan(&kind) == nil {
			kinds = append(kinds, clipboardsync.Kind(kind))
		}
	}
	return kinds
}

func (s *Service) clipboardGrant(peerID, direction string, kind clipboardsync.Kind, generation uint64) (ClipboardGrant, bool) {
	s.clipboardGrantMu.Lock()
	defer s.clipboardGrantMu.Unlock()
	return s.clipboardGrantLocked(peerID, direction, kind, generation)
}

func (s *Service) clipboardGrantLocked(peerID, direction string, kind clipboardsync.Kind, generation uint64) (ClipboardGrant, bool) {
	var grant ClipboardGrant
	err := s.store.db.QueryRow(`SELECT peer_id,direction,kind,enabled,revision,updated_at,authorization_generation FROM clipboard_grants WHERE peer_id=? AND direction=? AND kind=? AND enabled=1 AND authorization_generation=?`, peerID, direction, kind, generation).Scan(&grant.PeerID, &grant.Direction, &grant.Kind, &grant.Enabled, &grant.Revision, &grant.UpdatedAt, &grant.AuthorizationGeneration)
	return grant, err == nil && grant.Enabled
}

func (s *Service) clipboardGrants(peerID, direction string, generation uint64) []clipboardsync.Grant {
	s.clipboardGrantMu.Lock()
	defer s.clipboardGrantMu.Unlock()
	rows, err := s.store.db.Query(`SELECT kind,revision FROM clipboard_grants WHERE peer_id=? AND direction=? AND enabled=1 AND authorization_generation=? ORDER BY kind`, peerID, direction, generation)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var grants []clipboardsync.Grant
	for rows.Next() {
		var kind string
		var revision uint64
		if rows.Scan(&kind, &revision) == nil {
			grants = append(grants, clipboardsync.Grant{Kind: clipboardsync.Kind(kind), Revision: revision})
		}
	}
	return grants
}

func containsClipboardKind(kinds []clipboardsync.Kind, kind clipboardsync.Kind) bool {
	for _, candidate := range kinds {
		if candidate == kind {
			return true
		}
	}
	return false
}

func (s *Service) ClipboardChanged(ctx context.Context, change ClipboardChange) error {
	if s.clipboardPaused.Load() || change.Generation == 0 {
		return clipboardsync.ErrStale
	}
	observation, err := s.clipboardSync.ObserveLocalEvent(change.Generation)
	if err != nil {
		return err
	}
	if len(change.Kinds) == 0 {
		return nil
	}
	s.clipboardAdapterMu.RLock()
	adapter := s.clipboardAdapter
	s.clipboardAdapterMu.RUnlock()
	if adapter.Read == nil {
		return errors.New("CLIPBOARD_ADAPTER_UNAVAILABLE")
	}
	s.directPoolMu.Lock()
	peers := make([]*PeerSession, 0, len(s.directPool))
	for _, pooled := range s.directPool {
		if pooled.peer != nil && pooled.peer.Data != nil {
			peers = append(peers, pooled.peer)
		}
	}
	s.directPoolMu.Unlock()
	for _, kind := range change.Kinds {
		targets := make([]struct {
			peer     *PeerSession
			lease    clipboardsync.Lease
			revision uint64
		}, 0, len(peers))
		for _, peer := range peers {
			if !peer.ClipboardSync || s.checkAuthorizationGeneration(peer.PeerID, peer.AuthorizationGeneration) != nil {
				continue
			}
			grant, allowed := s.clipboardGrant(peer.PeerID, "send", kind, peer.AuthorizationGeneration)
			if lease, ok := s.clipboardSync.Outbound(peer.PeerID, peer.SessionID, peer.RemoteAuthorizationGeneration, kind); ok && allowed {
				targets = append(targets, struct {
					peer     *PeerSession
					lease    clipboardsync.Lease
					revision uint64
				}{peer, lease, grant.Revision})
			}
		}
		if len(targets) == 0 {
			continue
		}
		payload, current, err := adapter.Read(ctx, kind, change.Generation)
		if err != nil || current != change.Generation {
			continue
		}
		base, err := s.clipboardSync.PrepareObserved(observation, kind, payload)
		if err != nil {
			return err
		}
		for _, target := range targets {
			event, err := s.clipboardSync.BindScoped(base, target.lease.ID, target.revision)
			if err != nil {
				continue
			}
			s.startClipboardSend(target.peer, target.lease, event)
		}
		break
	}
	return nil
}

type clipboardGuardedWriter struct {
	ctx      context.Context
	dst      io.Writer
	state    *clipboardsync.State
	event    clipboardsync.Event
	deadline time.Time
}

func (w clipboardGuardedWriter) Write(payload []byte) (int, error) {
	total := 0
	for len(payload) > 0 {
		if w.ctx.Err() != nil || !time.Now().Before(w.deadline) || !w.state.LocalCurrent(w.event) {
			return total, clipboardsync.ErrStale
		}
		n := 64 << 10
		if len(payload) < n {
			n = len(payload)
		}
		written, err := w.dst.Write(payload[:n])
		total += written
		payload = payload[written:]
		if err != nil {
			return total, err
		}
		if written == 0 {
			return total, io.ErrShortWrite
		}
		if len(payload) > 0 {
			timer := time.NewTimer(4 * time.Millisecond)
			select {
			case <-w.ctx.Done():
				timer.Stop()
				return total, w.ctx.Err()
			case <-timer.C:
			}
		}
	}
	return total, nil
}

func (s *Service) startClipboardSend(peer *PeerSession, lease clipboardsync.Lease, event clipboardsync.Event) {
	s.operationMu.Lock()
	if s.isClosing() {
		s.operationMu.Unlock()
		return
	}
	s.clipboardWorkers.Add(1)
	s.operationMu.Unlock()
	go func() {
		defer s.clipboardWorkers.Done()
		ctx := peer.Data.Conn.Context()
		select {
		case s.clipboardSendSlots <- struct{}{}:
		case <-ctx.Done():
			return
		}
		defer func() { <-s.clipboardSendSlots }()
		deadline := lease.ExpiresAt()
		stream, err := peer.Data.Conn.OpenUniStreamSync(ctx)
		if err != nil {
			return
		}
		_ = stream.SetWriteDeadline(deadline)
		message := clipboardsync.Message{Type: "event", LeaseID: event.LeaseID, SessionID: peer.SessionID, Generation: peer.RemoteAuthorizationGeneration, OriginID: event.OriginID, Boot: event.Boot, OriginSeq: event.OriginSeq, Lamport: event.Lamport, Kind: event.Kind, Digest: event.Digest, OSGeneration: event.OSGeneration, SenderGrantRevision: event.SenderGrantRevision, ReceiverGrantRevision: event.ReceiverGrantRevision, PayloadBytes: uint32(len(event.Payload))}
		writer := clipboardGuardedWriter{ctx: ctx, dst: stream, state: s.clipboardSync, event: event, deadline: deadline}
		if err = clipboardsync.Write(writer, message, event.Payload); err != nil {
			stream.CancelWrite(0)
			return
		}
		_ = stream.Close()
	}()
}

func (s *Service) serveClipboardPeer(peer *PeerSession) {
	ctx := peer.Data.Conn.Context()
	renewDone := make(chan struct{})
	go func() {
		defer close(renewDone)
		s.sendClipboardLease(ctx, peer)
		s.setClipboardReady(peer, "send", s.clipboardSync.HasOutbound(peer.PeerID, peer.SessionID, peer.RemoteAuthorizationGeneration))
		ticker := time.NewTicker(7 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.sendClipboardLease(ctx, peer)
				s.setClipboardReady(peer, "send", s.clipboardSync.HasOutbound(peer.PeerID, peer.SessionID, peer.RemoteAuthorizationGeneration))
			}
		}
	}()
	defer func() { <-renewDone; s.clipboardSync.DropSession(peer.PeerID, peer.SessionID) }()
	for {
		stream, err := peer.Data.Conn.AcceptUniStream(ctx)
		if err != nil {
			return
		}
		_ = stream.SetReadDeadline(time.Now().Add(clipboardLeaseTTL))
		message, err := clipboardsync.ReadHeader(stream)
		if err != nil {
			stream.CancelRead(0)
			continue
		}
		if message.SessionID != peer.SessionID || !clipboardMessageGenerationValid(peer, message) || s.checkAuthorizationGeneration(peer.PeerID, peer.AuthorizationGeneration) != nil {
			stream.CancelRead(0)
			continue
		}
		if s.clipboardPaused.Load() {
			stream.CancelRead(0)
			continue
		}
		switch message.Type {
		case "lease":
			payload, payloadErr := clipboardsync.ReadPayload(stream, message)
			stream.CancelRead(0)
			if payloadErr != nil || len(payload) != 0 {
				continue
			}
			var grants []clipboardsync.Grant
			for _, remoteGrant := range message.Grants {
				if _, ok := s.clipboardGrant(peer.PeerID, "send", remoteGrant.Kind, peer.AuthorizationGeneration); ok {
					grants = append(grants, clipboardsync.Grant{Kind: remoteGrant.Kind, Revision: remoteGrant.Revision})
				}
			}
			s.clipboardAdapterMu.RLock()
			adapter := s.clipboardAdapter
			s.clipboardAdapterMu.RUnlock()
			if adapter.Generation == nil {
				continue
			}
			if err := s.clipboardSync.InstallScoped(message.LeaseID, peer.PeerID, peer.SessionID, peer.RemoteAuthorizationGeneration, grants, time.Duration(message.TTLMillis)*time.Millisecond, adapter.Generation()); err == nil {
				s.setClipboardReady(peer, "send", true)
			}
		case "event":
			if message.OriginID != peer.PeerID {
				stream.CancelRead(0)
				continue
			}
			if !containsClipboardKind(s.clipboardKinds(peer.PeerID, "receive", peer.AuthorizationGeneration), message.Kind) {
				stream.CancelRead(0)
				continue
			}
			event := clipboardsync.Event{LeaseID: message.LeaseID, OriginID: message.OriginID, Boot: message.Boot, OriginSeq: message.OriginSeq, Lamport: message.Lamport, Kind: message.Kind, Digest: message.Digest, OSGeneration: message.OSGeneration, SenderGrantRevision: message.SenderGrantRevision, ReceiverGrantRevision: message.ReceiverGrantRevision}
			candidate, err := s.clipboardSync.BeginHeader(peer.PeerID, peer.SessionID, peer.AuthorizationGeneration, event)
			if err != nil {
				stream.CancelRead(0)
				continue
			}
			select {
			case s.clipboardReceiveSlots <- struct{}{}:
			case <-ctx.Done():
				stream.CancelRead(0)
				return
			}
			func() {
				defer func() { <-s.clipboardReceiveSlots }()
				_ = stream.SetReadDeadline(candidate.Deadline)
				payload, readErr := clipboardsync.ReadPayload(stream, message)
				stream.CancelRead(0)
				if readErr != nil {
					return
				}
				candidate, readErr = s.clipboardSync.AttachPayload(candidate, payload)
				if readErr != nil {
					return
				}
				s.clipboardAdapterMu.RLock()
				adapter := s.clipboardAdapter
				s.clipboardAdapterMu.RUnlock()
				if adapter.Write == nil {
					return
				}
				_ = s.commitClipboardCandidate(peer, candidate, adapter)
			}()
		}
	}
}

func (s *Service) commitClipboardCandidate(peer *PeerSession, candidate clipboardsync.Candidate, adapter ClipboardAdapter) error {
	if peer == nil || adapter.Write == nil {
		return errors.New("CLIPBOARD_ADAPTER_UNAVAILABLE")
	}
	if adapter.Validate != nil {
		if err := adapter.Validate(candidate.Kind, candidate.Payload); err != nil {
			return err
		}
	}
	s.clipboardGrantMu.Lock()
	defer s.clipboardGrantMu.Unlock()
	return s.clipboardSync.Commit(candidate, func(expected uint64, kind clipboardsync.Kind, body []byte) (uint64, error) {
		if s.clipboardPaused.Load() || s.checkAuthorizationGeneration(peer.PeerID, peer.AuthorizationGeneration) != nil {
			return 0, clipboardsync.ErrStale
		}
		grant, enabled := s.clipboardGrantLocked(peer.PeerID, "receive", kind, peer.AuthorizationGeneration)
		if generation, err := s.clipboardPeerGeneration(peer.PeerID); err != nil || generation != peer.AuthorizationGeneration || !enabled || grant.Revision != candidate.ReceiverGrantRevision || s.clipboardPaused.Load() || !time.Now().Before(candidate.Deadline) {
			return 0, clipboardsync.ErrStale
		}
		return adapter.Write(kind, body, expected, candidate.Deadline)
	})
}

func (s *Service) sendClipboardLease(parent context.Context, peer *PeerSession) {
	if s.clipboardPaused.Load() || !peer.ClipboardSync || peer.RemoteAuthorizationGeneration == 0 {
		s.setClipboardReady(peer, "receive", false)
		return
	}
	grants := s.clipboardGrants(peer.PeerID, "receive", peer.AuthorizationGeneration)
	if len(grants) == 0 {
		s.setClipboardReady(peer, "receive", false)
		return
	}
	lease, err := s.clipboardSync.IssueScoped(peer.PeerID, peer.SessionID, peer.AuthorizationGeneration, grants, clipboardLeaseTTL)
	if err != nil {
		s.setClipboardReady(peer, "receive", false)
		return
	}
	ctx, cancel := context.WithTimeout(parent, 2*time.Second)
	defer cancel()
	stream, err := peer.Data.Conn.OpenUniStreamSync(ctx)
	if err != nil {
		s.setClipboardReady(peer, "receive", false)
		return
	}
	message := clipboardsync.Message{Type: "lease", LeaseID: lease.ID, SessionID: peer.SessionID, Generation: peer.AuthorizationGeneration, Grants: grants, TTLMillis: uint32(clipboardLeaseTTL / time.Millisecond)}
	err = clipboardsync.Write(stream, message, nil)
	err = errors.Join(err, stream.Close())
	if err != nil {
		stream.CancelWrite(0)
		s.setClipboardReady(peer, "receive", false)
		return
	}
	s.setClipboardReady(peer, "receive", true)
}

func (s *Service) setClipboardReady(peer *PeerSession, direction string, ready bool) {
	if ready && s.clipboardPaused.Load() {
		ready = false
	}
	s.directPoolMu.Lock()
	defer s.directPoolMu.Unlock()
	for key, pooled := range s.directPool {
		if pooled.peer != peer {
			continue
		}
		if direction == "send" {
			pooled.clipboardSendReady = ready
		} else {
			pooled.clipboardReceiveReady = ready
		}
		if ready {
			if pooled.timer != nil {
				pooled.timer.Stop()
				pooled.timer = nil
			}
		} else if !pooled.clipboardSendReady && !pooled.clipboardReceiveReady {
			s.schedulePoolIdleLocked(key, pooled)
		}
	}
}

func (s *Service) ResetClipboard(paused bool, generation uint64) {
	s.clipboardPaused.Store(paused)
	s.clipboardSync.Reset(paused, generation)
	s.directPoolMu.Lock()
	for key, pooled := range s.directPool {
		pooled.clipboardSendReady = false
		pooled.clipboardReceiveReady = false
		s.schedulePoolIdleLocked(key, pooled)
	}
	s.directPoolMu.Unlock()
}

func (s *Service) EnsureClipboardSessions(ctx context.Context, cfg DirectConfig) error {
	if s.clipboardPaused.Load() || s.isClosing() {
		return nil
	}
	s.clipboardGrantMu.Lock()
	rows, err := s.store.db.Query(`SELECT DISTINCT peer_id,authorization_generation FROM clipboard_grants WHERE enabled=1`)
	if err != nil {
		s.clipboardGrantMu.Unlock()
		return err
	}
	type target struct {
		id         string
		generation uint64
	}
	var targets []target
	for rows.Next() {
		var item target
		if rows.Scan(&item.id, &item.generation) == nil {
			if current, currentErr := s.clipboardPeerGeneration(item.id); currentErr == nil && current == item.generation {
				targets = append(targets, item)
			}
		}
	}
	rowsErr := rows.Close()
	s.clipboardGrantMu.Unlock()
	if rowsErr != nil {
		return rowsErr
	}
	results := make(chan error, len(targets))
	var workers sync.WaitGroup
	for _, item := range targets {
		item := item
		workers.Add(1)
		go func() {
			defer workers.Done()
			key := directPoolKey(item.id, item.generation)
			s.directPoolMu.Lock()
			pooled := s.directPool[key]
			healthy := pooled != nil && pooled.peer != nil && pooled.peer.Data != nil && pooled.peer.Data.Conn.Context().Err() == nil
			s.directPoolMu.Unlock()
			if healthy {
				return
			}
			connectCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			peerCfg := cfg
			peerCfg.expectedAuthorizationGeneration = item.generation
			peer, connectErr := s.acquirePeerSession(connectCtx, item.id, peerCfg)
			cancel()
			if connectErr != nil {
				results <- connectErr
				return
			}
			results <- s.releasePeerSession(peer, nil)
		}()
	}
	workers.Wait()
	close(results)
	var result error
	for err := range results {
		result = errors.Join(result, err)
	}
	return result
}
