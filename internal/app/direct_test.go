package app

import (
	"bytes"
	"context"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Wen5555/LinkSend/internal/identity"
	"github.com/Wen5555/LinkSend/internal/server"
	"github.com/Wen5555/LinkSend/internal/transfer"
)

func TestDirectServiceSendAndReceive(t *testing.T) {
	root := t.TempDir()
	token := "direct-service-test-token-012345678901234567890"
	srv, err := server.New(server.Config{Listen: "127.0.0.1:0", Database: filepath.Join(root, "control.db"), BootstrapToken: token, AllowInsecureLoopback: true, AllowLoopbackCandidates: true})
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	h := httptest.NewServer(srv.Handler())
	defer h.Close()
	aID, _ := identity.Generate()
	bID, _ := identity.Generate()
	a, err := New(Config{DataDir: filepath.Join(root, "a"), ServerURL: h.URL, AllowInsecureLoopback: true, Identity: aID})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(a.Shutdown)
	b, err := New(Config{DataDir: filepath.Join(root, "b"), ServerURL: h.URL, AllowInsecureLoopback: true, Identity: bID})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(b.Shutdown)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err = a.Bootstrap(ctx, token, "a"); err != nil {
		t.Fatal(err)
	}
	inv, err := a.CreateInvitation(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = b.Join(ctx, inv.Token, "b"); err != nil {
		t.Fatal(err)
	}
	if err = a.Trust(ctx, bID.ID(), bID.ID()); err != nil {
		t.Fatal(err)
	}
	if err = b.Trust(ctx, aID.ID(), aID.ID()); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "source.txt")
	data := []byte("authenticated application service transfer")
	if err = os.WriteFile(source, data, 0600); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(root, "dest")
	acceptErr := make(chan error, 1)
	resultCh := make(chan DirectTransferResult, 1)
	receiverReady := make(chan struct{})
	var receiverReadyOnce sync.Once
	go func() {
		result, receiveErr := b.ReceiveOnceDetailed(ctx, aID.ID(), dest, DirectConfig{
			AllowLoopback: true,
			onPhase: func(phase string) {
				if phase == "waiting" {
					receiverReadyOnce.Do(func() { close(receiverReady) })
				}
			},
		}, func(transfer.Manifest) bool { return true }, nil)
		if receiveErr != nil {
			acceptErr <- receiveErr
			return
		}
		resultCh <- result
		acceptErr <- nil
	}()
	select {
	case <-receiverReady:
	case err = <-acceptErr:
		if err == nil {
			t.Fatal("receiver stopped before entering the signaling wait state")
		}
		t.Fatal(err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	sent, err := a.SendFilesDetailed(ctx, bID.ID(), []string{source}, DirectConfig{AllowLoopback: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = <-acceptErr; err != nil {
		t.Fatal(err)
	}
	received := <-resultCh
	if sent.Transfer.Digest != received.Transfer.Digest || received.Transfer.Bytes != int64(len(data)) {
		t.Fatalf("transfer mismatch: sent=%+v received=%+v", sent, received)
	}
	for _, evidence := range []DirectEvidence{sent.Evidence, received.Evidence} {
		if evidence.PeerID == "" || evidence.BaseSocket == "" || evidence.Interface == "" || evidence.AddressFamily != "ipv4" || evidence.LocalCandidate == "" || evidence.RemoteCandidate == "" || evidence.TransportProtocol != "quic" || evidence.Relay || evidence.TLSVersion != 0x0304 || evidence.ALPN != identity.ALPN {
			t.Fatalf("incomplete direct evidence: %+v", evidence)
		}
		if evidence.STUNRequestsSent == 0 || evidence.STUNResponsesReceived == 0 || evidence.SignalingBytesSent == 0 || evidence.SignalingBytesReceived == 0 || len(evidence.ICEStateTimeline) == 0 {
			t.Fatalf("missing structured connection counters/timeline: %+v", evidence)
		}
	}
}

func TestSimultaneousConnectUsesDeterministicSessionOverRealQUIC(t *testing.T) {
	root := t.TempDir()
	token := "simultaneous-connect-token-012345678901234567"
	srv, err := server.New(server.Config{Listen: "127.0.0.1:0", Database: filepath.Join(root, "control.db"), BootstrapToken: token, AllowInsecureLoopback: true, AllowLoopbackCandidates: true})
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	h := httptest.NewServer(srv.Handler())
	defer h.Close()
	aID, _ := identity.Generate()
	bID, _ := identity.Generate()
	a, err := New(Config{DataDir: filepath.Join(root, "a"), ServerURL: h.URL, AllowInsecureLoopback: true, Identity: aID})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(a.Shutdown)
	b, err := New(Config{DataDir: filepath.Join(root, "b"), ServerURL: h.URL, AllowInsecureLoopback: true, Identity: bID})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(b.Shutdown)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if _, err = a.Bootstrap(ctx, token, "a"); err != nil {
		t.Fatal(err)
	}
	inv, err := a.CreateInvitation(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = b.Join(ctx, inv.Token, "b"); err != nil {
		t.Fatal(err)
	}
	if err = a.Trust(ctx, bID.ID(), bID.ID()); err != nil {
		t.Fatal(err)
	}
	if err = b.Trust(ctx, aID.ID(), aID.ID()); err != nil {
		t.Fatal(err)
	}

	type result struct {
		id      string
		session *PeerSession
		err     error
	}
	ready := sync.WaitGroup{}
	ready.Add(2)
	makeConfig := func() DirectConfig {
		var once sync.Once
		return DirectConfig{
			AllowLoopback: true,
			CheckTimeout:  10 * time.Second,
			onSession: func(string, string) {
				once.Do(func() {
					ready.Done()
					ready.Wait()
				})
			},
		}
	}
	results := make(chan result, 2)
	go func() {
		session, connectErr := a.ConnectDirect(ctx, bID.ID(), makeConfig())
		results <- result{id: aID.ID(), session: session, err: connectErr}
	}()
	go func() {
		session, connectErr := b.ConnectDirect(ctx, aID.ID(), makeConfig())
		results <- result{id: bID.ID(), session: session, err: connectErr}
	}()
	connected := map[string]*PeerSession{}
	for range 2 {
		got := <-results
		if got.err != nil {
			t.Fatalf("simultaneous connect failed for %s: %v", got.id, got.err)
		}
		connected[got.id] = got.session
		defer got.session.Close()
	}
	if connected[aID.ID()].SessionID != connected[bID.ID()].SessionID {
		t.Fatalf("peers selected different sessions: %s != %s", connected[aID.ID()].SessionID, connected[bID.ID()].SessionID)
	}
	lowerID, higherID := aID.ID(), bID.ID()
	if higherID < lowerID {
		lowerID, higherID = higherID, lowerID
	}
	payload := []byte("simultaneous authenticated QUIC")
	received := make(chan []byte, 1)
	go func() {
		stream, acceptErr := connected[higherID].Data.Conn.AcceptStream(ctx)
		if acceptErr != nil {
			received <- nil
			return
		}
		got := make([]byte, len(payload))
		if _, acceptErr = io.ReadFull(stream, got); acceptErr != nil {
			received <- nil
			return
		}
		received <- got
	}()
	stream, err := connected[lowerID].Data.Conn.OpenStreamSync(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = stream.Write(payload); err != nil {
		t.Fatal(err)
	}
	if err = stream.Close(); err != nil {
		t.Fatal(err)
	}
	if got := <-received; !bytes.Equal(got, payload) {
		t.Fatalf("simultaneous session QUIC payload mismatch: %q", got)
	}
}
