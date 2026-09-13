package app

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/Wen5555/LinkSend/internal/protocol"
	"github.com/Wen5555/LinkSend/internal/signaling"
	"github.com/Wen5555/LinkSend/internal/transfer"
	"github.com/Wen5555/LinkSend/internal/transport"
	quic "github.com/quic-go/quic-go"
)

const directSessionIdleTTL = 3 * time.Second

type pooledPeerSession struct {
	peer       *PeerSession
	generation uint64
	inUse      bool
	receiving  bool
	serving    bool
	timer      *time.Timer
}

func directPoolKey(peerID string, generation uint64) string {
	return fmt.Sprintf("%s/%d", peerID, generation)
}

func (s *Service) directPoolKeyLock(key string) *sync.Mutex {
	s.directPoolMu.Lock()
	defer s.directPoolMu.Unlock()
	lock := s.directPoolLocks[key]
	if lock == nil {
		lock = &sync.Mutex{}
		s.directPoolLocks[key] = lock
	}
	return lock
}

// Caller holds directPoolMu. Shutdown takes the same lock before draining the
// map, so no WaitGroup Add can race after its close-and-wait boundary.
func (s *Service) startPooledPeerServerLocked(key string, pooled *pooledPeerSession) {
	s.directPoolWG.Add(1)
	go func() {
		defer s.directPoolWG.Done()
		s.servePooledPeer(key, pooled)
	}()
}

func (s *Service) acquirePeerSession(ctx context.Context, peerID string, cfg DirectConfig) (*PeerSession, error) {
	generation := cfg.expectedAuthorizationGeneration
	if generation == 0 {
		var err error
		generation, err = s.currentAuthorizationGeneration(peerID)
		if err != nil {
			return nil, protocol.Wrap(protocol.AuthenticationFailed, "current peer grant required", err)
		}
		cfg.expectedAuthorizationGeneration = generation
	}
	key := directPoolKey(peerID, generation)
	keyLock := s.directPoolKeyLock(key)
	keyLock.Lock()
	defer keyLock.Unlock()
	s.directPoolMu.Lock()
	if s.isClosing() {
		s.directPoolMu.Unlock()
		return nil, errors.New("APP_CLOSING")
	}
	pooled := s.directPool[key]
	if pooled != nil && pooled.inUse {
		s.directPoolMu.Unlock()
		return nil, errors.New("BUSY: peer already has an active file stream")
	}
	if pooled != nil && pooled.peer != nil && pooled.peer.Data != nil && pooled.peer.Data.Conn.Context().Err() == nil {
		pooled.inUse = true
		if pooled.timer != nil {
			pooled.timer.Stop()
			pooled.timer = nil
		}
		peer := pooled.peer
		s.directPoolMu.Unlock()
		if err := s.checkAuthorizationGeneration(peerID, generation); err != nil {
			s.releasePeerSession(peer, err)
			return nil, err
		}
		cfg.phase("reusing_authenticated_session")
		cfg.session(peer.SessionID, peer.PeerID)
		cfg.evidence(peer.Evidence())
		return peer, nil
	}
	s.directPoolMu.Unlock()

	peer, err := s.connectWithOnlineGrace(ctx, peerID, cfg)
	if err != nil {
		return nil, err
	}
	key = directPoolKey(peerID, peer.AuthorizationGeneration)
	if !peer.SessionReuse {
		return peer, nil
	}
	if hook := s.directPoolBeforePublish; hook != nil {
		hook("dial")
	}
	s.directPoolMu.Lock()
	if s.isClosing() {
		s.directPoolMu.Unlock()
		_ = peer.Close()
		return nil, errors.New("APP_CLOSING")
	}
	var replaced *PeerSession
	if previous := s.directPool[key]; previous != nil && previous.peer != peer {
		if previous.timer != nil {
			previous.timer.Stop()
		}
		replaced = previous.peer
	}
	pooled = &pooledPeerSession{peer: peer, generation: peer.AuthorizationGeneration, inUse: true, serving: true}
	s.directPool[key] = pooled
	s.startPooledPeerServerLocked(key, pooled)
	s.directPoolMu.Unlock()
	if replaced != nil {
		_ = replaced.Close()
	}
	return peer, nil
}

