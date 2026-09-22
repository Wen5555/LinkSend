package signaling

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wen5555/LinkSend/internal/identity"
	"github.com/Wen5555/LinkSend/internal/protocol"
	"github.com/Wen5555/LinkSend/internal/server"
)

func TestJoinRejectsLegacyMembershipServerBeforeRegistration(t *testing.T) {
	var registrations atomic.Int32
	h := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "ok",
				"capabilities": protocol.Capabilities{
					ProtocolVersion: protocol.Version,
					ProductVersion:  "0.5.0",
					Transport:       "quic",
				},
			})
			return
		}
		registrations.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(protocol.Error{Code: protocol.AuthenticationFailed, Detail: "registration proof invalid"})
	}))
	defer h.Close()
	id, err := identity.Generate()
	if err != nil {
		t.Fatal(err)
	}
	_, err = testClient(t, h.URL, id).Join(t.Context(), "ABCD-EFGH", "desktop")
	if protocol.ErrorCode(err) != protocol.VersionIncompatible || !strings.Contains(err.Error(), "server capabilities incompatible") {
		t.Fatalf("legacy server result=%v code=%s", err, protocol.ErrorCode(err))
	}
	if registrations.Load() != 0 {
		t.Fatalf("legacy registration endpoint was called %d times", registrations.Load())
	}
}

func testServer(t *testing.T) (*server.Server, *httptest.Server) {
	t.Helper()
	s, err := server.New(server.Config{Listen: "127.0.0.1:0", Database: ":memory:", AllowInsecureLoopback: true, AllowLoopbackCandidates: true, BootstrapToken: "01234567890123456789012345678901"})
	if err != nil {
		t.Fatal(err)
	}
	h := httptest.NewServer(s.Handler())
	t.Cleanup(func() { h.Close(); _ = s.Close() })
	return s, h
}

func testClient(t *testing.T, raw string, id *identity.Identity) *Client {
	t.Helper()
	c, err := New(Config{ServerURL: raw, Identity: id, AllowInsecureLoopback: true})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestClientGatewayFailuresPreserveStatusAndProtocolErrors(t *testing.T) {
	const privateDetail = "gateway-private-body-marker"
	for _, tc := range []struct {
		name   string
		status int
		body   string
		code   protocol.Code
		detail string
	}{
		{name: "proxy_525_html", status: 525, body: "<html>" + privateDetail + "</html>", code: protocol.SignalingUnreachable},
		{name: "gateway_504_empty", status: http.StatusGatewayTimeout, code: protocol.SignalingTimeout},
		{name: "service_503_unrecognized_json", status: http.StatusServiceUnavailable, body: `{"message":"` + privateDetail + `"}`, code: protocol.SignalingUnreachable},
		{name: "server_500_oversized_json", status: http.StatusInternalServerError, body: `{"code":"AUTHENTICATION_FAILED","detail":"` + privateDetail + strings.Repeat("x", protocol.MaxMessageBytes) + `"}`, code: protocol.SignalingUnreachable},
		{name: "forbidden_html_is_not_retryable", status: http.StatusForbidden, body: "<html>" + privateDetail + "</html>", code: protocol.InvalidMessage},
		{name: "server_500_preserves_authentication", status: http.StatusInternalServerError, body: `{"code":"AUTHENTICATION_FAILED","detail":"pair again"}`, code: protocol.AuthenticationFailed, detail: "pair again"},
		{name: "gateway_504_preserves_version", status: http.StatusGatewayTimeout, body: `{"code":"VERSION_INCOMPATIBLE","detail":"upgrade the server"}`, code: protocol.VersionIncompatible, detail: "upgrade the server"},
		{name: "rate_limit_preserves_protocol", status: http.StatusTooManyRequests, body: `{"code":"RATE_LIMITED","detail":"request limit"}`, code: protocol.RateLimited, detail: "request limit"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, surface := range []string{"signed_http", "wss_handshake"} {
				t.Run(surface, func(t *testing.T) {
					h := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						w.WriteHeader(tc.status)
						_, _ = io.WriteString(w, tc.body)
					}))
					defer h.Close()
					id, err := identity.Generate()
					if err != nil {
						t.Fatal(err)
					}
					client, err := New(Config{ServerURL: h.URL, Identity: id, HTTPClient: h.Client()})
					if err != nil {
						t.Fatal(err)
					}
					ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
					defer cancel()
					if surface == "signed_http" {
						_, err = client.Devices(ctx)
					} else {
						var session *Session
						session, err = client.Connect(ctx)
						if session != nil {
							_ = session.Close()
						}
					}
					var remote *RemoteError
					if !errors.As(err, &remote) || remote.Status != tc.status {
						t.Fatalf("HTTP status was lost: %v", err)
					}
					if code := protocol.ErrorCode(err); code != tc.code {
						t.Errorf("HTTP %d classified as %s, want %s", remote.Status, code, tc.code)
					}
					if tc.detail != "" && remote.Cause.Detail != tc.detail {
						t.Errorf("valid protocol detail was changed: %q", remote.Cause.Detail)
					}
					if strings.Contains(err.Error(), privateDetail) || strings.Contains(err.Error(), "<html>") {
						t.Fatal("unrecognized gateway response body leaked into the error")
					}
				})
			}
		})
	}
}

