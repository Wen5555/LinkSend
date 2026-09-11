// Package transport establishes authenticated QUIC sessions on nominated ICE paths.
package transport

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"sync"
	"time"

	"github.com/Wen5555/LinkSend/internal/connectivity"
	"github.com/Wen5555/LinkSend/internal/identity"
	"github.com/Wen5555/LinkSend/internal/protocol"
	quic "github.com/quic-go/quic-go"
)

const streamAbortErrorCode quic.StreamErrorCode = 0x100

// QUICStream keeps quic-go stream-specific abort operations inside transport.
// Normal Close still only completes the send direction; Abort is reserved for
// local cancellation or protocol failure and cancels both directions.
type QUICStream struct{ *quic.Stream }

func WrapStream(stream *quic.Stream) *QUICStream { return &QUICStream{Stream: stream} }

func (s *QUICStream) Abort() {
	if s == nil || s.Stream == nil {
		return
	}
	s.CancelRead(streamAbortErrorCode)
	s.CancelWrite(streamAbortErrorCode)
}

// FlushTerminal half-closes the terminal response and waits for peer teardown.
// Close alone only queues FIN; immediately resetting the stream or connection
// can discard the buffered rejection/error. The peer normally aborts after
// reading it. Bound uncooperative peers without extending the transfer deadline.
func (s *QUICStream) FlushTerminal(ctx context.Context) error {
	if err := s.Close(); err != nil {
		return err
	}
	deadline := time.Now().Add(2 * time.Second)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	if err := s.SetReadDeadline(deadline); err != nil {
		return err
	}
	var buf [256]byte
	for read := 0; read < 4096; {
		n, err := s.Read(buf[:])
		read += n
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrNoProgress
		}
	}
	return errors.New("terminal peer response exceeded limit")
}

func QUICConfig() *quic.Config {
	return &quic.Config{HandshakeIdleTimeout: 5 * time.Second, MaxIdleTimeout: 30 * time.Second, KeepAlivePeriod: 5 * time.Second, MaxIncomingStreams: 8, MaxIncomingUniStreams: -1, Allow0RTT: false, InitialStreamReceiveWindow: 2 << 20, MaxStreamReceiveWindow: 8 << 20, InitialConnectionReceiveWindow: 4 << 20, MaxConnectionReceiveWindow: 32 << 20}
}

type Session struct {
	Conn     *quic.Conn
	Path     connectivity.Path
	endpoint *connectivity.Endpoint
	once     sync.Once
}

func (s *Session) Evidence() (connectivity.Stats, uint16, string) {
	if s == nil || s.Conn == nil || s.endpoint == nil {
		return connectivity.Stats{}, 0, ""
	}
	tlsState := s.Conn.ConnectionState().TLS
	return s.endpoint.Stats(), tlsState.Version, tlsState.NegotiatedProtocol
}

// Establish uses the ICE controlling role as QUIC client. Both TLS configs must
// be identity-pinned. It never calls DialEarly / ListenEarly or sends 0-RTT data.
func Establish(ctx context.Context, e *connectivity.Endpoint, path connectivity.Path, tlsConfig *tls.Config, controlling bool) (*Session, error) {
	if tlsConfig == nil || tlsConfig.MinVersion < tls.VersionTLS13 || tlsConfig.VerifyConnection == nil || len(tlsConfig.Certificates) == 0 {
		return nil, errors.New("E_AUTHENTICATION: pinned TLS 1.3 identity config required")
	}
	if !controlling && tlsConfig.ClientAuth != tls.RequireAnyClientCert && tlsConfig.ClientAuth != tls.RequireAndVerifyClientCert {
		return nil, errors.New("E_AUTHENTICATION: client certificate required")
	}
	addr, err := net.ResolveUDPAddr("udp", path.RemoteAddress)
	if err != nil {
		return nil, err
	}
	var conn *quic.Conn
	if controlling {
		conn, err = e.QUIC().Dial(ctx, addr, tlsConfig, QUICConfig())
	} else {
		var listener *quic.Listener
		listener, err = e.QUIC().Listen(tlsConfig, QUICConfig())
		if err == nil {
			conn, err = listener.Accept(ctx)
			_ = listener.Close()
		}
	}
	if err != nil {
		if errors.Is(ctx.Err(), context.Canceled) {
			return nil, protocol.Fail(protocol.Cancelled, "QUIC handshake cancelled")
		}
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, protocol.Fail(protocol.QUICHandshakeTimeout, "QUIC handshake timed out")
		}
		if errors.Is(err, identity.ErrAuthentication) {
			return nil, protocol.Wrap(protocol.AuthenticationFailed, "TLS peer identity verification failed", err)
		}
		return nil, protocol.Wrap(protocol.QUICHandshakeFailed, "QUIC handshake failed", err)
	}
	if conn.RemoteAddr().String() != addr.String() {
		_ = conn.CloseWithError(1, "nominated path mismatch")
		return nil, errors.New("E_DIRECT_FAILED: QUIC remote differs from nominated path")
	}
	s := &Session{Conn: conn, Path: path, endpoint: e}
	go func() {
		select {
		case <-e.PathChanged():
			_ = s.Close()
		case <-e.Done():
			_ = s.Close()
		case <-conn.Context().Done():
		}
	}()
	return s, nil
}

func (s *Session) Close() error {
	var err error
	s.once.Do(func() { err = s.Conn.CloseWithError(0, "closed"); _ = s.endpoint.Close() })
	return err
}
