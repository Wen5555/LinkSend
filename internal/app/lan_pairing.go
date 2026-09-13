package app

import (
	"context"
	"errors"
	"net"
	"sort"
	"sync"
	"time"

	"github.com/Wen5555/LinkSend/internal/discovery"
	"github.com/Wen5555/LinkSend/internal/identity"
	"github.com/Wen5555/LinkSend/internal/protocol"
	"github.com/Wen5555/LinkSend/internal/signaling"
)

type LANPairRequestInfo struct {
	RequestID string `json:"request_id"`
	PeerID    string `json:"peer_id"`
	PeerName  string `json:"peer_name"`
	ExpiresAt string `json:"expires_at"`
}

type LANPairResult struct {
	PeerID      string `json:"peer_id"`
	State       string `json:"state"`
	ServerState string `json:"server_state"`
}

type pendingLANPair struct {
	info     LANPairRequestInfo
	decision chan bool
}

type lanPairCoordinator struct {
	mu      sync.Mutex
	pending map[string]*pendingLANPair
}

type prefetchedSignalSession struct {
	directSignalSession
	first *signaling.Wire
}

func (s *prefetchedSignalSession) Read(ctx context.Context) (signaling.Wire, error) {
	if s.first != nil {
		first := *s.first
		s.first = nil
		return first, nil
	}
	return s.directSignalSession.Read(ctx)
}

func newLANPairFrame(phase, requestID, nonce, sender, recipient string, generation uint64, expiresAt int64) signaling.LANPairFrame {
	return signaling.LANPairFrame{Version: 2, Phase: phase, RequestID: requestID, Nonce: nonce, Sender: sender, Recipient: recipient, Generation: generation, IssuedAt: time.Now().Unix(), ExpiresAt: expiresAt}
}

func (s *Service) PendingLANPairings() []LANPairRequestInfo {
	s.lanPair.mu.Lock()
	defer s.lanPair.mu.Unlock()
	out := make([]LANPairRequestInfo, 0, len(s.lanPair.pending))
	for _, pending := range s.lanPair.pending {
		out = append(out, pending.info)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ExpiresAt < out[j].ExpiresAt })
	return out
}

func (s *Service) RespondLANPair(requestID string, accept bool) error {
	s.lanPair.mu.Lock()
	pending := s.lanPair.pending[requestID]
	s.lanPair.mu.Unlock()
	if pending == nil {
		return errors.New("LAN_PAIR_REQUEST_NOT_FOUND")
	}
	select {
	case pending.decision <- accept:
		return nil
	default:
		return errors.New("LAN_PAIR_ALREADY_DECIDED")
	}
}

func (s *Service) RequestLANPair(ctx context.Context, peerID string) (LANPairResult, error) {
	manager := s.lanManager()
	if manager == nil {
		return LANPairResult{}, protocol.Fail(protocol.LANDiscoveryUnavailable, "LAN discovery is not running")
	}
	generation, err := identity.LANPairGeneration(s.cfg.DataDir, peerID)
	if err != nil {
		return LANPairResult{}, err
	}
	session, peer, route, err := manager.Dial(ctx, peerID)
	if err != nil {
		return LANPairResult{}, err
	}
	defer session.Close()
	requestID, nonce := protocol.RandomID(), protocol.RandomID()
	expires := time.Now().Add(60 * time.Second).Unix()
	if err = session.SendLANPair(ctx, newLANPairFrame("request", requestID, nonce, s.identity.ID(), peerID, generation, expires)); err != nil {
		return LANPairResult{}, err
	}
	wire, err := session.Read(ctx)
	if err != nil {
		return LANPairResult{}, err
	}
	if wire.Type != "lan_pair" || wire.LANPair == nil {
		return LANPairResult{}, protocol.Fail(protocol.InvalidMessage, "LAN pairing response missing")
	}
	response := *wire.LANPair
	if err = response.Verify(peer.PublicKey, s.identity.ID(), time.Now()); err != nil || response.RequestID != requestID || response.Nonce != nonce {
		return LANPairResult{}, protocol.Fail(protocol.AuthenticationFailed, "LAN pairing response transcript mismatch")
	}
	if response.Phase == "reject" {
		return LANPairResult{PeerID: peerID, State: "rejected", ServerState: "not_joined"}, protocol.Fail(protocol.Unpaired, "LAN pairing rejected")
	}
	if response.Phase != "accept" {
		return LANPairResult{}, protocol.Fail(protocol.InvalidMessage, "LAN pairing accept missing")
	}
	if err = session.SendLANPair(ctx, newLANPairFrame("commit", requestID, nonce, s.identity.ID(), peerID, generation, expires)); err != nil {
		return LANPairResult{}, err
	}
	wire, err = session.Read(ctx)
	if err != nil {
		return LANPairResult{}, err
	}
	if wire.Type != "lan_pair" || wire.LANPair == nil || wire.LANPair.Phase != "ack" || wire.LANPair.RequestID != requestID || wire.LANPair.Nonce != nonce {
		return LANPairResult{}, protocol.Fail(protocol.InvalidMessage, "LAN pairing acknowledgement missing")
	}
	if err = wire.LANPair.Verify(peer.PublicKey, s.identity.ID(), time.Now()); err != nil {
		return LANPairResult{}, err
	}
	if err = identity.CommitLANPeer(s.cfg.DataDir, identity.TrustedPeer{ID: peer.ID, Name: peer.Name, PublicKey: peer.PublicKey, LastLANAddress: s.lanRememberAddress(route.RemoteAddress)}, generation); err != nil {
		return LANPairResult{}, err
	}
	return LANPairResult{PeerID: peerID, State: "lan_paired", ServerState: "not_joined"}, nil
}

