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
	if err = control.db.QueryRow("SELECT version FROM schema_version").Scan(&version); err != nil || version != 4 {
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
	if !columns["used_by"] || !columns["used_at"] || !columns["target_id"] {
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

func TestMembershipV2OrdinaryMemberRevokeAndFreshRejoin(t *testing.T) {
	ctx := context.Background()
	s, err := OpenControl(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	aID, _ := identity.Generate()
	bID, _ := identity.Generate()
	a, err := s.Bootstrap(ctx, "a", aID.PublicKey())
	if err != nil {
		t.Fatal(err)
	}
	code, _, _ := s.Invitation(ctx, a)
	b, err := s.Join(ctx, code, "b", bID.PublicKey())
	if err != nil {
		t.Fatal(err)
	}
	freshSameGroup, _, _ := s.Invitation(ctx, a)
	bAgain, err := s.Join(ctx, freshSameGroup, "b-again", bID.PublicKey())
	if err != nil || bAgain.Admin || bAgain.Incarnation != b.Incarnation {
		t.Fatalf("fresh same-group code changed privilege/incarnation: %+v %v", bAgain, err)
	}
	invite, _, err := s.Invitation(ctx, b)
	if err != nil {
		t.Fatal("ordinary member could not invite:", err)
	}
	cID, _ := identity.Generate()
	c, err := s.Join(ctx, invite, "c", cID.PublicKey())
	if err != nil {
		t.Fatal(err)
	}
	revision, err := s.Revoke(ctx, b, RevokeRequest{RequestID: "remove-c", TargetID: c.ID, TargetIncarnation: c.Incarnation, ExpectedRevision: c.MembershipRevision})
	if err != nil || revision <= c.MembershipRevision {
		t.Fatalf("ordinary member revoke failed: revision=%d err=%v", revision, err)
	}
	if _, err = s.Revoke(ctx, b, RevokeRequest{RequestID: "remove-c", TargetID: c.ID, TargetIncarnation: c.Incarnation, ExpectedRevision: c.MembershipRevision}); err != nil {
		t.Fatalf("idempotent revoke failed: %v", err)
	}
	fresh, _, _ := s.Invitation(ctx, b)
	rejoined, err := s.Join(ctx, fresh, "c2", cID.PublicKey())
	if err != nil {
		t.Fatal("fresh code did not rejoin revoked identity:", err)
	}
	if rejoined.Incarnation == c.Incarnation || rejoined.MembershipRevision <= revision {
		t.Fatalf("rejoin did not create a new incarnation/revision: old=%+v new=%+v", c, rejoined)
	}
	if _, err = s.Join(ctx, code, "c-old", cID.PublicKey()); !errors.Is(err, ErrInvitationUsed) {
		t.Fatalf("old code replay result=%v", err)
	}
}

func TestMembershipV2RejectsStaleActorAndCrossGroupTarget(t *testing.T) {
	ctx := context.Background()
	s, err := OpenControl(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	aID, _ := identity.Generate()
	bID, _ := identity.Generate()
	cID, _ := identity.Generate()
	a, _ := s.Bootstrap(ctx, "a", aID.PublicKey())
	code, _, _ := s.Invitation(ctx, a)
	b, _ := s.Join(ctx, code, "b", bID.PublicKey())
	code, _, _ = s.Invitation(ctx, a)
	c, _ := s.Join(ctx, code, "c", cID.PublicKey())
	if _, err = s.Revoke(ctx, b, RevokeRequest{RequestID: "remove-a", TargetID: a.ID, TargetIncarnation: a.Incarnation, ExpectedRevision: c.MembershipRevision}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Revoke(ctx, a, RevokeRequest{RequestID: "stale-actor", TargetID: c.ID, TargetIncarnation: c.Incarnation, ExpectedRevision: c.MembershipRevision}); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("revoked actor committed late request: %v", err)
	}
	otherID, _ := identity.Generate()
	otherGroup := otherID.ID()
	incarnation, _ := randomIncarnation()
	if _, err = s.db.Exec("INSERT INTO groups(id,revision) VALUES(?,1)", otherGroup); err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.Exec("INSERT INTO devices(id,group_id,name,public_key,incarnation) VALUES(?,?,?,?,?)", otherID.ID(), otherGroup, "other", otherID.PublicKey(), incarnation); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Revoke(ctx, b, RevokeRequest{RequestID: "cross-group", TargetID: otherID.ID(), TargetIncarnation: incarnation, ExpectedRevision: 1}); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("cross-group revoke result=%v", err)
	}
}

func TestSignedInitializationAndExplicitCrossGroupSwitch(t *testing.T) {
	ctx := context.Background()
	s, err := OpenControl(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	aID, _ := identity.Generate()
	xID, _ := identity.Generate()
	a, err := s.Initialize(ctx, "a", aID.PublicKey())
	if err != nil {
		t.Fatal(err)
	}
	repeated, err := s.Initialize(ctx, "a-again", aID.PublicKey())
	if err != nil || repeated.GroupID != a.GroupID || repeated.Incarnation != a.Incarnation {
		t.Fatalf("initialization not idempotent: %+v %v", repeated, err)
	}
	x, err := s.Initialize(ctx, "x", xID.PublicKey())
	if err != nil || x.GroupID == a.GroupID {
		t.Fatalf("initialization selected existing group: %+v %v", x, err)
	}
	oldInvite, _, err := s.Invitation(ctx, a)
	if err != nil {
		t.Fatal(err)
	}
	targetInvite, _, err := s.Invitation(ctx, x)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Join(ctx, targetInvite, "a", aID.PublicKey()); !errors.Is(err, ErrInvitationConflict) {
		t.Fatalf("silent cross-group switch result=%v", err)
	}
	switched, err := s.JoinWithSwitch(ctx, targetInvite, "a", aID.PublicKey(), &SwitchExpectation{CurrentGroup: a.GroupID, CurrentIncarnation: a.Incarnation, CurrentRevision: a.MembershipRevision})
	if err != nil || switched.GroupID != x.GroupID || switched.Incarnation == a.Incarnation {
		t.Fatalf("explicit switch failed: %+v %v", switched, err)
	}
	newPeer, _ := identity.Generate()
	if _, err = s.Join(ctx, oldInvite, "late", newPeer.PublicKey()); !errors.Is(err, ErrInvitationInvalid) {
		t.Fatalf("old-group inviter credential survived switch: %v", err)
	}
}