func TestClientRegistrationAndSignedHTTP(t *testing.T) {
	_, h := testServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	a, err := identity.Generate()
	if err != nil {
		t.Fatal(err)
	}
	c := testClient(t, h.URL, a)
	if _, err = c.Health(ctx); err != nil {
		t.Fatal(err)
	}
	admin, err := c.Bootstrap(ctx, "01234567890123456789012345678901", "admin")
	if err != nil {
		t.Fatal(err)
	}
	if !admin.Admin || admin.ID != a.ID() {
		t.Fatalf("unexpected bootstrap device: %+v", admin)
	}
	invit, err := c.CreateInvitation(ctx)
	if err != nil || len(invit.Token) != 9 || invit.Token[4] != '-' {
		t.Fatalf("invitation: %v %+v", err, invit)
	}
	if invit.Remaining <= 0 || invit.Remaining > 10*time.Minute {
		t.Fatalf("unexpected server-relative lifetime: %v", invit.Remaining)
	}
	b, err := identity.Generate()
	if err != nil {
		t.Fatal(err)
	}
	joined, err := testClient(t, h.URL, b).Join(ctx, invit.Token, "peer")
	if err != nil {
		t.Fatal(err)
	}
	if joined.ID != b.ID() || joined.GroupID != admin.GroupID {
		t.Fatalf("unexpected joined device: %+v", joined)
	}
	// Any paired device may mint a short-lived one-time pairing code; the
	// desktop product does not impose an administrator-only gate.
	peerClient := testClient(t, h.URL, b)
	secondInvite, err := peerClient.CreateInvitation(ctx)
	if err != nil || len(secondInvite.Token) != 9 || secondInvite.Token[4] != '-' {
		t.Fatalf("member invitation: %v %+v", err, secondInvite)
	}
	devices, err := c.Devices(ctx)
	if err != nil || len(devices) != 2 {
		t.Fatalf("devices: %v %#v", err, devices)
	}
}

func TestJoinRetryAndPairingErrorsAreStable(t *testing.T) {
	_, h := testServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	admin, _ := identity.Generate()
	ca := testClient(t, h.URL, admin)
	if _, err := ca.Bootstrap(ctx, "01234567890123456789012345678901", "admin"); err != nil {
		t.Fatal(err)
	}
	invitation, err := ca.CreateInvitation(ctx)
	if err != nil {
		t.Fatal(err)
	}
	peer, _ := identity.Generate()
	cp := testClient(t, h.URL, peer)
	first, err := cp.Join(ctx, invitation.Token, "peer")
	if err != nil {
		t.Fatal(err)
	}
	retry, err := cp.Join(ctx, invitation.Token, "peer retry")
	if err != nil || retry.ID != first.ID {
		t.Fatalf("idempotent retry: %+v %v", retry, err)
	}
	other, _ := identity.Generate()
	_, err = testClient(t, h.URL, other).Join(ctx, invitation.Token, "other")
	if protocol.ErrorCode(err) != protocol.PairingCodeUsed {
		t.Fatalf("used code error=%v code=%s", err, protocol.ErrorCode(err))
	}
	_, err = testClient(t, h.URL, other).Join(ctx, "NOPE-NOPE", "other")
	if protocol.ErrorCode(err) != protocol.PairingCodeInvalid {
		t.Fatalf("invalid code error=%v code=%s", err, protocol.ErrorCode(err))
	}
}

