package app

import (
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

var (
	ErrMetadataConflict = errors.New("METADATA_REVISION_CONFLICT")
	ErrMetadataInvalid  = errors.New("METADATA_INVALID")
	ErrMetadataNotFound = errors.New("METADATA_NOT_FOUND")
)

const (
	defaultSendDraftID  = "main"
	maxDraftPaths       = 4096
	maxDraftPathsJSON   = 1 << 20
	maxDesktopPathBytes = 32768
	desktopTimestamp    = "2006-01-02T15:04:05.000000000Z"
)

// DeviceProfile contains local presentation and destination preferences only.
// Key pins, local denial, and auto-accept remain owned by the identity layer.
type DeviceProfile struct {
	PeerID           string `json:"peer_id"`
	Alias            string `json:"alias"`
	MyDevice         bool   `json:"my_device"`
	Pinned           bool   `json:"pinned"`
	Position         int    `json:"position"`
	ReceiveDirectory string `json:"receive_directory"`
	LastUsedAt       string `json:"last_used_at"`
	Revision         uint64 `json:"revision"`
}

// SendDraft is local metadata. Paths are never used as file-content IPC, and
// persisting a draft does not authorise a send or open any of its file bodies.
type SendDraft struct {
	ID        string   `json:"id"`
	Paths     []string `json:"paths"`
	PeerID    string   `json:"peer_id"`
	Revision  uint64   `json:"revision"`
	UpdatedAt string   `json:"updated_at"`
}

type desktopStore struct {
	db *sql.DB
}

func openDesktopStore(path string) (*desktopStore, error) {
	db, err := historyDB(path)
	if err != nil {
		return nil, err
	}
	return &desktopStore{db: db}, nil
}

func (s *desktopStore) Close() error { return s.db.Close() }

func migrateDesktopMetadata(tx *sql.Tx) error {
	statements := []string{
		`CREATE TABLE device_profiles (
			peer_id TEXT PRIMARY KEY,
			alias TEXT NOT NULL DEFAULT '',
			my_device INTEGER NOT NULL DEFAULT 0 CHECK (my_device IN (0,1)),
			pinned INTEGER NOT NULL DEFAULT 0 CHECK (pinned IN (0,1)),
			position INTEGER NOT NULL DEFAULT 0 CHECK (position >= 0),
			receive_directory TEXT NOT NULL DEFAULT '',
			last_used_at TEXT NOT NULL DEFAULT '',
			revision INTEGER NOT NULL CHECK (revision > 0)
		)`,
		`CREATE INDEX device_profiles_order ON device_profiles(pinned DESC,position,peer_id)`,
		`CREATE TABLE send_drafts (
			id TEXT PRIMARY KEY,
			source_paths TEXT NOT NULL CHECK (json_valid(source_paths) AND json_type(source_paths)='array'),
			peer_id TEXT NOT NULL DEFAULT '',
			revision INTEGER NOT NULL CHECK (revision > 0),
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE send_queue (
			id TEXT PRIMARY KEY,
			request_id TEXT NOT NULL UNIQUE,
			peer_id TEXT NOT NULL,
			source_paths TEXT NOT NULL CHECK (json_valid(source_paths) AND json_type(source_paths)='array'),
			source_digest TEXT NOT NULL DEFAULT '',
			state TEXT NOT NULL,
			position INTEGER NOT NULL,
			task_id TEXT NOT NULL DEFAULT '',
			expires_at TEXT NOT NULL DEFAULT '',
			last_error TEXT NOT NULL DEFAULT '',
			wait_for_peer INTEGER NOT NULL DEFAULT 0 CHECK (wait_for_peer IN (0,1)),
			revision INTEGER NOT NULL CHECK (revision > 0),
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE INDEX send_queue_dispatch ON send_queue(state,position,created_at,id)`,
		`CREATE INDEX send_queue_peer ON send_queue(peer_id,state)`,
	}
	for _, statement := range statements {
		if _, err := tx.Exec(statement); err != nil {
			return fmt.Errorf("TASK_STORE_MIGRATION_FAILED: %w", err)
		}
	}
	return nil
}

const deviceProfileColumns = `peer_id,alias,my_device,pinned,position,receive_directory,last_used_at,revision`

type metadataScanner interface {
	Scan(...any) error
}

func scanDeviceProfile(row metadataScanner) (DeviceProfile, error) {
	var p DeviceProfile
	err := row.Scan(&p.PeerID, &p.Alias, &p.MyDevice, &p.Pinned, &p.Position, &p.ReceiveDirectory, &p.LastUsedAt, &p.Revision)
	if errors.Is(err, sql.ErrNoRows) {
		err = ErrMetadataNotFound
	}
	return p, err
}

func (s *desktopStore) DeviceProfiles() ([]DeviceProfile, error) {
	rows, err := s.db.Query(`SELECT ` + deviceProfileColumns + ` FROM device_profiles ORDER BY pinned DESC,position,peer_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	profiles := make([]DeviceProfile, 0)
	for rows.Next() {
		p, err := scanDeviceProfile(rows)
		if err != nil {
			return nil, err
		}
		profiles = append(profiles, p)
	}
	return profiles, rows.Err()
}

func (s *desktopStore) DeviceProfile(peerID string) (DeviceProfile, error) {
	if !validMetadataPeerID(peerID) {
		return DeviceProfile{}, ErrMetadataInvalid
	}
	return scanDeviceProfile(s.db.QueryRow(`SELECT `+deviceProfileColumns+` FROM device_profiles WHERE peer_id=?`, peerID))
}

// Revision is the caller's expected revision. Zero means insert-only. The
// actual committed row is returned; stale commands never overwrite preferences.
func (s *desktopStore) SaveDeviceProfile(p DeviceProfile) (DeviceProfile, error) {
	if err := validateDeviceProfile(&p); err != nil {
		return DeviceProfile{}, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return DeviceProfile{}, err
	}
	defer tx.Rollback()
	var result sql.Result
	if p.Revision == 0 {
		result, err = tx.Exec(`INSERT INTO device_profiles (`+deviceProfileColumns+`) VALUES(?,?,?,?,?,?,'',1)
			ON CONFLICT(peer_id) DO NOTHING`, p.PeerID, p.Alias, p.MyDevice, p.Pinned, p.Position, p.ReceiveDirectory)
	} else {
		result, err = tx.Exec(`UPDATE device_profiles SET alias=?,my_device=?,pinned=?,position=?,receive_directory=?,revision=revision+1
			WHERE peer_id=? AND revision=?`, p.Alias, p.MyDevice, p.Pinned, p.Position, p.ReceiveDirectory, p.PeerID, p.Revision)
	}
	if err != nil {
		return DeviceProfile{}, err
	}
	if err = requireMetadataUpdate(result); err != nil {
		return DeviceProfile{}, err
	}
	saved, err := scanDeviceProfile(tx.QueryRow(`SELECT `+deviceProfileColumns+` FROM device_profiles WHERE peer_id=?`, p.PeerID))
	if err != nil {
		return DeviceProfile{}, err
	}
	if err = tx.Commit(); err != nil {
		return DeviceProfile{}, err
	}
	return saved, nil
}

// TouchDevice is called only after a real interaction. Canonical fixed-width
// UTC timestamps allow the SQL guard to reject late older completion events.
func (s *desktopStore) TouchDevice(peerID string, at time.Time) error {
	at = at.UTC()
	if !validMetadataPeerID(peerID) || at.IsZero() || at.Year() < 1 || at.Year() > 9999 {
		return ErrMetadataInvalid
	}
	stamp := at.Format(desktopTimestamp)
	result, err := s.db.Exec(`INSERT INTO device_profiles(peer_id,last_used_at,revision) VALUES(?,?,1)
		ON CONFLICT(peer_id) DO UPDATE SET last_used_at=excluded.last_used_at,revision=device_profiles.revision+1
		WHERE excluded.last_used_at>device_profiles.last_used_at AND device_profiles.revision<?`, peerID, stamp, int64(math.MaxInt64))
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil || count != 0 {
		return err
	}
	current, err := s.DeviceProfile(peerID)
	if err != nil {
		return err
	}
	if current.LastUsedAt < stamp {
		return ErrMetadataConflict // revision exhausted; no new value was committed
	}
	return nil // a duplicate or older real event leaves last-use unchanged
}

func validateDeviceProfile(p *DeviceProfile) error {
	if !validMetadataText(p.Alias, 512) {
		return ErrMetadataInvalid
	}
	p.Alias = strings.TrimSpace(p.Alias)
	if !validMetadataPeerID(p.PeerID) || !validMetadataRevision(p.Revision) || p.Position < 0 ||
		!validMetadataText(p.Alias, 512) || utf8.RuneCountInString(p.Alias) > 128 {
		return ErrMetadataInvalid
	}
	if p.ReceiveDirectory != "" {
		if !validMetadataText(p.ReceiveDirectory, maxDesktopPathBytes) || !filepath.IsAbs(p.ReceiveDirectory) {
			return ErrMetadataInvalid
		}
		p.ReceiveDirectory = filepath.Clean(p.ReceiveDirectory)
		info, err := os.Stat(p.ReceiveDirectory)
		if err != nil {
			return fmt.Errorf("%w: receive directory unavailable: %w", ErrMetadataInvalid, err)
		}
		if !info.IsDir() {
			return fmt.Errorf("%w: receive destination is not a directory", ErrMetadataInvalid)
		}
	}
	return nil
}

func (s *desktopStore) Draft(id string) (SendDraft, error) {
	id, err := normalizeDraftID(id)
	if err != nil {
		return SendDraft{}, err
	}
	return scanSendDraft(s.db.QueryRow(`SELECT id,source_paths,peer_id,revision,updated_at FROM send_drafts WHERE id=?`, id))
}

func (s *desktopStore) Drafts() ([]SendDraft, error) {
	rows, err := s.db.Query(`SELECT id,source_paths,peer_id,revision,updated_at FROM send_drafts ORDER BY updated_at DESC,id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	drafts := make([]SendDraft, 0)
	for rows.Next() {
		d, err := scanSendDraft(rows)
		if err != nil {
			return nil, err
		}
		drafts = append(drafts, d)
	}
	return drafts, rows.Err()
}

func scanSendDraft(row metadataScanner) (SendDraft, error) {
	var d SendDraft
	var paths string
	if err := row.Scan(&d.ID, &paths, &d.PeerID, &d.Revision, &d.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			err = ErrMetadataNotFound
		}
		return SendDraft{}, err
	}
	if len(paths) > maxDraftPathsJSON || json.Unmarshal([]byte(paths), &d.Paths) != nil || d.Paths == nil || len(d.Paths) > maxDraftPaths {
		return SendDraft{}, fmt.Errorf("%w: invalid stored draft paths", ErrMetadataInvalid)
	}
	return d, nil
}

func (s *desktopStore) SaveDraft(d SendDraft, workingDir string) (SendDraft, error) {
	var err error
	d.ID, err = normalizeDraftID(d.ID)
	if err != nil || !validMetadataRevision(d.Revision) || (d.PeerID != "" && !validMetadataPeerID(d.PeerID)) {
		return SendDraft{}, ErrMetadataInvalid
	}
	d.Paths, err = normalizeDraftPaths(d.Paths, workingDir)
	if err != nil {
		return SendDraft{}, err
	}
	paths, err := json.Marshal(d.Paths)
	if err != nil || len(paths) > maxDraftPathsJSON {
		return SendDraft{}, fmt.Errorf("%w: draft paths exceed metadata limit", ErrMetadataInvalid)
	}
	d.UpdatedAt = time.Now().UTC().Format(desktopTimestamp)
	tx, err := s.db.Begin()
	if err != nil {
		return SendDraft{}, err
	}
	defer tx.Rollback()
	var result sql.Result
	if d.Revision == 0 {
		result, err = tx.Exec(`INSERT INTO send_drafts(id,source_paths,peer_id,revision,updated_at) VALUES(?,?,?,1,?)
			ON CONFLICT(id) DO NOTHING`, d.ID, string(paths), d.PeerID, d.UpdatedAt)
	} else {
		result, err = tx.Exec(`UPDATE send_drafts SET source_paths=?,peer_id=?,revision=revision+1,updated_at=? WHERE id=? AND revision=?`, string(paths), d.PeerID, d.UpdatedAt, d.ID, d.Revision)
	}
	if err != nil {
		return SendDraft{}, err
	}
	if err = requireMetadataUpdate(result); err != nil {
		return SendDraft{}, err
	}
	if err = tx.Commit(); err != nil {
		return SendDraft{}, err
	}
	d.Revision++
	return d, nil
}

func requireMetadataUpdate(result sql.Result) error {
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return ErrMetadataConflict
	}
	return nil
}

