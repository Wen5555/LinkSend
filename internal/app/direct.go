package app

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/Wen5555/LinkSend/internal/connectivity"
	"github.com/Wen5555/LinkSend/internal/content"
	"github.com/Wen5555/LinkSend/internal/discovery"
	"github.com/Wen5555/LinkSend/internal/identity"
	"github.com/Wen5555/LinkSend/internal/protocol"
	"github.com/Wen5555/LinkSend/internal/signaling"
	"github.com/Wen5555/LinkSend/internal/transfer"
	"github.com/Wen5555/LinkSend/internal/transport"
)

const firstGeneration uint64 = 1

const candidateExchangeBudget = 10 * time.Second

type DirectConfig struct {
	BindAddress                     string
	InterfacePriority               []string
	ExcludedInterfaces              []string
	STUNURLs                        []string
	AllowLoopback                   bool
	CheckTimeout                    time.Duration
	WaitTimeout                     time.Duration // receiver only: time allowed for an incoming request
	ReceiveConflictPolicy           transfer.ConflictPolicy
	DeviceConflictPolicies          map[string]transfer.ConflictPolicy
	onPhase                         func(string) // local application observation; never serialized
	onSession                       func(string, string)
	onEvidence                      func(DirectEvidence)
	onChunkSent                     func(transfer.ChunkTransmission)
	knownInterface                  string
	expectedSourceDigest            string
	beforeDispatch                  func(TaskSnapshot) error
	contentSnapshot                 *content.Snapshot
	contentAllowFallback            bool
	contentForceFile                bool
	disableSessionReuse             bool
	disableClipboardSync            bool
	contentTaskID                   string
	contentAttemptID                string
	expectedAuthorizationGeneration uint64
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
	mu                            sync.Mutex
	PeerID                        string
	SessionID                     string
	Path                          connectivity.Path
	Data                          *transport.Session
	Timings                       DirectTimings
	AuthorizationGeneration       uint64
	RemoteAuthorizationGeneration uint64
	signal                        directSignalSession
	signalBase                    signaling.SessionStats
	ownsSignal                    bool
	stopHeartbeat                 context.CancelFunc
	localPeer                     bool
	peerName                      string
	peerPublicKey                 ed25519.PublicKey
	lanAddress                    string
	SessionReuse                  bool
	ClipboardSync                 bool
	clipboardLeaseWake            chan struct{}
}

func (p *PeerSession) requestClipboardLeaseRenewal() {
	if p == nil {
		return
	}
	p.mu.Lock()
	wake := p.clipboardLeaseWake
	p.mu.Unlock()
	if wake == nil {
		return
	}
	select {
	case wake <- struct{}{}:
	default:
	}
}

type directSignalSession interface {
	SendEnvelope(context.Context, protocol.Envelope) error
	SendHeartbeat(context.Context) error
	Read(context.Context) (signaling.Wire, error)
	Stats() signaling.SessionStats
	Close() error
}

type DirectTimings struct {
	PeerLookupMS       float64 `json:"peer_lookup_ms"`
	SignalingConnectMS float64 `json:"signaling_connect_ms"`
	EndpointSetupMS    float64 `json:"endpoint_setup_ms"`
	PeerResponseMS     float64 `json:"peer_response_ms"`
	ICEMS              float64 `json:"ice_ms"`
	QUICHandshakeMS    float64 `json:"quic_handshake_ms"`
	TotalConnectMS     float64 `json:"total_connect_ms"`
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
	Timings                DirectTimings                `json:"timings"`
}

func (p *PeerSession) Evidence() DirectEvidence {
	if p == nil || p.Data == nil {
		return DirectEvidence{}
	}
	stats, tlsVersion, alpn := p.Data.Evidence()
	var signalStats signaling.SessionStats
	p.mu.Lock()
	if p.signal != nil {
		signalStats = p.signal.Stats()
		signalStats.BytesSent -= min(signalStats.BytesSent, p.signalBase.BytesSent)
		signalStats.BytesReceived -= min(signalStats.BytesReceived, p.signalBase.BytesReceived)
	}
	p.mu.Unlock()
	return makeDirectEvidence(p.PeerID, p.SessionID, p.Path, p.Timings, stats, signalStats, tlsVersion, alpn)
}

func durationMS(start time.Time) float64 {
	return float64(time.Since(start)) / float64(time.Millisecond)
}

type DirectTransferResult struct {
	Transfer          transfer.Result        `json:"transfer"`
	Evidence          DirectEvidence         `json:"evidence"`
	FailurePhase      string                 `json:"failure_phase,omitempty"`
	FailureDiagnostic *TaskFailureDiagnostic `json:"failure_diagnostic,omitempty"`
}

func (p *PeerSession) Close() error {
	if p == nil {
		return nil
	}
	stopHeartbeat, signal, ownsSignal := p.takeControl()
	var err error
	if stopHeartbeat != nil {
		stopHeartbeat()
	}
	if signal != nil && ownsSignal {
		err = errors.Join(err, signal.Close())
	}
	if p.Data != nil {
		err = errors.Join(err, p.Data.Close())
	}
	return err
}

func (p *PeerSession) takeControl() (context.CancelFunc, directSignalSession, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	stop, signal, owns := p.stopHeartbeat, p.signal, p.ownsSignal
	p.stopHeartbeat, p.signal, p.ownsSignal = nil, nil, false
	return stop, signal, owns
}

