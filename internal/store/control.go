// Package store persists control-plane membership. It has no file-transfer API.
package store

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base32"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Wen5555/LinkSend/internal/identity"
	_ "modernc.org/sqlite"
)

var ErrUnauthorized = errors.New("unauthorized or revoked device")

// Pairing errors remain deliberately coarse enough not to disclose membership
// details, while still letting a legitimate user recover from an expired or
// already-consumed code. All specific errors wrap ErrInvitation for callers
// that only need the legacy classification.
var ErrInvitation = errors.New("pairing invitation rejected")
var ErrInvitationInvalid = fmt.Errorf("invalid pairing invitation: %w", ErrInvitation)
var ErrInvitationExpired = fmt.Errorf("expired pairing invitation: %w", ErrInvitation)
var ErrInvitationUsed = fmt.Errorf("used pairing invitation: %w", ErrInvitation)
var ErrInvitationConflict = fmt.Errorf("pairing identity conflict: %w", ErrInvitation)

type Device struct {
	ID                 string `json:"id"`
	GroupID            string `json:"group_id"`
	Name               string `json:"name"`
	PublicKey          []byte `json:"public_key"`
	Admin              bool   `json:"admin"`
	Revoked            bool   `json:"revoked"`
	Online             bool   `json:"online"`
	Incarnation        string `json:"incarnation"`
	MembershipRevision uint64 `json:"membership_revision"`
}

type RevokeRequest struct {
	RequestID         string `json:"request_id"`
	TargetID          string `json:"target_id"`
	TargetIncarnation string `json:"target_incarnation"`
	ExpectedRevision  uint64 `json:"expected_revision"`
}

type SwitchExpectation struct {
	CurrentGroup       string `json:"current_group"`
	CurrentIncarnation string `json:"current_incarnation"`
	CurrentRevision    uint64 `json:"current_revision"`
}
type Control struct{ db *sql.DB }

type rowScanner interface{ Scan(...any) error }

func scanDevice(row rowScanner, d *Device) error {
	return row.Scan(&d.ID, &d.GroupID, &d.Name, &d.PublicKey, &d.Admin, &d.Revoked, &d.Incarnation, &d.MembershipRevision)
}

func randomIncarnation() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", raw[:]), nil
}

func OpenControl(path string) (*Control, error) {
	if path != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return nil, err
		}
		if info, err := os.Lstat(path); err == nil && !info.Mode().IsRegular() {
			return nil, errors.New("database path must be a regular file")
		}
		f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
		if err != nil {
			return nil, err
		}
		if err = f.Close(); err != nil {
			return nil, err
		}
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	_, err = db.Exec(`PRAGMA busy_timeout=5000; PRAGMA foreign_keys=ON; PRAGMA journal_mode=WAL;
CREATE TABLE IF NOT EXISTS groups(id TEXT PRIMARY KEY,revision INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS devices(id TEXT PRIMARY KEY,group_id TEXT NOT NULL,name TEXT NOT NULL,public_key BLOB NOT NULL,admin INTEGER NOT NULL DEFAULT 0,revoked INTEGER NOT NULL DEFAULT 0,incarnation TEXT NOT NULL DEFAULT '');
CREATE TABLE IF NOT EXISTS invitations(hash BLOB PRIMARY KEY,group_id TEXT NOT NULL,inviter TEXT NOT NULL,expires INTEGER NOT NULL,used INTEGER NOT NULL DEFAULT 0,used_by TEXT NOT NULL DEFAULT '',used_at INTEGER NOT NULL DEFAULT 0,inviter_incarnation TEXT NOT NULL DEFAULT '',created_revision INTEGER NOT NULL DEFAULT 0,used_incarnation TEXT NOT NULL DEFAULT '',target_id TEXT NOT NULL DEFAULT '');
CREATE TABLE IF NOT EXISTS membership_requests(request_id TEXT PRIMARY KEY,actor_id TEXT NOT NULL,target_id TEXT NOT NULL,target_incarnation TEXT NOT NULL,result_revision INTEGER NOT NULL,created_at INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS schema_version(version INTEGER PRIMARY KEY);`)
	if err != nil {
		db.Close()
		return nil, err
	}
	var versions []int
	rows, err := db.Query("SELECT version FROM schema_version")
	if err != nil {
		db.Close()
		return nil, err
	}
	for rows.Next() {
		var version int
		if err = rows.Scan(&version); err != nil {
			rows.Close()
			db.Close()
			return nil, err
		}
		versions = append(versions, version)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		db.Close()
		return nil, err
	}
	if err = rows.Close(); err != nil {
		db.Close()
		return nil, err
	}
	if len(versions) == 0 {
		if _, err = db.Exec("INSERT INTO schema_version(version) VALUES(4)"); err != nil {
			db.Close()
			return nil, err
		}
	} else if len(versions) == 1 && versions[0] == 1 {
		if err = migrateControlV1ToV2(db); err != nil {
			db.Close()
			return nil, err
		}
		if err = migrateControlV2ToV3(db); err != nil {
			db.Close()
			return nil, err
		}
		if err = migrateControlV3ToV4(db); err != nil {
			db.Close()
			return nil, err
		}
	} else if len(versions) == 1 && versions[0] == 2 {
		if err = migrateControlV2ToV3(db); err != nil {
			db.Close()
			return nil, err
		}
		if err = migrateControlV3ToV4(db); err != nil {
			db.Close()
			return nil, err
		}
	} else if len(versions) == 1 && versions[0] == 3 {
		if err = migrateControlV3ToV4(db); err != nil {
			db.Close()
			return nil, err
		}
	} else if len(versions) != 1 || versions[0] != 4 {
		db.Close()
		return nil, errors.New("unsupported control database schema version")
	}
	return &Control{db: db}, nil
}

