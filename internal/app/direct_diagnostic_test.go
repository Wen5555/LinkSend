package app

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/Wen5555/LinkSend/internal/connectivity"
	"github.com/Wen5555/LinkSend/internal/identity"
	"github.com/Wen5555/LinkSend/internal/protocol"
	"github.com/Wen5555/LinkSend/internal/signaling"
	"github.com/Wen5555/LinkSend/internal/transfer"
	"github.com/pion/stun/v4"
)

// This fixture answers actual Binding requests on a local UDP socket. The
// nominated remote ICE socket is a separate bound sink which never answers.
// A STUN response is deliberately not sufficient to establish a connection.
func diagnosticSTUNServer(t *testing.T) string {
	t.Helper()
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		buffer := make([]byte, 2048)
		for {
			n, from, err := conn.ReadFromUDP(buffer)
			if err != nil {
				return
			}
			request := &stun.Message{Raw: append([]byte(nil), buffer[:n]...)}
			if request.Decode() != nil || request.Type != stun.BindingRequest {
				continue
			}
			response, err := stun.Build(stun.NewTransactionIDSetter(request.TransactionID), stun.BindingSuccess,
				&stun.XORMappedAddress{IP: net.IPv4(203, 0, 113, 20), Port: from.Port}, stun.Fingerprint)
			if err != nil {
				return
			}
			_, _ = conn.WriteToUDP(response.Raw, from)
		}
	}()
	t.Cleanup(func() { _ = conn.Close(); <-done })
	return "stun:" + conn.LocalAddr().String()
}

func sendUnreachableDiagnosticCandidates(ctx context.Context, session *signaling.Session, local, remote, sessionID, address string) error {
	_, port, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil {
		return err
	}
	candidate, err := protocol.NewEnvelope("candidate", local, remote, sessionID, firstGeneration,
		protocol.Candidate{Candidate: "1 1 udp 2130706431 127.0.0.1 " + strconv.Itoa(portNumber) + " typ host"})
	if err != nil {
		return err
	}
	if err = session.SendEnvelope(ctx, candidate); err != nil {
		return err
	}
	end, err := protocol.NewEnvelope("end_of_candidates", local, remote, sessionID, firstGeneration, map[string]any{})
	if err != nil {
		return err
	}
	return session.SendEnvelope(ctx, end)
}

