package app

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"time"

	"github.com/Wen5555/LinkSend/internal/connectivity"
	"github.com/Wen5555/LinkSend/internal/identity"
	"github.com/Wen5555/LinkSend/internal/protocol"
	"github.com/Wen5555/LinkSend/internal/sharegrant"
	"github.com/Wen5555/LinkSend/internal/signaling"
	"github.com/Wen5555/LinkSend/internal/transport"
)

// connectSharePeer uses a positive, one-task authorization owner. It does not
// enroll a peer or enter the ordinary peer/generation connection pool. Preview
// and transfer callers supply different immutable scopes and new connections.
func (s *Service) connectSharePeer(ctx context.Context, scope sharegrant.SignalScope, pinned signaling.ShareStatus, cfg DirectConfig, authorize func() error) (_ *PeerSession, err error) {
	if scope.Validate() != nil || authorize == nil {
		return nil, protocol.Fail(protocol.ShareRejected, "temporary task authorization required")
	}
	self := base64.RawStdEncoding.EncodeToString(s.identity.PublicKey())
	initiating := self == pinned.SenderKey
	if !initiating && self != pinned.ReceiverKey {
		return nil, protocol.Fail(protocol.ShareRejected, "temporary actor does not match original grant")
	}
	key := pinned.SenderKey
	if initiating {
		key = pinned.ReceiverKey
	}
	public, decodeErr := base64.RawStdEncoding.DecodeString(key)
	if decodeErr != nil || len(public) != ed25519.PublicKeySize {
		return nil, protocol.Fail(protocol.ShareRejected, "temporary peer key invalid")
	}
	peer := signaling.Device{ID: identity.DeviceID(public), PublicKey: ed25519.PublicKey(public), Name: "Temporary share"}
	check := func() error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := s.checkPeerAllowed(peer.ID); err != nil {
			return err
		}
		return authorize()
	}
	if err = check(); err != nil {
		return nil, err
	}
	budget := cfg.CheckTimeout
	if budget <= 0 {
		budget = 30 * time.Second
	}
	started := time.Now()
	var timings DirectTimings
	cfg.disableSessionReuse, cfg.disableClipboardSync, cfg.disableClipboardFiles = true, true, true
	// The ordinary authorization_generation field must stay zero. Grant and
	// ICE generations are independently authenticated by ShareSignalScope.
	cfg.expectedAuthorizationGeneration = 0
	client, err := s.client()
	if err != nil {
		return nil, err
	}
	connectCtx, cancelConnect := context.WithTimeout(ctx, budget)
	defer cancelConnect()
	cfg.phase("signaling_connect")
	stage := time.Now()
	signal, err := client.ConnectShare(connectCtx, scope, pinned, signaling.ShareSignalOptions{AllowLoopbackCandidates: cfg.AllowLoopback})
	if err != nil {
		return nil, err
	}
	keep := false
	stopHeartbeat := startHeartbeat(ctx, signal)
	defer func() {
		if !keep {
			stopHeartbeat()
			_ = signal.Close()
		}
	}()
	if err = signal.WaitPeer(connectCtx); err != nil {
		return nil, err
	}
	timings.SignalingConnectMS = durationMS(stage)
	if err = check(); err != nil {
		return nil, err
	}
	cfg.phase("endpoint_setup")
	stage = time.Now()
	endpoint, err := s.newEndpoint(cfg)
	if err != nil {
		return nil, classifyEndpointSetupError(err)
	}
	defer func() {
		if !keep {
			_ = endpoint.Close()
		}
	}()
	cfg.phase("candidate_gathering")
	if err = endpoint.Gather(); err != nil {
		return nil, classifyEndpointSetupError(err)
	}
	timings.EndpointSetupMS = durationMS(stage)
	var path connectivity.Path
	var data *transport.Session
	var sessionID string
	if initiating {
		sessionID = protocol.RandomID()
		cfg.session(sessionID, peer.ID)
		credentials := endpoint.Credentials()
		request, makeErr := protocol.NewEnvelope("connect_request", s.identity.ID(), peer.ID, sessionID, firstGeneration, iceDescription{Ufrag: credentials.Ufrag, Password: credentials.Password})
		if makeErr != nil {
			return nil, makeErr
		}
		requestCtx, cancelRequest := context.WithTimeout(ctx, budget)
		defer cancelRequest()
		stage = time.Now()
		if err = signal.SendEnvelope(requestCtx, request); err != nil {
			return nil, err
		}
		response, readErr := readSignalMessage(requestCtx, signal)
		if readErr != nil {
			return nil, readErr
		}
		if response.Message == nil {
			return nil, protocol.Fail(protocol.InvalidMessage, "temporary response missing")
		}
		message := *response.Message
		if message.Type == "status" {
			return nil, peerSessionFailure(&message, peer, s.identity.ID(), sessionID, firstGeneration)
		}
		if message.Type != "connect_response" || message.SessionID != sessionID || message.Generation != firstGeneration || message.Sender != peer.ID || message.Recipient != s.identity.ID() || message.Verify(peer.PublicKey, time.Now()) != nil || sharegrant.ValidateSignalPayload(message, cfg.AllowLoopback) != nil {
			return nil, protocol.Fail(protocol.AuthenticationFailed, "temporary response binding invalid")
		}
		timings.PeerResponseMS = durationMS(stage)
		var remote iceDescription
		if json.Unmarshal(message.Payload, &remote) != nil {
			return nil, protocol.Fail(protocol.InvalidMessage, "temporary ICE description invalid")
		}
		cfg.phase("ice_checking")
		stage = time.Now()
		path, err = connectWithTrickle(ctx, signal, endpoint, s.identity, peer, sessionID, firstGeneration, connectivity.Credentials{Ufrag: remote.Ufrag, Password: remote.Password}, true)
		timings.ICEMS = durationMS(stage)
		if err == nil {
			tlsConfig, tlsErr := s.identity.TLSConfig(peer.PublicKey, false)
			if tlsErr != nil {
				return nil, tlsErr
			}
			cfg.phase("quic_handshake")
			stage = time.Now()
			quicCtx, cancelQUIC := context.WithTimeout(ctx, budget)
			data, err = transport.Establish(quicCtx, endpoint, path, tlsConfig, true)
			cancelQUIC()
			timings.QUICHandshakeMS = durationMS(stage)
		}
	} else {
		wait := cfg.WaitTimeout
		if wait <= 0 {
			wait = budget
		}
		waitCtx, cancelWait := context.WithTimeout(ctx, wait)
		defer cancelWait()
		request, readErr := readSignalMessage(waitCtx, signal)
		if readErr != nil {
			return nil, readErr
		}
		if request.Message == nil || request.Message.Type != "connect_request" || request.Message.Sender != peer.ID || request.Message.Recipient != s.identity.ID() || request.Message.Generation != firstGeneration || request.Message.Verify(peer.PublicKey, time.Now()) != nil || sharegrant.ValidateSignalPayload(*request.Message, cfg.AllowLoopback) != nil {
			return nil, protocol.Fail(protocol.AuthenticationFailed, "temporary request binding invalid")
		}
		sessionID = request.Message.SessionID
		cfg.session(sessionID, peer.ID)
		if err = check(); err != nil {
			return nil, err
		}
		path, data, err = s.establishResponder(ctx, signal, endpoint, peer, *request.Message, budget, cfg)
	}
	timings.TotalConnectMS = durationMS(started)
	if err != nil {
		cfg.evidence(makeDirectEvidence(peer.ID, sessionID, endpoint.Observation(), timings, endpoint.Stats(), signal.Stats(), 0, ""))
		return nil, err
	}
	if err = check(); err != nil {
		_ = data.Close()
		return nil, err
	}
	result := &PeerSession{PeerID: peer.ID, SessionID: sessionID, Path: path, Data: data, Timings: timings, signal: signal, ownsSignal: true, stopHeartbeat: stopHeartbeat, peerPublicKey: append(ed25519.PublicKey(nil), peer.PublicKey...)}
	keep = true
	cfg.phase("connected")
	cfg.evidence(result.Evidence())
	return result, nil
}