func migrateControlV3ToV4(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec("ALTER TABLE invitations ADD COLUMN target_id TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if _, err = tx.Exec("UPDATE schema_version SET version=4 WHERE version=3"); err != nil {
		return err
	}
	return tx.Commit()
}

func migrateControlV2ToV3(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	statements := []string{
		"CREATE TABLE IF NOT EXISTS groups(id TEXT PRIMARY KEY,revision INTEGER NOT NULL)",
		"INSERT INTO groups(id,revision) SELECT DISTINCT group_id,1 FROM devices",
		"ALTER TABLE devices ADD COLUMN incarnation TEXT NOT NULL DEFAULT ''",
		"UPDATE devices SET incarnation=lower(hex(randomblob(16)))",
		"ALTER TABLE invitations ADD COLUMN inviter_incarnation TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE invitations ADD COLUMN created_revision INTEGER NOT NULL DEFAULT 0",
		"ALTER TABLE invitations ADD COLUMN used_incarnation TEXT NOT NULL DEFAULT ''",
		"UPDATE invitations SET used=1 WHERE used=0",
		"CREATE TABLE IF NOT EXISTS membership_requests(request_id TEXT PRIMARY KEY,actor_id TEXT NOT NULL,target_id TEXT NOT NULL,target_incarnation TEXT NOT NULL,result_revision INTEGER NOT NULL,created_at INTEGER NOT NULL)",
		"UPDATE schema_version SET version=3 WHERE version=2",
	}
	for _, statement := range statements {
		if _, err = tx.Exec(statement); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func migrateControlV1ToV2(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec("ALTER TABLE invitations ADD COLUMN used_by TEXT NOT NULL DEFAULT ''"); err != nil {
		return fmt.Errorf("add invitation consumer: %w", err)
	}
	if _, err = tx.Exec("ALTER TABLE invitations ADD COLUMN used_at INTEGER NOT NULL DEFAULT 0"); err != nil {
		return fmt.Errorf("add invitation consumption time: %w", err)
	}
	if _, err = tx.Exec("UPDATE schema_version SET version=2 WHERE version=1"); err != nil {
		return fmt.Errorf("update control schema version: %w", err)
	}
	return tx.Commit()
}
func (s *Control) Close() error { return s.db.Close() }

func (s *Control) Bootstrap(ctx context.Context, name string, key []byte) (Device, error) {
	if len(key) != 32 || len(name) == 0 || len(name) > 128 {
		return Device{}, errors.New("invalid device identity")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Device{}, err
	}
	defer tx.Rollback()
	var count int
	if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM devices").Scan(&count); err != nil {
		return Device{}, err
	}
	if count != 0 {
		return Device{}, errors.New("bootstrap already completed")
	}
	id := identity.DeviceID(key)
	incarnation, err := randomIncarnation()
	if err != nil {
		return Device{}, err
	}
	d := Device{ID: id, GroupID: id, Name: name, PublicKey: key, Admin: true, Incarnation: incarnation, MembershipRevision: 1}
	if _, err = tx.ExecContext(ctx, "INSERT INTO groups(id,revision) VALUES(?,1)", d.GroupID); err != nil {
		return Device{}, err
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO devices(id,group_id,name,public_key,admin,incarnation) VALUES(?,?,?,?,1,?)", d.ID, d.GroupID, d.Name, d.PublicKey, d.Incarnation); err != nil {
		return Device{}, err
	}
	return d, tx.Commit()
}

// Initialize creates a new isolated group for a signed identity that has no
// active membership. Repeating the operation for the same active identity is
// idempotent and never selects another existing group.
func (s *Control) Initialize(ctx context.Context, name string, key []byte) (Device, error) {
	if len(key) != 32 || len(name) == 0 || len(name) > 128 {
		return Device{}, ErrUnauthorized
	}
	id := identity.DeviceID(key)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Device{}, err
	}
	defer tx.Rollback()
	var existing Device
	err = scanDevice(tx.QueryRowContext(ctx, "SELECT d.id,d.group_id,d.name,d.public_key,d.admin,d.revoked,d.incarnation,g.revision FROM devices d JOIN groups g ON g.id=d.group_id WHERE d.id=?", id), &existing)
	if err == nil && !existing.Revoked {
		if !bytes.Equal(existing.PublicKey, key) {
			return Device{}, ErrUnauthorized
		}
		return existing, nil
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return Device{}, err
	}
	missing := errors.Is(err, sql.ErrNoRows)
	groupID, err := randomIncarnation()
	if err != nil {
		return Device{}, err
	}
	incarnation, err := randomIncarnation()
	if err != nil {
		return Device{}, err
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO groups(id,revision) VALUES(?,1)", groupID); err != nil {
		return Device{}, err
	}
	device := Device{ID: id, GroupID: groupID, Name: name, PublicKey: key, Incarnation: incarnation, MembershipRevision: 1}
	if missing {
		_, err = tx.ExecContext(ctx, "INSERT INTO devices(id,group_id,name,public_key,incarnation) VALUES(?,?,?,?,?)", id, groupID, name, key, incarnation)
	} else {
		_, err = tx.ExecContext(ctx, "UPDATE devices SET group_id=?,name=?,public_key=?,admin=0,revoked=0,incarnation=? WHERE id=? AND revoked=1", groupID, name, key, incarnation, id)
	}
	if err != nil {
		return Device{}, err
	}
	return device, tx.Commit()
}

