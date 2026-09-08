package app

import (
	"context"
	"crypto/ed25519"
	"crypto/tls"
	"errors"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"time"

	"github.com/Wen5555/LinkSend/internal/connectivity"
	"github.com/Wen5555/LinkSend/internal/identity"
	"github.com/Wen5555/LinkSend/internal/protocol"
	"github.com/Wen5555/LinkSend/internal/server"
	"github.com/Wen5555/LinkSend/internal/signaling"
	"github.com/Wen5555/LinkSend/internal/transfer"
	"github.com/Wen5555/LinkSend/internal/transport"
)

type DemoReport struct {
	Mode                    string            `json:"mode"`
	SourceDigest            string            `json:"source_digest"`
	ReceivedDigest          string            `json:"received_digest"`
	FileBytes               int64             `json:"file_bytes"`
	SignalingForwardedBytes uint64            `json:"signaling_forwarded_bytes"`
	SignalingReceivedBytes  uint64            `json:"signaling_received_bytes"`
	Path                    connectivity.Path `json:"path"`
	Relay                   bool              `json:"relay"`
	AuthenticatedTLS        bool              `json:"authenticated_tls13"`
	ServerURL               string            `json:"server_url"`
}

type iceDescription struct {
	Ufrag    string `json:"ufrag"`
	Password string `json:"password"`
}

