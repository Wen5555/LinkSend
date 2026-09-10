package store

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/Wen5555/LinkSend/internal/identity"
)

func TestOpenControlRejectsUnknownSchemaVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "control.db")
	control, err := OpenControl(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = control.db.Exec("INSERT INTO schema_version(version) VALUES(2)"); err != nil {
		t.Fatal(err)
	}
	if err = control.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = OpenControl(path); err == nil {
		t.Fatal("accepted unknown schema version")
	}
}

func TestJoinTestCodeAddsDeviceToLocalGroup(t *testing.T) {
	s, err := OpenControl(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	admin, err := identity.Generate()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Bootstrap(context.Background(), "admin", admin.PublicKey()); err != nil {
		t.Fatal(err)
	}
	peer, err := identity.Generate()
	if err != nil {
		t.Fatal(err)
	}
	joined, err := s.JoinTestCode(context.Background(), "peer", peer.PublicKey(), "")
	if err != nil {
		t.Fatal(err)
	}
	if joined.GroupID != admin.ID() || joined.ID != identity.DeviceID(peer.PublicKey()) {
		t.Fatalf("unexpected test join result: %+v", joined)
	}
	repeated, err := s.JoinTestCode(context.Background(), "peer-again", peer.PublicKey(), "")
	if err != nil {
		t.Fatalf("repeat test pairing should be idempotent: %v", err)
	}
	if repeated.ID != joined.ID || repeated.GroupID != joined.GroupID {
		t.Fatalf("repeat returned a different device: %+v", repeated)
	}
}
