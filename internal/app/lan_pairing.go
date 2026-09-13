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
	mu       sync.Mutex
	pending  map[string]*pendingLANPair
	outgoing map[string]string
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
	if pending, ok, pendingErr := identity.PendingProvisionalLAN(s.cfg.DataDir, peerID); pendingErr == nil && ok {
		if s.recoverLANPair(ctx, manager, peer, pending.RequestID, pending.Nonce, pending.Generation, pending.ExpiresAt.Unix()) {
			return LANPairResult{PeerID: peerID, State: "lan_paired", ServerState: "pending"}, nil
		}
	}
	requestID, nonce := protocol.RandomID(), protocol.RandomID()
	s.lanPair.mu.Lock()
	if s.lanPair.outgoing == nil {
		s.lanPair.outgoing = map[string]string{}
	}
	if _, exists := s.lanPair.outgoing[peerID]; exists {
		s.lanPair.mu.Unlock()
		return LANPairResult{}, errors.New("LAN_PAIR_ALREADY_PENDING")
	}
	s.lanPair.outgoing[peerID] = requestID
	s.lanPair.mu.Unlock()
	defer func() { s.lanPair.mu.Lock(); delete(s.lanPair.outgoing, peerID); s.lanPair.mu.Unlock() }()
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
	peerGrant := identity.TrustedPeer{ID: peer.ID, Name: peer.Name, PublicKey: peer.PublicKey, LastLANAddress: s.lanRememberAddress(route.RemoteAddress)}
	if err = identity.BeginProvisionalLAN(s.cfg.DataDir, identity.ProvisionalLANGrant{RequestID: requestID, Nonce: nonce, Peer: peerGrant, Generation: generation, State: "accepted", ExpiresAt: time.Unix(expires, 0).UTC()}); err != nil {
		return LANPairResult{}, err
	}
	if err = session.SendLANPair(ctx, newLANPairFrame("commit", requestID, nonce, s.identity.ID(), peerID, generation, expires)); err != nil {
		return LANPairResult{}, err
	}
	wire, err = session.Read(ctx)
	if err != nil {
		if s.recoverLANPair(ctx, manager, peer, requestID, nonce, generation, expires) {
			return LANPairResult{PeerID: peerID, State: "lan_paired", ServerState: "pending"}, nil
		}
		return LANPairResult{}, err
	}
	if wire.Type != "lan_pair" || wire.LANPair == nil || wire.LANPair.Phase != "ready" || wire.LANPair.RequestID != requestID || wire.LANPair.Nonce != nonce {
		return LANPairResult{}, protocol.Fail(protocol.InvalidMessage, "LAN pairing ready acknowledgement missing")
	}
	if err = wire.LANPair.Verify(peer.PublicKey, s.identity.ID(), time.Now()); err != nil {
		return LANPairResult{}, err
	}
	if err = session.SendLANPair(ctx, newLANPairFrame("confirm", requestID, nonce, s.identity.ID(), peerID, generation, expires)); err != nil {
		if s.recoverLANPair(ctx, manager, peer, requestID, nonce, generation, expires) {
			return LANPairResult{PeerID: peerID, State: "lan_paired", ServerState: "pending"}, nil
		}
		return LANPairResult{}, err
	}
	wire, err = session.Read(ctx)
	if err != nil || wire.Type != "lan_pair" || wire.LANPair == nil || wire.LANPair.Phase != "done" || wire.LANPair.RequestID != requestID || wire.LANPair.Nonce != nonce {
		if err != nil && s.recoverLANPair(ctx, manager, peer, requestID, nonce, generation, expires) {
			return LANPairResult{PeerID: peerID, State: "lan_paired", ServerState: "pending"}, nil
		}
		return LANPairResult{}, protocol.Fail(protocol.InvalidMessage, "LAN pairing completion acknowledgement missing")
	}
	if err = wire.LANPair.Verify(peer.PublicKey, s.identity.ID(), time.Now()); err != nil {
		return LANPairResult{}, err
	}
	if err = identity.CommitProvisionalLAN(s.cfg.DataDir, requestID, nonce); err != nil {
		return LANPairResult{}, err
	}
	serverState := "not_joined"
	credentialCtx, cancelCredential := context.WithTimeout(ctx, 500*time.Millisecond)
	credentialWire, credentialErr := session.Read(credentialCtx)
	cancelCredential()
	if credentialErr == nil && credentialWire.Type == "lan_pair" && credentialWire.LANPair != nil && credentialWire.LANPair.Phase == "credential" && credentialWire.LANPair.RequestID == requestID && credentialWire.LANPair.Nonce == nonce && credentialWire.LANPair.Verify(peer.PublicKey, s.identity.ID(), time.Now()) == nil {
		if c, clientErr := s.client(); clientErr == nil {
			if _, joinErr := c.Join(ctx, credentialWire.LANPair.Credential, s.name("")); joinErr == nil {
				if devices, listErr := c.Devices(ctx); listErr == nil {
					_ = s.syncPairedDevices(devices)
					serverState = "joined"
				}
			} else if protocol.ErrorCode(joinErr) == protocol.PairingIdentityConflict {
				serverState = "switch_required"
			} else {
				serverState = "pending"
			}
		}
	}
	return LANPairResult{PeerID: peerID, State: "lan_paired", ServerState: serverState}, nil
}