func (s *Service) detachPeerControl(peer *PeerSession) error {
	if peer == nil {
		return nil
	}
	stopHeartbeat, signal, ownsSignal := peer.takeControl()
	if stopHeartbeat != nil {
		stopHeartbeat()
	}
	err := s.rememberLANPeer(peer)
	if signal != nil && ownsSignal {
		wss, reusable := signal.(*signaling.Session)
		if !reusable || !s.keepSpareSignal(wss) {
			err = errors.Join(err, signal.Close())
		}
	}
	return err
}

func (s *Service) releasePeerSession(peer *PeerSession, operationErr error) error {
	if peer == nil {
		return operationErr
	}
	if !peer.SessionReuse {
		if operationErr == nil {
			return s.closePeerAfterTransfer(peer)
		}
		return errors.Join(operationErr, peer.Close())
	}
	reusable := reusablePeerStreamResult(operationErr)
	if !reusable || peer.Data == nil || peer.Data.Conn.Context().Err() != nil {
		s.removePeerSession(peer)
		return errors.Join(operationErr, peer.Close())
	}
	if err := s.detachPeerControl(peer); err != nil {
		s.removePeerSession(peer)
		return errors.Join(operationErr, err, peer.Close())
	}
	key := directPoolKey(peer.PeerID, peer.AuthorizationGeneration)
	keyLock := s.directPoolKeyLock(key)
	keyLock.Lock()
	defer keyLock.Unlock()
	s.directPoolMu.Lock()
	pooled := s.directPool[key]
	if pooled == nil || pooled.peer != peer {
		s.directPoolMu.Unlock()
		return errors.Join(operationErr, peer.Close())
	}
	pooled.inUse = false
	s.schedulePoolIdleLocked(key, pooled)
	s.directPoolMu.Unlock()
	return operationErr
}

func reusablePeerStreamResult(operationErr error) bool {
	if operationErr == nil || protocol.ErrorCode(operationErr) == protocol.Cancelled ||
		errors.Is(operationErr, context.Canceled) || errors.Is(operationErr, transfer.ErrCancelled) ||
		errors.Is(operationErr, transfer.ErrRejected) {
		return true
	}
	var streamErr *quic.StreamError
	if errors.As(operationErr, &streamErr) {
		return true
	}
	switch protocol.ErrorCode(operationErr) {
	case protocol.ReceiveRejected, protocol.DiskFull, protocol.PermissionDenied,
		protocol.FileConflict, protocol.UnsafePath, protocol.IntegrityFailed,
		protocol.SourceChanged, protocol.ReceivePlanUnsupported, protocol.ReceivePlanMismatch,
		protocol.ReceivePlanInvalid, protocol.ReceivePlanPersistence,
		protocol.ReceiveDirectoryUnavailable, protocol.ContentUnsupported,
		protocol.ContentInvalid, protocol.ContentMismatch:
		return true
	default:
		return false
	}
}

func (s *Service) schedulePoolIdleLocked(key string, pooled *pooledPeerSession) {
	if pooled.inUse || pooled.receiving {
		return
	}
	if pooled.timer != nil {
		pooled.timer.Stop()
	}
	pooled.timer = time.AfterFunc(directSessionIdleTTL, func() {
		s.directPoolMu.Lock()
		current := s.directPool[key]
		if current == pooled && !current.inUse && !current.receiving {
			delete(s.directPool, key)
		} else {
			current = nil
		}
		s.directPoolMu.Unlock()
		if current != nil {
			_ = current.peer.Close()
		}
	})
}