func (s *Service) closePeerAfterTransfer(p *PeerSession) error {
	if p == nil {
		return nil
	}
	stopHeartbeat, signal, ownsSignal := p.takeControl()
	if stopHeartbeat != nil {
		stopHeartbeat()
	}
	var err error
	err = errors.Join(err, s.rememberLANPeer(p))
	if signal != nil && ownsSignal {
		wss, reusable := signal.(*signaling.Session)
		if !reusable || !s.keepSpareSignal(wss) {
			err = errors.Join(err, signal.Close())
		}
	}
	if p.Data != nil {
		p.Data.CloseAfterTerminal()
	}
	return err
}

func (s *Service) ConnectDirect(ctx context.Context, peerID string, cfg DirectConfig) (*PeerSession, error) {
	if err := s.checkPeerAllowed(peerID); err != nil {
		return nil, err
	}
	authorizationGeneration := cfg.expectedAuthorizationGeneration
	if authorizationGeneration == 0 {
		var err error
		authorizationGeneration, err = s.currentAuthorizationGeneration(peerID)
		if err != nil {
			return nil, protocol.Wrap(protocol.AuthenticationFailed, "current peer grant required", err)
		}
	}
	if err := s.checkAuthorizationGeneration(peerID, authorizationGeneration); err != nil {
		return nil, err
	}
	cfg.expectedAuthorizationGeneration = authorizationGeneration
	connectStarted := time.Now()
	var timings DirectTimings
	phaseBudget := cfg.CheckTimeout
	if phaseBudget <= 0 {
		phaseBudget = 20 * time.Second
	}
	connectCtx, cancelConnect := context.WithTimeout(ctx, phaseBudget)
	defer cancelConnect()
	cfg.phase("peer_lookup")
	stageStarted := time.Now()
	var peer signaling.Device
	var signalSession directSignalSession
	var localPeer bool
	var lanErr error
	if manager := s.lanManager(); manager != nil {
		if nearby, ok := manager.Device(peerID); ok {
			peer = signaling.Device{ID: nearby.ID, Name: nearby.Name, PublicKey: ed25519.PublicKey(nearby.PublicKey)}
			cfg.phase("lan_control_connect")
			var route discovery.Route
			var lanSession *discovery.Session
			lanSession, nearby, route, lanErr = manager.Dial(connectCtx, peerID)
			if lanErr == nil {
				peer = signaling.Device{ID: nearby.ID, Name: nearby.Name, PublicKey: ed25519.PublicKey(nearby.PublicKey)}
				cfg.BindAddress = net.JoinHostPort(route.LocalAddress, "0")
				cfg.knownInterface = route.Interface
				signalSession = lanSession
				localPeer = true
			}
		}
	}
	var err error
	if signalSession == nil {
		peer, err = s.trustedDevice(connectCtx, peerID)
		if err != nil && lanErr != nil {
			return nil, lanErr
		}
	}
	timings.PeerLookupMS = durationMS(stageStarted)
	if err != nil {
		return nil, classifyPhaseError(connectCtx, err, protocol.SignalingTimeout, "peer lookup")
	}
	cfg.phase("signaling_connect")
	stageStarted = time.Now()
	if signalSession == nil {
		c, clientErr := s.client()
		if clientErr != nil {
			return nil, clientErr
		}
		wss := s.takeSpareSignal()
		if wss != nil {
			probeCtx, cancelProbe := context.WithTimeout(connectCtx, 2*time.Second)
			err = wss.SendHeartbeat(probeCtx)
			cancelProbe()
			if err != nil {
				_ = wss.Close()
				wss = nil
			}
		}
		if wss == nil {
			wss, err = c.Connect(connectCtx)
		}
		signalSession = wss
	}
	timings.SignalingConnectMS = durationMS(stageStarted)
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
	cfg.phase("endpoint_setup")
	stageStarted = time.Now()
	endpoint, err := s.newEndpoint(cfg)
	if err != nil {
		return nil, classifyEndpointSetupError(err)
	}
	closeEndpoint := true
	defer func() {
		if closeEndpoint {
			_ = endpoint.Close()
		}
	}()
	cfg.phase("candidate_gathering")
	if err = endpoint.Gather(); err != nil {
		return nil, classifyEndpointSetupError(fmt.Errorf("GATHERING: %w", err))
	}
	timings.EndpointSetupMS = durationMS(stageStarted)
	cfg.phase("session_prepare")
	sessionID := protocol.RandomID()
	cfg.session(sessionID, peer.ID)
	request, err := protocol.NewEnvelope("connect_request", s.identity.ID(), peer.ID, sessionID, firstGeneration, iceDescription{Ufrag: endpoint.Credentials().Ufrag, Password: endpoint.Credentials().Password, SessionReuse: !cfg.disableSessionReuse, ClipboardSync: !cfg.disableClipboardSync, AuthorizationGeneration: authorizationGeneration})
	if err != nil {
		return nil, err
	}
	signalCtx, stopSignal := context.WithTimeout(ctx, phaseBudget)
	defer stopSignal()
	cfg.phase("requesting_peer")
	stageStarted = time.Now()
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
		path, data, establishErr := s.establishResponder(ctx, signalSession, endpoint, peer, *incoming, phaseBudget, cfg)
		if establishErr != nil {
			return nil, establishErr
		}
		cfg.phase("connected")
		if err = s.checkAuthorizationGeneration(peer.ID, authorizationGeneration); err != nil {
			_ = data.Close()
			return nil, err
		}
		timings.PeerResponseMS = durationMS(stageStarted)
		timings.TotalConnectMS = durationMS(connectStarted)
		cfg.evidence((&PeerSession{PeerID: peer.ID, SessionID: incoming.SessionID, Path: path, Data: data, Timings: timings}).Evidence())
		closeSignal = false
		closeEndpoint = false
		description, _ := envelopeICEDescription(*incoming)
		result := &PeerSession{PeerID: peer.ID, SessionID: incoming.SessionID, Path: path, Data: data, Timings: timings, AuthorizationGeneration: authorizationGeneration, RemoteAuthorizationGeneration: description.AuthorizationGeneration, signal: signalSession, ownsSignal: true, stopHeartbeat: stopHeartbeat, localPeer: localPeer, peerName: peer.Name, peerPublicKey: append(ed25519.PublicKey(nil), peer.PublicKey...), lanAddress: lanRemoteAddress(signalSession), SessionReuse: description.SessionReuse, ClipboardSync: description.ClipboardSync && description.AuthorizationGeneration > 0 && !cfg.disableClipboardSync}
		s.deviceConnections.remember(result)
		return result, nil
	}
	if responseWire.Message == nil || responseWire.Message.Type != "connect_response" || responseWire.Message.SessionID != sessionID || responseWire.Message.Generation != firstGeneration || responseWire.Message.Sender != peer.ID || responseWire.Message.Recipient != s.identity.ID() {
		if responseWire.Message != nil && responseWire.Message.Type == "status" {
			return nil, peerSessionFailure(responseWire.Message, peer, s.identity.ID(), sessionID, firstGeneration)
		}
		return nil, protocol.Fail(protocol.InvalidMessage, "unexpected connect response")
	}
	if err = responseWire.Message.Verify(peer.PublicKey, time.Now()); err != nil {
		return nil, err
	}
	timings.PeerResponseMS = durationMS(stageStarted)
	var remote iceDescription
	if err = json.Unmarshal(responseWire.Message.Payload, &remote); err != nil || remote.Ufrag == "" || remote.Password == "" {
		return nil, protocol.Fail(protocol.InvalidMessage, "invalid remote ICE credentials")
	}
	cfg.phase("ice_checking")
	stageStarted = time.Now()
	path, err := connectWithTrickle(ctx, signalSession, endpoint, s.identity, peer, sessionID, firstGeneration, connectivity.Credentials{Ufrag: remote.Ufrag, Password: remote.Password}, true)
	timings.ICEMS = durationMS(stageStarted)
	if err != nil {
		return nil, err
	}
	tlsConfig, err := s.identity.TLSConfig(peer.PublicKey, false)
	if err != nil {
		return nil, err
	}
	cfg.phase("quic_handshake")
	stageStarted = time.Now()
	quicCtx, stopQUIC := context.WithTimeout(ctx, phaseBudget)
	data, err := transport.Establish(quicCtx, endpoint, path, tlsConfig, true)
	stopQUIC()
	timings.QUICHandshakeMS = durationMS(stageStarted)
	if err != nil {
		timings.TotalConnectMS = durationMS(connectStarted)
		cfg.evidence(makeDirectEvidence(peer.ID, sessionID, path, timings, endpoint.Stats(), signalSession.Stats(), 0, ""))
		return nil, err
	}
	cfg.phase("connected")
	if err = s.checkAuthorizationGeneration(peer.ID, authorizationGeneration); err != nil {
		_ = data.Close()
		return nil, err
	}
	timings.TotalConnectMS = durationMS(connectStarted)
	cfg.evidence((&PeerSession{PeerID: peer.ID, SessionID: sessionID, Path: path, Data: data, Timings: timings}).Evidence())
	closeSignal = false
	closeEndpoint = false
	result := &PeerSession{PeerID: peer.ID, SessionID: sessionID, Path: path, Data: data, Timings: timings, AuthorizationGeneration: authorizationGeneration, RemoteAuthorizationGeneration: remote.AuthorizationGeneration, signal: signalSession, ownsSignal: true, stopHeartbeat: stopHeartbeat, localPeer: localPeer, peerName: peer.Name, peerPublicKey: append(ed25519.PublicKey(nil), peer.PublicKey...), lanAddress: lanRemoteAddress(signalSession), SessionReuse: remote.SessionReuse, ClipboardSync: remote.ClipboardSync && remote.AuthorizationGeneration > 0 && !cfg.disableClipboardSync}
	s.deviceConnections.remember(result)
	return result, nil
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
	cfg.phase("signaling_connect")
	signalSession, err := c.Connect(connectCtx)
	if err != nil {
		return nil, classifyPhaseError(connectCtx, err, protocol.SignalingTimeout, "signaling connection")
	}
	stopHeartbeat := startHeartbeat(ctx, signalSession)
	peer, err := s.acceptDirectOnSession(ctx, expectedPeerID, cfg, signalSession)
	if err != nil {
		stopHeartbeat()
		_ = signalSession.Close()
		return nil, err
	}
	peer.ownsSignal = true
	peer.stopHeartbeat = stopHeartbeat
	return peer, nil
}

