package app

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/Wen5555/LinkSend/internal/transfer"
	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

const maxInboxPage = 100

var (
	ErrInboxQuery           = errors.New("INVALID_INBOX_QUERY")
	ErrInboxProtected       = errors.New("INBOX_RECORD_IN_USE")
	ErrInboxFileMissing     = errors.New("INBOX_FILE_MOVED_OR_DELETED")
	ErrInboxFileChanged     = errors.New("INBOX_FILE_CHANGED")
	ErrInboxFileUnavailable = errors.New("INBOX_FILE_NOT_AVAILABLE")
	ErrInboxOwnership       = errors.New("INBOX_STAGING_OWNERSHIP_MISMATCH")
)

type InboxQuery struct {
	PeerID    string   `json:"peer_id"`
	Direction string   `json:"direction"`
	States    []string `json:"states"`
	After     string   `json:"after"`
	Before    string   `json:"before"`
	Search    string   `json:"search"`
	Cursor    string   `json:"cursor"`
	Limit     int      `json:"limit"`
}

// InboxItem is a reduced historical view, never a recovery record or an export
// of absolute filesystem locations and private connection diagnostics.
type InboxItem struct {
	TaskID             string `json:"task_id"`
	PeerID             string `json:"peer_id"`
	Direction          string `json:"direction"`
	State              string `json:"state"`
	Summary            string `json:"summary"`
	FileCount          int    `json:"file_count"`
	VerifiedBytes      int64  `json:"verified_bytes"`
	CommittedBytes     int64  `json:"committed_bytes"`
	BilateralConfirmed bool   `json:"bilateral_confirmed"`
	StartedAt          string `json:"started_at"`
	EndedAt            string `json:"ended_at"`
	Revision           uint64 `json:"revision"`
	CanResend          bool   `json:"can_resend"`
	CanForget          bool   `json:"can_forget"`
}

type InboxPage struct {
	Items      []InboxItem `json:"items"`
	NextCursor string      `json:"next_cursor"`
}

type InboxFile struct {
	FileID   uint32 `json:"file_id"`
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	Size     int64  `json:"size"`
	Selected bool   `json:"selected"`
}

type InboxFilesPage struct {
	Files      []InboxFile `json:"files"`
	NextCursor string      `json:"next_cursor"`
	Available  bool        `json:"available"`
}

type inboxCursor struct {
	Version int    `json:"v"`
	Filter  string `json:"filter"`
	Started int64  `json:"started"`
	ID      string `json:"id"`
}

func inboxFold(value string) string { return cases.Fold().String(norm.NFC.String(value)) }
func inboxStarted(value string) int64 {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil || parsed.Year() < 1970 || parsed.Year() > 2200 {
		return 0
	}
	return parsed.UnixNano()
}
func inboxText(value string, limit int) string {
	value = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, value)
	r := []rune(value)
	if len(r) > limit {
		return string(r[:limit]) + "…"
	}
	return value
}
func validInboxID(id string) bool {
	return id != "" && len(id) <= 128 && utf8.ValidString(id) && !strings.ContainsFunc(id, unicode.IsControl)
}