func TestICEFailureRetainsActualSTUNAndSignalingEvidence(t *testing.T) {
	for _, role := range []string{"initiator", "responder"} {
		t.Run(role, func(t *testing.T) {
			f := newDirectFixtureServices(t)
			ctx, cancel := context.WithTimeout(t.Context(), 12*time.Second)
			defer cancel()
			stunURL := diagnosticSTUNServer(t)
			blackhole, err := net.ListenPacket("udp4", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer blackhole.Close()
			cfg, finish := observeDirectFailure(DirectConfig{BindAddress: "127.0.0.1:0", AllowLoopback: true,
				STUNURLs: []string{stunURL}, CheckTimeout: 2 * time.Second})
			remoteID, localID := f.bID, f.aID
			remoteService, localService := f.b, f.a
			if role == "responder" {
				remoteID, localID = f.aID, f.bID
				remoteService, localService = f.a, f.b
			}
			remoteClient, err := remoteService.client()
			if err != nil {
				t.Fatal(err)
			}
			remoteSignal, err := remoteClient.Connect(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer remoteSignal.Close()
			remoteWork := make(chan error, 1)
			var connectionErr error
			if role == "initiator" {
				go func() {
					wire, err := readSignalMessage(ctx, remoteSignal)
					if err != nil {
						remoteWork <- err
						return
					}
					if wire.Message == nil || wire.Message.Type != "connect_request" {
						remoteWork <- errors.New("expected signed connection request")
						return
					}
					response, err := protocol.NewEnvelope("connect_response", remoteID.ID(), localID.ID(), wire.Message.SessionID,
						firstGeneration, iceDescription{Ufrag: "diagnostic_remote", Password: "01234567890123456789012345678901"})
					if err == nil {
						err = remoteSignal.SendEnvelope(ctx, response)
					}
					if err == nil {
						err = sendUnreachableDiagnosticCandidates(ctx, remoteSignal, remoteID.ID(), localID.ID(), wire.Message.SessionID, blackhole.LocalAddr().String())
					}
					remoteWork <- err
				}()
				peer, err := localService.ConnectDirect(ctx, remoteID.ID(), cfg)
				connectionErr = err
				if peer != nil {
					_ = peer.Close()
					t.Fatal("unresponsive ICE sink unexpectedly connected")
				}
			} else {
				localClient, err := localService.client()
				if err != nil {
					t.Fatal(err)
				}
				localSignal, err := localClient.Connect(ctx)
				if err != nil {
					t.Fatal(err)
				}
				defer localSignal.Close()
				request, err := protocol.NewEnvelope("connect_request", remoteID.ID(), localID.ID(), protocol.RandomID(), firstGeneration,
					iceDescription{Ufrag: "diagnostic_remote", Password: "01234567890123456789012345678901"})
				if err != nil {
					t.Fatal(err)
				}
				if err = remoteSignal.SendEnvelope(ctx, request); err != nil {
					t.Fatal(err)
				}
				wire, err := readSignalMessage(ctx, localSignal)
				if err != nil || wire.Message == nil || wire.Message.Type != "connect_request" {
					t.Fatalf("missing signed request: %v", err)
				}
				endpoint, err := localService.newEndpoint(cfg)
				if err != nil {
					t.Fatal(err)
				}
				defer endpoint.Close()
				if err = endpoint.Gather(); err != nil {
					t.Fatal(err)
				}
				cfg.session(wire.Message.SessionID, remoteID.ID())
				go func() {
					response, err := readSignalMessage(ctx, remoteSignal)
					if err == nil && (response.Message == nil || response.Message.Type != "connect_response") {
						err = errors.New("expected signed connection response")
					}
					if err == nil {
						err = sendUnreachableDiagnosticCandidates(ctx, remoteSignal, remoteID.ID(), localID.ID(), wire.Message.SessionID, blackhole.LocalAddr().String())
					}
					remoteWork <- err
				}()
				_, data, err := localService.establishResponder(ctx, localSignal, endpoint,
					signaling.Device{ID: remoteID.ID(), PublicKey: remoteID.PublicKey()}, *wire.Message, cfg.CheckTimeout, cfg)
				connectionErr = err
				if data != nil {
					_ = data.Close()
					t.Fatal("unresponsive ICE sink unexpectedly authenticated")
				}
			}
			select {
			case err = <-remoteWork:
				if err != nil {
					t.Fatal(err)
				}
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			var result DirectTransferResult
			finish(&result, connectionErr)
			if connectionErr == nil || protocol.ErrorCode(connectionErr) != protocol.CheckTimeout || result.FailurePhase != "ice_checking" {
				t.Fatalf("expected actual ICE timeout, phase=%q err=%v", result.FailurePhase, connectionErr)
			}
			evidence := result.Evidence
			if evidence.STUNRequestsSent == 0 || evidence.STUNResponsesReceived == 0 || evidence.SignalingBytesSent == 0 || evidence.SignalingBytesReceived == 0 || evidence.BaseSocket == "" || evidence.Interface == "" || evidence.AddressFamily != "ipv4" || evidence.Timings.ICEMS <= 0 {
				t.Fatalf("ICE failure lost real observations: %+v", evidence)
			}
			if evidence.TLSVersion != 0 || evidence.ALPN != "" || evidence.Relay || evidence.ConnectionMethod != "" || evidence.LocalCandidate != "" || evidence.RemoteCandidate != "" || evidence.RemoteAddress != "" || result.Transfer.Bytes != 0 {
				t.Fatalf("failed ICE fabricated a nominated path, TLS or file transfer: %+v", evidence)
			}
		})
	}
}

func TestDetailedDirectFailureRetainsObservedPhase(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("<html>upstream unavailable</html>"))
	}))
	defer server.Close()
	for _, method := range []string{"send", "prepared", "receive"} {
		t.Run(method, func(t *testing.T) {
			svc, err := New(Config{DataDir: t.TempDir(), ServerURL: server.URL, AllowInsecureLoopback: true})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(svc.Shutdown)
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			path := filepath.Join(t.TempDir(), "fixture.txt")
			if err = os.WriteFile(path, []byte("fixture"), 0600); err != nil {
				t.Fatal(err)
			}
			prepared, err := transfer.Prepare(ctx, []string{path}, 0)
			if err != nil {
				t.Fatal(err)
			}
			defer prepared.Close()
			var observed string
			cfg := DirectConfig{onPhase: func(phase string) { observed = phase }}
			var result DirectTransferResult
			switch method {
			case "send":
				result, err = svc.SendFilesDetailed(ctx, strings.Repeat("a", 64), []string{path}, cfg, nil)
			case "prepared":
				result, err = svc.SendPreparedWithHooksDetailed(ctx, strings.Repeat("a", 64), prepared, cfg, transfer.SendHooks{})
			case "receive":
				result, err = svc.ReceiveOnceDetailed(ctx, "", t.TempDir(), cfg, func(transfer.Manifest) bool { t.Fatal("unexpected consent"); return false }, nil)
			}
			if err == nil || observed == "" {
				t.Fatalf("expected observed connection failure: %v", err)
			}
			encoded, marshalErr := json.Marshal(result)
			if marshalErr != nil {
				t.Fatal(marshalErr)
			}
			var output map[string]json.RawMessage
			if err = json.Unmarshal(encoded, &output); err != nil {
				t.Fatal(err)
			}
			var phase string
			_ = json.Unmarshal(output["failure_phase"], &phase)
			if phase != observed {
				t.Fatalf("detailed failure lost actual phase: got %q want %q", phase, observed)
			}
			if result.Transfer.Bytes != 0 || result.Evidence.TLSVersion != 0 || result.Evidence.ALPN != "" || result.Evidence.SessionID != "" {
				t.Fatal("pre-connection failure fabricated transfer/session/TLS evidence")
			}
		})
	}
}