func (s *Control) Device(ctx context.Context, id string) (Device, error) {
	var d Device
	err := scanDevice(s.db.QueryRowContext(ctx, "SELECT d.id,d.group_id,d.name,d.public_key,d.admin,d.revoked,d.incarnation,g.revision FROM devices d JOIN groups g ON g.id=d.group_id WHERE d.id=?", id), &d)
	if errors.Is(err, sql.ErrNoRows) || d.Revoked {
		return Device{}, ErrUnauthorized
	}
	return d, err
}
func (s *Control) Devices(ctx context.Context, group string) ([]Device, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT d.id,d.group_id,d.name,d.public_key,d.admin,d.revoked,d.incarnation,g.revision FROM devices d JOIN groups g ON g.id=d.group_id WHERE d.group_id=? AND d.revoked=0 ORDER BY d.name,d.id", group)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Device{}
	for rows.Next() {
		var d Device
		if err = rows.Scan(&d.ID, &d.GroupID, &d.Name, &d.PublicKey, &d.Admin, &d.Revoked, &d.Incarnation, &d.MembershipRevision); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}
func (s *Control) Invitation(ctx context.Context, member Device) (string, time.Time, error) {
	current, err := s.Device(ctx, member.ID)
	if err != nil || current.Revoked {
		return "", time.Time{}, ErrUnauthorized
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", time.Time{}, err
	}
	defer tx.Rollback()
	now := time.Now()
	// Retain a short diagnostic window without allowing one-time invitations to
	// grow the database forever. No token or plaintext secret is stored.
	if _, err = tx.ExecContext(ctx, "DELETE FROM invitations WHERE expires<?", now.Add(-24*time.Hour).Unix()); err != nil {
		return "", time.Time{}, err
	}
	expires := now.Add(10 * time.Minute)
	for attempt := 0; attempt < 4; attempt++ {
		var raw [5]byte
		if _, err = rand.Read(raw[:]); err != nil {
			return "", time.Time{}, err
		}
		compact := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw[:])
		token := compact[:4] + "-" + compact[4:]
		sum := sha256.Sum256([]byte(compact))
		result, insertErr := tx.ExecContext(ctx, "INSERT OR IGNORE INTO invitations(hash,group_id,inviter,expires,inviter_incarnation,created_revision) VALUES(?,?,?,?,?,?)", sum[:], current.GroupID, current.ID, expires.Unix(), current.Incarnation, current.MembershipRevision)
		if insertErr != nil {
			return "", time.Time{}, insertErr
		}
		inserted, rowsErr := result.RowsAffected()
		if rowsErr != nil {
			return "", time.Time{}, rowsErr
		}
		if inserted == 1 {
			if err = tx.Commit(); err != nil {
				return "", time.Time{}, err
			}
			return token, expires, nil
		}
	}
	return "", time.Time{}, errors.New("pairing code collision retry exhausted")
}

