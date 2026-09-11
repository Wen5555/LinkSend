package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wen5555/LinkSend/internal/protocol"
)

func TestProfileIdentityAndDiagnosticsAreLocal(t *testing.T) {
	svc, err := New(Config{DataDir: t.TempDir(), ServerURL: "https://example.invalid/?token=should-not-leak"})
	if err == nil {
		t.Fatal("expected invalid server URL with query")
	}
	svc, err = New(Config{DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	info := svc.Identity()
	if len(info.ID) != 64 || len(info.PublicKey) != 64 {
		t.Fatalf("invalid identity info: %+v", info)
	}
	d := svc.Diagnostics(context.Background())
	if d.Relay || d.ServerHealth != "not_configured" || d.Identity.ID != info.ID {
		t.Fatalf("unexpected diagnostics: %+v", d)
	}
	if !d.HistoryPersisted || !d.RestartRecoverySupported || !d.ByteResumeSupported {
		t.Fatalf("local persistence and resume capabilities were not reported independently: %+v", d)
	}
	b, _ := json.Marshal(d)
	if strings.Contains(string(b), svc.cfg.DataDir) {
		t.Fatal("diagnostics leaked data directory")
	}
	if !errors.Is(svc.Send(context.Background(), nil, ""), ErrNotImplemented) {
		t.Fatal("send must report explicit not implemented error")
	}
}

func TestMembershipFailureSeparatesUnavailableFromAuth(t *testing.T) {
	if got := membershipFailure(protocol.Wrap(protocol.SignalingUnreachable, "network", errors.New("dial failed"))); got.State != "unavailable" {
		t.Fatalf("unreachable classified as %q: %+v", got.State, got)
	}
	if got := membershipFailure(protocol.Fail(protocol.AuthenticationFailed, "denied")); got.State != "auth_failed" {
		t.Fatalf("auth failure classified as %q: %+v", got.State, got)
	}
}

func TestDiagnosticsRedactsHealthFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "secret=/Users/example/token", http.StatusInternalServerError)
	}))
	defer server.Close()
	svc, err := New(Config{DataDir: t.TempDir(), ServerURL: server.URL, AllowInsecureLoopback: true})
	if err != nil {
		t.Fatal(err)
	}
	d := svc.Diagnostics(context.Background())
	if d.ServerHealth != "error" || d.HealthFailure != "INVALID_MESSAGE" {
		t.Fatalf("unexpected redacted health: %+v", d)
	}
	b, _ := json.Marshal(d)
	if strings.Contains(string(b), "secret=") {
		t.Fatalf("diagnostics leaked remote detail: %s", b)
	}
}
