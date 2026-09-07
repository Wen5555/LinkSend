package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"example.com/linksend/internal/connectivity"
	"example.com/linksend/internal/identity"
	"example.com/linksend/internal/protocol"
	"example.com/linksend/internal/signaling"
	"example.com/linksend/internal/transfer"
	"example.com/linksend/internal/transport"
)

const firstGeneration uint64 = 1

type DirectConfig struct {
	BindAddress   string
	STUNURLs      []string
	AllowLoopback bool
	CheckTimeout  time.Duration
}

type PeerSession struct {
	PeerID        string
	Path          connectivity.Path
	Data          *transport.Session
	signal        *signaling.Session
	stopHeartbeat context.CancelFunc
}

func (p *PeerSession) Close() error {
	if p == nil {
		return nil
	}
	var err error
	if p.stopHeartbeat != nil {
		p.stopHeartbeat()
	}
	if p.signal != nil {
		err = errors.Join(err, p.signal.Close())
	}
	if p.Data != nil {
		err = errors.Join(err, p.Data.Close())
	}
	return err
}

func (s *Service) ConnectDirect(ctx context.Context, peerID string, cfg DirectConfig) (*PeerSession, error) {
	peer, err := s.trustedDevice(ctx, peerID)
	if err != nil {
		return nil, err
	}
	c, err := s.client()
	if err != nil {
		return nil, err
	}
	signalSession, err := c.Connect(ctx)
	if err != nil {
		return nil, err
	}
	closeSignal := true
	stopHeartbeat := startHeartbeat(ctx, signalSession)
	defer func() {
		if closeSignal {
			stopHeartbeat()
		}
	}()
	defer func() {
		if closeSignal {
			_ = signalSession.Close()
		}
	}()
	endpoint, err := s.newEndpoint(cfg)
	if err != nil {
		return nil, err
	}
	closeEndpoint := true
	defer func() {
		if closeEndpoint {
			_ = endpoint.Close()
		}
	}()
	if err = endpoint.Gather(); err != nil {
		return nil, fmt.Errorf("GATHERING: %w", err)
	}
	localCandidates := collectCandidates(endpoint)
	if len(localCandidates) == 0 {
		return nil, protocol.Fail(protocol.NoCandidates, "local endpoint gathered no usable candidates")
	}
	sessionID := protocol.RandomID()
	request, err := protocol.NewEnvelope("connect_request", s.identity.ID(), peer.ID, sessionID, firstGeneration, iceDescription{Ufrag: endpoint.Credentials().Ufrag, Password: endpoint.Credentials().Password})
	if err != nil {
		return nil, err
	}
	if err = signalSession.SendEnvelope(ctx, request); err != nil {
		return nil, err
	}
	responseWire, err := readSignalMessage(ctx, signalSession)
	if err != nil {
		return nil, err
	}
	if responseWire.Message == nil || responseWire.Message.Type != "connect_response" || responseWire.Message.SessionID != sessionID || responseWire.Message.Sender != peer.ID || responseWire.Message.Recipient != s.identity.ID() {
		return nil, protocol.Fail(protocol.InvalidMessage, "unexpected connect response")
	}
	if err = responseWire.Message.Verify(peer.PublicKey, time.Now()); err != nil {
		return nil, err
	}
	var remote iceDescription
	if err = json.Unmarshal(responseWire.Message.Payload, &remote); err != nil || remote.Ufrag == "" || remote.Password == "" {
		return nil, protocol.Fail(protocol.InvalidMessage, "invalid remote ICE credentials")
	}
	if err = exchangeCandidates(ctx, signalSession, endpoint, localCandidates, s.identity, peer, sessionID); err != nil {
		return nil, err
	}
	path, err := endpoint.Connect(ctx, connectivity.Credentials{Ufrag: remote.Ufrag, Password: remote.Password}, true)
	if err != nil {
		return nil, protocol.Fail(protocol.CheckTimeout, err.Error())
	}
	tlsConfig, err := s.identity.TLSConfig(peer.PublicKey, false)
	if err != nil {
		return nil, err
	}
	data, err := transport.Establish(ctx, endpoint, path, tlsConfig, true)
	if err != nil {
		return nil, err
	}
	closeSignal = false
	closeEndpoint = false
	return &PeerSession{PeerID: peer.ID, Path: path, Data: data, signal: signalSession, stopHeartbeat: stopHeartbeat}, nil
}

