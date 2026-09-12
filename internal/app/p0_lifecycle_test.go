package app

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wen5555/LinkSend/internal/identity"
	"github.com/Wen5555/LinkSend/internal/protocol"
	"github.com/Wen5555/LinkSend/internal/server"
)

type directFixtureServices struct {
	a, b          *Service
	aID           *identity.Identity
	bID           *identity.Identity
	server        *server.Server
	http          *httptest.Server
	wsConnections *atomic.Int64
}

func newDirectFixtureServices(t *testing.T) directFixtureServices {
	t.Helper()
	root := t.TempDir()
	token := "p0-direct-fixture-token-01234567890123456789"
	srv, err := server.New(server.Config{Listen: "127.0.0.1:0", Database: filepath.Join(root, "control.db"), BootstrapToken: token, AllowInsecureLoopback: true, AllowLoopbackCandidates: true})
	if err != nil {
		t.Fatal(err)
	}
	var wsConnections atomic.Int64
	handler := srv.Handler()
	h := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/ws" {
			wsConnections.Add(1)
		}
		handler.ServeHTTP(w, r)
	}))
	t.Cleanup(func() {
		h.Close()
		_ = srv.Close()
	})
	aID, err := identity.Generate()
	if err != nil {
		t.Fatal(err)
	}
	bID, err := identity.Generate()
	if err != nil {
		t.Fatal(err)
	}
	a, err := New(Config{DataDir: filepath.Join(root, "a"), ServerURL: h.URL, AllowInsecureLoopback: true, Identity: aID})
	if err != nil {
		t.Fatal(err)
	}
	b, err := New(Config{DataDir: filepath.Join(root, "b"), ServerURL: h.URL, AllowInsecureLoopback: true, Identity: bID})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
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
	return directFixtureServices{a: a, b: b, aID: aID, bID: bID, server: srv, http: h, wsConnections: &wsConnections}
}

func connectDirectPair(t *testing.T, f directFixtureServices, ctx context.Context) (*PeerSession, *PeerSession) {
	t.Helper()
	waiting := make(chan struct{})
	receiver := make(chan struct {
		peer *PeerSession
		err  error
	}, 1)
	go func() {
		peer, err := f.b.AcceptDirect(ctx, f.aID.ID(), DirectConfig{
			AllowLoopback: true,
			CheckTimeout:  5 * time.Second,
			WaitTimeout:   5 * time.Second,
			onPhase: func(phase string) {
				if phase == "waiting" {
					select {
					case <-waiting:
					default:
						close(waiting)
					}
				}
			},
		})
		receiver <- struct {
			peer *PeerSession
			err  error
		}{peer, err}
	}()
	select {
	case <-waiting:
	case <-ctx.Done():
		t.Fatal("receiver did not enter signaling wait")
	}
	sender, err := f.a.ConnectDirect(ctx, f.bID.ID(), DirectConfig{AllowLoopback: true, CheckTimeout: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	got := <-receiver
	if got.err != nil {
		_ = sender.Close()
		t.Fatal(got.err)
	}
	return sender, got.peer
}

func TestEstablishedQUICSurvivesSignalingDisconnect(t *testing.T) {
	f := newDirectFixtureServices(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	sender, receiver := connectDirectPair(t, f, ctx)
	defer sender.Close()
	defer receiver.Close()
	if err := sender.signal.Close(); err != nil {
		t.Fatal(err)
	}
	if err := receiver.signal.Close(); err != nil {
		t.Fatal(err)
	}

	payload := []byte("healthy QUIC remains independent from signaling")
	received := make(chan []byte, 1)
	go func() {
		stream, err := receiver.Data.Conn.AcceptStream(ctx)
		if err != nil {
			received <- nil
			return
		}
		got := make([]byte, len(payload))
		if _, err = io.ReadFull(stream, got); err != nil {
			received <- nil
			return
		}
		received <- got
	}()
	stream, err := sender.Data.Conn.OpenStreamSync(ctx)
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
		t.Fatalf("QUIC data failed after signaling disconnect: %q", got)
	}
}

func TestUnresponsivePeerTimesOutAndFreshAttemptConnects(t *testing.T) {
	f := newDirectFixtureServices(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client, err := f.b.client()
	if err != nil {
		t.Fatal(err)
	}
	unresponsive, err := client.Connect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.a.ConnectDirect(ctx, f.bID.ID(), DirectConfig{AllowLoopback: true, CheckTimeout: 150 * time.Millisecond})
	if protocol.ErrorCode(err) != protocol.SignalingTimeout {
		_ = unresponsive.Close()
		t.Fatalf("unresponsive peer error = %v, want signaling timeout", err)
	}
	_ = unresponsive.Close()

	sender, receiver := connectDirectPair(t, f, ctx)
	defer sender.Close()
	defer receiver.Close()
	if sender.SessionID == "" || sender.SessionID != receiver.SessionID {
		t.Fatalf("fresh attempt did not converge on one session: %q != %q", sender.SessionID, receiver.SessionID)
	}
}

func TestExpectedPeerMismatchReturnsSignedFailureWithoutTimeout(t *testing.T) {
	f := newDirectFixtureServices(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	waiting := make(chan struct{})
	receiverErr := make(chan error, 1)
	go func() {
		_, acceptErr := f.b.AcceptDirect(ctx, "different-trusted-device", DirectConfig{
			AllowLoopback: true,
			CheckTimeout:  4 * time.Second,
			WaitTimeout:   4 * time.Second,
			onPhase: func(phase string) {
				if phase == "waiting" {
					select {
					case <-waiting:
					default:
						close(waiting)
					}
				}
			},
		})
		receiverErr <- acceptErr
	}()
	select {
	case <-waiting:
	case <-ctx.Done():
		t.Fatal("receiver did not start")
	}
	started := time.Now()
	_, err := f.a.ConnectDirect(ctx, f.bID.ID(), DirectConfig{AllowLoopback: true, CheckTimeout: 4 * time.Second})
	if protocol.ErrorCode(err) != protocol.AuthenticationFailed {
		t.Fatalf("sender error = %v, want %s", err, protocol.AuthenticationFailed)
	}
	if time.Since(started) >= 4*time.Second {
		t.Fatalf("sender waited for timeout instead of signed rejection: %v", time.Since(started))
	}
	if err = <-receiverErr; protocol.ErrorCode(err) != protocol.AuthenticationFailed {
		t.Fatalf("receiver error = %v, want %s", err, protocol.AuthenticationFailed)
	}
}