func (s *Service) handleIncomingLANPair(ctx context.Context, incoming discovery.Incoming, request signaling.LANPairFrame) {
	defer incoming.Session.Close()
	if request.Phase != "request" || request.Verify(incoming.Peer.PublicKey, s.identity.ID(), time.Now()) != nil {
		return
	}
	generation, err := identity.LANPairGeneration(s.cfg.DataDir, incoming.Peer.ID)
	if err != nil {
		return
	}
	pending := &pendingLANPair{info: LANPairRequestInfo{RequestID: request.RequestID, PeerID: incoming.Peer.ID, PeerName: incoming.Peer.Name, ExpiresAt: time.Unix(request.ExpiresAt, 0).UTC().Format(time.RFC3339)}, decision: make(chan bool, 1)}
	s.lanPair.mu.Lock()
	if s.lanPair.pending == nil {
		s.lanPair.pending = map[string]*pendingLANPair{}
	}
	if s.lanPair.pending[request.RequestID] != nil {
		s.lanPair.mu.Unlock()
		return
	}
	s.lanPair.pending[request.RequestID] = pending
	s.lanPair.mu.Unlock()
	defer func() { s.lanPair.mu.Lock(); delete(s.lanPair.pending, request.RequestID); s.lanPair.mu.Unlock() }()
	deadline := time.Until(time.Unix(request.ExpiresAt, 0))
	if deadline <= 0 {
		return
	}
	timer := time.NewTimer(deadline)
	defer timer.Stop()
	accepted := false
	select {
	case accepted = <-pending.decision:
	case <-timer.C:
		return
	case <-ctx.Done():
		return
	}
	phase := "reject"
	if accepted {
		phase = "accept"
	}
	if err = incoming.Session.SendLANPair(ctx, newLANPairFrame(phase, request.RequestID, request.Nonce, s.identity.ID(), incoming.Peer.ID, generation, request.ExpiresAt)); err != nil || !accepted {
		return
	}
	wire, err := incoming.Session.Read(ctx)
	if err != nil || wire.Type != "lan_pair" || wire.LANPair == nil {
		return
	}
	commit := *wire.LANPair
	if commit.Phase != "commit" || commit.RequestID != request.RequestID || commit.Nonce != request.Nonce || commit.Generation != request.Generation || commit.Verify(incoming.Peer.PublicKey, s.identity.ID(), time.Now()) != nil {
		return
	}
	if err = identity.CommitLANPeer(s.cfg.DataDir, identity.TrustedPeer{ID: incoming.Peer.ID, Name: incoming.Peer.Name, PublicKey: incoming.Peer.PublicKey, LastLANAddress: s.lanRememberAddress(incoming.Route.RemoteAddress)}, generation); err != nil {
		return
	}
	_ = incoming.Session.SendLANPair(ctx, newLANPairFrame("ack", request.RequestID, request.Nonce, s.identity.ID(), incoming.Peer.ID, generation, request.ExpiresAt))
}

func (s *Service) lanRememberAddress(address string) string {
	if s.cfg.AllowInsecureLoopback && net.ParseIP(address).IsLoopback() {
		return ""
	}
	return address
}