func (s *Control) LANInvitation(ctx context.Context, member Device, targetID string) (string, time.Time, error) {
	if len(targetID) != 64 || targetID == member.ID {
		return "", time.Time{}, ErrUnauthorized
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", time.Time{}, err
	}
	defer tx.Rollback()
	var current Device
	if err = scanDevice(tx.QueryRowContext(ctx, "SELECT d.id,d.group_id,d.name,d.public_key,d.admin,d.revoked,d.incarnation,g.revision FROM devices d JOIN groups g ON g.id=d.group_id WHERE d.id=?", member.ID), &current); err != nil || current.Revoked || current.Incarnation != member.Incarnation {
		return "", time.Time{}, ErrUnauthorized
	}
	var raw [32]byte
	if _, err = rand.Read(raw[:]); err != nil {
		return "", time.Time{}, err
	}
	token := base64.RawURLEncoding.EncodeToString(raw[:])
	sum := sha256.Sum256([]byte(token))
	expires := time.Now().Add(time.Minute)
	if _, err = tx.ExecContext(ctx, "INSERT INTO invitations(hash,group_id,inviter,expires,inviter_incarnation,created_revision,target_id) VALUES(?,?,?,?,?,?,?)", sum[:], current.GroupID, current.ID, expires.Unix(), current.Incarnation, current.MembershipRevision, targetID); err != nil {
		return "", time.Time{}, err
	}
	return token, expires, tx.Commit()
}
func (s *Control) Join(ctx context.Context, token, name string, key []byte) (Device, error) {
	return s.JoinWithSwitch(ctx, token, name, key, nil)
}

