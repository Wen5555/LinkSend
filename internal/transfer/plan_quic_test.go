package transfer

import (
	"bytes"
	"context"
	"crypto/tls"
	"net"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wen5555/LinkSend/internal/identity"
	"github.com/Wen5555/LinkSend/internal/transport"
	quic "github.com/quic-go/quic-go"
)

// This fixture is real encrypted QUIC over two loopback UDP sockets. It is not
// a LAN or NAT proof. Both peers use the production identity TLS pin verifier.
func receivePlanQUICPair(t *testing.T) (*quic.Conn, *quic.Conn, string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 8*time.Second)
	t.Cleanup(cancel)
	sender, err := identity.Generate()
	if err != nil {
		t.Fatal(err)
	}
	receiver, err := identity.Generate()
	if err != nil {
		t.Fatal(err)
	}
	clientTLS, err := sender.TLSConfig(receiver.PublicKey(), false)
	if err != nil {
		t.Fatal(err)
	}
	serverTLS, err := receiver.TLSConfig(sender.PublicKey(), true)
	if err != nil {
		t.Fatal(err)
	}
	a, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	b, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		_ = a.Close()
		t.Fatal(err)
	}
	ta, tb := &quic.Transport{Conn: a}, &quic.Transport{Conn: b}
	t.Cleanup(func() { _ = ta.Close(); _ = tb.Close(); _ = a.Close(); _ = b.Close() })
	listener, err := tb.Listen(serverTLS, transport.QUICConfig())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	type accepted struct {
		conn *quic.Conn
		err  error
	}
	done := make(chan accepted, 1)
	go func() { conn, err := listener.Accept(ctx); done <- accepted{conn, err} }()
	client, err := ta.Dial(ctx, b.LocalAddr(), clientTLS, transport.QUICConfig())
	if err != nil {
		t.Fatal(err)
	}
	server := <-done
	if server.err != nil {
		t.Fatal(server.err)
	}
	t.Cleanup(func() { _ = client.CloseWithError(0, ""); _ = server.conn.CloseWithError(0, "") })
	if client.ConnectionState().TLS.Version != tls.VersionTLS13 || server.conn.ConnectionState().TLS.Version != tls.VersionTLS13 {
		t.Fatal("QUIC did not authenticate with TLS1.3")
	}
	t.Logf("actual loopback QUIC sockets: %s -> %s, TLS1.3, pinned peer %s", a.LocalAddr(), b.LocalAddr(), sender.ID())
	return client, server.conn, sender.ID()
}

func TestReceivePlanRealQUICPartialAndNoContent(t *testing.T) {
	for _, empty := range []bool{false, true} {
		t.Run(map[bool]string{false: "selected_file", true: "all_skipped"}[empty], func(t *testing.T) {
			client, server, peer := receivePlanQUICPair(t)
			p, contents := planFixture(t)
			dest := t.TempDir()
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			type outcome struct {
				result Result
				err    error
			}
			done := make(chan outcome, 1)
			go func() {
				stream, err := server.AcceptStream(ctx)
				if err != nil {
					done <- outcome{err: err}
					return
				}
				ids := []uint32{0}
				if empty {
					ids = []uint32{}
				}
				result, err := ReceiveWithOptions(ctx, transport.WrapStream(stream), ReceiveOptions{Directory: dest, Peer: peer, Plan: func(ctx context.Context, offer Offer) (ReceivePlan, error) {
					return BuildReceivePlan(ctx, dest, offer.Manifest, PlanRequest{SelectedIDs: ids})
				}})
				done <- outcome{result, err}
			}()
			stream, err := client.OpenStreamSync(ctx)
			if err != nil {
				t.Fatal(err)
			}
			result, err := Send(ctx, transport.WrapStream(stream), p, nil)
			received := <-done
			if err != nil || received.err != nil {
				t.Fatalf("sender=%v receiver=%v", err, received.err)
			}
			want := int64(len(contents["a.txt"]))
			if empty {
				want = 0
			}
			if result.Bytes != want || received.result.Bytes != want || result.Digest != p.Manifest.Digest() || result.SelectionDigest != received.result.SelectionDigest {
				t.Fatalf("actual QUIC subset mismatch: %+v %+v", result, received.result)
			}
			if empty {
				if result.State != "NoContent" || received.result.State != "NoContent" {
					t.Fatal("empty accepted subset claimed ordinary completion")
				}
			} else {
				data, err := os.ReadFile(filepath.Join(dest, "a.txt"))
				if err != nil || !bytes.Equal(data, contents["a.txt"]) {
					t.Fatal("QUIC selected payload differs")
				}
			}
			if _, err = os.Stat(filepath.Join(dest, "b.txt")); !os.IsNotExist(err) {
				t.Fatal("QUIC delivered an unselected file")
			}
		})
	}
}

type receivePlanConfirmationGate struct {
	*transport.QUICStream
	completed chan struct{}
	release   chan struct{}
	gate      atomic.Bool
	once      sync.Once
}

func (s *receivePlanConfirmationGate) Write(p []byte) (int, error) {
	n, err := s.QUICStream.Write(p)
	if err == nil && bytes.Contains(p[:n], []byte(`"op":"completed"`)) {
		s.gate.Store(true)
		s.once.Do(func() { close(s.completed) })
	}
	return n, err
}
func (s *receivePlanConfirmationGate) Read(p []byte) (int, error) {
	if s.gate.CompareAndSwap(true, false) {
		select {
		case <-s.release:
		case <-s.Context().Done():
			return 0, s.Context().Err()
		}
	}
	return s.QUICStream.Read(p)
}

func TestReceivePlanRealQUICRequiresTerminalSelectionConfirmation(t *testing.T) {
	client, server, peer := receivePlanQUICPair(t)
	p, _ := planFixture(t)
	dest := t.TempDir()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	completed, release := make(chan struct{}), make(chan struct{})
	received := make(chan error, 1)
	go func() {
		stream, err := server.AcceptStream(ctx)
		if err == nil {
			_, err = ReceiveWithOptions(ctx, &receivePlanConfirmationGate{QUICStream: transport.WrapStream(stream), completed: completed, release: release}, ReceiveOptions{Directory: dest, Peer: peer, Plan: func(ctx context.Context, o Offer) (ReceivePlan, error) {
				return BuildReceivePlan(ctx, dest, o.Manifest, PlanRequest{SelectedIDs: []uint32{0}})
			}})
		}
		received <- err
	}()
	stream, err := client.OpenStreamSync(ctx)
	if err != nil {
		t.Fatal(err)
	}
	sent := make(chan error, 1)
	go func() { _, err := Send(ctx, transport.WrapStream(stream), p, nil); sent <- err }()
	select {
	case <-completed:
	case <-ctx.Done():
		t.Fatal("receiver did not reach committed terminal frame")
	}
	select {
	case err := <-sent:
		t.Fatalf("sender returned before receiver consumed confirmation: %v", err)
	case <-time.After(30 * time.Millisecond):
	}
	close(release)
	if err := <-sent; err != nil {
		t.Fatal(err)
	}
	if err := <-received; err != nil {
		t.Fatal(err)
	}
}
