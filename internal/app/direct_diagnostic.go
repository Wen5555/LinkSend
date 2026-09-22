package app

import (
	"sync"

	"github.com/Wen5555/LinkSend/internal/connectivity"
	"github.com/Wen5555/LinkSend/internal/signaling"
)

// observeDirectFailure keeps the direct API's result useful when no authenticated
// PeerSession was returned. Existing task observers continue to receive events.
func observeDirectFailure(cfg DirectConfig) (DirectConfig, func(*DirectTransferResult, error)) {
	var mu sync.Mutex
	var phase string
	var evidence DirectEvidence
	onPhase, onSession, onEvidence := cfg.onPhase, cfg.onSession, cfg.onEvidence
	cfg.onPhase = func(value string) {
		mu.Lock()
		phase = value
		mu.Unlock()
		if onPhase != nil {
			onPhase(value)
		}
	}
	cfg.onSession = func(id, peer string) {
		mu.Lock()
		evidence.SessionID, evidence.PeerID = id, peer
		mu.Unlock()
		if onSession != nil {
			onSession(id, peer)
		}
	}
	cfg.onEvidence = func(value DirectEvidence) {
		mu.Lock()
		evidence = value
		mu.Unlock()
		if onEvidence != nil {
			onEvidence(value)
		}
	}
	return cfg, func(result *DirectTransferResult, err error) {
		if err == nil {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		if result.FailurePhase == "" {
			result.FailurePhase = phase
		}
		result.FailureDiagnostic = taskHandshakeDiagnostic(err, result.FailurePhase)
		if result.Evidence.SessionID == "" {
			result.Evidence = evidence
		}
	}
}

// A nominated ICE path is observable even when QUIC authentication fails. Zero
// TLS version / empty ALPN explicitly mean that TLS was not established.
func makeDirectEvidence(peerID, sessionID string, path connectivity.Path, timings DirectTimings,
	stats connectivity.Stats, signalStats signaling.SessionStats, tlsVersion uint16, alpn string) DirectEvidence {
	return DirectEvidence{SessionID: sessionID, Generation: path.Generation, PeerID: peerID,
		BaseSocket: path.BaseSocket, Interface: path.Interface, AddressFamily: path.AddressFamily,
		LocalCandidate: path.LocalCandidate, RemoteCandidate: path.RemoteCandidate,
		LocalType: path.LocalType, RemoteType: path.RemoteType, RemoteAddress: path.RemoteAddress,
		ConnectionMethod: path.ConnectionMethod, TransportProtocol: path.TransportProtocol, Relay: path.Relay,
		STUNBytesSent: stats.STUNBytesSent, STUNBytesReceived: stats.STUNBytesReceived,
		STUNRequestsSent: stats.STUNRequestsSent, STUNResponsesReceived: stats.STUNResponsesReceived,
		RejectedPackets: stats.RejectedPackets, SignalingBytesSent: signalStats.BytesSent,
		SignalingBytesReceived: signalStats.BytesReceived,
		ICEStateTimeline:       append([]connectivity.ICEStateEvent(nil), path.ICEStateTimeline...),
		TLSVersion:             tlsVersion, ALPN: alpn, Timings: timings}
}
