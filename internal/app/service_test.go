package app

import (
	"context"
	"errors"
	"testing"
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
	if !errors.Is(svc.Send(context.Background(), nil, ""), ErrNotImplemented) {
		t.Fatal("send must report explicit not implemented error")
	}
}