// RunLocalDemo exercises the real control and data paths on one host. It is
// intentionally labelled loopback: it proves authenticated direct transfer,
// not LAN or double-NAT reachability.
func RunLocalDemo(ctx context.Context) (DemoReport, error) {
	root, err := os.MkdirTemp("", "linksend-demo-")
	if err != nil {
		return DemoReport{}, err
	}
	defer os.RemoveAll(root)
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	token := "linksend-demo-bootstrap-token-01234567890123456789"
	srv, err := server.New(server.Config{Listen: "127.0.0.1:0", Database: filepath.Join(root, "control.db"), BootstrapToken: token, AllowInsecureLoopback: true, AllowLoopbackCandidates: true})
	if err != nil {
		return DemoReport{}, err
	}
	defer srv.Close()
	httpServer := httptest.NewServer(srv.Handler())
	defer httpServer.Close()
	aID, err := identity.LoadOrCreate(filepath.Join(root, "profile-a"))
	if err != nil {
		return DemoReport{}, err
	}
	bID, err := identity.LoadOrCreate(filepath.Join(root, "profile-b"))
	if err != nil {
		return DemoReport{}, err
	}
	a, err := signaling.New(signaling.Config{ServerURL: httpServer.URL, Identity: aID, AllowInsecureLoopback: true})
	if err != nil {
		return DemoReport{}, err
	}
	b, err := signaling.New(signaling.Config{ServerURL: httpServer.URL, Identity: bID, AllowInsecureLoopback: true})
	if err != nil {
		return DemoReport{}, err
	}
	if _, err = a.Bootstrap(ctx, token, "demo-a"); err != nil {
		return DemoReport{}, fmt.Errorf("bootstrap: %w", err)
	}
	invitation, err := a.CreateInvitation(ctx)
	if err != nil {
		return DemoReport{}, fmt.Errorf("invitation: %w", err)
	}
	if _, err = b.Join(ctx, invitation.Token, "demo-b"); err != nil {
		return DemoReport{}, fmt.Errorf("join: %w", err)
	}
	if err = identity.TrustPeer(filepath.Join(root, "profile-a"), identity.TrustedPeer{ID: bID.ID(), Name: "demo-b", PublicKey: bID.PublicKey()}, bID.ID()); err != nil {
		return DemoReport{}, err
	}
	if err = identity.TrustPeer(filepath.Join(root, "profile-b"), identity.TrustedPeer{ID: aID.ID(), Name: "demo-a", PublicKey: aID.PublicKey()}, aID.ID()); err != nil {
		return DemoReport{}, err
	}
	sa, err := a.Connect(ctx)
	if err != nil {
		return DemoReport{}, fmt.Errorf("connect a signaling: %w", err)
	}
	defer sa.Close()
	sb, err := b.Connect(ctx)
	if err != nil {
		return DemoReport{}, fmt.Errorf("connect b signaling: %w", err)
	}
	defer sb.Close()
	ea, err := connectivity.New(connectivity.Config{BindAddress: "127.0.0.1:0", AllowLoopback: true, Generation: 1, CheckTimeout: 10 * time.Second})
	if err != nil {
		return DemoReport{}, err
	}
	defer ea.Close()
	eb, err := connectivity.New(connectivity.Config{BindAddress: "127.0.0.1:0", AllowLoopback: true, Generation: 1, CheckTimeout: 10 * time.Second})
	if err != nil {
		return DemoReport{}, err
	}
	defer eb.Close()
	if err = ea.Gather(); err != nil {
		return DemoReport{}, fmt.Errorf("gather a: %w", err)
	}
	if err = eb.Gather(); err != nil {
		return DemoReport{}, fmt.Errorf("gather b: %w", err)
	}
	aCandidates := collectCandidates(ea)
	bCandidates := collectCandidates(eb)
	if len(aCandidates) == 0 || len(bCandidates) == 0 {
		return DemoReport{}, errors.New("NO_CANDIDATES: loopback fixture gathered no host candidates")
	}
	sessionID := protocol.RandomID()
	request, err := protocol.NewEnvelope("connect_request", aID.ID(), bID.ID(), sessionID, 1, iceDescription{Ufrag: ea.Credentials().Ufrag, Password: ea.Credentials().Password})
	if err != nil {
		return DemoReport{}, err
	}
	if err = sa.SendEnvelope(ctx, request); err != nil {
		return DemoReport{}, fmt.Errorf("send connect request: %w", err)
	}
	if err = receiveEnvelope(ctx, sb, request, aID.PublicKey(), "connect_request"); err != nil {
		return DemoReport{}, err
	}
	response, err := protocol.NewEnvelope("connect_response", bID.ID(), aID.ID(), sessionID, 1, iceDescription{Ufrag: eb.Credentials().Ufrag, Password: eb.Credentials().Password})
	if err != nil {
		return DemoReport{}, err
	}
	if err = sb.SendEnvelope(ctx, response); err != nil {
		return DemoReport{}, fmt.Errorf("send connect response: %w", err)
	}
	if err = receiveEnvelope(ctx, sa, response, bID.PublicKey(), "connect_response"); err != nil {
		return DemoReport{}, err
	}
	for _, candidate := range aCandidates {
		if err = sendCandidate(ctx, sa, sb, eb, aID, bID, sessionID, candidate); err != nil {
			return DemoReport{}, err
		}
	}
	for _, candidate := range bCandidates {
		if err = sendCandidate(ctx, sb, sa, ea, bID, aID, sessionID, candidate); err != nil {
			return DemoReport{}, err
		}
	}
	endA, _ := protocol.NewEnvelope("end_of_candidates", aID.ID(), bID.ID(), sessionID, 1, map[string]any{})
	endB, _ := protocol.NewEnvelope("end_of_candidates", bID.ID(), aID.ID(), sessionID, 1, map[string]any{})
	if err = sa.SendEnvelope(ctx, endA); err != nil {
		return DemoReport{}, err
	}
	if err = receiveEnvelope(ctx, sb, endA, aID.PublicKey(), "end_of_candidates"); err != nil {
		return DemoReport{}, err
	}
	if err = sb.SendEnvelope(ctx, endB); err != nil {
		return DemoReport{}, err
	}
	if err = receiveEnvelope(ctx, sa, endB, bID.PublicKey(), "end_of_candidates"); err != nil {
		return DemoReport{}, err
	}
	pathA, _, sessions, err := establishPair(ctx, ea, eb, aID, bID)
	if err != nil {
		return DemoReport{}, err
	}
	defer sessions[0].Close()
	defer sessions[1].Close()
	sourceDir := filepath.Join(root, "source")
	destDir := filepath.Join(root, "received")
	if err = os.MkdirAll(sourceDir, 0700); err != nil {
		return DemoReport{}, err
	}
	if err = os.MkdirAll(filepath.Join(sourceDir, "empty"), 0700); err != nil {
		return DemoReport{}, err
	}
	payload := make([]byte, 1<<20)
	for i := range payload {
		payload[i] = byte((i*31 + 7) % 251)
	}
	sourceFile := filepath.Join(sourceDir, "demo.bin")
	if err = os.WriteFile(sourceFile, payload, 0600); err != nil {
		return DemoReport{}, err
	}
	prepared, err := transfer.Prepare(ctx, []string{sourceDir}, 64<<10)
	if err != nil {
		return DemoReport{}, fmt.Errorf("prepare: %w", err)
	}
	defer prepared.Close()
	resultCh := make(chan error, 1)
	receivedResultCh := make(chan transfer.Result, 1)
	go func() {
		stream, e := sessions[1].Conn.AcceptStream(ctx)
		if e != nil {
			resultCh <- e
			return
		}
		result, receiveErr := transfer.Receive(ctx, stream, destDir, aID.ID(), func(transfer.Manifest) bool { return true }, nil)
		if receiveErr == nil {
			receivedResultCh <- result
		}
		e = receiveErr
		resultCh <- e
	}()
	stream, err := sessions[0].Conn.OpenStreamSync(ctx)
	if err != nil {
		return DemoReport{}, err
	}
	sent, err := transfer.Send(ctx, stream, prepared, nil)
	if err != nil {
		return DemoReport{}, fmt.Errorf("send transfer: %w", err)
	}
	if err = <-resultCh; err != nil {
		return DemoReport{}, fmt.Errorf("receive transfer: %w", err)
	}
	receivedResult := <-receivedResultCh
	received, err := transfer.Prepare(ctx, []string{filepath.Join(destDir, "source")}, 64<<10)
	if err != nil {
		return DemoReport{}, fmt.Errorf("hash received: %w", err)
	}
	if receivedResult.Digest != sent.Digest || receivedResult.Bytes != sent.Bytes {
		return DemoReport{}, errors.New("INTEGRITY_FAILED: transfer completion digest mismatch")
	}
	sourceFiles := make(map[string]transfer.FileEntry)
	for _, entry := range prepared.Manifest.Files {
		sourceFiles[entry.Path] = entry
	}
	for _, entry := range received.Manifest.Files {
		original, ok := sourceFiles[entry.Path]
		if !ok || original.Type != entry.Type || original.Size != entry.Size || original.Hash != entry.Hash {
			return DemoReport{}, fmt.Errorf("INTEGRITY_FAILED: content mismatch at %s", entry.Path)
		}
	}
	receivedDigest := receivedResult.Digest
	_ = received.Close()
	counters := srv.Counters()
	if sent.Digest != prepared.Manifest.Digest() || receivedDigest != sent.Digest {
		return DemoReport{}, errors.New("INTEGRITY_FAILED: demo manifest digest mismatch")
	}
	if counters.ForwardedBytes >= uint64(len(payload)) {
		return DemoReport{}, fmt.Errorf("unexpected signaling volume: %d bytes for %d file bytes", counters.ForwardedBytes, len(payload))
	}
	return DemoReport{Mode: "loopback-direct", SourceDigest: prepared.Manifest.Digest(), ReceivedDigest: receivedDigest, FileBytes: int64(len(payload)), SignalingForwardedBytes: counters.ForwardedBytes, SignalingReceivedBytes: counters.ReceivedBytes, Path: pathA, Relay: false, AuthenticatedTLS: sessions[0].Conn.ConnectionState().TLS.Version == 0x0304, ServerURL: httpServer.URL}, nil
}