func normalizeDraftID(id string) (string, error) {
	if id == "" {
		return defaultSendDraftID, nil
	}
	if !validMetadataText(id, 128) || strings.TrimSpace(id) != id {
		return "", ErrMetadataInvalid
	}
	return id, nil
}

func normalizeDraftPaths(paths []string, workingDir string) ([]string, error) {
	if len(paths) > maxDraftPaths {
		return nil, fmt.Errorf("%w: too many draft paths", ErrMetadataInvalid)
	}
	if workingDir != "" && (!filepath.IsAbs(workingDir) || !validMetadataText(workingDir, maxDesktopPathBytes)) {
		return nil, fmt.Errorf("%w: working directory must be absolute", ErrMetadataInvalid)
	}
	result := make([]string, 0, len(paths))
	seen := make(map[string]bool, len(paths))
	encodedBytes := 2 // JSON array delimiters
	for _, path := range paths {
		if path == "" || !validMetadataText(path, maxDesktopPathBytes) {
			return nil, fmt.Errorf("%w: invalid draft path", ErrMetadataInvalid)
		}
		if !filepath.IsAbs(path) {
			if workingDir == "" || filepath.VolumeName(path) != "" || (runtime.GOOS == "windows" && strings.ContainsAny(path[:1], `/\`)) {
				return nil, fmt.Errorf("%w: relative path requires its working directory", ErrMetadataInvalid)
			}
			path = filepath.Join(workingDir, path)
		}
		path = filepath.Clean(path)
		if len(path) > maxDesktopPathBytes {
			return nil, ErrMetadataInvalid
		}
		key := path
		if runtime.GOOS == "windows" {
			key = strings.ToLower(path)
		}
		if !seen[key] {
			encoded, err := json.Marshal(path)
			if err != nil {
				return nil, ErrMetadataInvalid
			}
			encodedBytes += len(encoded)
			if len(result) != 0 {
				encodedBytes++
			}
			if encodedBytes > maxDraftPathsJSON {
				return nil, fmt.Errorf("%w: draft paths exceed metadata limit", ErrMetadataInvalid)
			}
			seen[key] = true
			result = append(result, path)
		}
	}
	return result, nil
}

func validMetadataPeerID(id string) bool {
	if len(id) != 64 || id != strings.ToLower(id) {
		return false
	}
	_, err := hex.DecodeString(id)
	return err == nil
}

func validMetadataRevision(revision uint64) bool { return revision < math.MaxInt64 }

func validMetadataText(text string, maxBytes int) bool {
	if len(text) > maxBytes || !utf8.ValidString(text) {
		return false
	}
	for _, r := range text {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}
