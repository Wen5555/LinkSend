package app

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/Wen5555/LinkSend/internal/clipboardsync"
	"github.com/Wen5555/LinkSend/internal/identity"
	"github.com/Wen5555/LinkSend/internal/protocol"
)

type ClipboardGrant struct {
	PeerID                  string `json:"peer_id"`
	Direction               string `json:"direction"`
	Kind                    string `json:"kind"`
	Enabled                 bool   `json:"enabled"`
	Revision                uint64 `json:"revision"`
	UpdatedAt               string `json:"updated_at"`
	AuthorizationGeneration uint64 `json:"authorization_generation"`
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
	s.clipboardGrantMu.Lock()
	defer s.clipboardGrantMu.Unlock()
	rows, err := s.store.db.QueryContext(ctx, `SELECT peer_id,direction,kind,enabled,revision,updated_at,authorization_generation
		FROM clipboard_grants WHERE peer_id=? ORDER BY direction,kind`, peerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var grants []ClipboardGrant
	for rows.Next() {
		var grant ClipboardGrant
		if err = rows.Scan(&grant.PeerID, &grant.Direction, &grant.Kind, &grant.Enabled, &grant.Revision, &grant.UpdatedAt, &grant.AuthorizationGeneration); err != nil {
			return nil, err
		}
		if grant.Enabled {
			generation, generationErr := s.clipboardPeerGeneration(grant.PeerID)
			grant.Enabled = generationErr == nil && generation == grant.AuthorizationGeneration
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
	s.clipboardGrantMu.Lock()
	defer s.clipboardGrantMu.Unlock()
	var authorizationGeneration uint64
	if patch.Enabled {
		if authorizationGeneration, err = s.clipboardPeerGeneration(patch.PeerID); err != nil {
			return ClipboardGrant{}, err
		}
	}
	if hook := s.clipboardGrantBeforeCommit; hook != nil {
		hook()
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
		Enabled: patch.Enabled, Revision: current + 1, UpdatedAt: now, AuthorizationGeneration: authorizationGeneration}
	result, err := tx.ExecContext(ctx, `INSERT INTO clipboard_grants(peer_id,direction,kind,enabled,revision,updated_at,authorization_generation)
		VALUES(?,?,?,?,?,?,?) ON CONFLICT(peer_id,direction,kind) DO UPDATE SET
		enabled=excluded.enabled,revision=excluded.revision,updated_at=excluded.updated_at,authorization_generation=excluded.authorization_generation WHERE clipboard_grants.revision=?`,
		next.PeerID, next.Direction, next.Kind, next.Enabled, next.Revision, next.UpdatedAt, next.AuthorizationGeneration, current)
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
	s.clipboardSync.InvalidateGrant(patch.PeerID, patch.Direction == "send", clipboardsync.Kind(patch.Kind))
	s.notifyChange()
	return next, nil
}

func (s *Service) clearClipboardGrants(peerID string) error {
	if s.store == nil {
		return nil
	}
	s.clipboardGrantMu.Lock()
	defer s.clipboardGrantMu.Unlock()
	return s.clearClipboardGrantsLocked(peerID)
}

func (s *Service) clearClipboardGrantsLocked(peerID string) error {
	_, err := s.store.db.Exec(`UPDATE clipboard_grants SET enabled=0,revision=revision+1,updated_at=? WHERE peer_id=?`, time.Now().UTC().Format(time.RFC3339Nano), peerID)
	s.invalidateClipboardState()
	s.notifyChange()
	return err
}

func (s *Service) ClipboardSyncEnabled() bool {
	if s.store == nil {
		return false
	}
	s.clipboardGrantMu.Lock()
	defer s.clipboardGrantMu.Unlock()
	rows, err := s.store.db.Query(`SELECT peer_id,authorization_generation FROM clipboard_grants WHERE enabled=1`)
	if err != nil {
		return false
	}
	defer rows.Close()
	for rows.Next() {
		var peerID string
		var generation uint64
		if rows.Scan(&peerID, &generation) == nil {
			if current, currentErr := s.clipboardPeerGeneration(peerID); currentErr == nil && current == generation {
				return true
			}
		}
	}
	return false
}

func (s *Service) clipboardPeerGeneration(peerID string) (uint64, error) {
	if err := s.checkPeerAllowed(peerID); err != nil {
		return 0, err
	}
	s.trustMu.Lock()
	defer s.trustMu.Unlock()
	peers, err := identity.LoadTrust(s.cfg.DataDir)
	if err != nil {
		return 0, err
	}
	for _, peer := range peers {
		if peer.ID == peerID && peer.ID != s.identity.ID() && peer.GrantGeneration > 0 {
			return peer.GrantGeneration, nil
		}
	}
	return 0, protocol.Fail(protocol.Unpaired, "current paired peer grant required")
}
