package app

import (
	"errors"

	"github.com/Wen5555/LinkSend/internal/identity"
	"github.com/Wen5555/LinkSend/internal/protocol"
)

// checkPeerAllowed is shared by LAN, signaling, consent and restart recovery.
// A read failure is an authorization failure, never permission to re-pair.
func (s *Service) checkPeerAllowed(peerID string) error {
	if err := identity.CheckPeerAllowed(s.cfg.DataDir, peerID); err != nil {
		return protocol.Wrap(protocol.AuthenticationFailed, "local peer authorization denied", err)
	}
	return nil
}

// BlockPeer is local and works even when the membership server is unavailable.
// Persist the denial before stopping tasks so discovery cannot restore the pin.
func (s *Service) BlockPeer(peerID string) error {
	if peerID == s.identity.ID() {
		return errors.New("INVALID_ARGUMENT: cannot block this device")
	}
	s.trustMu.Lock()
	err := identity.RevokePeer(s.cfg.DataDir, peerID)
	s.trustMu.Unlock()
	if err != nil {
		return err
	}
	for _, task := range s.Tasks() {
		if task.PeerID == peerID && !isTerminal(task.State) {
			_ = s.CancelTask(task.ID)
		}
	}
	return nil
}

// UnblockPeer removes only the local denial. Pairing/explicit LAN consent is
// still required; neither the previous pin nor auto-accept is restored here.
func (s *Service) UnblockPeer(peerID string) error {
	s.trustMu.Lock()
	defer s.trustMu.Unlock()
	return identity.AllowPeer(s.cfg.DataDir, peerID)
}