func TestClientWSSChallengeAndEnvelope(t *testing.T) {
	_, h := testServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	a, _ := identity.Generate()
	b, _ := identity.Generate()
	ca := testClient(t, h.URL, a)
	if _, err := ca.Bootstrap(ctx, "01234567890123456789012345678901", "admin"); err != nil {
		t.Fatal(err)
	}
	invit, err := ca.CreateInvitation(ctx)
	if err != nil {
		t.Fatal(err)
	}
	cb := testClient(t, h.URL, b)
	if _, err = cb.Join(ctx, invit.Token, "peer"); err != nil {
		t.Fatal(err)
	}
	sa, err := ca.Connect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer sa.Close()
	sb, err := cb.Connect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer sb.Close()
	// The server does not trust or validate peer keys at the WSS handshake;
	// envelope verification is explicitly performed by the receiver.
	sessionID := protocol.RandomID()
	env, err := protocol.NewEnvelope("connect_request", a.ID(), b.ID(), sessionID, 1, map[string]string{"role": "controlling"})
	if err != nil {
		t.Fatal(err)
	}
	if err = sa.SendEnvelope(ctx, env); err != nil {
		t.Fatal(err)
	}
	wire, err := sb.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if wire.Message == nil || wire.Message.Sender != a.ID() {
		t.Fatalf("unexpected wire: %+v", wire)
	}
	if err = wire.Message.Verify(a.PublicKey(), time.Now()); err != nil {
		t.Fatal(err)
	}
	status, err := protocol.NewEnvelope("status", a.ID(), b.ID(), sessionID, 1, map[string]string{"state": "checking"})
	if err != nil {
		t.Fatal(err)
	}
	if err = sa.SendEnvelope(ctx, status); err != nil {
		t.Fatal(err)
	}
	wire, err = sb.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if wire.Message == nil || wire.Message.Type != "status" {
		t.Fatalf("unexpected status wire: %+v", wire)
	}
}

func TestRejectInsecureNonLoopback(t *testing.T) {
	id, _ := identity.Generate()
	if _, err := New(Config{ServerURL: "http://192.0.2.1:8080", Identity: id, AllowInsecureLoopback: true}); err == nil {
		t.Fatal("accepted insecure non-loopback signaling URL")
	}
}

func TestLANConsentCredentialIsBoundToTargetIdentity(t *testing.T) {
	_, h := testServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	targetID, _ := identity.Generate()
	memberID, _ := identity.Generate()
	target := testClient(t, h.URL, targetID)
	member := testClient(t, h.URL, memberID)
	joinedMember, err := member.Bootstrap(ctx, "01234567890123456789012345678901", "member")
	if err != nil {
		t.Fatal(err)
	}
	requestID, nonce, expires := protocol.RandomID(), protocol.RandomID(), time.Now().Add(time.Minute).Unix()
	request := LANPairFrame{Version: 2, Phase: "request", RequestID: requestID, Nonce: nonce, Sender: targetID.ID(), Recipient: memberID.ID(), Generation: 1, IssuedAt: time.Now().Unix(), ExpiresAt: expires}
	request.Signature = targetID.Sign(request.SigningBytes())
	approval := LANPairFrame{Version: 2, Phase: "accept", RequestID: requestID, Nonce: nonce, Sender: memberID.ID(), Recipient: targetID.ID(), Generation: 1, IssuedAt: time.Now().Unix(), ExpiresAt: expires}
	approval.Signature = memberID.Sign(approval.SigningBytes())
	credential, err := member.CreateLANCredential(ctx, targetID.PublicKey(), request, approval)
	if err != nil {
		t.Fatal(err)
	}
	joinedTarget, err := target.Join(ctx, credential.Token, "target")
	if err != nil || joinedTarget.GroupID != joinedMember.GroupID {
		t.Fatalf("bound credential join failed: %+v %v", joinedTarget, err)
	}
	otherID, _ := identity.Generate()
	if _, err = testClient(t, h.URL, otherID).Join(ctx, credential.Token, "other"); protocol.ErrorCode(err) != protocol.PairingCodeUsed && protocol.ErrorCode(err) != protocol.PairingIdentityConflict {
		t.Fatalf("credential was usable by another identity: %v", err)
	}
}

func TestClientExplicitMembershipSwitchBindsCurrentIncarnation(t *testing.T) {
	_, h := testServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	aID, _ := identity.Generate()
	xID, _ := identity.Generate()
	a := testClient(t, h.URL, aID)
	x := testClient(t, h.URL, xID)
	current, err := a.Initialize(ctx, "a")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = x.Initialize(ctx, "x"); err != nil {
		t.Fatal(err)
	}
	invitation, err := x.CreateInvitation(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = a.Join(ctx, invitation.Token, "a"); protocol.ErrorCode(err) != protocol.PairingIdentityConflict {
		t.Fatalf("implicit cross-group switch result=%v", err)
	}
	switched, err := a.SwitchGroup(ctx, invitation.Token, "a", current)
	if err != nil {
		t.Fatal(err)
	}
	if switched.GroupID == current.GroupID || switched.Incarnation == current.Incarnation {
		t.Fatalf("switch did not replace relationship: old=%+v new=%+v", current, switched)
	}
}
