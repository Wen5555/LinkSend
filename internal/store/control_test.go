package store

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Wen5555/LinkSend/internal/identity"
)

func TestOpenControlRejectsUnknownSchemaVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "control.db")
	control, err := OpenControl(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = control.db.Exec("INSERT INTO schema_version(version) VALUES(999)"); err != nil {
		t.Fatal(err)
	}
	if err = control.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = OpenControl(path); err == nil {
		t.Fatal("accepted unknown schema version")
	}
}

func TestOpenControlMigratesInvitationConsumers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "control-v1.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`CREATE TABLE devices(id TEXT PRIMARY KEY,group_id TEXT NOT NULL,name TEXT NOT NULL,public_key BLOB NOT NULL,admin INTEGER NOT NULL DEFAULT 0,revoked INTEGER NOT NULL DEFAULT 0);
CREATE TABLE invitations(hash BLOB PRIMARY KEY,group_id TEXT NOT NULL,inviter TEXT NOT NULL,expires INTEGER NOT NULL,used INTEGER NOT NULL DEFAULT 0);
CREATE TABLE schema_version(version INTEGER PRIMARY KEY);
INSERT INTO schema_version(version) VALUES(1);`); err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	control, err := OpenControl(path)
	if err != nil {
		t.Fatal(err)
	}
	defer control.Close()
	var version int
	if err = control.db.QueryRow("SELECT version FROM schema_version").Scan(&version); err != nil || version != 2 {
		t.Fatalf("schema version=%d err=%v", version, err)
	}
	rows, err := control.db.Query("PRAGMA table_info(invitations)")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	columns := map[string]bool{}
	for rows.Next() {
		var cid int
		var name, kind string
		var notNull, primaryKey int
		var defaultValue any
		if err = rows.Scan(&cid, &name, &kind, &notNull, &defaultValue, &primaryKey); err != nil {
			t.Fatal(err)
		}
		columns[name] = true
	}
	if !columns["used_by"] || !columns["used_at"] {
		t.Fatalf("migration columns missing: %#v", columns)
	}
}

func TestHumanPairingCodeIsNormalizedAndSingleUse(t *testing.T) {
	s, err := OpenControl(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	admin, _ := identity.Generate()
	adminDevice, err := s.Bootstrap(context.Background(), "admin", admin.PublicKey())
	if err != nil {
		t.Fatal(err)
	}
	code, _, err := s.Invitation(context.Background(), adminDevice)
	if err != nil || len(code) != 9 || code[4] != '-' {
		t.Fatalf("unexpected pairing code: %q %v", code, err)
	}
	peer, _ := identity.Generate()
	typed := strings.ToLower(strings.ReplaceAll(code, "-", " "))
	if _, err = s.Join(context.Background(), typed, "peer", peer.PublicKey()); err != nil {
		t.Fatalf("human-entered pairing code was rejected: %v", err)
	}
	repeated, err := s.Join(context.Background(), code, "peer retry", peer.PublicKey())
	if err != nil || repeated.ID != peer.ID() {
		t.Fatalf("same identity retry should be idempotent: %+v %v", repeated, err)
	}
	second, _ := identity.Generate()
	if _, err = s.Join(context.Background(), code, "second", second.PublicKey()); !errors.Is(err, ErrInvitationUsed) {
		t.Fatalf("single-use pairing code error=%v", err)
	}
}

func TestPairingCodeErrorsRemainDistinct(t *testing.T) {
	s, err := OpenControl(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	admin, _ := identity.Generate()
	adminDevice, err := s.Bootstrap(context.Background(), "admin", admin.PublicKey())
	if err != nil {
		t.Fatal(err)
	}
	peer, _ := identity.Generate()
	if _, err = s.Join(context.Background(), "not-a-code", "peer", peer.PublicKey()); !errors.Is(err, ErrInvitationInvalid) {
		t.Fatalf("invalid format error=%v", err)
	}
	code, _, err := s.Invitation(context.Background(), adminDevice)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.Exec("UPDATE invitations SET expires=?", time.Now().Add(-time.Minute).Unix()); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Join(context.Background(), code, "peer", peer.PublicKey()); !errors.Is(err, ErrInvitationExpired) {
		t.Fatalf("expired code error=%v", err)
	}
}

func TestInvitationPrunesOnlyOldHistory(t *testing.T) {
	s, err := OpenControl(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	admin, _ := identity.Generate()
	member, err := s.Bootstrap(context.Background(), "admin", admin.PublicKey())
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = s.Invitation(context.Background(), member); err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.Exec("UPDATE invitations SET expires=?", time.Now().Add(-25*time.Hour).Unix()); err != nil {
		t.Fatal(err)
	}
	if _, _, err = s.Invitation(context.Background(), member); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = s.db.QueryRow("SELECT count(*) FROM invitations").Scan(&count); err != nil || count != 1 {
		t.Fatalf("invitation history count=%d err=%v", count, err)
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