func (s *Control) JoinWithSwitch(ctx context.Context, token, name string, key []byte, switchFrom *SwitchExpectation) (Device, error) {
	normalized, ok := normalizePairingCode(token)
	if !ok {
		return Device{}, ErrInvitationInvalid
	}
	if len(key) != 32 || len(name) == 0 || len(name) > 128 {
		return Device{}, ErrInvitationConflict
	}
	sum := sha256.Sum256([]byte(normalized))
	deviceID := identity.DeviceID(key)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Device{}, err
	}
	defer tx.Rollback()
	var group, usedBy, inviterIncarnation, usedIncarnation, currentInviterIncarnation, targetID string
	var expires int64
	var used, inviterRevoked int
	var createdRevision, currentRevision uint64
	err = tx.QueryRowContext(ctx, "SELECT i.group_id,i.expires,i.used,i.used_by,i.inviter_incarnation,i.created_revision,i.used_incarnation,i.target_id,d.revoked,d.incarnation,g.revision FROM invitations i JOIN devices d ON d.id=i.inviter JOIN groups g ON g.id=i.group_id WHERE i.hash=?", sum[:]).Scan(&group, &expires, &used, &usedBy, &inviterIncarnation, &createdRevision, &usedIncarnation, &targetID, &inviterRevoked, &currentInviterIncarnation, &currentRevision)
	if errors.Is(err, sql.ErrNoRows) {
		return Device{}, ErrInvitationInvalid
	}
	if err != nil {
		return Device{}, err
	}
	if inviterRevoked != 0 || inviterIncarnation == "" || inviterIncarnation != currentInviterIncarnation || createdRevision == 0 || createdRevision > currentRevision {
		return Device{}, ErrInvitationInvalid
	}
	if targetID != "" && targetID != deviceID {
		return Device{}, ErrInvitationConflict
	}
	if used != 0 {
		if usedBy == deviceID {
			if existing, existingErr := joinedDevice(ctx, tx, deviceID, group, key, usedIncarnation); existingErr == nil {
				return existing, nil
			}
		}
		return Device{}, ErrInvitationUsed
	}
	if expires <= time.Now().Unix() {
		return Device{}, ErrInvitationExpired
	}
	var active Device
	activeErr := scanDevice(tx.QueryRowContext(ctx, "SELECT d.id,d.group_id,d.name,d.public_key,d.admin,d.revoked,d.incarnation,g.revision FROM devices d JOIN groups g ON g.id=d.group_id WHERE d.id=?", deviceID), &active)
	if activeErr == nil && !active.Revoked && active.GroupID != group {
		if switchFrom == nil || switchFrom.CurrentGroup != active.GroupID || switchFrom.CurrentIncarnation != active.Incarnation || switchFrom.CurrentRevision != active.MembershipRevision || !bytes.Equal(active.PublicKey, key) {
			return Device{}, ErrInvitationConflict
		}
		newIncarnation, newErr := randomIncarnation()
		if newErr != nil {
			return Device{}, newErr
		}
		res, updateErr := tx.ExecContext(ctx, "UPDATE invitations SET used=1,used_by=?,used_at=?,used_incarnation=? WHERE hash=? AND used=0", deviceID, time.Now().Unix(), newIncarnation, sum[:])
		if updateErr != nil {
			return Device{}, updateErr
		}
		if rows, rowsErr := res.RowsAffected(); rowsErr != nil || rows != 1 {
			return Device{}, ErrInvitationUsed
		}
		if _, updateErr = tx.ExecContext(ctx, "UPDATE groups SET revision=revision+1 WHERE id IN (?,?)", active.GroupID, group); updateErr != nil {
			return Device{}, updateErr
		}
		if _, updateErr = tx.ExecContext(ctx, "UPDATE devices SET group_id=?,name=?,admin=0,revoked=0,incarnation=? WHERE id=? AND incarnation=? AND revoked=0", group, name, newIncarnation, deviceID, active.Incarnation); updateErr != nil {
			return Device{}, updateErr
		}
		if updateErr = tx.QueryRowContext(ctx, "SELECT revision FROM groups WHERE id=?", group).Scan(&currentRevision); updateErr != nil {
			return Device{}, updateErr
		}
		result := Device{ID: deviceID, GroupID: group, Name: name, PublicKey: key, Incarnation: newIncarnation, MembershipRevision: currentRevision}
		return result, tx.Commit()
	}
	if activeErr != nil && !errors.Is(activeErr, sql.ErrNoRows) {
		return Device{}, activeErr
	}

	// An already-enrolled, non-revoked identity is a successful idempotent
	// pairing result. The fresh invitation is still consumed exactly once.
	existing, existingErr := joinedDevice(ctx, tx, deviceID, group, key, "")
	if existingErr != nil && !errors.Is(existingErr, sql.ErrNoRows) {
		return Device{}, ErrInvitationConflict
	}

	incarnation := ""
	if existingErr == nil {
		incarnation = existing.Incarnation
	} else if incarnation, err = randomIncarnation(); err != nil {
		return Device{}, err
	}
	res, err := tx.ExecContext(ctx, "UPDATE invitations SET used=1,used_by=?,used_at=?,used_incarnation=? WHERE hash=? AND used=0", deviceID, time.Now().Unix(), incarnation, sum[:])
	if err != nil {
		return Device{}, err
	}
	n, err := res.RowsAffected()
	if err != nil || n != 1 {
		return Device{}, ErrInvitationUsed
	}
	if existingErr == nil {
		if err = tx.Commit(); err != nil {
			return Device{}, err
		}
		return existing, nil
	}
	if _, err = tx.ExecContext(ctx, "UPDATE groups SET revision=revision+1 WHERE id=?", group); err != nil {
		return Device{}, err
	}
	if err = tx.QueryRowContext(ctx, "SELECT revision FROM groups WHERE id=?", group).Scan(&currentRevision); err != nil {
		return Device{}, err
	}
	d := Device{ID: deviceID, GroupID: group, Name: name, PublicKey: key, Incarnation: incarnation, MembershipRevision: currentRevision}
	result, err := tx.ExecContext(ctx, "UPDATE devices SET group_id=?,name=?,public_key=?,admin=0,revoked=0,incarnation=? WHERE id=? AND revoked=1", d.GroupID, d.Name, d.PublicKey, d.Incarnation, d.ID)
	if err != nil {
		return Device{}, err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return Device{}, err
	}
	if changed == 0 {
		if _, err = tx.ExecContext(ctx, "INSERT INTO devices(id,group_id,name,public_key,incarnation) VALUES(?,?,?,?,?)", d.ID, d.GroupID, d.Name, d.PublicKey, d.Incarnation); err != nil {
			return Device{}, fmt.Errorf("register pairing identity: %w", err)
		}
	}
	return d, tx.Commit()
}

