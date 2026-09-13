package app

import (
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/Wen5555/LinkSend/internal/connectivity"
	"github.com/Wen5555/LinkSend/internal/discovery"
	"github.com/Wen5555/LinkSend/internal/identity"
	"github.com/Wen5555/LinkSend/internal/signaling"
)

type lanRuntime struct {
	manager        *discovery.Manager
	cancel         context.CancelFunc
	done           chan struct{}
	mu             sync.RWMutex
	directory      string
	cfg            DirectConfig
	restartPending bool
}

func (s *Service) startLANDiscovery(directory string, cfg DirectConfig) {
	s.lanLifecycleMu.Lock()
	defer s.lanLifecycleMu.Unlock()
	if s.isClosing() {
		return
	}
	s.lanMu.RLock()
	current := s.lan
	s.lanMu.RUnlock()
	if current != nil && s.tasks.hasActive() {
		current.mu.Lock()
		current.directory = directory
		current.cfg = cfg
		current.restartPending = true
		current.mu.Unlock()
		return
	}
	_ = s.stopLANDiscoveryOwned()
	manager, err := discovery.Start(discovery.Config{Identity: s.identity, Name: s.name(""), ExcludedInterfaces: cfg.ExcludedInterfaces, AllowLoopback: cfg.AllowLoopback || s.cfg.AllowInsecureLoopback})
	if err != nil {
		s.lanMu.Lock()
		s.lanError = safeLANError(err)
		s.lanMu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	runtime := &lanRuntime{manager: manager, cancel: cancel, done: make(chan struct{}), directory: directory, cfg: cfg}
	s.lanMu.Lock()
	s.lan = runtime
	s.lanError = ""
	s.lanMu.Unlock()
	s.listenerWorkers.Add(1)
	go func() {
		defer s.listenerWorkers.Done()
		s.runLANDiscovery(ctx, runtime)
	}()
	if peers, loadErr := identity.LoadTrust(s.cfg.DataDir); loadErr == nil {
		for _, peer := range peers {
			addresses := append([]string{}, peer.LANAddressHistory...)
			if peer.LastLANAddress != "" {
				addresses = append(addresses, peer.LastLANAddress)
			}
			seen := map[string]bool{}
			for _, address := range addresses {
				if !seen[address] {
					_ = manager.RememberAddress(address)
					seen[address] = true
				}
			}
		}
	}
}

func (s *Service) runLANDiscovery(ctx context.Context, runtime *lanRuntime) {
	defer close(runtime.done)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	lastNetwork := networkSnapshotKey()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			current := networkSnapshotKey()
			if current != "" && current != lastNetwork {
				lastNetwork = current
				_ = s.NetworkChanged("network")
			}
		case incoming, ok := <-runtime.manager.Incoming():
			if !ok {
				return
			}
			if s.tasks.hasActive() {
				_ = incoming.Session.Close()
				continue
			}
			runtime.mu.RLock()
			directory, cfg := runtime.directory, runtime.cfg
			runtime.mu.RUnlock()
			s.operationMu.Lock()
			if s.isClosing() {
				s.operationMu.Unlock()
				_ = incoming.Session.Close()
				continue
			}
			s.tasks.workers.Add(1)
			s.operationMu.Unlock()
			go func() {
				defer s.tasks.workers.Done()
				s.receiveLANSession(ctx, incoming, directory, cfg)
			}()
		}
	}
}

func networkSnapshotKey() string {
	addresses, err := connectivity.DiscoverInterfaceAddresses(true)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%v", addresses)
}

func (s *Service) ensureLANDiscovery() {
	s.lanMu.RLock()
	runtime := s.lan
	s.lanMu.RUnlock()
	if runtime != nil {
		runtime.mu.RLock()
		pending, directory, cfg := runtime.restartPending, runtime.directory, runtime.cfg
		runtime.mu.RUnlock()
		if !pending {
			return
		}
		s.startLANDiscovery(directory, cfg)
		return
	}
	s.inbox.mu.Lock()
	enabled, directory, cfg := s.inbox.enabled, s.inbox.directory, s.inbox.cfg
	s.inbox.mu.Unlock()
	if enabled && directory != "" && !s.tasks.hasActive() {
		s.startLANDiscovery(directory, cfg)
	}
}