func TestDirectPreparationFailureIsNotAConnectionFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer server.Close()
	svc, err := New(Config{DataDir: t.TempDir(), ServerURL: server.URL, AllowInsecureLoopback: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(svc.Shutdown)
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	result, err := svc.SendFilesDetailed(ctx, strings.Repeat("a", 64), []string{filepath.Join(t.TempDir(), "missing")}, DirectConfig{}, nil)
	if err == nil || result.FailurePhase != "preparing" || result.FailureDiagnostic != nil {
		t.Fatalf("preparation error attributed to connection: phase=%s error=%v", result.FailurePhase, err)
	}
}

func TestDirectRejectionKeepsTransferPhaseAndActualTLS(t *testing.T) {
	f := newDirectFixtureServices(t)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	ready := make(chan struct{})
	var readyOnce sync.Once
	type outcome struct {
		result DirectTransferResult
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		result, err := f.b.ReceiveOnceDetailed(ctx, f.aID.ID(), t.TempDir(), DirectConfig{AllowLoopback: true, CheckTimeout: 3 * time.Second,
			onPhase: func(phase string) {
				if phase == "waiting" {
					readyOnce.Do(func() { close(ready) })
				}
			}}, func(transfer.Manifest) bool { return false }, nil)
		done <- outcome{result, err}
	}()
	select {
	case <-ready:
	case early := <-done:
		t.Fatalf("receiver stopped early: %v", early.err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	path := filepath.Join(t.TempDir(), "rejected.txt")
	if err := os.WriteFile(path, []byte("reject before file bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	sent, sendErr := f.a.SendFilesDetailed(ctx, f.bID.ID(), []string{path}, DirectConfig{AllowLoopback: true, CheckTimeout: 3 * time.Second}, nil)
	var received outcome
	select {
	case received = <-done:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	for _, value := range []outcome{{sent, sendErr}, received} {
		if value.err == nil || value.result.FailurePhase != "transfer" || value.result.FailureDiagnostic != nil {
			t.Fatalf("rejection reported as handshake/open-stream failure: %+v %v", value.result, value.err)
		}
		if value.result.Evidence.TLSVersion != 0x0304 || value.result.Evidence.ALPN != identity.ALPN || value.result.Transfer.Bytes != 0 {
			t.Fatalf("rejection lost established TLS or claimed file bytes: %+v", value.result)
		}
	}
}

func TestDirectHandshakeFailureKeepsSafeObservedEvidence(t *testing.T) {
	var phases, sessions, snapshots int
	cfg, finish := observeDirectFailure(DirectConfig{onPhase: func(string) { phases++ }, onSession: func(string, string) { sessions++ }, onEvidence: func(DirectEvidence) { snapshots++ }})
	cfg.session("session", "peer")
	cfg.phase("quic_handshake")
	path := connectivity.Path{Generation: 1, BaseSocket: "192.0.2.1:1234", RemoteAddress: "192.0.2.2:1234", LocalCandidate: "host", RemoteCandidate: "host",
		ICEStateTimeline: []connectivity.ICEStateEvent{{State: "connected", At: "fixture"}}}
	snapshot := makeDirectEvidence("peer", "session", path, DirectTimings{ICEMS: 1}, connectivity.Stats{STUNRequestsSent: 2}, signaling.SessionStats{BytesSent: 3}, 0, "")
	path.ICEStateTimeline[0].State = "mutated"
	cfg.evidence(snapshot)
	err := protocol.Wrap(protocol.QUICHandshakeFailed, "QUIC handshake failed", &net.OpError{Op: "write", Net: "udp", Err: syscall.Errno(10065)})
	var result DirectTransferResult
	finish(&result, err)
	if result.FailurePhase != "quic_handshake" || result.FailureDiagnostic == nil || result.FailureDiagnostic.Category != "udp_socket" || result.FailureDiagnostic.Code != "0x2751" {
		t.Fatalf("typed handshake diagnostic missing: %+v", result)
	}
	if result.Evidence.TLSVersion != 0 || result.Evidence.ALPN != "" || result.Evidence.SessionID != "session" || result.Evidence.STUNRequestsSent != 2 || result.Evidence.SignalingBytesSent != 3 || result.Evidence.ICEStateTimeline[0].State != "connected" {
		t.Fatal("failed handshake lost observations or fabricated negotiated TLS")
	}
	if phases != 1 || sessions != 1 || snapshots != 1 {
		t.Fatal("existing observers were not preserved")
	}
	encoded, _ := json.Marshal(result)
	if strings.Contains(string(encoded), "password") || strings.Contains(string(encoded), "private_key") {
		t.Fatal("credential data in direct evidence")
	}
	var success DirectTransferResult
	finish(&success, nil)
	if success.FailurePhase != "" || success.FailureDiagnostic != nil {
		t.Fatal("success received failure-only metadata")
	}
	lookupCfg, lookupFinish := observeDirectFailure(DirectConfig{})
	lookupCfg.phase("peer_lookup")
	lookupFinish(&success, protocol.Wrap(protocol.AuthenticationFailed, "peer lookup denied", errors.New("private cause")))
	if success.FailureDiagnostic != nil {
		t.Fatal("pre-handshake authentication mislabeled as TLS")
	}
}
