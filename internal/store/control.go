// Package store persists control-plane membership. It has no file-transfer API.
package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"example.com/linksend/internal/identity"
	_ "modernc.org/sqlite"
)

var ErrUnauthorized = errors.New("unauthorized or revoked device")
var ErrInvitation = errors.New("invalid, expired or used invitation")

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
CREATE TABLE IF NOT EXISTS invitations(hash BLOB PRIMARY KEY,group_id TEXT NOT NULL,inviter TEXT NOT NULL,expires INTEGER NOT NULL,used INTEGER NOT NULL DEFAULT 0);
CREATE TABLE IF NOT EXISTS schema_version(version INTEGER PRIMARY KEY);
INSERT OR IGNORE INTO schema_version(version) VALUES(1);`)
	if err != nil {
		db.Close()
		return nil, err
	}
	return &Control{db: db}, nil
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
func (s *Control) Invitation(ctx context.Context, admin Device) (string, time.Time, error) {
	current, err := s.Device(ctx, admin.ID)
	if err != nil || !current.Admin {
		return "", time.Time{}, ErrUnauthorized
	}
	var raw [32]byte
	if _, err = rand.Read(raw[:]); err != nil {
		return "", time.Time{}, err
	}
	token := base64.RawURLEncoding.EncodeToString(raw[:])
	sum := sha256.Sum256([]byte(token))
	expires := time.Now().Add(10 * time.Minute)
	_, err = s.db.ExecContext(ctx, "INSERT INTO invitations(hash,group_id,inviter,expires) VALUES(?,?,?,?)", sum[:], current.GroupID, current.ID, expires.Unix())
	return token, expires, err
}
func (s *Control) Join(ctx context.Context, token, name string, key []byte) (Device, error) {
	if len(token) != 43 || len(key) != 32 || len(name) == 0 || len(name) > 128 {
		return Device{}, ErrInvitation
	}
	sum := sha256.Sum256([]byte(token))
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Device{}, err
	}
	defer tx.Rollback()
	var group string
	err = tx.QueryRowContext(ctx, "SELECT i.group_id FROM invitations i JOIN devices d ON d.id=i.inviter WHERE i.hash=? AND i.used=0 AND i.expires>? AND d.revoked=0", sum[:], time.Now().Unix()).Scan(&group)
	if errors.Is(err, sql.ErrNoRows) {
		return Device{}, ErrInvitation
	}
	if err != nil {
		return Device{}, err
	}
	res, err := tx.ExecContext(ctx, "UPDATE invitations SET used=1 WHERE hash=? AND used=0", sum[:])
	if err != nil {
		return Device{}, err
	}
	n, err := res.RowsAffected()
	if err != nil || n != 1 {
		return Device{}, ErrInvitation
	}
	d := Device{ID: identity.DeviceID(key), GroupID: group, Name: name, PublicKey: key}
	if _, err = tx.ExecContext(ctx, "INSERT INTO devices(id,group_id,name,public_key) VALUES(?,?,?,?)", d.ID, d.GroupID, d.Name, d.PublicKey); err != nil {
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
