package transport

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"testing"
	"time"

	"github.com/Wen5555/LinkSend/internal/connectivity"
	"github.com/Wen5555/LinkSend/internal/identity"
	quic "github.com/quic-go/quic-go"
)

func fixturePair(t testing.TB, native bool, badPin bool) (*quic.Conn, *quic.Conn) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	t.Cleanup(cancel)
	a, err := identity.Generate()
	if err != nil {
		t.Fatal(err)
	}
	b, err := identity.Generate()
	if err != nil {
		t.Fatal(err)
	}
	clientTLS, err := a.TLSConfig(b.PublicKey(), false)
	if err != nil {
		t.Fatal(err)
	}
	serverTLS, err := b.TLSConfig(a.PublicKey(), true)
	if err != nil {
		t.Fatal(err)
	}
	if badPin {
		c, _ := identity.Generate()
		clientTLS, err = a.TLSConfig(c.PublicKey(), false)
		if err != nil {
			t.Fatal(err)
		}
	}
	type result struct {
		conn *quic.Conn
		err  error
	}
	results := make(chan result, 1)
	if native {
		ua, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
		if err != nil {
			t.Fatal(err)
		}
		ub, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
		if err != nil {
			t.Fatal(err)
		}
		ta, tb := &quic.Transport{Conn: ua}, &quic.Transport{Conn: ub}
		t.Cleanup(func() { _ = ta.Close(); _ = tb.Close(); _ = ua.Close(); _ = ub.Close() })
		l, err := tb.Listen(serverTLS, QUICConfig())
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = l.Close() })
		go func() { c, err := l.Accept(ctx); results <- result{c, err} }()
		ca, err := ta.Dial(ctx, ub.LocalAddr(), clientTLS, QUICConfig())
		if err != nil {
			t.Fatal(err)
		}
		r := <-results
		if r.err != nil {
			t.Fatal(r.err)
		}
		t.Cleanup(func() { _ = ca.CloseWithError(0, ""); _ = r.conn.CloseWithError(0, "") })
		return ca, r.conn
	}
	endpoints := make([]*connectivity.Endpoint, 2)
	for i := range endpoints {
		endpoints[i], err = connectivity.New(connectivity.Config{BindAddress: "127.0.0.1:0", AllowLoopback: true, Generation: 1})
		if err != nil {
			t.Fatal(err)
		}
		e := endpoints[i]
		t.Cleanup(func() { _ = e.Close() })
	}
	for _, e := range endpoints {
		if err = e.Gather(); err != nil {
			t.Fatal(err)
		}
	}
	for i, e := range endpoints {
		for c := range e.Candidates() {
			if err = endpoints[1-i].AddRemoteCandidate(c); err != nil {
				t.Fatal(err)
			}
		}
	}
	go func() {
		p, err := endpoints[1].Connect(ctx, endpoints[0].Credentials(), false)
		if err != nil {
			results <- result{err: err}
			return
		}
		s, err := Establish(ctx, endpoints[1], p, serverTLS, false)
		if err != nil {
			results <- result{err: err}
			return
		}
		results <- result{s.Conn, nil}
	}()
	p, err := endpoints[0].Connect(ctx, endpoints[1].Credentials(), true)
	if err != nil {
		t.Fatal(err)
	}
	s, err := Establish(ctx, endpoints[0], p, clientTLS, true)
	if badPin {
		if err == nil {
			_ = s.Close()
			t.Fatal("accepted wrong identity pin")
		}
		cancel()
		<-results
		return nil, nil
	}
	if err != nil {
		t.Fatal(err)
	}
	r := <-results
	if r.err != nil {
		t.Fatal(r.err)
	}
	t.Logf("base=%s remote=%s candidate_pair=%s -> %s method=%s relay=%v", p.BaseSocket, p.RemoteAddress, p.LocalType, p.RemoteType, p.ConnectionMethod, p.Relay)
	t.Cleanup(func() { _ = s.Close(); _ = r.conn.CloseWithError(0, "") })
	return s.Conn, r.conn
}

func transferBytes(ctx context.Context, client, server *quic.Conn, payload []byte) error {
	result := make(chan error, 1)
	go func() {
		s, err := server.AcceptStream(ctx)
		if err != nil {
			result <- err
			return
		}
		got, err := io.ReadAll(io.LimitReader(s, int64(len(payload))+1))
		if err != nil {
			result <- err
			return
		}
		if !bytes.Equal(got, payload) {
			result <- fmt.Errorf("payload mismatch: %d", len(got))
			return
		}
		digest := sha256.Sum256(got)
		if _, err = s.Write(digest[:]); err == nil {
			err = s.Close()
		}
		result <- err
	}()
	s, err := client.OpenStreamSync(ctx)
	if err != nil {
		return err
	}
	if _, err = s.Write(payload); err != nil {
		return err
	}
	if err = s.Close(); err != nil {
		return err
	}
	ack, err := io.ReadAll(io.LimitReader(s, 33))
	if err != nil {
		return err
	}
	digest := sha256.Sum256(payload)
	if !bytes.Equal(ack, digest[:]) {
		return fmt.Errorf("receiver acknowledgement mismatch")
	}
	return <-result
}

func TestICEQUICAuthenticatedBidirectional(t *testing.T) {
	a, b := fixturePair(t, false, false)
	if a.ConnectionState().TLS.Version != tls.VersionTLS13 {
		t.Fatal("TLS 1.3 required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	payload := bytes.Repeat([]byte("LinkSend authenticated bytes 0123456789\x00"), 32768)
	if err := transferBytes(ctx, a, b, payload); err != nil {
		t.Fatal(err)
	}
	if err := transferBytes(ctx, b, a, []byte("reverse message")); err != nil {
		t.Fatal(err)
	}
	t.Logf("verified encrypted stream payload=%d bytes, reverse=%d bytes; no signaling data path in fixture", len(payload), len("reverse message"))
}

func TestICEQUICRejectWrongPin(t *testing.T) { fixturePair(t, false, true) }

func TestICEQUICCloseReleasesStream(t *testing.T) {
	a, b := fixturePair(t, false, false)
	closed := make(chan error, 1)
	go func() { _, err := b.AcceptStream(context.Background()); closed <- err }()
	_ = a.CloseWithError(0, "test closed")
	select {
	case err := <-closed:
		if err == nil {
			t.Fatal("unexpected stream")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("close failed to release stream")
	}
}

func BenchmarkTransport(b *testing.B) {
	for _, native := range []bool{true, false} {
		name := "ice"
		if native {
			name = "native"
		}
		b.Run(name, func(b *testing.B) {
			client, server := fixturePair(b, native, false)
			payload := bytes.Repeat([]byte("0123456789abcdef"), 1<<16)
			b.SetBytes(int64(len(payload)))
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				err := transferBytes(ctx, client, server, payload)
				cancel()
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
