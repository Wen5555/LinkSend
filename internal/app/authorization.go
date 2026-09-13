package app

import (
	"context"
	"errors"
	"time"

	"github.com/Wen5555/LinkSend/internal/identity"
	"github.com/Wen5555/LinkSend/internal/protocol"
)

func (s *Service) currentAuthorizationGeneration(peerID string) (uint64, error) {
	generation, err := identity.AuthorizationGeneration(s.cfg.DataDir, peerID)
	if err == nil {
		return generation, nil
	}
	c, clientErr := s.client()
	if clientErr != nil {
		return 0, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	devices, clientErr := c.Devices(ctx)
	if clientErr != nil {
		return 0, err
	}
	if clientErr = s.syncPairedDevices(devices); clientErr != nil {
		return 0, clientErr
	}
	return identity.AuthorizationGeneration(s.cfg.DataDir, peerID)
}

// checkPeerAllowed is shared by LAN, signaling, consent and restart recovery.
// A read failure is an authorization failure, never permission to re-pair.
func (s *Service) checkPeerAllowed(peerID string) error {
	if err := identity.CheckPeerAllowed(s.cfg.DataDir, peerID); err != nil {
		return protocol.Wrap(protocol.AuthenticationFailed, "local peer authorization denied", err)
	}
	return nil
}

func (s *Service) checkTaskGrant(snapshot TaskSnapshot) error {
	if err := s.checkPeerAllowed(snapshot.PeerID); err != nil {
		return err
	}
	generation, err := identity.AuthorizationGeneration(s.cfg.DataDir, snapshot.PeerID)
	if err != nil || snapshot.AuthorizationGeneration == 0 || generation != snapshot.AuthorizationGeneration {
		return protocol.Wrap(protocol.AuthenticationFailed, "task belongs to an older peer authorization", err)
	}
	return nil
}

func (s *Service) checkAuthorizationGeneration(peerID string, expected uint64) error {
	current, err := identity.AuthorizationGeneration(s.cfg.DataDir, peerID)
	if err != nil || expected == 0 || current != expected {
		return protocol.Wrap(protocol.AuthenticationFailed, "peer authorization generation changed", err)
	}
	return nil
}

func (s *Service) cancelPeerTasks(peerID string) {
	for _, task := range s.Tasks() {
		if task.PeerID == peerID && !isTerminal(task.State) {
			_ = s.CancelTask(task.ID)
		}
	}
}

// BlockPeer is local and works even when the membership server is unavailable.
// Persist the denial before stopping tasks so discovery cannot restore the pin.
func (s *Service) BlockPeer(peerID string) error {
	done, workErr := s.beginProfileWork()
	if workErr != nil {
		return workErr
	}
	defer done()
	if peerID == s.identity.ID() {
		return errors.New("INVALID_ARGUMENT: cannot block this device")
	}
	s.trustMu.Lock()
	err := identity.RevokePeer(s.cfg.DataDir, peerID)
	s.trustMu.Unlock()
	if err != nil {
		return err
	}
	grantErr := s.clearClipboardGrants(peerID)
	s.closePooledSessions(peerID)
	s.cancelPeerTasks(peerID)
	return grantErr
}

// UnblockPeer removes only the local denial. Pairing/explicit LAN consent is
// still required; neither the previous pin nor auto-accept is restored here.
func (s *Service) UnblockPeer(peerID string) error {
	done, workErr := s.beginProfileWork()
	if workErr != nil {
		return workErr
	}
	defer done()
	s.trustMu.Lock()
	defer s.trustMu.Unlock()
	return identity.AllowPeer(s.cfg.DataDir, peerID)
}