func collectCandidates(e *connectivity.Endpoint) []connectivity.Candidate {
	var out []connectivity.Candidate
	for c := range e.Candidates() {
		out = append(out, c)
	}
	return out
}

func receiveEnvelope(ctx context.Context, session *signaling.Session, expected protocol.Envelope, key ed25519.PublicKey, kind string) error {
	wire, err := session.Read(ctx)
	if err != nil {
		return fmt.Errorf("receive %s: %w", kind, err)
	}
	if wire.Message == nil || wire.Message.Type != kind || wire.Message.MessageID != expected.MessageID || wire.Message.SessionID != expected.SessionID || wire.Message.Recipient != expected.Recipient {
		return fmt.Errorf("INVALID_MESSAGE: unexpected %s envelope", kind)
	}
	if err = wire.Message.Verify(key, time.Now()); err != nil {
		return fmt.Errorf("verify %s: %w", kind, err)
	}
	return nil
}

func sendCandidate(ctx context.Context, from, to *signaling.Session, target *connectivity.Endpoint, sender, recipient *identity.Identity, sessionID string, candidate connectivity.Candidate) error {
	env, err := protocol.NewEnvelope("candidate", sender.ID(), recipient.ID(), sessionID, candidate.Generation, protocol.Candidate{Candidate: candidate.Value})
	if err != nil {
		return err
	}
	if err = from.SendEnvelope(ctx, env); err != nil {
		return err
	}
	wire, err := to.Read(ctx)
	if err != nil {
		return err
	}
	if wire.Message == nil || wire.Message.MessageID != env.MessageID {
		return errors.New("INVALID_MESSAGE: candidate routing mismatch")
	}
	if err = wire.Message.Verify(sender.PublicKey(), time.Now()); err != nil {
		return err
	}
	return target.AddRemoteCandidate(candidate)
}