func joinedDevice(ctx context.Context, tx *sql.Tx, deviceID, group string, key []byte, expectedIncarnation string) (Device, error) {
	var d Device
	err := scanDevice(tx.QueryRowContext(ctx, "SELECT d.id,d.group_id,d.name,d.public_key,d.admin,d.revoked,d.incarnation,g.revision FROM devices d JOIN groups g ON g.id=d.group_id WHERE d.id=?", deviceID), &d)
	if err != nil {
		return Device{}, err
	}
	if d.Revoked && expectedIncarnation == "" && bytes.Equal(d.PublicKey, key) {
		return Device{}, sql.ErrNoRows
	}
	if d.Revoked || d.GroupID != group || !bytes.Equal(d.PublicKey, key) || expectedIncarnation != "" && d.Incarnation != expectedIncarnation {
		return Device{}, ErrInvitationConflict
	}
	return d, nil
}

// normalizePairingCode accepts the current human-readable ABCD-EFGH code and
// legacy 43-character invitations during the compatibility window.
func normalizePairingCode(token string) (string, bool) {
	token = strings.TrimSpace(token)
	if len(token) == 43 {
		if _, err := base64.RawURLEncoding.DecodeString(token); err == nil {
			return token, true
		}
		return "", false
	}
	compact := strings.ToUpper(strings.NewReplacer("-", "", " ", "").Replace(token))
	if len(compact) != 8 {
		return "", false
	}
	for _, r := range compact {
		if (r < 'A' || r > 'Z') && (r < '2' || r > '7') {
			return "", false
		}
	}
	return compact, true
}

