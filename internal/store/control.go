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
	ID        string `json:"id"`
	GroupID   string `json:"group_id"`
	Name      string `json:"name"`
	PublicKey []byte `json:"public_key"`
	Admin     bool   `json:"admin"`
	Revoked   bool   `json:"revoked"`
	Online    bool   `json:"online"`
}
type Control struct{ db *sql.DB }

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
CREATE TABLE IF NOT EXISTS devices(id TEXT PRIMARY KEY,group_id TEXT NOT NULL,name TEXT NOT NULL,public_key BLOB NOT NULL,admin INTEGER NOT NULL DEFAULT 0,revoked INTEGER NOT NULL DEFAULT 0);
CREATE TABLE IF NOT EXISTS invitations(hash BLOB PRIMARY KEY,group_id TEXT NOT NULL,inviter TEXT NOT NULL,expires INTEGER NOT NULL,used INTEGER NOT NULL DEFAULT 0,used_by TEXT NOT NULL DEFAULT '',used_at INTEGER NOT NULL DEFAULT 0);
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
		if _, err = db.Exec("INSERT INTO schema_version(version) VALUES(2)"); err != nil {
			db.Close()
			return nil, err
		}
	} else if len(versions) == 1 && versions[0] == 1 {
		if err = migrateControlV1ToV2(db); err != nil {
			db.Close()
			return nil, err
		}
	} else if len(versions) != 1 || versions[0] != 2 {
		db.Close()
		return nil, errors.New("unsupported control database schema version")
	}
	return &Control{db: db}, nil
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
	d := Device{ID: id, GroupID: id, Name: name, PublicKey: key, Admin: true}
	if _, err = tx.ExecContext(ctx, "INSERT INTO devices(id,group_id,name,public_key,admin) VALUES(?,?,?,?,1)", d.ID, d.GroupID, d.Name, d.PublicKey); err != nil {
		return Device{}, err
	}
	return d, tx.Commit()
}
func (s *Control) Device(ctx context.Context, id string) (Device, error) {
	var d Device
	err := s.db.QueryRowContext(ctx, "SELECT id,group_id,name,public_key,admin,revoked FROM devices WHERE id=?", id).Scan(&d.ID, &d.GroupID, &d.Name, &d.PublicKey, &d.Admin, &d.Revoked)
	if errors.Is(err, sql.ErrNoRows) || d.Revoked {
		return Device{}, ErrUnauthorized
	}
	return d, err
}
func (s *Control) Devices(ctx context.Context, group string) ([]Device, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id,group_id,name,public_key,admin,revoked FROM devices WHERE group_id=? AND revoked=0 ORDER BY name,id", group)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Device{}
	for rows.Next() {
		var d Device
		if err = rows.Scan(&d.ID, &d.GroupID, &d.Name, &d.PublicKey, &d.Admin, &d.Revoked); err != nil {
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
		result, insertErr := tx.ExecContext(ctx, "INSERT OR IGNORE INTO invitations(hash,group_id,inviter,expires) VALUES(?,?,?,?)", sum[:], current.GroupID, current.ID, expires.Unix())
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
func (s *Control) Join(ctx context.Context, token, name string, key []byte) (Device, error) {
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
	var group, usedBy string
	var expires int64
	var used, inviterRevoked int
	err = tx.QueryRowContext(ctx, "SELECT i.group_id,i.expires,i.used,i.used_by,d.revoked FROM invitations i JOIN devices d ON d.id=i.inviter WHERE i.hash=?", sum[:]).Scan(&group, &expires, &used, &usedBy, &inviterRevoked)
	if errors.Is(err, sql.ErrNoRows) {
		return Device{}, ErrInvitationInvalid
	}
	if err != nil {
		return Device{}, err
	}
	if used != 0 {
		if usedBy == deviceID {
			if existing, existingErr := joinedDevice(ctx, tx, deviceID, group, key); existingErr == nil {
				return existing, nil
			}
		}
		return Device{}, ErrInvitationUsed
	}
	if inviterRevoked != 0 {
		return Device{}, ErrInvitationInvalid
	}
	if expires <= time.Now().Unix() {
		return Device{}, ErrInvitationExpired
	}

	// An already-enrolled, non-revoked identity is a successful idempotent
	// pairing result. The fresh invitation is still consumed exactly once.
	existing, existingErr := joinedDevice(ctx, tx, deviceID, group, key)
	if existingErr != nil && !errors.Is(existingErr, sql.ErrNoRows) {
		return Device{}, ErrInvitationConflict
	}

	res, err := tx.ExecContext(ctx, "UPDATE invitations SET used=1,used_by=?,used_at=? WHERE hash=? AND used=0", deviceID, time.Now().Unix(), sum[:])
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
	d := Device{ID: deviceID, GroupID: group, Name: name, PublicKey: key}
	if _, err = tx.ExecContext(ctx, "INSERT INTO devices(id,group_id,name,public_key) VALUES(?,?,?,?)", d.ID, d.GroupID, d.Name, d.PublicKey); err != nil {
		return Device{}, fmt.Errorf("register pairing identity: %w", err)
	}
	return d, tx.Commit()
}

func joinedDevice(ctx context.Context, tx *sql.Tx, deviceID, group string, key []byte) (Device, error) {
	var d Device
	err := tx.QueryRowContext(ctx, "SELECT id,group_id,name,public_key,admin,revoked FROM devices WHERE id=?", deviceID).Scan(&d.ID, &d.GroupID, &d.Name, &d.PublicKey, &d.Admin, &d.Revoked)
	if err != nil {
		return Device{}, err
	}
	if d.Revoked || d.GroupID != group || !bytes.Equal(d.PublicKey, key) {
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
	d := Device{ID: identity.DeviceID(key), GroupID: group, Name: name, PublicKey: key, Admin: true}
	// Repeating the fixed development code with the same profile is expected
	// during UI/e2e runs. Return the existing non-revoked member instead of
	// surfacing a SQLite UNIQUE error; production Join remains single-use.
	var existing Device
	err = tx.QueryRowContext(ctx, "SELECT id,group_id,name,public_key,admin,revoked FROM devices WHERE id=?", d.ID).Scan(&existing.ID, &existing.GroupID, &existing.Name, &existing.PublicKey, &existing.Admin, &existing.Revoked)
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
	if _, err = tx.ExecContext(ctx, "INSERT INTO devices(id,group_id,name,public_key,admin) VALUES(?,?,?,?,1)", d.ID, d.GroupID, d.Name, d.PublicKey); err != nil {
		return Device{}, fmt.Errorf("register device: %w", err)
	}
	return d, tx.Commit()
}
func (s *Control) Revoke(ctx context.Context, admin Device, id string) error {
	current, err := s.Device(ctx, admin.ID)
	if err != nil || !current.Admin || id == current.ID {
		return ErrUnauthorized
	}
	res, err := s.db.ExecContext(ctx, "UPDATE devices SET revoked=1 WHERE id=? AND group_id=? AND id<>?", id, current.GroupID, current.ID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrUnauthorized
	}
	return nil
}