func establishPair(ctx context.Context, ea, eb *connectivity.Endpoint, aID, bID *identity.Identity) (connectivity.Path, connectivity.Path, [2]*transport.Session, error) {
	clientTLS, err := aID.TLSConfig(bID.PublicKey(), false)
	if err != nil {
		return connectivity.Path{}, connectivity.Path{}, [2]*transport.Session{}, err
	}
	serverTLS, err := bID.TLSConfig(aID.PublicKey(), true)
	if err != nil {
		return connectivity.Path{}, connectivity.Path{}, [2]*transport.Session{}, err
	}
	type result struct {
		path connectivity.Path
		s    *transport.Session
		err  error
	}
	resultB := make(chan result, 1)
	go func() {
		path, e := eb.Connect(ctx, connectivity.Credentials{Ufrag: ea.Credentials().Ufrag, Password: ea.Credentials().Password}, false)
		if e != nil {
			resultB <- result{err: e}
			return
		}
		s, e := transport.Establish(ctx, eb, path, serverTLS, false)
		resultB <- result{path: path, s: s, err: e}
	}()
	pathA, err := ea.Connect(ctx, connectivity.Credentials{Ufrag: eb.Credentials().Ufrag, Password: eb.Credentials().Password}, true)
	if err != nil {
		return connectivity.Path{}, connectivity.Path{}, [2]*transport.Session{}, err
	}
	sA, err := transport.Establish(ctx, ea, pathA, clientTLS, true)
	if err != nil {
		return connectivity.Path{}, connectivity.Path{}, [2]*transport.Session{}, err
	}
	rB := <-resultB
	if rB.err != nil {
		_ = sA.Close()
		return connectivity.Path{}, connectivity.Path{}, [2]*transport.Session{}, rB.err
	}
	if sA.Conn.ConnectionState().TLS.Version != tls.VersionTLS13 || rB.s.Conn.ConnectionState().TLS.Version != tls.VersionTLS13 {
		_ = sA.Close()
		_ = rB.s.Close()
		return connectivity.Path{}, connectivity.Path{}, [2]*transport.Session{}, errors.New("AUTHENTICATION_FAILED: TLS 1.3 was not negotiated")
	}
	return pathA, rB.path, [2]*transport.Session{sA, rB.s}, nil
}