// AcceptDirect waits for one authenticated request from expectedPeerID. An
// empty peer ID accepts any currently trusted group member after verification.
func (s *Service) AcceptDirect(ctx context.Context, expectedPeerID string, cfg DirectConfig) (*PeerSession, error) {
	c, err := s.client()
	if err != nil {
		return nil, err
	}
	signalSession, err := c.Connect(ctx)
	if err != nil {
		return nil, err
	}
	closeSignal := true
	stopHeartbeat := startHeartbeat(ctx, signalSession)
	defer func() {
		if closeSignal {
			stopHeartbeat()
		}
	}()
	defer func() {
		if closeSignal {
			_ = signalSession.Close()
		}
	}()
	requestWire, err := readSignalMessage(ctx, signalSession)
	if err != nil {
		return nil, err
	}
	if requestWire.Message == nil || requestWire.Message.Type != "connect_request" || requestWire.Message.Recipient != s.identity.ID() {
		return nil, protocol.Fail(protocol.InvalidMessage, "unexpected connect request")
	}
	peer, err := s.trustedDevice(ctx, requestWire.Message.Sender)
	if err != nil {
		return nil, err
	}
	if expectedPeerID != "" && expectedPeerID != peer.ID {
		return nil, protocol.Fail(protocol.AuthenticationFailed, "requesting device does not match expected peer")
	}
	if err = requestWire.Message.Verify(peer.PublicKey, time.Now()); err != nil {
		return nil, err
	}
	var remote iceDescription
	if err = json.Unmarshal(requestWire.Message.Payload, &remote); err != nil || remote.Ufrag == "" || remote.Password == "" {
		return nil, protocol.Fail(protocol.InvalidMessage, "invalid remote ICE credentials")
	}
	endpoint, err := s.newEndpoint(cfg)
	if err != nil {
		return nil, err
	}
	closeEndpoint := true
	defer func() {
		if closeEndpoint {
			_ = endpoint.Close()
		}
	}()
	if err = endpoint.Gather(); err != nil {
		return nil, fmt.Errorf("GATHERING: %w", err)
	}
	localCandidates := collectCandidates(endpoint)
	if len(localCandidates) == 0 {
		return nil, protocol.Fail(protocol.NoCandidates, "local endpoint gathered no usable candidates")
	}
	response, err := protocol.NewEnvelope("connect_response", s.identity.ID(), peer.ID, requestWire.Message.SessionID, requestWire.Message.Generation, iceDescription{Ufrag: endpoint.Credentials().Ufrag, Password: endpoint.Credentials().Password})
	if err != nil {
		return nil, err
	}
	if err = signalSession.SendEnvelope(ctx, response); err != nil {
		return nil, err
	}
	if err = exchangeCandidates(ctx, signalSession, endpoint, localCandidates, s.identity, peer, requestWire.Message.SessionID); err != nil {
		return nil, err
	}
	path, err := endpoint.Connect(ctx, connectivity.Credentials{Ufrag: remote.Ufrag, Password: remote.Password}, false)
	if err != nil {
		return nil, protocol.Fail(protocol.CheckTimeout, err.Error())
	}
	tlsConfig, err := s.identity.TLSConfig(peer.PublicKey, true)
	if err != nil {
		return nil, err
	}
	data, err := transport.Establish(ctx, endpoint, path, tlsConfig, false)
	if err != nil {
		return nil, err
	}
	closeSignal = false
	closeEndpoint = false
	return &PeerSession{PeerID: peer.ID, Path: path, Data: data, signal: signalSession, stopHeartbeat: stopHeartbeat}, nil
}

func (s *Service) SendFiles(ctx context.Context, peerID string, paths []string, cfg DirectConfig, progress func(transfer.Progress)) (transfer.Result, error) {
	prepared, err := transfer.Prepare(ctx, paths, 0)
	if err != nil {
		return transfer.Result{}, err
	}
	defer prepared.Close()
	peer, err := s.ConnectDirect(ctx, peerID, cfg)
	if err != nil {
		return transfer.Result{}, err
	}
	defer peer.Close()
	stream, err := peer.Data.Conn.OpenStreamSync(ctx)
	if err != nil {
		return transfer.Result{}, err
	}
	return transfer.Send(ctx, stream, prepared, progress)
}

func (s *Service) ReceiveOnce(ctx context.Context, expectedPeerID, directory string, cfg DirectConfig, accept func(transfer.Manifest) bool, progress func(transfer.Progress)) (transfer.Result, error) {
	peer, err := s.AcceptDirect(ctx, expectedPeerID, cfg)
	if err != nil {
		return transfer.Result{}, err
	}
	defer peer.Close()
	stream, err := peer.Data.Conn.AcceptStream(ctx)
	if err != nil {
		return transfer.Result{}, err
	}
	return transfer.Receive(ctx, stream, directory, peer.PeerID, accept, progress)
}

