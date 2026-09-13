package app

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

type ClipboardGrant struct {
	PeerID    string `json:"peer_id"`
	Direction string `json:"direction"`
	Kind      string `json:"kind"`
	Enabled   bool   `json:"enabled"`
	Revision  uint64 `json:"revision"`
	UpdatedAt string `json:"updated_at"`
}

type ClipboardGrantPatch struct {
	PeerID           string `json:"peer_id"`
	Direction        string `json:"direction"`
	Kind             string `json:"kind"`
	Enabled          bool   `json:"enabled"`
	ExpectedRevision uint64 `json:"expected_revision"`
}

func validClipboardGrant(direction, kind string) bool {
	return (direction == "send" || direction == "receive") &&
		(kind == "text" || kind == "link" || kind == "image")
}

func (s *Service) ClipboardGrants(ctx context.Context, peerID string) ([]ClipboardGrant, error) {
	done, err := s.beginWorkspaceWork()
	if err != nil {
		return nil, err
	}
	defer done()
	if strings.TrimSpace(peerID) == "" {
		return nil, errors.New("INVALID_ARGUMENT: peer required")
	}
	rows, err := s.store.db.QueryContext(ctx, `SELECT peer_id,direction,kind,enabled,revision,updated_at
		FROM clipboard_grants WHERE peer_id=? ORDER BY direction,kind`, peerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var grants []ClipboardGrant
	for rows.Next() {
		var grant ClipboardGrant
		if err = rows.Scan(&grant.PeerID, &grant.Direction, &grant.Kind, &grant.Enabled, &grant.Revision, &grant.UpdatedAt); err != nil {
			return nil, err
		}
		grants = append(grants, grant)
	}
	return grants, rows.Err()
}

func (s *Service) SetClipboardGrant(ctx context.Context, patch ClipboardGrantPatch) (ClipboardGrant, error) {
	done, err := s.beginWorkspaceWork()
	if err != nil {
		return ClipboardGrant{}, err
	}
	defer done()
	if strings.TrimSpace(patch.PeerID) == "" || !validClipboardGrant(patch.Direction, patch.Kind) {
		return ClipboardGrant{}, errors.New("INVALID_ARGUMENT: clipboard grant")
	}
	if patch.Enabled {
		if err = s.checkPeerAllowed(patch.PeerID); err != nil {
			return ClipboardGrant{}, err
		}
	}
	tx, err := s.store.db.BeginTx(ctx, nil)
	if err != nil {
		return ClipboardGrant{}, err
	}
	defer tx.Rollback()
	var current uint64
	err = tx.QueryRowContext(ctx, `SELECT revision FROM clipboard_grants WHERE peer_id=? AND direction=? AND kind=?`,
		patch.PeerID, patch.Direction, patch.Kind).Scan(&current)
	if errors.Is(err, sql.ErrNoRows) {
		current, err = 0, nil
	}
	if err != nil {
		return ClipboardGrant{}, err
	}
	if current != patch.ExpectedRevision {
		return ClipboardGrant{}, ErrMetadataConflict
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	next := ClipboardGrant{PeerID: patch.PeerID, Direction: patch.Direction, Kind: patch.Kind,
		Enabled: patch.Enabled, Revision: current + 1, UpdatedAt: now}
	result, err := tx.ExecContext(ctx, `INSERT INTO clipboard_grants(peer_id,direction,kind,enabled,revision,updated_at)
		VALUES(?,?,?,?,?,?) ON CONFLICT(peer_id,direction,kind) DO UPDATE SET
		enabled=excluded.enabled,revision=excluded.revision,updated_at=excluded.updated_at WHERE clipboard_grants.revision=?`,
		next.PeerID, next.Direction, next.Kind, next.Enabled, next.Revision, next.UpdatedAt, current)
	if err != nil {
		return ClipboardGrant{}, err
	}
	if changed, rowsErr := result.RowsAffected(); rowsErr != nil || changed != 1 {
		if rowsErr != nil {
			return ClipboardGrant{}, rowsErr
		}
		return ClipboardGrant{}, ErrMetadataConflict
	}
	if err = tx.Commit(); err != nil {
		return ClipboardGrant{}, err
	}
	s.notifyChange()
	return next, nil
}

func (s *Service) clearClipboardGrants(peerID string) error {
	if s.store == nil {
		return nil
	}
	_, err := s.store.db.Exec(`DELETE FROM clipboard_grants WHERE peer_id=?`, peerID)
	return err
}

func (s *Service) ClipboardSyncEnabled() bool {
	if s.store == nil {
		return false
	}
	var count int
	return s.store.db.QueryRow(`SELECT count(*) FROM clipboard_grants WHERE enabled=1`).Scan(&count) == nil && count > 0
}
