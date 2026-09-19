package transport

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"syscall"

	"github.com/Wen5555/LinkSend/internal/identity"
	quic "github.com/quic-go/quic-go"
)

// HandshakeFailureInfo is a bounded, local-only classification of a failed
// QUIC / TLS handshake. It deliberately excludes peer-provided error text,
// certificate material, addresses, and application payloads.
type HandshakeFailureInfo struct {
	Category string
	Code     string
	Origin   string
}

// HandshakeFailure classifies verified error types without parsing their
// strings. Unknown remains explicit so diagnostics never turn a guess into an
// authentication claim.
func HandshakeFailure(err error) HandshakeFailureInfo {
	if errors.Is(err, identity.ErrAuthentication) {
		return HandshakeFailureInfo{Category: "tls_identity_verification"}
	}
	var certificate *tls.CertificateVerificationError
	if errors.As(err, &certificate) {
		return HandshakeFailureInfo{Category: "tls_certificate_verification"}
	}
	var transportError *quic.TransportError
	if errors.As(err, &transportError) {
		origin := "local"
		if transportError.Remote {
			origin = "remote"
		}
		category := "quic_transport_error"
		if transportError.ErrorCode.IsCryptoError() {
			category = "quic_tls_alert"
		}
		return HandshakeFailureInfo{Category: category, Code: transportError.ErrorCode.String(), Origin: origin}
	}
	var applicationError *quic.ApplicationError
	if errors.As(err, &applicationError) {
		origin := "local"
		if applicationError.Remote {
			origin = "remote"
		}
		// Application error code 0 is meaningfully distinct from an explicit
		// non-zero peer rejection. Keep only its numeric code, never the
		// peer-supplied ErrorMessage.
		return HandshakeFailureInfo{Category: "quic_application_close", Code: numericHandshakeCode(uint64(applicationError.ErrorCode)), Origin: origin}
	}
	var version *quic.VersionNegotiationError
	if errors.As(err, &version) {
		return HandshakeFailureInfo{Category: "quic_version_negotiation"}
	}
	var handshakeTimeout *quic.HandshakeTimeoutError
	if errors.As(err, &handshakeTimeout) {
		return HandshakeFailureInfo{Category: "quic_handshake_timeout"}
	}
	var idleTimeout *quic.IdleTimeoutError
	if errors.As(err, &idleTimeout) {
		return HandshakeFailureInfo{Category: "quic_idle_timeout"}
	}
	var statelessReset *quic.StatelessResetError
	if errors.As(err, &statelessReset) {
		return HandshakeFailureInfo{Category: "quic_stateless_reset"}
	}
	var operation *net.OpError
	if errors.As(err, &operation) {
		var errno syscall.Errno
		if errors.As(operation, &errno) {
			return HandshakeFailureInfo{Category: "udp_socket", Code: numericHandshakeCode(uint64(errno))}
		}
		return HandshakeFailureInfo{Category: "udp_socket"}
	}
	return HandshakeFailureInfo{Category: "quic_unknown"}
}

func numericHandshakeCode(code uint64) string {
	return fmt.Sprintf("0x%x", code)
}