func (s *Service) newEndpoint(cfg DirectConfig) (*connectivity.Endpoint, error) {
	if cfg.BindAddress == "" {
		if !cfg.AllowLoopback && !s.cfg.AllowInsecureLoopback {
			return nil, errors.New("E_NO_CANDIDATE: provide a concrete --bind address for this platform")
		}
		cfg.BindAddress = "127.0.0.1:0"
	}
	if cfg.CheckTimeout <= 0 {
		cfg.CheckTimeout = 20 * time.Second
	}
	return connectivity.New(connectivity.Config{BindAddress: cfg.BindAddress, STUNURLs: cfg.STUNURLs, AllowLoopback: cfg.AllowLoopback || s.cfg.AllowInsecureLoopback, Generation: firstGeneration, CheckTimeout: cfg.CheckTimeout})
}

func (s *Service) trustedDevice(ctx context.Context, id string) (signaling.Device, error) {
	if id == "" || id == s.identity.ID() {
		return signaling.Device{}, protocol.Fail(protocol.Unpaired, "peer identity is required")
	}
	c, err := s.client()
	if err != nil {
		return signaling.Device{}, err
	}
	devices, err := c.Devices(ctx)
	if err != nil {
		return signaling.Device{}, err
	}
	peers, err := identity.LoadTrust(s.cfg.DataDir)
	if err != nil {
		return signaling.Device{}, err
	}
	for _, d := range devices {
		if d.ID != id {
			continue
		}
		for _, p := range peers {
			if p.ID == d.ID && string(p.PublicKey) == string(d.PublicKey) {
				return d, nil
			}
		}
		return signaling.Device{}, protocol.Fail(protocol.Unpaired, "peer fingerprint is not locally trusted")
	}
	return signaling.Device{}, protocol.Fail(protocol.PeerOffline, "peer is not a current group member")
}

func readSignalMessage(ctx context.Context, session *signaling.Session) (signaling.Wire, error) {
	for {
		wire, err := session.Read(ctx)
		if err != nil {
			return wire, err
		}
		if wire.Type == "heartbeat" {
			continue
		}
		return wire, nil
	}
}

func startHeartbeat(ctx context.Context, session *signaling.Session) context.CancelFunc {
	heartbeatCtx, cancel := context.WithCancel(ctx)
	go func() {
		ticker := time.NewTicker(20 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-heartbeatCtx.Done():
				return
			case <-ticker.C:
				writeCtx, writeCancel := context.WithTimeout(heartbeatCtx, 5*time.Second)
				_ = session.SendHeartbeat(writeCtx)
				writeCancel()
			}
		}
	}()
	return cancel
}

func exchangeCandidates(ctx context.Context, session *signaling.Session, endpoint *connectivity.Endpoint, local []connectivity.Candidate, sender *identity.Identity, peer signaling.Device, sessionID string) error {
	errCh := make(chan error, 2)
	go func() {
		for _, candidate := range local {
			env, err := protocol.NewEnvelope("candidate", sender.ID(), peer.ID, sessionID, candidate.Generation, protocol.Candidate{Candidate: candidate.Value})
			if err != nil {
				errCh <- err
				return
			}
			if err = session.SendEnvelope(ctx, env); err != nil {
				errCh <- err
				return
			}
		}
		end, err := protocol.NewEnvelope("end_of_candidates", sender.ID(), peer.ID, sessionID, firstGeneration, map[string]any{})
		if err == nil {
			err = session.SendEnvelope(ctx, end)
		}
		errCh <- err
	}()
	go func() {
		for {
			wire, err := readSignalMessage(ctx, session)
			if err != nil {
				errCh <- err
				return
			}
			if wire.Message == nil || wire.Message.SessionID != sessionID || wire.Message.Sender != peer.ID || wire.Message.Recipient != sender.ID() {
				errCh <- protocol.Fail(protocol.InvalidMessage, "candidate session binding mismatch")
				return
			}
			if err = wire.Message.Verify(peer.PublicKey, time.Now()); err != nil {
				errCh <- err
				return
			}
			switch wire.Message.Type {
			case "candidate":
				var candidate protocol.Candidate
				if err = json.Unmarshal(wire.Message.Payload, &candidate); err != nil {
					errCh <- protocol.Fail(protocol.InvalidMessage, "invalid candidate payload")
					return
				}
				if err = endpoint.AddRemoteCandidate(connectivity.Candidate{Value: candidate.Candidate, Generation: wire.Message.Generation}); err != nil {
					errCh <- err
					return
				}
			case "end_of_candidates":
				errCh <- nil
				return
			default:
				errCh <- protocol.Fail(protocol.InvalidMessage, "unexpected candidate exchange message")
				return
			}
		}
	}()
	first := <-errCh
	second := <-errCh
	return errors.Join(first, second)
}