// acceptDirectOnSession performs one negotiation on an already authenticated
// signaling connection. The persistent desktop inbox uses this to eliminate
// the offline gap between consecutive transfers; ownership stays with its
// caller and healthy QUIC remains independent from WSS.
func (s *Service) acceptDirectOnSession(ctx context.Context, expectedPeerID string, cfg DirectConfig, signalSession directSignalSession) (*PeerSession, error) {
	return s.acceptDirectOnSessionPeer(ctx, expectedPeerID, cfg, signalSession, nil)
}

func (s *Service) acceptDirectOnSessionPeer(ctx context.Context, expectedPeerID string, cfg DirectConfig, signalSession directSignalSession, authenticatedPeer *signaling.Device) (*PeerSession, error) {
	phaseBudget := cfg.CheckTimeout
	if phaseBudget <= 0 {
		phaseBudget = 20 * time.Second
	}
	waitBudget := cfg.WaitTimeout
	if waitBudget <= 0 {
		waitBudget = phaseBudget
	}
	cfg.phase("waiting")
	signalBase := signalSession.Stats()
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
	cfg.phase("peer_lookup")
	peerCtx, cancelPeer := context.WithTimeout(ctx, phaseBudget)
	defer cancelPeer()
	var peer signaling.Device
	if authenticatedPeer != nil {
		peer = *authenticatedPeer
		if peer.ID != requestWire.Message.Sender || len(peer.PublicKey) != ed25519.PublicKeySize {
			return nil, protocol.Fail(protocol.AuthenticationFailed, "LAN control identity does not match request")
		}
	} else {
		peer, err = s.trustedDevice(peerCtx, requestWire.Message.Sender)
	}
	if err != nil {
		return nil, classifyPhaseError(peerCtx, err, protocol.SignalingTimeout, "peer lookup")
	}
	if err = requestWire.Message.Verify(peer.PublicKey, time.Now()); err != nil {
		return nil, err
	}
	if err = s.checkPeerAllowed(peer.ID); err != nil {
		sendSessionFailure(signalSession, s.identity, peer, *requestWire.Message, err)
		return nil, err
	}
	authorizationGeneration := cfg.expectedAuthorizationGeneration
	if authorizationGeneration == 0 {
		authorizationGeneration, err = s.currentAuthorizationGeneration(peer.ID)
	}
	if err != nil {
		return nil, protocol.Wrap(protocol.AuthenticationFailed, "peer authorization changed before session", err)
	}
	if err = s.checkAuthorizationGeneration(peer.ID, authorizationGeneration); err != nil {
		return nil, err
	}
	cfg.expectedAuthorizationGeneration = authorizationGeneration
	reportFailure := func(setupErr error) error {
		sendSessionFailure(signalSession, s.identity, peer, *requestWire.Message, setupErr)
		return setupErr
	}
	if expectedPeerID != "" && expectedPeerID != peer.ID {
		return nil, reportFailure(protocol.Fail(protocol.AuthenticationFailed, "requesting device does not match expected peer"))
	}
	cfg.session(requestWire.Message.SessionID, peer.ID)
	cfg.phase("endpoint_setup")
	endpoint, err := s.newEndpoint(cfg)
	if err != nil {
		return nil, reportFailure(classifyEndpointSetupError(err))
	}
	closeEndpoint := true
	defer func() {
		if closeEndpoint {
			_ = endpoint.Close()
		}
	}()
	cfg.phase("candidate_gathering")
	if err = endpoint.Gather(); err != nil {
		return nil, reportFailure(classifyEndpointSetupError(fmt.Errorf("GATHERING: %w", err)))
	}
	path, data, err := s.establishResponder(ctx, signalSession, endpoint, peer, *requestWire.Message, phaseBudget, cfg)
	if err != nil {
		return nil, reportFailure(err)
	}
	cfg.phase("connected")
	if err = s.checkAuthorizationGeneration(peer.ID, authorizationGeneration); err != nil {
		_ = data.Close()
		return nil, reportFailure(err)
	}
	description, _ := envelopeICEDescription(*requestWire.Message)
	result := &PeerSession{PeerID: peer.ID, SessionID: requestWire.Message.SessionID, Path: path, Data: data, AuthorizationGeneration: authorizationGeneration, RemoteAuthorizationGeneration: description.AuthorizationGeneration, signal: signalSession, signalBase: signalBase, SessionReuse: description.SessionReuse, ClipboardSync: description.ClipboardSync && description.AuthorizationGeneration > 0 && !cfg.disableClipboardSync}
	cfg.evidence(result.Evidence())
	closeEndpoint = false
	s.deviceConnections.remember(result)
	return result, nil
}