// JoinTestCode adds a device to an explicitly selected compatibility group. It
// is intentionally separate from Join so the production invitation path keeps
// its high-entropy, single-use semantics. A fixed-code join is an authorized
// administrator bootstrap path: new members and existing members are marked
// admin, while revoked identities remain rejected.
func (s *Control) JoinTestCode(ctx context.Context, name string, key []byte, groupID string) (Device, error) {
	if len(key) != 32 || len(name) == 0 || len(name) > 128 {
		return Device{}, ErrInvitation
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Device{}, err
	}
	defer tx.Rollback()
	group := groupID
	if group == "" {
		err = tx.QueryRowContext(ctx, "SELECT group_id FROM devices WHERE revoked=0 ORDER BY admin DESC, id LIMIT 1").Scan(&group)
		if errors.Is(err, sql.ErrNoRows) {
			return Device{}, ErrInvitation
		}
		if err != nil {
			return Device{}, err
		}
	} else {
		var exists int
		if err = tx.QueryRowContext(ctx, "SELECT 1 FROM devices WHERE group_id=? AND revoked=0 ORDER BY admin DESC, id LIMIT 1", group).Scan(&exists); errors.Is(err, sql.ErrNoRows) {
			return Device{}, ErrInvitation
		} else if err != nil {
			return Device{}, err
		}
	}
	incarnation, err := randomIncarnation()
	if err != nil {
		return Device{}, err
	}
	var revision uint64
	if err = tx.QueryRowContext(ctx, "SELECT revision FROM groups WHERE id=?", group).Scan(&revision); err != nil {
		return Device{}, err
	}
	d := Device{ID: identity.DeviceID(key), GroupID: group, Name: name, PublicKey: key, Admin: true, Incarnation: incarnation, MembershipRevision: revision}
	// Repeating the fixed development code with the same profile is expected
	// during UI/e2e runs. Return the existing non-revoked member instead of
	// surfacing a SQLite UNIQUE error; production Join remains single-use.
	var existing Device
	err = scanDevice(tx.QueryRowContext(ctx, "SELECT d.id,d.group_id,d.name,d.public_key,d.admin,d.revoked,d.incarnation,g.revision FROM devices d JOIN groups g ON g.id=d.group_id WHERE d.id=?", d.ID), &existing)
	if err == nil {
		if existing.Revoked || existing.GroupID != group || string(existing.PublicKey) != string(key) {
			return Device{}, ErrInvitation
		}
		if !existing.Admin {
			if _, err = tx.ExecContext(ctx, "UPDATE devices SET admin=1,name=? WHERE id=?", name, existing.ID); err != nil {
				return Device{}, err
			}
			existing.Admin = true
			existing.Name = name
		}
		return existing, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return Device{}, err
	}
	if _, err = tx.ExecContext(ctx, "UPDATE groups SET revision=revision+1 WHERE id=?", group); err != nil {
		return Device{}, err
	}
	if err = tx.QueryRowContext(ctx, "SELECT revision FROM groups WHERE id=?", group).Scan(&d.MembershipRevision); err != nil {
		return Device{}, err
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO devices(id,group_id,name,public_key,admin,incarnation) VALUES(?,?,?,?,1,?)", d.ID, d.GroupID, d.Name, d.PublicKey, d.Incarnation); err != nil {
		return Device{}, fmt.Errorf("register device: %w", err)
	}
	return d, tx.Commit()
}
func (s *Control) Revoke(ctx context.Context, actor Device, request RevokeRequest) (uint64, error) {
	if request.RequestID == "" || len(request.RequestID) > 128 || request.TargetID == "" || request.TargetID == actor.ID || request.TargetIncarnation == "" || request.ExpectedRevision == 0 {
		return 0, ErrUnauthorized
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var repeatedActor, repeatedTarget, repeatedIncarnation string
	var repeatedRevision uint64
	err = tx.QueryRowContext(ctx, "SELECT actor_id,target_id,target_incarnation,result_revision FROM membership_requests WHERE request_id=?", request.RequestID).Scan(&repeatedActor, &repeatedTarget, &repeatedIncarnation, &repeatedRevision)
	if err == nil {
		if repeatedActor == actor.ID && repeatedTarget == request.TargetID && repeatedIncarnation == request.TargetIncarnation {
			return repeatedRevision, nil
		}
		return 0, ErrUnauthorized
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	var currentActor, target Device
	if err = scanDevice(tx.QueryRowContext(ctx, "SELECT d.id,d.group_id,d.name,d.public_key,d.admin,d.revoked,d.incarnation,g.revision FROM devices d JOIN groups g ON g.id=d.group_id WHERE d.id=?", actor.ID), &currentActor); err != nil || currentActor.Revoked || currentActor.Incarnation != actor.Incarnation {
		return 0, ErrUnauthorized
	}
	if err = scanDevice(tx.QueryRowContext(ctx, "SELECT d.id,d.group_id,d.name,d.public_key,d.admin,d.revoked,d.incarnation,g.revision FROM devices d JOIN groups g ON g.id=d.group_id WHERE d.id=?", request.TargetID), &target); err != nil || target.Revoked || target.GroupID != currentActor.GroupID || target.Incarnation != request.TargetIncarnation || target.MembershipRevision != request.ExpectedRevision {
		return 0, ErrUnauthorized
	}
	if _, err = tx.ExecContext(ctx, "UPDATE devices SET revoked=1 WHERE id=? AND incarnation=? AND revoked=0", target.ID, target.Incarnation); err != nil {
		return 0, err
	}
	if _, err = tx.ExecContext(ctx, "UPDATE groups SET revision=revision+1 WHERE id=?", currentActor.GroupID); err != nil {
		return 0, err
	}
	var revision uint64
	if err = tx.QueryRowContext(ctx, "SELECT revision FROM groups WHERE id=?", currentActor.GroupID).Scan(&revision); err != nil {
		return 0, err
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO membership_requests(request_id,actor_id,target_id,target_incarnation,result_revision,created_at) VALUES(?,?,?,?,?,?)", request.RequestID, actor.ID, target.ID, target.Incarnation, revision, time.Now().Unix()); err != nil {
		return 0, err
	}
	return revision, tx.Commit()
}