func (s *Service) recoverLANPair(ctx context.Context, manager *discovery.Manager, peer discovery.Device, requestID, nonce string, generation uint64, expires int64) bool {
	if time.Now().Unix() >= expires {
		return false
	}
	session, _, _, err := manager.Dial(ctx, peer.ID)
	if err != nil {
		return false
	}
	defer session.Close()
	if err = session.SendLANPair(ctx, newLANPairFrame("query", requestID, nonce, s.identity.ID(), peer.ID, generation, expires)); err != nil {
		return false
	}
	wire, err := session.Read(ctx)
	if err != nil || wire.Type != "lan_pair" || wire.LANPair == nil || wire.LANPair.RequestID != requestID || wire.LANPair.Nonce != nonce || wire.LANPair.Verify(peer.PublicKey, s.identity.ID(), time.Now()) != nil {
		return false
	}
	if wire.LANPair.Phase == "done" {
		if status, _ := identity.LANPairStatus(s.cfg.DataDir, peer.ID, requestID, nonce); status != "done" {
			if identity.CommitProvisionalLAN(s.cfg.DataDir, requestID, nonce) != nil {
				return false
			}
		}
		return true
	}
	if wire.LANPair.Phase != "ready" {
		return false
	}
	if status, _ := identity.LANPairStatus(s.cfg.DataDir, peer.ID, requestID, nonce); status != "done" {
		if identity.CommitProvisionalLAN(s.cfg.DataDir, requestID, nonce) != nil {
			return false
		}
	}
	if err = session.SendLANPair(ctx, newLANPairFrame("confirm", requestID, nonce, s.identity.ID(), peer.ID, generation, expires)); err != nil {
		return false
	}
	wire, err = session.Read(ctx)
	return err == nil && wire.Type == "lan_pair" && wire.LANPair != nil && wire.LANPair.Phase == "done" && wire.LANPair.RequestID == requestID && wire.LANPair.Nonce == nonce && wire.LANPair.Verify(peer.PublicKey, s.identity.ID(), time.Now()) == nil
}