func migrateInboxMetadata(tx *sql.Tx) error {
	statements := []string{
		`ALTER TABLE tasks ADD COLUMN inbox_peer TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE tasks ADD COLUMN inbox_direction TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE tasks ADD COLUMN inbox_state TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE tasks ADD COLUMN inbox_started INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE tasks ADD COLUMN inbox_summary TEXT NOT NULL DEFAULT ''`,
		`CREATE INDEX inbox_task_date ON tasks(inbox_started DESC,id DESC)`,
		`CREATE INDEX inbox_task_peer ON tasks(inbox_peer,inbox_started DESC,id DESC)`,
		`CREATE INDEX inbox_task_direction ON tasks(inbox_direction,inbox_started DESC,id DESC)`,
		`CREATE INDEX inbox_task_state ON tasks(inbox_state,inbox_started DESC,id DESC)`,
		`CREATE INDEX inbox_queue_task ON send_queue(task_id,state)`,
		`CREATE TABLE inbox_tombstones (task_id TEXT PRIMARY KEY,revision INTEGER NOT NULL,forgotten_at TEXT NOT NULL)`,
		`CREATE TABLE inbox_manifests (task_id TEXT PRIMARY KEY,manifest_digest TEXT NOT NULL,manifest BLOB NOT NULL)`,
		`CREATE TABLE inbox_files (id INTEGER PRIMARY KEY,task_id TEXT NOT NULL,file_id INTEGER NOT NULL,path TEXT NOT NULL,name_fold TEXT NOT NULL,kind TEXT NOT NULL,size INTEGER NOT NULL,digest TEXT NOT NULL,saved_path TEXT NOT NULL DEFAULT '',UNIQUE(task_id,file_id))`,
		`CREATE INDEX inbox_files_task ON inbox_files(task_id,file_id)`,
		`CREATE VIRTUAL TABLE inbox_file_search USING fts5(name_fold,content='inbox_files',content_rowid='id',tokenize='trigram')`,
		`CREATE TRIGGER inbox_files_insert AFTER INSERT ON inbox_files BEGIN INSERT INTO inbox_file_search(rowid,name_fold) VALUES(new.id,new.name_fold); END`,
		`CREATE TRIGGER inbox_files_delete AFTER DELETE ON inbox_files BEGIN INSERT INTO inbox_file_search(inbox_file_search,rowid,name_fold) VALUES('delete',old.id,old.name_fold); END`,
		`CREATE TABLE inbox_staging (task_id TEXT PRIMARY KEY,transfer_id TEXT NOT NULL,peer_id TEXT NOT NULL,root_path TEXT NOT NULL,stage_identity TEXT NOT NULL,manifest_digest TEXT NOT NULL,parts BLOB NOT NULL,registered_at TEXT NOT NULL)`,
	}
	for _, statement := range statements {
		if _, err := tx.Exec(statement); err != nil {
			return fmt.Errorf("INBOX_MIGRATION_FAILED: %w", err)
		}
	}
	// Backfill bounded batches. Malformed snapshots remain for the existing
	// startup quarantine path, rather than poisoning the whole migration.
	last := ""
	for {
		rows, err := tx.Query(`SELECT id,snapshot FROM tasks WHERE id>? ORDER BY id LIMIT 256`, last)
		if err != nil {
			return err
		}
		type item struct {
			id   string
			data []byte
		}
		batch := make([]item, 0, 256)
		for rows.Next() {
			var row item
			if err = rows.Scan(&row.id, &row.data); err != nil {
				_ = rows.Close()
				return err
			}
			batch = append(batch, row)
		}
		err = rows.Err()
		_ = rows.Close()
		if err != nil {
			return err
		}
		if len(batch) == 0 {
			break
		}
		for _, row := range batch {
			last = row.id
			var snap TaskSnapshot
			if len(row.data) > 1<<20 || json.Unmarshal(row.data, &snap) != nil || snap.ID != row.id {
				continue
			}
			if _, err = tx.Exec(`UPDATE tasks SET inbox_peer=?,inbox_direction=?,inbox_state=?,inbox_started=?,inbox_summary=? WHERE id=?`, snap.PeerID, snap.Direction, snap.State, inboxStarted(snap.StartedAt), inboxFold(snap.SourceSummary+" "+snap.ManifestSummary), row.id); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Service) Inbox(ctx context.Context, query InboxQuery) (InboxPage, error) {
	done, err := s.beginWorkspaceWork()
	if err != nil {
		return InboxPage{}, err
	}
	defer done()
	if query.Limit == 0 {
		query.Limit = 50
	}
	if query.Limit < 1 || query.Limit > maxInboxPage || len(query.States) > 16 || len(query.Search) > 256 || !utf8.ValidString(query.Search) || strings.ContainsFunc(query.Search, unicode.IsControl) {
		return InboxPage{}, ErrInboxQuery
	}
	if query.PeerID != "" && !validMetadataPeerID(query.PeerID) {
		return InboxPage{}, ErrInboxQuery
	}
	if query.Direction != "" && query.Direction != "send" && query.Direction != "receive" {
		return InboxPage{}, ErrInboxQuery
	}
	query.Search = inboxFold(strings.TrimSpace(query.Search))
	query.States = append([]string(nil), query.States...)
	sort.Strings(query.States)
	where := []string{"1=1"}
	args := make([]any, 0)
	if query.PeerID != "" {
		where = append(where, "inbox_peer=?")
		args = append(args, query.PeerID)
	}
	if query.Direction != "" {
		where = append(where, "inbox_direction=?")
		args = append(args, query.Direction)
	}
	if len(query.States) > 0 {
		marks := make([]string, len(query.States))
		for i, state := range query.States {
			if state == "" || len(state) > 40 || strings.ContainsFunc(state, func(r rune) bool { return (r < 'a' || r > 'z') && r != '_' }) {
				return InboxPage{}, ErrInboxQuery
			}
			marks[i] = "?"
			args = append(args, state)
		}
		where = append(where, "inbox_state IN ("+strings.Join(marks, ",")+")")
	}
	for _, bound := range []struct{ value, operator string }{{query.After, ">="}, {query.Before, "<"}} {
		if bound.value != "" {
			stamp := inboxStarted(bound.value)
			if stamp == 0 {
				return InboxPage{}, ErrInboxQuery
			}
			where = append(where, "inbox_started"+bound.operator+"?")
			args = append(args, stamp)
		}
	}
	if query.After != "" && query.Before != "" && inboxStarted(query.After) >= inboxStarted(query.Before) {
		return InboxPage{}, ErrInboxQuery
	}
	if query.Search != "" {
		if utf8.RuneCountInString(query.Search) >= 3 {
			where = append(where, `(id IN (SELECT f.task_id FROM inbox_file_search JOIN inbox_files f ON f.id=inbox_file_search.rowid WHERE inbox_file_search MATCH ?) OR (NOT EXISTS(SELECT 1 FROM inbox_manifests m WHERE m.task_id=tasks.id) AND instr(inbox_summary,?)>0))`)
			args = append(args, `"`+strings.ReplaceAll(query.Search, `"`, `""`)+`"`, query.Search)
		} else {
			where = append(where, `(EXISTS(SELECT 1 FROM inbox_files f WHERE f.task_id=tasks.id AND instr(f.name_fold,?)>0) OR (NOT EXISTS(SELECT 1 FROM inbox_manifests m WHERE m.task_id=tasks.id) AND instr(inbox_summary,?)>0))`)
			args = append(args, query.Search, query.Search)
		}
	}
	filter := query
	filter.Cursor = ""
	filter.Limit = 0
	encoded, _ := json.Marshal(filter)
	digest := sha256.Sum256(encoded)
	filterHash := hex.EncodeToString(digest[:])
	if query.Cursor != "" {
		if len(query.Cursor) > 1024 {
			return InboxPage{}, ErrInboxQuery
		}
		data, e := base64.RawURLEncoding.DecodeString(query.Cursor)
		var cursor inboxCursor
		if e != nil || json.Unmarshal(data, &cursor) != nil || cursor.Version != 1 || cursor.Filter != filterHash || !validInboxID(cursor.ID) {
			return InboxPage{}, ErrInboxQuery
		}
		where = append(where, "(inbox_started<? OR (inbox_started=? AND id<?))")
		args = append(args, cursor.Started, cursor.Started, cursor.ID)
	}
	args = append(args, query.Limit+1)
	rows, err := s.store.db.QueryContext(ctx, `SELECT snapshot,inbox_started,recovery,EXISTS(SELECT 1 FROM send_queue q WHERE q.task_id=tasks.id AND q.state NOT IN ('completed','cancelled','expired')) FROM tasks WHERE `+strings.Join(where, " AND ")+` ORDER BY inbox_started DESC,id DESC LIMIT ?`, args...)
	if err != nil {
		return InboxPage{}, err
	}
	defer rows.Close()
	page := InboxPage{Items: make([]InboxItem, 0, query.Limit)}
	var last inboxCursor
	for rows.Next() {
		var data []byte
		var recoveryData []byte
		var queueReferenced bool
		var started int64
		if err = rows.Scan(&data, &started, &recoveryData, &queueReferenced); err != nil {
			return InboxPage{}, err
		}
		if len(page.Items) == query.Limit {
			encoded, _ := json.Marshal(last)
			page.NextCursor = base64.RawURLEncoding.EncodeToString(encoded)
			break
		}
		var snap TaskSnapshot
		var recovery taskRecovery
		if len(data) > 1<<20 || json.Unmarshal(data, &snap) != nil || len(recoveryData) > 2<<20 || (len(recoveryData) > 0 && json.Unmarshal(recoveryData, &recovery) != nil) {
			return InboxPage{}, errors.New("INVALID_HISTORY_SNAPSHOT")
		}
		summary := snap.ManifestSummary
		if summary == "" {
			summary = snap.SourceSummary
		}
		page.Items = append(page.Items, InboxItem{TaskID: snap.ID, PeerID: snap.PeerID, Direction: snap.Direction, State: snap.State, Summary: inboxText(summary, 240), FileCount: snap.FileCount, VerifiedBytes: snap.VerifiedBytes, CommittedBytes: snap.CommittedBytes, BilateralConfirmed: snap.BilateralConfirmed, StartedAt: snap.StartedAt, EndedAt: snap.EndedAt, Revision: snap.Revision, CanResend: snap.Direction == "send" && isTerminal(snap.State) && recoveryUsable(recovery), CanForget: !inboxTaskProtected(snap, recovery) && !queueReferenced})
		last = inboxCursor{Version: 1, Filter: filterHash, Started: started, ID: snap.ID}
	}
	return page, rows.Err()
}

func (s *Service) InboxFiles(ctx context.Context, taskID, cursor string, limit int) (InboxFilesPage, error) {
	done, err := s.beginWorkspaceWork()
	if err != nil {
		return InboxFilesPage{}, err
	}
	defer done()
	if !validInboxID(taskID) || limit < 0 || limit > maxInboxPage {
		return InboxFilesPage{}, ErrInboxQuery
	}
	if limit == 0 {
		limit = 50
	}
	last := int64(-1)
	if cursor != "" {
		if len(cursor) > 512 {
			return InboxFilesPage{}, ErrInboxQuery
		}
		data, e := base64.RawURLEncoding.DecodeString(cursor)
		var saved struct {
			Task string
			Last int64
		}
		if len(cursor) > 512 || e != nil || json.Unmarshal(data, &saved) != nil || saved.Task != taskID || saved.Last < 0 || saved.Last >= transfer.MaxEntries {
			return InboxFilesPage{}, ErrInboxQuery
		}
		last = saved.Last
	}
	var count int
	if err = s.store.db.QueryRowContext(ctx, `SELECT count(*) FROM inbox_manifests WHERE task_id=?`, taskID).Scan(&count); err != nil {
		return InboxFilesPage{}, err
	}
	page := InboxFilesPage{Files: make([]InboxFile, 0), Available: count != 0}
	if count == 0 {
		return page, nil
	}
	rows, err := s.store.db.QueryContext(ctx, `SELECT file_id,path,kind,size,saved_path FROM inbox_files WHERE task_id=? AND file_id>? ORDER BY file_id LIMIT ?`, taskID, last, limit+1)
	if err != nil {
		return page, err
	}
	defer rows.Close()
	for rows.Next() {
		var file InboxFile
		var source, saved string
		if err = rows.Scan(&file.FileID, &source, &file.Kind, &file.Size, &saved); err != nil {
			return page, err
		}
		if len(page.Files) == limit {
			data, _ := json.Marshal(struct {
				Task string
				Last int64
			}{taskID, int64(page.Files[len(page.Files)-1].FileID)})
			page.NextCursor = base64.RawURLEncoding.EncodeToString(data)
			break
		}
		file.Name = source
		if saved != "" {
			file.Name = saved
		}
		file.Selected = saved != ""
		page.Files = append(page.Files, file)
	}
	return page, rows.Err()
}

// IndexInboxManifest stores all offered names once, including names beyond the
// compact first-three summary. Original manifests remain local Go metadata.
func (s *Service) IndexInboxManifest(ctx context.Context, taskID string, m transfer.Manifest) error {
	done, err := s.beginWorkspaceWork()
	if err != nil {
		return err
	}
	defer done()
	if !validInboxID(taskID) {
		return ErrInboxQuery
	}
	if err = m.Validate(); err != nil {
		return err
	}
	tx, err := s.store.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = indexInboxManifest(tx, taskID, m); err != nil {
		return err
	}
	return tx.Commit()
}

func indexInboxManifest(tx *sql.Tx, taskID string, m transfer.Manifest) error {
	var data []byte
	if err := tx.QueryRow(`SELECT snapshot FROM tasks WHERE id=?`, taskID).Scan(&data); err != nil {
		return err
	}
	var snap TaskSnapshot
	if json.Unmarshal(data, &snap) != nil || snap.ID != taskID || snap.ManifestDigest != m.Digest() || snap.TransferID != m.TransferID {
		return transfer.ErrPlanMismatch
	}
	var existing string
	err := tx.QueryRow(`SELECT manifest_digest FROM inbox_manifests WHERE task_id=?`, taskID).Scan(&existing)
	if err == nil {
		if existing != m.Digest() {
			return transfer.ErrPlanMismatch
		}
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	manifest, _ := json.Marshal(m)
	if _, err = tx.Exec(`INSERT INTO inbox_manifests(task_id,manifest_digest,manifest) VALUES(?,?,?)`, taskID, m.Digest(), manifest); err != nil {
		return err
	}
	statement, err := tx.Prepare(`INSERT INTO inbox_files(task_id,file_id,path,name_fold,kind,size,digest,saved_path) VALUES(?,?,?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer statement.Close()
	for _, file := range m.Files {
		saved := ""
		if snap.Direction == "send" {
			saved = file.Path
		}
		if _, err = statement.Exec(taskID, file.ID, file.Path, inboxFold(path.Base(file.Path)), file.Type, file.Size, file.Hash, saved); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) workspaceTasks() []TaskSnapshot {
	items := s.Tasks()
	sort.Slice(items, func(i, j int) bool {
		a, b := !isTerminal(items[i].State), !isTerminal(items[j].State)
		if a != b {
			return a
		}
		return items[i].StartedAt > items[j].StartedAt
	})
	if len(items) > 200 {
		items = items[:200]
	}
	return items
}