func (s *Service) adoptInboundPeerSession(peer *PeerSession) {
	if peer == nil || peer.Data == nil {
		_ = peer.Close()
		return
	}
	if !peer.SessionReuse {
		_ = s.closePeerAfterTransfer(peer)
		return
	}
	if s.detachPeerControl(peer) != nil {
		_ = peer.Close()
		return
	}
	key := directPoolKey(peer.PeerID, peer.AuthorizationGeneration)
	keyLock := s.directPoolKeyLock(key)
	keyLock.Lock()
	defer keyLock.Unlock()
	if hook := s.directPoolBeforePublish; hook != nil {
		hook("adopt")
	}
	pooled := &pooledPeerSession{peer: peer, generation: peer.AuthorizationGeneration, serving: true}
	s.directPoolMu.Lock()
	if s.isClosing() {
		s.directPoolMu.Unlock()
		_ = peer.Close()
		return
	}
	if previous := s.directPool[key]; previous != nil && previous.peer != peer {
		if previous.peer.Data != nil && previous.peer.Data.Conn.Context().Err() == nil {
			s.directPoolMu.Unlock()
			_ = peer.Close()
			return
		}
		if previous.timer != nil {
			previous.timer.Stop()
		}
		_ = previous.peer.Close()
	}
	s.directPool[key] = pooled
	s.schedulePoolIdleLocked(key, pooled)
	s.startPooledPeerServerLocked(key, pooled)
	s.directPoolMu.Unlock()
}

func (s *Service) servePooledPeer(key string, pooled *pooledPeerSession) {
	for {
		stream, err := pooled.peer.Data.Conn.AcceptStream(pooled.peer.Data.Conn.Context())
		if err != nil {
			return
		}
		s.directPoolMu.Lock()
		if s.directPool[key] != pooled {
			s.directPoolMu.Unlock()
			transport.WrapStream(stream).Abort()
			return
		}
		pooled.receiving = true
		if pooled.timer != nil {
			pooled.timer.Stop()
			pooled.timer = nil
		}
		s.directPoolMu.Unlock()

		s.inbox.mu.Lock()
		directory := s.inbox.directory
		enabled := s.inbox.enabled
		s.inbox.mu.Unlock()
		wrapped := transport.WrapStream(stream)
		if !enabled || !s.receiveIncomingStream(pooled.peer.Data.Conn.Context(), pooled.peer, directory, wrapped) {
			wrapped.Abort()
		}
		s.directPoolMu.Lock()
		if s.directPool[key] != pooled {
			s.directPoolMu.Unlock()
			return
		}
		pooled.receiving = false
		s.schedulePoolIdleLocked(key, pooled)
		s.directPoolMu.Unlock()
	}
}

func (s *Service) removePeerSession(peer *PeerSession) {
	s.directPoolMu.Lock()
	for key, pooled := range s.directPool {
		if pooled.peer == peer {
			if pooled.timer != nil {
				pooled.timer.Stop()
			}
			delete(s.directPool, key)
		}
	}
	s.directPoolMu.Unlock()
}

func (s *Service) closePooledSessions(peerID string) {
	s.directPoolMu.Lock()
	var peers []*PeerSession
	for key, pooled := range s.directPool {
		if peerID == "" || pooled.peer.PeerID == peerID {
			if pooled.timer != nil {
				pooled.timer.Stop()
			}
			peers = append(peers, pooled.peer)
			delete(s.directPool, key)
		}
	}
	s.directPoolMu.Unlock()
	for _, peer := range peers {
		_ = peer.Close()
	}
}

func (s *Service) closeIdlePooledSessions() {
	s.directPoolMu.Lock()
	var peers []*PeerSession
	for key, pooled := range s.directPool {
		if !pooled.inUse && !pooled.receiving {
			if pooled.timer != nil {
				pooled.timer.Stop()
			}
			peers = append(peers, (pooled.peer))
			delete(s.directPool, key)
		}
	}
	s.directPoolMu.Unlock()
	for _, peer := range peers {
		_ = peer.Close()
	}
}
