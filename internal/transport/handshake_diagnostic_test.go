package transport

import (
	"context"
	"errors"
	"net"
	"syscall"
	"testing"

	"github.com/Wen5555/LinkSend/internal/connectivity"
	"github.com/Wen5555/LinkSend/internal/identity"
	"github.com/Wen5555/LinkSend/internal/protocol"
	quic "github.com/quic-go/quic-go"
)

func TestHandshakeFailureUsesTypedCategoriesWithoutErrorText(t *testing.T) {
	cases := []struct {
		name     string
		err      error
		category string
		code     string
		origin   string
		protocol protocol.Code
	}{
		{name: "identity", err: identity.ErrAuthentication, category: "tls_identity_verification", protocol: protocol.AuthenticationFailed},
		{name: "remote tls alert", err: &quic.TransportError{Remote: true, ErrorCode: quic.TransportErrorCode(0x12a), ErrorMessage: "untrusted remote text"}, category: "quic_tls_alert", code: "CRYPTO_ERROR 0x12a", origin: "remote", protocol: protocol.QUICHandshakeFailed},
		{name: "local transport", err: &quic.TransportError{ErrorCode: quic.ConnectionRefused, ErrorMessage: "local secret"}, category: "quic_transport_error", code: "CONNECTION_REFUSED", origin: "local", protocol: protocol.QUICHandshakeFailed},
		{name: "remote application close zero", err: &quic.ApplicationError{Remote: true, ErrorCode: 0, ErrorMessage: "peer text must not leak"}, category: "quic_application_close", code: "0x0", origin: "remote", protocol: protocol.QUICHandshakeFailed},
		{name: "remote application close nonzero", err: &quic.ApplicationError{Remote: true, ErrorCode: 0x42, ErrorMessage: "peer rejection text"}, category: "quic_application_close", code: "0x42", origin: "remote", protocol: protocol.QUICHandshakeFailed},
		{name: "udp socket errno", err: &net.OpError{Op: "read", Net: "udp", Err: syscall.Errno(10054)}, category: "udp_socket", code: "0x2746", protocol: protocol.QUICHandshakeFailed},
		{name: "version", err: &quic.VersionNegotiationError{}, category: "quic_version_negotiation", protocol: protocol.QUICHandshakeFailed},
		{name: "unknown", err: errors.New("untrusted error text"), category: "quic_unknown", protocol: protocol.QUICHandshakeFailed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := finishEstablish(context.Background(), nil, connectivity.Path{}, nil, nil, tc.err)
			if protocol.ErrorCode(err) != tc.protocol {
				t.Fatalf("protocol code=%s, want %s", protocol.ErrorCode(err), tc.protocol)
			}
			info := HandshakeFailure(err)
			if info.Category != tc.category || info.Code != tc.code || info.Origin != tc.origin {
				t.Fatalf("info=%+v", info)
			}
			if info.Code == tc.err.Error() || info.Category == tc.err.Error() {
				t.Fatalf("raw error text leaked into diagnostic: %+v", info)
			}
		})
	}
}
