package app

import (
	"context"
	"errors"
	"time"

	"github.com/Wen5555/LinkSend/internal/clipboardsync"
)

const clipboardLeaseTTL = 10 * time.Second

type ClipboardChange struct {
	Generation uint64
	Kinds      []clipboardsync.Kind
}

type ClipboardAdapter struct {
	Generation func() uint64
	Read       func(context.Context, clipboardsync.Kind, uint64) ([]byte, uint64, error)
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

func containsClipboardKind(kinds []clipboardsync.Kind, kind clipboardsync.Kind) bool {
	for _, candidate := range kinds {
		if candidate == kind {
			return true
		}
	}
	return false
}

func (s *Service) ClipboardChanged(ctx context.Context, change ClipboardChange) error {
	s.clipboardSendMu.Lock()
	defer s.clipboardSendMu.Unlock()
	if s.clipboardPaused.Load() || change.Generation == 0 || len(change.Kinds) == 0 {
		return clipboardsync.ErrStale
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
	var result error
	for _, kind := range change.Kinds {
		targets := make([]struct {
			peer  *PeerSession
			lease clipboardsync.Lease
		}, 0, len(peers))
		for _, peer := range peers {
			if s.checkAuthorizationGeneration(peer.PeerID, peer.AuthorizationGeneration) != nil || !containsClipboardKind(s.clipboardKinds(peer.PeerID, "send", peer.AuthorizationGeneration), kind) {
				continue
			}
			if lease, ok := s.clipboardSync.Outbound(peer.PeerID, peer.SessionID, peer.AuthorizationGeneration, kind); ok {
				targets = append(targets, struct {
					peer  *PeerSession
					lease clipboardsync.Lease
				}{peer, lease})
			}
		}
		if len(targets) == 0 {
			continue
		}
		payload, current, err := adapter.Read(ctx, kind, change.Generation)
		if err != nil || current != change.Generation {
			result = errors.Join(result, err)
			continue
		}
		base, err := s.clipboardSync.PrepareLocal(current, kind, payload)
		if err != nil {
			return errors.Join(result, err)
		}
		for _, target := range targets {
			event, err := s.clipboardSync.Bind(base, target.lease.ID)
			if err != nil {
				result = errors.Join(result, err)
				continue
			}
			peer := target.peer
			message := clipboardsync.Message{Type: "event", LeaseID: event.LeaseID, SessionID: peer.SessionID, Generation: peer.AuthorizationGeneration, OriginID: event.OriginID, Boot: event.Boot, OriginSeq: event.OriginSeq, Lamport: event.Lamport, Kind: event.Kind, Digest: event.Digest, OSGeneration: event.OSGeneration, PayloadBytes: uint32(len(event.Payload))}
			stream, err := peer.Data.Conn.OpenUniStreamSync(ctx)
			if err == nil {
				err = clipboardsync.Write(stream, message, event.Payload)
				err = errors.Join(err, stream.Close())
			}
			result = errors.Join(result, err)
		}
		break
	}
	return result
}

func (s *Service) serveClipboardPeer(peer *PeerSession) {
	ctx := peer.Data.Conn.Context()
	renewDone := make(chan struct{})
	go func() {
		defer close(renewDone)
		s.sendClipboardLease(ctx, peer)
		s.setClipboardReady(peer, "send", s.clipboardSync.HasOutbound(peer.PeerID, peer.SessionID, peer.AuthorizationGeneration))
		ticker := time.NewTicker(7 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.sendClipboardLease(ctx, peer)
				s.setClipboardReady(peer, "send", s.clipboardSync.HasOutbound(peer.PeerID, peer.SessionID, peer.AuthorizationGeneration))
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
		if message.SessionID != peer.SessionID || message.Generation != peer.AuthorizationGeneration || s.checkAuthorizationGeneration(peer.PeerID, peer.AuthorizationGeneration) != nil {
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
			allowed := s.clipboardKinds(peer.PeerID, "send", peer.AuthorizationGeneration)
			var kinds []clipboardsync.Kind
			for _, kind := range message.Kinds {
				if containsClipboardKind(allowed, kind) {
					kinds = append(kinds, kind)
				}
			}
			s.clipboardAdapterMu.RLock()
			adapter := s.clipboardAdapter
			s.clipboardAdapterMu.RUnlock()
			if adapter.Generation == nil {
				continue
			}
			if err := s.clipboardSync.Install(message.LeaseID, peer.PeerID, peer.SessionID, peer.AuthorizationGeneration, kinds, time.Duration(message.TTLMillis)*time.Millisecond, adapter.Generation()); err == nil {
				s.setClipboardReady(peer, "send", true)
			}
		case "event":
			if !containsClipboardKind(s.clipboardKinds(peer.PeerID, "receive", peer.AuthorizationGeneration), message.Kind) {
				stream.CancelRead(0)
				continue
			}
			event := clipboardsync.Event{LeaseID: message.LeaseID, OriginID: message.OriginID, Boot: message.Boot, OriginSeq: message.OriginSeq, Lamport: message.Lamport, Kind: message.Kind, Digest: message.Digest, OSGeneration: message.OSGeneration}
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
				_ = s.clipboardSync.Commit(candidate, func(expected uint64, kind clipboardsync.Kind, body []byte) (uint64, error) {
					if s.checkAuthorizationGeneration(peer.PeerID, peer.AuthorizationGeneration) != nil {
						return 0, clipboardsync.ErrStale
					}
					s.clipboardGrantMu.Lock()
					defer s.clipboardGrantMu.Unlock()
					if generation, err := s.clipboardPeerGeneration(peer.PeerID); err != nil || generation != peer.AuthorizationGeneration || !containsClipboardKind(s.clipboardKindsLocked(peer.PeerID, "receive", generation), kind) {
						return 0, clipboardsync.ErrStale
					}
					return adapter.Write(kind, body, expected, candidate.Deadline)
				})
			}()
		}
	}
}

func (s *Service) sendClipboardLease(parent context.Context, peer *PeerSession) {
	if s.clipboardPaused.Load() {
		s.setClipboardReady(peer, "receive", false)
		return
	}
	kinds := s.clipboardKinds(peer.PeerID, "receive", peer.AuthorizationGeneration)
	if len(kinds) == 0 {
		s.setClipboardReady(peer, "receive", false)
		return
	}
	s.setClipboardReady(peer, "receive", true)
	lease, err := s.clipboardSync.Issue(peer.PeerID, peer.SessionID, peer.AuthorizationGeneration, kinds, clipboardLeaseTTL)
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(parent, 2*time.Second)
	defer cancel()
	stream, err := peer.Data.Conn.OpenUniStreamSync(ctx)
	if err != nil {
		return
	}
	message := clipboardsync.Message{Type: "lease", LeaseID: lease.ID, SessionID: peer.SessionID, Generation: peer.AuthorizationGeneration, Kinds: kinds, TTLMillis: uint32(clipboardLeaseTTL / time.Millisecond)}
	err = clipboardsync.Write(stream, message, nil)
	_ = stream.Close()
	if err != nil {
		stream.CancelWrite(0)
	}
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
	if paused {
		s.directPoolMu.Lock()
		for key, pooled := range s.directPool {
			pooled.clipboardSendReady = false
			pooled.clipboardReceiveReady = false
			s.schedulePoolIdleLocked(key, pooled)
		}
		s.directPoolMu.Unlock()
	}
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
	var result error
	for _, item := range targets {
		key := directPoolKey(item.id, item.generation)
		s.directPoolMu.Lock()
		pooled := s.directPool[key]
		healthy := pooled != nil && pooled.peer != nil && pooled.peer.Data != nil && pooled.peer.Data.Conn.Context().Err() == nil
		s.directPoolMu.Unlock()
		if healthy {
			continue
		}
		connectCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		cfg.expectedAuthorizationGeneration = item.generation
		peer, connectErr := s.acquirePeerSession(connectCtx, item.id, cfg)
		cancel()
		if connectErr != nil {
			result = errors.Join(result, connectErr)
			continue
		}
		result = errors.Join(result, s.releasePeerSession(peer, nil))
	}
	return result
}