func (s *Service) receiveLANSession(ctx context.Context, incoming discovery.Incoming, directory string, cfg DirectConfig) {
	readCtx, cancelRead := context.WithTimeout(ctx, 5*time.Second)
	first, err := incoming.Session.Read(readCtx)
	cancelRead()
	if err != nil {
		_ = incoming.Session.Close()
		return
	}
	if first.Type == "lan_pair" && first.LANPair != nil {
		s.handleIncomingLANPair(ctx, incoming, *first.LANPair)
		return
	}
	if err := s.checkPeerAllowed(incoming.Peer.ID); err != nil {
		_ = incoming.Session.Close()
		return
	}
	cfg.knownInterface = incoming.Route.Interface
	cfg.BindAddress = net.JoinHostPort(incoming.Route.LocalAddress, "0")
	peer := signaling.Device{ID: incoming.Peer.ID, Name: incoming.Peer.Name, PublicKey: ed25519.PublicKey(incoming.Peer.PublicKey)}
	session, err := s.acceptDirectOnSessionPeer(ctx, "", cfg, &prefetchedSignalSession{directSignalSession: incoming.Session, first: &first}, &peer)
	if err != nil {
		_ = incoming.Session.Close()
		return
	}
	session.localPeer = true
	session.peerName = peer.Name
	session.peerPublicKey = append(ed25519.PublicKey(nil), peer.PublicKey...)
	session.lanAddress = incoming.Route.RemoteAddress
	if s.receiveIncoming(ctx, session, directory) {
		_ = s.closePeerAfterTransfer(session)
	} else {
		_ = session.Close()
	}
}

func (s *Service) stopLANDiscovery() error {
	s.lanLifecycleMu.Lock()
	defer s.lanLifecycleMu.Unlock()
	return s.stopLANDiscoveryOwned()
}

func (s *Service) stopLANDiscoveryOwned() error {
	s.lanMu.Lock()
	runtime := s.lan
	s.lan = nil
	s.lanMu.Unlock()
	if runtime == nil {
		return nil
	}
	runtime.cancel()
	err := runtime.manager.Close()
	select {
	case <-runtime.done:
		return err
	case <-time.After(5 * time.Second):
		return errors.Join(err, errors.New("LAN_DISCOVERY_STOP_TIMEOUT"))
	}
}

func (s *Service) lanManager() *discovery.Manager {
	s.lanMu.RLock()
	defer s.lanMu.RUnlock()
	if s.lan == nil {
		return nil
	}
	return s.lan.manager
}

func (s *Service) lanPeers() []discovery.Device {
	manager := s.lanManager()
	if manager == nil {
		return nil
	}
	return manager.Peers()
}

func (s *Service) lanStatus() (bool, int, string) {
	s.lanMu.RLock()
	runtime, lastError := s.lan, s.lanError
	s.lanMu.RUnlock()
	if runtime == nil {
		return false, 0, lastError
	}
	return true, len(runtime.manager.Peers()), lastError
}

func (s *Service) RefreshLANDiscovery() error {
	manager := s.lanManager()
	if manager == nil {
		return errors.New("LAN_DISCOVERY_UNAVAILABLE: local discovery is not running")
	}
	return manager.Refresh()
}

// NetworkChanged is the backend entrypoint for OS network/sleep notifications
// and snapshot polling. It invalidates future endpoint selection and refreshes
// discovery without cancelling a healthy active QUIC task.
func (s *Service) NetworkChanged(reason string) error {
	switch reason {
	case "network", "wake", "interface", "address", "route":
	default:
		return errors.New("INVALID_ARGUMENT: unknown network change reason")
	}
	s.recoveryMu.Lock()
	if time.Since(s.lastNetworkChange) < 250*time.Millisecond {
		s.recoveryMu.Unlock()
		return nil
	}
	s.lastNetworkChange = time.Now()
	s.recoveryMu.Unlock()
	s.networkMu.Lock()
	s.network = cachedNetworkSelection{}
	s.networkMu.Unlock()
	manager := s.lanManager()
	if manager == nil {
		s.ensureLANDiscovery()
		return nil
	}
	return manager.Refresh()
}

func (s *Service) ProbeLANAddress(address string) error {
	manager := s.lanManager()
	if manager == nil {
		return errors.New("LAN_DISCOVERY_UNAVAILABLE: local discovery is not running")
	}
	return manager.ProbeAddress(address)
}

func (s *Service) rememberLANPeer(peer *PeerSession) error {
	if peer == nil || !peer.localPeer || len(peer.peerPublicKey) != ed25519.PublicKeySize || identity.DeviceID(peer.peerPublicKey) != peer.PeerID {
		return nil
	}
	s.trustMu.Lock()
	defer s.trustMu.Unlock()
	return identity.TrustPairedPeer(s.cfg.DataDir, identity.TrustedPeer{ID: peer.PeerID, Name: peer.peerName, PublicKey: peer.peerPublicKey, LastLANAddress: peer.lanAddress})
}

func lanRemoteAddress(session directSignalSession) string {
	if local, ok := session.(*discovery.Session); ok {
		return local.RemoteAddress()
	}
	return ""
}

func safeLANError(err error) string {
	if err == nil {
		return ""
	}
	return "LAN_DISCOVERY_UNAVAILABLE"
}