func (s *Service) establishResponder(ctx context.Context, signalSession directSignalSession, endpoint *connectivity.Endpoint, peer signaling.Device, request protocol.Envelope, phaseBudget time.Duration, cfg DirectConfig) (connectivity.Path, *transport.Session, error) {
	signalBase := signalSession.Stats()
	var remote iceDescription
	if err := json.Unmarshal(request.Payload, &remote); err != nil || remote.Ufrag == "" || remote.Password == "" {
		return connectivity.Path{}, nil, protocol.Fail(protocol.InvalidMessage, "invalid remote ICE credentials")
	}
	tlsConfig, err := s.identity.TLSConfig(peer.PublicKey, true)
	if err != nil {
		return connectivity.Path{}, nil, err
	}
	listener, err := transport.PrepareListener(endpoint, tlsConfig)
	if err != nil {
		return connectivity.Path{}, nil, err
	}
	closeListener := true
	defer func() {
		if closeListener {
			_ = listener.Close()
		}
	}()
	response, err := protocol.NewEnvelope("connect_response", s.identity.ID(), peer.ID, request.SessionID, request.Generation, iceDescription{Ufrag: endpoint.Credentials().Ufrag, Password: endpoint.Credentials().Password, SessionReuse: remote.SessionReuse && !cfg.disableSessionReuse, ClipboardSync: remote.ClipboardSync && !cfg.disableClipboardSync, AuthorizationGeneration: cfg.expectedAuthorizationGeneration})
	if err != nil {
		return connectivity.Path{}, nil, err
	}
	signalCtx, stopSignal := context.WithTimeout(ctx, phaseBudget)
	if err = signalSession.SendEnvelope(signalCtx, response); err != nil {
		stopSignal()
		return connectivity.Path{}, nil, classifyPhaseError(signalCtx, err, protocol.SignalingTimeout, "connect response")
	}
	stopSignal()
	cfg.phase("ice_checking")
	stageStarted := time.Now()
	path, err := connectWithTrickle(ctx, signalSession, endpoint, s.identity, peer, request.SessionID, request.Generation, connectivity.Credentials{Ufrag: remote.Ufrag, Password: remote.Password}, false)
	timings := DirectTimings{ICEMS: durationMS(stageStarted)}
	if err != nil {
		return connectivity.Path{}, nil, err
	}
	cfg.phase("quic_handshake")
	stageStarted = time.Now()
	quicCtx, stopQUIC := context.WithTimeout(ctx, phaseBudget)
	data, err := transport.AcceptPrepared(quicCtx, endpoint, path, listener)
	stopQUIC()
	timings.QUICHandshakeMS = durationMS(stageStarted)
	if err != nil {
		signalStats := signalSession.Stats()
		signalStats.BytesSent -= min(signalStats.BytesSent, signalBase.BytesSent)
		signalStats.BytesReceived -= min(signalStats.BytesReceived, signalBase.BytesReceived)
		cfg.evidence(makeDirectEvidence(peer.ID, request.SessionID, path, timings, endpoint.Stats(), signalStats, 0, ""))
		return connectivity.Path{}, nil, err
	}
	closeListener = false
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

type sessionStatus struct {
	State string        `json:"state"`
	Code  protocol.Code `json:"code,omitempty"`
}

func envelopeSupportsSessionReuse(envelope protocol.Envelope) bool {
	description, ok := envelopeICEDescription(envelope)
	return ok && description.SessionReuse
}

func envelopeSupportsClipboardSync(envelope protocol.Envelope) bool {
	description, ok := envelopeICEDescription(envelope)
	return ok && description.ClipboardSync && description.AuthorizationGeneration > 0
}

func envelopeICEDescription(envelope protocol.Envelope) (iceDescription, bool) {
	var description iceDescription
	err := json.Unmarshal(envelope.Payload, &description)
	return description, err == nil && description.Ufrag != "" && description.Password != ""
}

func classifyEndpointSetupError(err error) error {
	if err == nil {
		return nil
	}
	var existing *protocol.Error
	if errors.As(err, &existing) {
		return err
	}
	return protocol.Wrap(protocol.NoCandidates, "local UDP endpoint is unavailable", err)
}

func allowedSessionFailure(code protocol.Code) bool {
	switch code {
	case protocol.CandidateTimeout, protocol.NoCandidates, protocol.NoViableCandidate,
		protocol.CheckTimeout, protocol.ICEFailed, protocol.QUICHandshakeTimeout,
		protocol.QUICHandshakeFailed, protocol.AuthenticationFailed, protocol.DirectFailed:
		return true
	default:
		return false
	}
}

func sessionFailureFromEnvelope(env *protocol.Envelope) error {
	var status sessionStatus
	if env == nil || json.Unmarshal(env.Payload, &status) != nil || status.State != "failed" || !allowedSessionFailure(status.Code) {
		return protocol.Fail(protocol.InvalidMessage, "invalid peer session failure status")
	}
	return protocol.Fail(status.Code, "peer reported connection setup failure")
}

func peerSessionFailure(env *protocol.Envelope, peer signaling.Device, localID, sessionID string, generation uint64) error {
	if env == nil || env.Sender != peer.ID || env.Recipient != localID || env.SessionID != sessionID || env.Generation != generation {
		return protocol.Fail(protocol.InvalidMessage, "peer session failure binding mismatch")
	}
	if err := env.Verify(peer.PublicKey, time.Now()); err != nil {
		return err
	}
	return sessionFailureFromEnvelope(env)
}

// sendSessionFailure is best effort and intentionally carries only a stable
// code. The private local cause stays out of signaling and peer-visible logs.
func sendSessionFailure(session directSignalSession, sender *identity.Identity, peer signaling.Device, request protocol.Envelope, setupErr error) {
	if session == nil || sender == nil || setupErr == nil {
		return
	}
	code := protocol.ErrorCode(setupErr)
	if !allowedSessionFailure(code) {
		code = protocol.DirectFailed
	}
	env, err := protocol.NewEnvelope("status", sender.ID(), peer.ID, request.SessionID, request.Generation, sessionStatus{State: "failed", Code: code})
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = session.SendEnvelope(ctx, env)
}

func (s *Service) SendFiles(ctx context.Context, peerID string, paths []string, cfg DirectConfig, progress func(transfer.Progress)) (transfer.Result, error) {
	detailed, err := s.SendFilesDetailed(ctx, peerID, paths, cfg, progress)
	return detailed.Transfer, err
}

func (s *Service) SendFilesDetailed(ctx context.Context, peerID string, paths []string, cfg DirectConfig, progress func(transfer.Progress)) (result DirectTransferResult, err error) {
	cfg, finish := observeDirectFailure(cfg)
	defer func() { finish(&result, err) }()
	cfg.phase("preparing")
	prepared, peer, err := s.prepareAndConnect(ctx, peerID, paths, cfg)
	if err != nil {
		return DirectTransferResult{}, err
	}
	defer prepared.Close()
	result, sendErr := s.sendPreparedOverPeer(ctx, peer, prepared, cfg, transfer.SendHooks{Progress: progress})
	err = s.releasePeerSession(peer, sendErr)
	if sendErr == nil && err != nil {
		result.FailurePhase = "closing"
	}
	return result, err
}

// SendPreparedDetailed reuses a caller-owned prepared manifest. Recovery uses
// this entry point after rebuilding and verifying the persisted manifest digest.
func (s *Service) SendPreparedDetailed(ctx context.Context, peerID string, prepared *transfer.Prepared, cfg DirectConfig, progress func(transfer.Progress)) (DirectTransferResult, error) {
	return s.SendPreparedWithHooksDetailed(ctx, peerID, prepared, cfg, transfer.SendHooks{Progress: progress})
}

func (s *Service) SendPreparedWithHooksDetailed(ctx context.Context, peerID string, prepared *transfer.Prepared, cfg DirectConfig, hooks transfer.SendHooks) (result DirectTransferResult, err error) {
	cfg, finish := observeDirectFailure(cfg)
	defer func() { finish(&result, err) }()
	if prepared == nil {
		return DirectTransferResult{FailurePhase: "preparing"}, errors.New("INVALID_PREPARED_TRANSFER")
	}
	if hooks.Progress != nil {
		hooks.Progress(transfer.Progress{TransferID: prepared.Manifest.TransferID, State: "Preparing", Total: prepared.Manifest.TotalBytes()})
	}
	cfg.phase("connecting")
	peer, err := s.acquirePeerSession(ctx, peerID, cfg)
	if err != nil {
		return DirectTransferResult{}, err
	}
	result, sendErr := s.sendPreparedOverPeer(ctx, peer, prepared, cfg, hooks)
	err = s.releasePeerSession(peer, sendErr)
	if sendErr == nil && err != nil {
		result.FailurePhase = "closing"
	}
	return result, err
}

func (s *Service) sendPreparedOverPeer(ctx context.Context, peer *PeerSession, prepared *transfer.Prepared, cfg DirectConfig, hooks transfer.SendHooks) (DirectTransferResult, error) {
	if err := s.checkAuthorizationGeneration(peer.PeerID, peer.AuthorizationGeneration); err != nil {
		return DirectTransferResult{Evidence: peer.Evidence(), FailurePhase: "authorization"}, err
	}
	options, err := s.sendContentOptions(ctx, prepared, cfg, hooks)
	if err != nil {
		return DirectTransferResult{Evidence: peer.Evidence(), FailurePhase: "preparing"}, err
	}
	cfg.phase("opening_stream")
	stream, err := peer.Data.Conn.OpenStreamSync(ctx)
	if err != nil {
		return DirectTransferResult{Evidence: peer.Evidence()}, err
	}
	result, err := transfer.SendWithOptions(ctx, transport.WrapStream(stream), prepared, options)
	if cfg.contentForceFile {
		result.FileFallback = true
	}
	detailed := DirectTransferResult{Transfer: result, Evidence: peer.Evidence()}
	if err != nil {
		detailed.FailurePhase = "transfer"
	}
	return detailed, err
}

type prepareResult struct {
	prepared *transfer.Prepared
	err      error
}

type peerResult struct {
	peer *PeerSession
	err  error
}

// prepareAndConnect overlaps source hashing with signaling/ICE/QUIC setup.
// Neither side sees file bytes or a manifest until both independent phases
// succeed, and cancellation closes whichever phase finished first.
func (s *Service) prepareAndConnect(ctx context.Context, peerID string, paths []string, cfg DirectConfig) (*transfer.Prepared, *PeerSession, error) {
	parallelCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	preparedCh := make(chan prepareResult, 1)
	peerCh := make(chan peerResult, 1)
	go func() {
		prepared, err := transfer.Prepare(parallelCtx, paths, 0)
		preparedCh <- prepareResult{prepared: prepared, err: err}
	}()
	go func() {
		cfg.phase("connecting")
		peer, err := s.acquirePeerSession(parallelCtx, peerID, cfg)
		peerCh <- peerResult{peer: peer, err: err}
	}()

	var preparedResult prepareResult
	var connectedResult peerResult
	for completed := 0; completed < 2; completed++ {
		select {
		case preparedResult = <-preparedCh:
			if preparedResult.err != nil {
				cancel()
			}
		case connectedResult = <-peerCh:
			if connectedResult.err != nil {
				cancel()
			}
		}
	}
	if preparedResult.err != nil || connectedResult.err != nil {
		if preparedResult.prepared != nil {
			_ = preparedResult.prepared.Close()
		}
		if connectedResult.peer != nil {
			_ = s.releasePeerSession(connectedResult.peer, nil)
		}
		if preparedResult.err != nil && protocol.ErrorCode(preparedResult.err) != protocol.Cancelled {
			// Preparation and connection run in parallel; identify the selected
			// error instead of retaining an unrelated connection observation.
			cfg.phase("preparing")
			return nil, nil, preparedResult.err
		}
		if connectedResult.err != nil {
			return nil, nil, connectedResult.err
		}
		return nil, nil, preparedResult.err
	}
	return preparedResult.prepared, connectedResult.peer, nil
}

// connectWithOnlineGrace covers the short hand-off in which the peer has just
// finished a transfer and is reopening its persistent inbox. ICE or transport
// failures are never blindly retried here because those may already own
// resources or require a fresh user-visible recovery attempt.
func (s *Service) connectWithOnlineGrace(ctx context.Context, peerID string, cfg DirectConfig) (*PeerSession, error) {
	var last error
	for attempt := 0; attempt < 4; attempt++ {
		peer, err := s.ConnectDirect(ctx, peerID, cfg)
		if err == nil {
			return peer, nil
		}
		last = err
		if protocol.ErrorCode(err) != protocol.PeerOffline || attempt == 3 {
			return nil, err
		}
		cfg.phase("waiting_peer")
		timer := time.NewTimer(time.Duration(attempt+1) * 350 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, protocol.Wrap(protocol.Cancelled, "peer wait cancelled", ctx.Err())
		case <-timer.C:
		}
		cfg.phase("connecting")
	}
	return nil, last
}

func (s *Service) ReceiveOnce(ctx context.Context, expectedPeerID, directory string, cfg DirectConfig, accept func(transfer.Manifest) bool, progress func(transfer.Progress)) (transfer.Result, error) {
	detailed, err := s.ReceiveOnceDetailed(ctx, expectedPeerID, directory, cfg, accept, progress)
	return detailed.Transfer, err
}

func (s *Service) ReceiveOnceDetailed(ctx context.Context, expectedPeerID, directory string, cfg DirectConfig, accept func(transfer.Manifest) bool, progress func(transfer.Progress)) (DirectTransferResult, error) {
	return s.ReceiveOnceWithOptionsDetailed(ctx, expectedPeerID, cfg, transfer.ReceiveOptions{Directory: directory, Accept: accept, Progress: progress})
}

func (s *Service) ReceiveOnceWithOptionsDetailed(ctx context.Context, expectedPeerID string, cfg DirectConfig, options transfer.ReceiveOptions) (result DirectTransferResult, err error) {
	cfg, finish := observeDirectFailure(cfg)
	defer func() { finish(&result, err) }()
	cfg.phase("connecting")
	peer, err := s.AcceptDirect(ctx, expectedPeerID, cfg)
	if err != nil {
		return DirectTransferResult{}, err
	}
	cfg.phase("opening_stream")
	stream, err := peer.Data.Conn.AcceptStream(ctx)
	if err != nil {
		return DirectTransferResult{Evidence: peer.Evidence()}, errors.Join(err, peer.Close())
	}
	options.Peer = peer.PeerID
	received, err := transfer.ReceiveWithOptions(ctx, transport.WrapStream(stream), options)
	detailed := DirectTransferResult{Transfer: received, Evidence: peer.Evidence()}
	if err == nil {
		closeErr := s.closePeerAfterTransfer(peer)
		if closeErr != nil {
			detailed.FailurePhase = "closing"
		}
		return detailed, closeErr
	}
	detailed.FailurePhase = "transfer"
	return detailed, errors.Join(err, peer.Close())
}

func (s *Service) newEndpoint(cfg DirectConfig) (*connectivity.Endpoint, error) {
	if cfg.CheckTimeout <= 0 {
		cfg.CheckTimeout = 20 * time.Second
	}
	allowLoopback := cfg.AllowLoopback || s.cfg.AllowInsecureLoopback
	bindAddress := cfg.BindAddress
	knownInterface := cfg.knownInterface
	if strings.TrimSpace(bindAddress) == "" {
		cfg.phase("interface_resolve")
		selected, err := s.automaticInterfaceAddress(allowLoopback, cfg.InterfacePriority, cfg.ExcludedInterfaces)
		if err != nil {
			return nil, err
		}
		bindAddress = net.JoinHostPort(selected.Address, "0")
		knownInterface = selected.Interface
	}
	cfg.phase("endpoint_construct")
	return connectivity.New(connectivity.Config{BindAddress: bindAddress, KnownInterface: knownInterface, InterfacePriority: cfg.InterfacePriority, ExcludedInterfaces: cfg.ExcludedInterfaces, STUNURLs: cfg.STUNURLs, AllowLoopback: allowLoopback, Generation: firstGeneration, CheckTimeout: cfg.CheckTimeout})
}

func (s *Service) automaticInterfaceAddress(allowLoopback bool, priority, excluded []string) (connectivity.InterfaceAddress, error) {
	key := fmt.Sprintf("%t|%s|%s", allowLoopback, strings.Join(priority, "\x00"), strings.Join(excluded, "\x00"))
	s.networkMu.Lock()
	defer s.networkMu.Unlock()
	if s.network.key == key && connectivity.InterfaceAddressPresent(s.network.address.Interface, s.network.address.Address) {
		return s.network.address, nil
	}
	selected, err := connectivity.ResolveInterfaceAddress(allowLoopback, priority, excluded)
	if err != nil {
		s.network = cachedNetworkSelection{}
		return connectivity.InterfaceAddress{}, err
	}
	s.network = cachedNetworkSelection{key: key, address: selected}
	return selected, nil
}

func (s *Service) prewarmNetwork(cfg DirectConfig) {
	if strings.TrimSpace(cfg.BindAddress) != "" {
		return
	}
	go func() {
		_, _ = s.automaticInterfaceAddress(cfg.AllowLoopback || s.cfg.AllowInsecureLoopback, cfg.InterfacePriority, cfg.ExcludedInterfaces)
	}()
}

func (s *Service) trustedDevice(ctx context.Context, id string) (signaling.Device, error) {
	if err := s.checkPeerAllowed(id); err != nil {
		return signaling.Device{}, err
	}
	if id == "" || id == s.identity.ID() {
		return signaling.Device{}, protocol.Fail(protocol.Unpaired, "peer identity is required")
	}
	peers, err := identity.LoadTrust(s.cfg.DataDir)
	if err != nil {
		return signaling.Device{}, err
	}
	for _, peer := range peers {
		if peer.ID == id && identity.DeviceID(peer.PublicKey) == id {
			return signaling.Device{ID: peer.ID, Name: peer.Name, PublicKey: peer.PublicKey}, nil
		}
	}
	// A newly joined group member may not be pinned by this already-running
	// device yet. Refresh only on a local miss; established peers stay off the
	// latency-sensitive HTTP devices path.
	c, err := s.client()
	if err != nil {
		return signaling.Device{}, err
	}
	devices, err := c.Devices(ctx)
	if err != nil {
		return signaling.Device{}, err
	}
	if err = s.syncPairedDevices(devices); err != nil {
		return signaling.Device{}, err
	}
	peers, err = identity.LoadTrust(s.cfg.DataDir)
	if err != nil {
		return signaling.Device{}, err
	}
	for _, device := range devices {
		if device.ID != id {
			continue
		}
		for _, peer := range peers {
			if peer.ID == device.ID && string(peer.PublicKey) == string(device.PublicKey) {
				return device, nil
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

func startHeartbeat(ctx context.Context, session directSignalSession) context.CancelFunc {
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
	stream := make(chan connectivity.Candidate, len(local))
	for _, candidate := range local {
		stream <- candidate
	}
	close(stream)
	return exchangeCandidateStreamWithBudget(ctx, session, endpoint, stream, sender, peer, sessionID, generation, nil, budget)
}

func exchangeCandidateStream(ctx context.Context, session candidateSession, endpoint *connectivity.Endpoint, local <-chan connectivity.Candidate, sender *identity.Identity, peer signaling.Device, sessionID string, generation uint64) error {
	return exchangeCandidateStreamWithBudget(ctx, session, endpoint, local, sender, peer, sessionID, generation, nil, candidateExchangeBudget)
}

func exchangeCandidateStreamWithBudget(ctx context.Context, session candidateSession, endpoint *connectivity.Endpoint, local <-chan connectivity.Candidate, sender *identity.Identity, peer signaling.Device, sessionID string, generation uint64, stop <-chan struct{}, budget time.Duration) error {
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
		sendEnd := func() error {
			end, err := protocol.NewEnvelope("end_of_candidates", sender.ID(), peer.ID, sessionID, generation, map[string]any{})
			if err == nil {
				err = session.SendEnvelope(phaseCtx, end)
			}
			return err
		}
		for {
			var candidate connectivity.Candidate
			var ok bool
			select {
			case candidate, ok = <-local:
				if !ok {
					errCh <- sendEnd()
					return
				}
			case <-stop:
				errCh <- sendEnd()
				return
			case <-phaseCtx.Done():
				errCh <- protocol.Wrap(protocol.Cancelled, "candidate sending cancelled", phaseCtx.Err())
				return
			}
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
			case "status":
				errCh <- sessionFailureFromEnvelope(wire.Message)
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

type pathResult struct {
	path connectivity.Path
	err  error
}

// connectWithTrickle overlaps candidate gathering, signed exchange and Pion
// checks. It still waits for both exchange directions to close cleanly before
// handing the signaling session to the established peer.
func connectWithTrickle(ctx context.Context, session candidateSession, endpoint *connectivity.Endpoint, sender *identity.Identity, peer signaling.Device, sessionID string, generation uint64, remote connectivity.Credentials, controlling bool) (connectivity.Path, error) {
	phaseCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	exchangeCh := make(chan error, 1)
	connectCh := make(chan pathResult, 1)
	stopGathering := make(chan struct{})
	go func() {
		exchangeCh <- exchangeCandidateStreamWithBudget(phaseCtx, session, endpoint, endpoint.Candidates(), sender, peer, sessionID, generation, stopGathering, candidateExchangeBudget)
	}()
	go func() {
		path, err := endpoint.ConnectProvisional(phaseCtx, remote, controlling)
		connectCh <- pathResult{path: path, err: err}
	}()

	var exchangeErr error
	var connected pathResult
	for completed := 0; completed < 2; completed++ {
		select {
		case exchangeErr = <-exchangeCh:
			if exchangeErr != nil {
				cancel()
			}
		case connected = <-connectCh:
			if connected.err != nil {
				cancel()
			} else if connected.path.ConnectionMethod == "lan_direct" {
				// A verified on-link host path is already the preferred V1 LAN
				// result. End trickle cleanly instead of waiting for unrelated
				// public STUN candidates.
				close(stopGathering)
			}
		}
	}
	if exchangeErr != nil && protocol.ErrorCode(exchangeErr) != protocol.Cancelled {
		return connectivity.Path{}, exchangeErr
	}
	if connected.err != nil {
		return connectivity.Path{}, classifyICEError(ctx, connected.err)
	}
	finalPath, err := endpoint.FinalizePath()
	if err != nil {
		return connectivity.Path{}, classifyICEError(ctx, err)
	}
	return finalPath, nil
}
