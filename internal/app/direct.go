package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Wen5555/LinkSend/internal/connectivity"
	"github.com/Wen5555/LinkSend/internal/identity"
	"github.com/Wen5555/LinkSend/internal/protocol"
	"github.com/Wen5555/LinkSend/internal/signaling"
	"github.com/Wen5555/LinkSend/internal/transfer"
	"github.com/Wen5555/LinkSend/internal/transport"
)

const firstGeneration uint64 = 1

const candidateExchangeBudget = 10 * time.Second

type DirectConfig struct {
	BindAddress        string
	InterfacePriority  []string
	ExcludedInterfaces []string
	STUNURLs           []string
	AllowLoopback      bool
	CheckTimeout       time.Duration
	WaitTimeout        time.Duration // receiver only: time allowed for an incoming request
	onPhase            func(string)  // local application observation; never serialized
	onSession          func(string, string)
	onEvidence         func(DirectEvidence)
	onChunkSent        func(transfer.ChunkTransmission)
}

func (c DirectConfig) phase(value string) {
	if c.onPhase != nil {
		c.onPhase(value)
	}
}
func (c DirectConfig) session(id, peer string) {
	if c.onSession != nil {
		c.onSession(id, peer)
	}
}
func (c DirectConfig) evidence(value DirectEvidence) {
	if c.onEvidence != nil {
		c.onEvidence(value)
	}
}

type PeerSession struct {
	PeerID        string
	SessionID     string
	Path          connectivity.Path
	Data          *transport.Session
	signal        *signaling.Session
	stopHeartbeat context.CancelFunc
}

// DirectEvidence is a read-only snapshot of the path actually selected by ICE
// and the TLS parameters actually negotiated by QUIC. It contains no ICE
// credentials, signaling tokens, or private key material.
type DirectEvidence struct {
	SessionID              string                       `json:"session_id"`
	Generation             uint64                       `json:"generation"`
	PeerID                 string                       `json:"peer_id"`
	BaseSocket             string                       `json:"base_socket"`
	Interface              string                       `json:"interface"`
	AddressFamily          string                       `json:"address_family"`
	LocalCandidate         string                       `json:"local_candidate"`
	RemoteCandidate        string                       `json:"remote_candidate"`
	LocalType              string                       `json:"local_type"`
	RemoteType             string                       `json:"remote_type"`
	RemoteAddress          string                       `json:"remote_address"`
	ConnectionMethod       string                       `json:"connection_method"`
	TransportProtocol      string                       `json:"transport_protocol"`
	Relay                  bool                         `json:"relay"`
	STUNBytesSent          uint64                       `json:"stun_bytes_sent"`
	STUNBytesReceived      uint64                       `json:"stun_bytes_received"`
	STUNRequestsSent       uint64                       `json:"stun_requests_sent"`
	STUNResponsesReceived  uint64                       `json:"stun_responses_received"`
	RejectedPackets        uint64                       `json:"rejected_packets"`
	SignalingBytesSent     uint64                       `json:"signaling_bytes_sent"`
	SignalingBytesReceived uint64                       `json:"signaling_bytes_received"`
	ICEStateTimeline       []connectivity.ICEStateEvent `json:"ice_state_timeline"`
	TLSVersion             uint16                       `json:"tls_version"`
	ALPN                   string                       `json:"alpn"`
}

func (p *PeerSession) Evidence() DirectEvidence {
	if p == nil || p.Data == nil {
		return DirectEvidence{}
	}
	stats, tlsVersion, alpn := p.Data.Evidence()
	var signalStats signaling.SessionStats
	if p.signal != nil {
		signalStats = p.signal.Stats()
	}
	return DirectEvidence{SessionID: p.SessionID, Generation: p.Path.Generation, PeerID: p.PeerID, BaseSocket: p.Path.BaseSocket, Interface: p.Path.Interface, AddressFamily: p.Path.AddressFamily, LocalCandidate: p.Path.LocalCandidate, RemoteCandidate: p.Path.RemoteCandidate, LocalType: p.Path.LocalType, RemoteType: p.Path.RemoteType, RemoteAddress: p.Path.RemoteAddress, ConnectionMethod: p.Path.ConnectionMethod, TransportProtocol: p.Path.TransportProtocol, Relay: p.Path.Relay, STUNBytesSent: stats.STUNBytesSent, STUNBytesReceived: stats.STUNBytesReceived, STUNRequestsSent: stats.STUNRequestsSent, STUNResponsesReceived: stats.STUNResponsesReceived, RejectedPackets: stats.RejectedPackets, SignalingBytesSent: signalStats.BytesSent, SignalingBytesReceived: signalStats.BytesReceived, ICEStateTimeline: append([]connectivity.ICEStateEvent(nil), p.Path.ICEStateTimeline...), TLSVersion: tlsVersion, ALPN: alpn}
}

