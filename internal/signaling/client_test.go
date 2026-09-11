package signaling

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wen5555/LinkSend/internal/identity"
	"github.com/Wen5555/LinkSend/internal/protocol"
	"github.com/Wen5555/LinkSend/internal/server"
)

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