func (s *Service) handleIncomingLANPair(ctx context.Context, incoming discovery.Incoming, request signaling.LANPairFrame) {
	defer incoming.Session.Close()
	if request.Phase == "query" {
		if request.Verify(incoming.Peer.PublicKey, s.identity.ID(), time.Now()) != nil {
			return
		}
		status, statusErr := identity.LANPairStatus(s.cfg.DataDir, incoming.Peer.ID, request.RequestID, request.Nonce)
		if statusErr != nil || status == "unknown" {
			return
		}
		if err := incoming.Session.SendLANPair(ctx, newLANPairFrame(status, request.RequestID, request.Nonce, s.identity.ID(), incoming.Peer.ID, request.Generation, request.ExpiresAt)); err != nil || status == "done" {
			if err == nil && status == "done" {
				s.sendRecoveredLANCredential(ctx, incoming, request)
			}
			return
		}
		wire, err := incoming.Session.Read(ctx)
		if err != nil || wire.Type != "lan_pair" || wire.LANPair == nil || wire.LANPair.Phase != "confirm" || wire.LANPair.RequestID != request.RequestID || wire.LANPair.Nonce != request.Nonce || wire.LANPair.Verify(incoming.Peer.PublicKey, s.identity.ID(), time.Now()) != nil {
			return
		}
		if err = identity.CommitProvisionalLAN(s.cfg.DataDir, request.RequestID, request.Nonce); err != nil {
			return
		}
		_ = incoming.Session.SendLANPair(ctx, newLANPairFrame("done", request.RequestID, request.Nonce, s.identity.ID(), incoming.Peer.ID, request.Generation, request.ExpiresAt))
		s.sendRecoveredLANCredential(ctx, incoming, request)
		return
	}
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
	if len(s.lanPair.pending) >= 64 {
		s.lanPair.mu.Unlock()
		return
	}
	outgoingID := s.lanPair.outgoing[incoming.Peer.ID]
	if outgoingID != "" && outgoingID < request.RequestID {
		s.lanPair.mu.Unlock()
		_ = incoming.Session.SendLANPair(ctx, newLANPairFrame("reject", request.RequestID, request.Nonce, s.identity.ID(), incoming.Peer.ID, generation, request.ExpiresAt))
		return
	}
	s.lanPair.pending[request.RequestID] = pending
	s.lanPair.mu.Unlock()
	if outgoingID != "" {
		pending.decision <- true
	}
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
	credential := ""
	if accepted {
		phase = "accept"
		peerGrant := identity.TrustedPeer{ID: incoming.Peer.ID, Name: incoming.Peer.Name, PublicKey: incoming.Peer.PublicKey, LastLANAddress: s.lanRememberAddress(incoming.Route.RemoteAddress)}
		if err = identity.BeginProvisionalLAN(s.cfg.DataDir, identity.ProvisionalLANGrant{RequestID: request.RequestID, Nonce: request.Nonce, Peer: peerGrant, Generation: generation, State: "accepted", ExpiresAt: time.Unix(request.ExpiresAt, 0).UTC()}); err != nil {
			return
		}
	}
	approval := newLANPairFrame(phase, request.RequestID, request.Nonce, s.identity.ID(), incoming.Peer.ID, generation, request.ExpiresAt)
	approval.Signature = s.identity.Sign(approval.SigningBytes())
	if accepted {
		if c, clientErr := s.client(); clientErr == nil {
			if invitation, credentialErr := c.CreateLANCredential(ctx, incoming.Peer.PublicKey, request, approval); credentialErr == nil {
				credential = invitation.Token
				_ = identity.SetProvisionalLANCredential(s.cfg.DataDir, request.RequestID, request.Nonce, credential)
			}
		}
	}
	if err = incoming.Session.SendLANPair(ctx, approval); err != nil || !accepted {
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
	if err = incoming.Session.SendLANPair(ctx, newLANPairFrame("ready", request.RequestID, request.Nonce, s.identity.ID(), incoming.Peer.ID, generation, request.ExpiresAt)); err != nil {
		return
	}
	wire, err = incoming.Session.Read(ctx)
	if err != nil || wire.Type != "lan_pair" || wire.LANPair == nil || wire.LANPair.Phase != "confirm" || wire.LANPair.RequestID != request.RequestID || wire.LANPair.Nonce != request.Nonce {
		return
	}
	if err = wire.LANPair.Verify(incoming.Peer.PublicKey, s.identity.ID(), time.Now()); err != nil {
		return
	}
	if err = identity.CommitProvisionalLAN(s.cfg.DataDir, request.RequestID, request.Nonce); err != nil {
		return
	}
	if err = incoming.Session.SendLANPair(ctx, newLANPairFrame("done", request.RequestID, request.Nonce, s.identity.ID(), incoming.Peer.ID, generation, request.ExpiresAt)); err != nil {
		return
	}
	if credential != "" {
		frame := newLANPairFrame("credential", request.RequestID, request.Nonce, s.identity.ID(), incoming.Peer.ID, generation, request.ExpiresAt)
		frame.Credential = credential
		_ = incoming.Session.SendLANPair(ctx, frame)
	}
}

func (s *Service) sendRecoveredLANCredential(ctx context.Context, incoming discovery.Incoming, request signaling.LANPairFrame) {
	credential, err := identity.LANPairCredential(s.cfg.DataDir, incoming.Peer.ID, request.RequestID, request.Nonce)
	if err != nil || credential == "" {
		return
	}
	frame := newLANPairFrame("credential", request.RequestID, request.Nonce, s.identity.ID(), incoming.Peer.ID, request.Generation, request.ExpiresAt)
	frame.Credential = credential
	_ = incoming.Session.SendLANPair(ctx, frame)
}

func (s *Service) lanRememberAddress(address string) string {
	if s.cfg.AllowInsecureLoopback && net.ParseIP(address).IsLoopback() {
		return ""
	}
	return address
}