type DirectTransferResult struct {
	Transfer transfer.Result `json:"transfer"`
	Evidence DirectEvidence  `json:"evidence"`
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
	phaseBudget := cfg.CheckTimeout
	if phaseBudget <= 0 {
		phaseBudget = 20 * time.Second
	}
	connectCtx, cancelConnect := context.WithTimeout(ctx, phaseBudget)
	defer cancelConnect()
	peer, err := s.trustedDevice(connectCtx, peerID)
	if err != nil {
		return nil, classifyPhaseError(connectCtx, err, protocol.SignalingTimeout, "peer lookup")
	}
	c, err := s.client()
	if err != nil {
		return nil, err
	}
	signalSession, err := c.Connect(connectCtx)
	if err != nil {
		return nil, classifyPhaseError(connectCtx, err, protocol.SignalingTimeout, "signaling connection")
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
	cfg.session(sessionID, peer.ID)
	request, err := protocol.NewEnvelope("connect_request", s.identity.ID(), peer.ID, sessionID, firstGeneration, iceDescription{Ufrag: endpoint.Credentials().Ufrag, Password: endpoint.Credentials().Password})
	if err != nil {
		return nil, err
	}
	signalCtx, stopSignal := context.WithTimeout(ctx, phaseBudget)
	defer stopSignal()
	if err = signalSession.SendEnvelope(signalCtx, request); err != nil {
		return nil, classifyPhaseError(signalCtx, err, protocol.SignalingTimeout, "connect request")
	}
	var responseWire signaling.Wire
	for {
		responseWire, err = readSignalMessage(signalCtx, signalSession)
		if err != nil {
			return nil, err
		}
		incoming := responseWire.Message
		if incoming == nil || incoming.Type != "connect_request" || incoming.Sender != peer.ID || incoming.Recipient != s.identity.ID() {
			break
		}
		if incoming.Generation != firstGeneration {
			return nil, protocol.Fail(protocol.InvalidMessage, "unsupported ICE generation")
		}
		if err = incoming.Verify(peer.PublicKey, time.Now()); err != nil {
			return nil, err
		}
		if s.identity.ID() < peer.ID {
			// Both peers initiated at once. The server keeps the request from
			// the smaller identity, so the winning initiator ignores the losing
			// signed request and continues waiting for its own response.
			continue
		}
		cfg.session(incoming.SessionID, peer.ID)
		cfg.phase("connecting")
		path, data, establishErr := s.establishResponder(ctx, signalSession, endpoint, localCandidates, peer, *incoming, phaseBudget)
		if establishErr != nil {
			return nil, establishErr
		}
		cfg.phase("connected")
		cfg.evidence((&PeerSession{PeerID: peer.ID, SessionID: incoming.SessionID, Path: path, Data: data}).Evidence())
		closeSignal = false
		closeEndpoint = false
		return &PeerSession{PeerID: peer.ID, SessionID: incoming.SessionID, Path: path, Data: data, signal: signalSession, stopHeartbeat: stopHeartbeat}, nil
	}
	if responseWire.Message == nil || responseWire.Message.Type != "connect_response" || responseWire.Message.SessionID != sessionID || responseWire.Message.Generation != firstGeneration || responseWire.Message.Sender != peer.ID || responseWire.Message.Recipient != s.identity.ID() {
		return nil, protocol.Fail(protocol.InvalidMessage, "unexpected connect response")
	}
	if err = responseWire.Message.Verify(peer.PublicKey, time.Now()); err != nil {
		return nil, err
	}
	var remote iceDescription
	if err = json.Unmarshal(responseWire.Message.Payload, &remote); err != nil || remote.Ufrag == "" || remote.Password == "" {
		return nil, protocol.Fail(protocol.InvalidMessage, "invalid remote ICE credentials")
	}
	if err = exchangeCandidates(ctx, signalSession, endpoint, localCandidates, s.identity, peer, sessionID, firstGeneration); err != nil {
		return nil, err
	}
	path, err := endpoint.Connect(ctx, connectivity.Credentials{Ufrag: remote.Ufrag, Password: remote.Password}, true)
	if err != nil {
		return nil, classifyICEError(ctx, err)
	}
	tlsConfig, err := s.identity.TLSConfig(peer.PublicKey, false)
	if err != nil {
		return nil, err
	}
	quicCtx, stopQUIC := context.WithTimeout(ctx, phaseBudget)
	data, err := transport.Establish(quicCtx, endpoint, path, tlsConfig, true)
	stopQUIC()
	if err != nil {
		return nil, err
	}
	cfg.phase("connected")
	cfg.evidence((&PeerSession{PeerID: peer.ID, SessionID: sessionID, Path: path, Data: data}).Evidence())
	closeSignal = false
	closeEndpoint = false
	return &PeerSession{PeerID: peer.ID, SessionID: sessionID, Path: path, Data: data, signal: signalSession, stopHeartbeat: stopHeartbeat}, nil
}

// AcceptDirect waits for one authenticated request from expectedPeerID. An
// empty peer ID accepts any currently trusted group member after verification.
func (s *Service) AcceptDirect(ctx context.Context, expectedPeerID string, cfg DirectConfig) (*PeerSession, error) {
	phaseBudget := cfg.CheckTimeout
	if phaseBudget <= 0 {
		phaseBudget = 20 * time.Second
	}
	connectCtx, cancelConnect := context.WithTimeout(ctx, phaseBudget)
	defer cancelConnect()
	c, err := s.client()
	if err != nil {
		return nil, err
	}
	signalSession, err := c.Connect(connectCtx)
	if err != nil {
		return nil, classifyPhaseError(connectCtx, err, protocol.SignalingTimeout, "signaling connection")
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
	waitBudget := cfg.WaitTimeout
	if waitBudget <= 0 {
		waitBudget = phaseBudget
	}
	cfg.phase("waiting")
	signalCtx, stopSignal := context.WithTimeout(ctx, waitBudget)
	requestWire, err := readSignalMessage(signalCtx, signalSession)
	stopSignal()
	if err != nil {
		return nil, err
	}
	if requestWire.Message == nil || requestWire.Message.Type != "connect_request" || requestWire.Message.Recipient != s.identity.ID() {
		return nil, protocol.Fail(protocol.InvalidMessage, "unexpected connect request")
	}
	if requestWire.Message.Generation != firstGeneration {
		return nil, protocol.Fail(protocol.InvalidMessage, "unsupported ICE generation")
	}
	peerCtx, cancelPeer := context.WithTimeout(ctx, phaseBudget)
	defer cancelPeer()
	peer, err := s.trustedDevice(peerCtx, requestWire.Message.Sender)
	if err != nil {
		return nil, classifyPhaseError(peerCtx, err, protocol.SignalingTimeout, "peer lookup")
	}
	if expectedPeerID != "" && expectedPeerID != peer.ID {
		return nil, protocol.Fail(protocol.AuthenticationFailed, "requesting device does not match expected peer")
	}
	if err = requestWire.Message.Verify(peer.PublicKey, time.Now()); err != nil {
		return nil, err
	}
	cfg.session(requestWire.Message.SessionID, peer.ID)
	cfg.phase("connecting")
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
	path, data, err := s.establishResponder(ctx, signalSession, endpoint, localCandidates, peer, *requestWire.Message, phaseBudget)
	if err != nil {
		return nil, err
	}
	cfg.phase("connected")
	cfg.evidence((&PeerSession{PeerID: peer.ID, SessionID: requestWire.Message.SessionID, Path: path, Data: data}).Evidence())
	closeSignal = false
	closeEndpoint = false
	return &PeerSession{PeerID: peer.ID, SessionID: requestWire.Message.SessionID, Path: path, Data: data, signal: signalSession, stopHeartbeat: stopHeartbeat}, nil
}

func (s *Service) establishResponder(ctx context.Context, signalSession *signaling.Session, endpoint *connectivity.Endpoint, localCandidates []connectivity.Candidate, peer signaling.Device, request protocol.Envelope, phaseBudget time.Duration) (connectivity.Path, *transport.Session, error) {
	var remote iceDescription
	if err := json.Unmarshal(request.Payload, &remote); err != nil || remote.Ufrag == "" || remote.Password == "" {
		return connectivity.Path{}, nil, protocol.Fail(protocol.InvalidMessage, "invalid remote ICE credentials")
	}
	response, err := protocol.NewEnvelope("connect_response", s.identity.ID(), peer.ID, request.SessionID, request.Generation, iceDescription{Ufrag: endpoint.Credentials().Ufrag, Password: endpoint.Credentials().Password})
	if err != nil {
		return connectivity.Path{}, nil, err
	}
	signalCtx, stopSignal := context.WithTimeout(ctx, phaseBudget)
	if err = signalSession.SendEnvelope(signalCtx, response); err != nil {
		stopSignal()
		return connectivity.Path{}, nil, classifyPhaseError(signalCtx, err, protocol.SignalingTimeout, "connect response")
	}
	stopSignal()
	if err = exchangeCandidates(ctx, signalSession, endpoint, localCandidates, s.identity, peer, request.SessionID, request.Generation); err != nil {
		return connectivity.Path{}, nil, err
	}
	path, err := endpoint.Connect(ctx, connectivity.Credentials{Ufrag: remote.Ufrag, Password: remote.Password}, false)
	if err != nil {
		return connectivity.Path{}, nil, classifyICEError(ctx, err)
	}
	tlsConfig, err := s.identity.TLSConfig(peer.PublicKey, true)
	if err != nil {
		return connectivity.Path{}, nil, err
	}
	quicCtx, stopQUIC := context.WithTimeout(ctx, phaseBudget)
	data, err := transport.Establish(quicCtx, endpoint, path, tlsConfig, false)
	stopQUIC()
	if err != nil {
		return connectivity.Path{}, nil, err
	}
	return path, data, nil
}

func classifyICEError(ctx context.Context, err error) error {
	if errors.Is(ctx.Err(), context.Canceled) || errors.Is(err, context.Canceled) {
		return protocol.Wrap(protocol.Cancelled, "ICE checks cancelled", err)
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, connectivity.ErrCheckTimeout) {
		return protocol.Wrap(protocol.CheckTimeout, "ICE checks timed out", err)
	}
	if errors.Is(err, connectivity.ErrNoViableCandidate) {
		return protocol.Wrap(protocol.NoViableCandidate, "ICE nominated no candidate", err)
	}
	return protocol.Wrap(protocol.ICEFailed, "ICE checks failed", err)
}

func (s *Service) SendFiles(ctx context.Context, peerID string, paths []string, cfg DirectConfig, progress func(transfer.Progress)) (transfer.Result, error) {
	detailed, err := s.SendFilesDetailed(ctx, peerID, paths, cfg, progress)
	return detailed.Transfer, err
}

func (s *Service) SendFilesDetailed(ctx context.Context, peerID string, paths []string, cfg DirectConfig, progress func(transfer.Progress)) (DirectTransferResult, error) {
	cfg.phase("preparing")
	prepared, err := transfer.Prepare(ctx, paths, 0)
	if err != nil {
		return DirectTransferResult{}, err
	}
	defer prepared.Close()
	return s.SendPreparedDetailed(ctx, peerID, prepared, cfg, progress)
}

// SendPreparedDetailed reuses a caller-owned prepared manifest. Recovery uses
// this entry point after rebuilding and verifying the persisted manifest digest.
func (s *Service) SendPreparedDetailed(ctx context.Context, peerID string, prepared *transfer.Prepared, cfg DirectConfig, progress func(transfer.Progress)) (DirectTransferResult, error) {
	return s.SendPreparedWithHooksDetailed(ctx, peerID, prepared, cfg, transfer.SendHooks{Progress: progress})
}

func (s *Service) SendPreparedWithHooksDetailed(ctx context.Context, peerID string, prepared *transfer.Prepared, cfg DirectConfig, hooks transfer.SendHooks) (DirectTransferResult, error) {
	if prepared == nil {
		return DirectTransferResult{}, errors.New("INVALID_PREPARED_TRANSFER")
	}
	if hooks.Progress != nil {
		hooks.Progress(transfer.Progress{TransferID: prepared.Manifest.TransferID, State: "Preparing", Total: prepared.Manifest.TotalBytes()})
	}
	cfg.phase("connecting")
	peer, err := s.ConnectDirect(ctx, peerID, cfg)
	if err != nil {
		return DirectTransferResult{}, err
	}
	defer peer.Close()
	stream, err := peer.Data.Conn.OpenStreamSync(ctx)
	if err != nil {
		return DirectTransferResult{Evidence: peer.Evidence()}, err
	}
	result, err := transfer.SendWithHooks(ctx, transport.WrapStream(stream), prepared, hooks)
	return DirectTransferResult{Transfer: result, Evidence: peer.Evidence()}, err
}

func (s *Service) ReceiveOnce(ctx context.Context, expectedPeerID, directory string, cfg DirectConfig, accept func(transfer.Manifest) bool, progress func(transfer.Progress)) (transfer.Result, error) {
	detailed, err := s.ReceiveOnceDetailed(ctx, expectedPeerID, directory, cfg, accept, progress)
	return detailed.Transfer, err
}

func (s *Service) ReceiveOnceDetailed(ctx context.Context, expectedPeerID, directory string, cfg DirectConfig, accept func(transfer.Manifest) bool, progress func(transfer.Progress)) (DirectTransferResult, error) {
	cfg.phase("connecting")
	peer, err := s.AcceptDirect(ctx, expectedPeerID, cfg)
	if err != nil {
		return DirectTransferResult{}, err
	}
	defer peer.Close()
	stream, err := peer.Data.Conn.AcceptStream(ctx)
	if err != nil {
		return DirectTransferResult{Evidence: peer.Evidence()}, err
	}
	result, err := transfer.Receive(ctx, transport.WrapStream(stream), directory, peer.PeerID, accept, progress)
	return DirectTransferResult{Transfer: result, Evidence: peer.Evidence()}, err
}

func (s *Service) newEndpoint(cfg DirectConfig) (*connectivity.Endpoint, error) {
	if cfg.CheckTimeout <= 0 {
		cfg.CheckTimeout = 20 * time.Second
	}
	return connectivity.New(connectivity.Config{BindAddress: cfg.BindAddress, InterfacePriority: cfg.InterfacePriority, ExcludedInterfaces: cfg.ExcludedInterfaces, STUNURLs: cfg.STUNURLs, AllowLoopback: cfg.AllowLoopback || s.cfg.AllowInsecureLoopback, Generation: firstGeneration, CheckTimeout: cfg.CheckTimeout})
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

func readSignalMessage(ctx context.Context, session interface {
	Read(context.Context) (signaling.Wire, error)
}) (signaling.Wire, error) {
	for {
		wire, err := session.Read(ctx)
		if err != nil {
			if errors.Is(ctx.Err(), context.Canceled) {
				return wire, protocol.Fail(protocol.Cancelled, "signaling read cancelled")
			}
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return wire, protocol.Fail(protocol.SignalingTimeout, "signaling read timed out")
			}
			return wire, err
		}
		if wire.Type == "heartbeat" {
			continue
		}
		return wire, nil
	}
}

func classifyPhaseError(ctx context.Context, err error, timeoutCode protocol.Code, phase string) error {
	if err == nil {
		return nil
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		return protocol.Wrap(protocol.Cancelled, phase+" cancelled", err)
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return protocol.Wrap(timeoutCode, phase+" timed out", err)
	}
	return err
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

type candidateSession interface {
	SendEnvelope(context.Context, protocol.Envelope) error
	Read(context.Context) (signaling.Wire, error)
}

func exchangeCandidates(ctx context.Context, session candidateSession, endpoint *connectivity.Endpoint, local []connectivity.Candidate, sender *identity.Identity, peer signaling.Device, sessionID string, generation uint64) error {
	return exchangeCandidatesWithBudget(ctx, session, endpoint, local, sender, peer, sessionID, generation, candidateExchangeBudget)
}

func exchangeCandidatesWithBudget(ctx context.Context, session candidateSession, endpoint *connectivity.Endpoint, local []connectivity.Candidate, sender *identity.Identity, peer signaling.Device, sessionID string, generation uint64, budget time.Duration) error {
	if budget <= 0 {
		budget = candidateExchangeBudget
	}
	if generation == 0 {
		return protocol.Fail(protocol.InvalidMessage, "ICE generation is required")
	}
	phaseCtx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	errCh := make(chan error, 2)
	go func() {
		for _, candidate := range local {
			if candidate.Generation != generation {
				errCh <- protocol.Fail(protocol.InvalidMessage, "local candidate generation mismatch")
				return
			}
			env, err := protocol.NewEnvelope("candidate", sender.ID(), peer.ID, sessionID, candidate.Generation, protocol.Candidate{Candidate: candidate.Value})
			if err != nil {
				errCh <- err
				return
			}
			if err = session.SendEnvelope(phaseCtx, env); err != nil {
				errCh <- err
				return
			}
		}
		end, err := protocol.NewEnvelope("end_of_candidates", sender.ID(), peer.ID, sessionID, generation, map[string]any{})
		if err == nil {
			err = session.SendEnvelope(phaseCtx, end)
		}
		errCh <- err
	}()
	go func() {
		for {
			wire, err := readSignalMessage(phaseCtx, session)
			if err != nil {
				errCh <- err
				return
			}
			if wire.Message == nil || wire.Message.SessionID != sessionID || wire.Message.Generation != generation || wire.Message.Sender != peer.ID || wire.Message.Recipient != sender.ID() {
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
	if first != nil {
		cancel()
	}
	second := <-errCh
	if second != nil {
		cancel()
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		return protocol.Fail(protocol.Cancelled, "candidate exchange cancelled")
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(phaseCtx.Err(), context.DeadlineExceeded) {
		return protocol.Fail(protocol.CandidateTimeout, "candidate exchange timed out")
	}
	return errors.Join(first, second)
}
