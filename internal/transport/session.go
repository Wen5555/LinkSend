// Package transport establishes authenticated QUIC sessions on nominated ICE paths.
package transport

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"example.com/linksend/internal/connectivity"
	quic "github.com/quic-go/quic-go"
)

func QUICConfig() *quic.Config {
	return &quic.Config{HandshakeIdleTimeout: 5 * time.Second, MaxIdleTimeout: 30 * time.Second, KeepAlivePeriod: 5 * time.Second, MaxIncomingStreams: 8, MaxIncomingUniStreams: -1, Allow0RTT: false, InitialStreamReceiveWindow: 2 << 20, MaxStreamReceiveWindow: 8 << 20, InitialConnectionReceiveWindow: 4 << 20, MaxConnectionReceiveWindow: 32 << 20}
}

type Session struct {
	Conn     *quic.Conn
	Path     connectivity.Path
	endpoint *connectivity.Endpoint
	once     sync.Once
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
		return nil, fmt.Errorf("E_AUTHENTICATION: %w", err)
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
