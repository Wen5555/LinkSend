package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/Wen5555/LinkSend/internal/identity"
	"github.com/Wen5555/LinkSend/internal/protocol"
	"github.com/Wen5555/LinkSend/internal/signaling"
)

func TestHealthReportsProductAndProtocolVersionsSeparately(t *testing.T) {
	s, err := New(Config{Listen: "127.0.0.1:8787", Database: filepath.Join(t.TempDir(), "control.db"), AllowInsecureLoopback: true})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	h := httptest.NewServer(s.Handler())
	defer h.Close()
	response, err := http.Get(h.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var body struct {
		Version      string                `json:"version"`
		Capabilities protocol.Capabilities `json:"capabilities"`
	}
	if err = json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Version != protocol.ProductVersion || body.Capabilities.ProductVersion != protocol.ProductVersion || body.Capabilities.ProtocolVersion != protocol.Version {
		t.Fatalf("health version drift: %+v", body)
	}
}

func TestLoopbackTestPairingCode(t *testing.T) {
	s, err := New(Config{
		Listen:                  "127.0.0.1:8787",
		Database:                filepath.Join(t.TempDir(), "control.db"),
		AllowInsecureLoopback:   true,
		AllowLoopbackCandidates: true,
		TestPairingCode:         "orion123",
		BootstrapToken:          "test-bootstrap-token-that-is-long-enough-123",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	h := httptest.NewServer(s.Handler())
	defer h.Close()
	ctx := context.Background()
	admin, err := identity.Generate()
	if err != nil {
		t.Fatal(err)
	}
	adminClient, err := signaling.New(signaling.Config{ServerURL: h.URL, Identity: admin, AllowInsecureLoopback: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = adminClient.Bootstrap(ctx, "test-bootstrap-token-that-is-long-enough-123", "admin"); err != nil {
		t.Fatal(err)
	}
	peer, err := identity.Generate()
	if err != nil {
		t.Fatal(err)
	}
	peerClient, err := signaling.New(signaling.Config{ServerURL: h.URL, Identity: peer, AllowInsecureLoopback: true})
	if err != nil {
		t.Fatal(err)
	}
	joined, err := peerClient.Join(ctx, "orion123", "peer")
	if err != nil {
		t.Fatalf("fixed loopback pairing code rejected: %v", err)
	}
	if !joined.Admin {
		t.Fatal("fixed pairing member was not granted admin")
	}
}

func TestPublicTestPairingCodeRequiresExplicitGroup(t *testing.T) {
	err := (Config{Listen: "0.0.0.0:8787", Database: "control.db", TLSCert: "server.crt", TLSKey: "server.key", TestPairingCode: "orion123"}).Validate()
	if err == nil {
		t.Fatal("accepted a public pairing code without explicit group")
	}
	if err := (Config{Listen: "0.0.0.0:8787", Database: "control.db", TLSCert: "server.crt", TLSKey: "server.key", TestPairingCode: "orion123", TestPairingGroup: "group-id"}).Validate(); err != nil {
		t.Fatalf("explicit public pairing group rejected: %v", err)
	}
}
