package app

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"image"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Wen5555/LinkSend/internal/content"
)

type contentManager struct {
	mu      sync.Mutex
	storeMu sync.Mutex
	store   *content.Store
	enabled atomic.Bool
}

type ContentTextRequest struct {
	RequestID string       `json:"request_id"`
	Kind      content.Kind `json:"kind"`
	Text      string       `json:"text"`
}
type ContentDraft struct {
	ID        string           `json:"id"`
	Snapshot  content.Snapshot `json:"snapshot"`
	Revision  uint64           `json:"revision"`
	State     string           `json:"state"`
	CreatedAt string           `json:"created_at"`
}
type EnqueueContentRequest struct {
	RequestID         string `json:"request_id"`
	DraftID           string `json:"draft_id"`
	DraftRevision     uint64 `json:"draft_revision"`
	PeerID            string `json:"peer_id"`
	AllowFileFallback bool   `json:"allow_file_fallback"`
	WaitForPeer       bool   `json:"wait_for_peer"`
	ExpiresAt         string `json:"expires_at"`
}
type ContentSettings struct {
	RetainSentSnapshots bool `json:"retain_sent_snapshots"`
}

func migrateContentMetadata(tx *sql.Tx) error {
	for _, statement := range []string{
		`CREATE TABLE content_drafts(id TEXT PRIMARY KEY,request_digest TEXT NOT NULL,snapshot_id TEXT NOT NULL,snapshot BLOB NOT NULL CHECK(json_valid(snapshot)),revision INTEGER NOT NULL CHECK(revision>0),state TEXT NOT NULL CHECK(state IN ('draft','queued','discarded')),created_at TEXT NOT NULL)`,
		`CREATE INDEX content_drafts_snapshot ON content_drafts(snapshot_id,state)`,
		`CREATE TABLE content_queue(queue_id TEXT PRIMARY KEY,draft_id TEXT NOT NULL,snapshot_id TEXT NOT NULL,snapshot BLOB NOT NULL CHECK(json_valid(snapshot)),allow_file_fallback INTEGER NOT NULL CHECK(allow_file_fallback IN (0,1)))`,
		`CREATE INDEX content_queue_snapshot ON content_queue(snapshot_id)`,
		`CREATE TABLE content_tasks(task_id TEXT PRIMARY KEY,direction TEXT NOT NULL CHECK(direction IN ('send','receive')),snapshot_id TEXT NOT NULL DEFAULT '',snapshot BLOB,descriptor BLOB,binding TEXT NOT NULL DEFAULT '',mode TEXT NOT NULL CHECK(mode IN ('pending','native','file')),allow_file_fallback INTEGER NOT NULL DEFAULT 0 CHECK(allow_file_fallback IN (0,1)))`,
		`CREATE INDEX content_tasks_snapshot ON content_tasks(snapshot_id)`,
	} {
		if _, err := tx.Exec(statement); err != nil {
			return err
		}
	}
	return nil
}

func contentRequestKey(requestID string) (string, error) {
	if !validInboxID(requestID) {
		return "", ErrMetadataInvalid
	}
	sum := sha256.Sum256([]byte("LinkSend/content-draft/" + requestID))
	return hex.EncodeToString(sum[:]), nil
}

func (s *Service) contentStoreLocked() (*content.Store, error) {
	s.content.storeMu.Lock()
	defer s.content.storeMu.Unlock()
	if s.content.store == nil {
		store, err := content.OpenStore(s.cfg.DataDir)
		if err != nil {
			return nil, err
		}
		s.content.store = store
	}
	return s.content.store, nil
}

func scanContentDraft(row interface{ Scan(...any) error }) (ContentDraft, string, error) {
	var draft ContentDraft
	var metadata []byte
	var requestDigest string
	err := row.Scan(&draft.ID, &requestDigest, &metadata, &draft.Revision, &draft.State, &draft.CreatedAt)
	if err == nil {
		if len(metadata) > 4096 {
			return draft, "", ErrMetadataInvalid
		}
		err = json.Unmarshal(metadata, &draft.Snapshot)
	}
	return draft, requestDigest, err
}

const contentDraftColumns = `id,request_digest,snapshot,revision,state,created_at`

func (s *Service) CreateContentText(ctx context.Context, request ContentTextRequest) (ContentDraft, error) {
	if request.Kind != content.Text && request.Kind != content.URL {
		return ContentDraft{}, content.ErrInvalidText
	}
	if len(request.Text) > content.MaxTextBytes {
		return ContentDraft{}, content.ErrLimit
	}
	digest := sha256.Sum256([]byte(string(request.Kind) + "\x00" + request.Text))
	return s.createContentDraft(ctx, request.RequestID, hex.EncodeToString(digest[:]), func(store *content.Store, owner string) (content.Snapshot, error) {
		return store.CreateText(ctx, request.Kind, request.Text, owner)
	})
}

// Capture is a Go-only callback. The desktop adapter obtains an image directly
// from the native clipboard after an explicit action; no image enters JS IPC.
func (s *Service) CreateClipboardImage(ctx context.Context, requestID string, capture func(context.Context) (image.Image, error)) (ContentDraft, error) {
	if capture == nil {
		return ContentDraft{}, ErrMetadataInvalid
	}
	return s.createContentDraft(ctx, requestID, "native-clipboard-image-v1", func(store *content.Store, owner string) (content.Snapshot, error) {
		img, err := capture(ctx)
		if err != nil {
			return content.Snapshot{}, err
		}
		return store.CreateImageFromImage(ctx, img, owner)
	})
}

func (s *Service) createContentDraft(ctx context.Context, requestID, digest string, create func(*content.Store, string) (content.Snapshot, error)) (ContentDraft, error) {
	done, err := s.beginWorkspaceWork()
	if err != nil {
		return ContentDraft{}, err
	}
	defer done()
	id, err := contentRequestKey(requestID)
	if err != nil {
		return ContentDraft{}, err
	}
	s.content.mu.Lock()
	defer s.content.mu.Unlock()
	previous, savedDigest, err := scanContentDraft(s.store.db.QueryRowContext(ctx, `SELECT `+contentDraftColumns+` FROM content_drafts WHERE id=?`, id))
	if err == nil {
		if savedDigest != digest {
			return ContentDraft{}, errors.New("IDEMPOTENCY_CONFLICT")
		}
		return previous, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return ContentDraft{}, err
	}
	var count int
	if err = s.store.db.QueryRowContext(ctx, `SELECT count(*) FROM content_drafts WHERE state='draft'`).Scan(&count); err != nil {
		return ContentDraft{}, err
	}
	if count >= 100 {
		return ContentDraft{}, errors.New("CONTENT_DRAFT_LIMIT")
	}
	store, err := s.contentStoreLocked()
	if err != nil {
		return ContentDraft{}, err
	}
	snapshot, err := create(store, "app-content:"+id)
	if err != nil {
		return ContentDraft{}, err
	}
	draft := ContentDraft{ID: id, Snapshot: snapshot, Revision: 1, State: "draft", CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	data, _ := json.Marshal(snapshot)
	_, err = s.store.db.ExecContext(ctx, `INSERT INTO content_drafts(id,request_digest,snapshot_id,snapshot,revision,state,created_at) VALUES(?,?,?,?,1,'draft',?)`, id, digest, snapshot.ID, data, draft.CreatedAt)
	if err != nil {
		// An orphan may remain after a crash, but it cannot lose a live reference.
		_ = store.Release(snapshot.ID, "app-content:"+id)
		return ContentDraft{}, err
	}
	s.notifyChange()
	return draft, nil
}

func (s *Service) ContentDrafts(ctx context.Context) ([]ContentDraft, error) {
	done, err := s.beginWorkspaceWork()
	if err != nil {
		return nil, err
	}
	defer done()
	rows, err := s.store.db.QueryContext(ctx, `SELECT `+contentDraftColumns+` FROM content_drafts WHERE state='draft' ORDER BY created_at,id LIMIT 100`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	drafts := make([]ContentDraft, 0)
	for rows.Next() {
		draft, _, e := scanContentDraft(rows)
		if e != nil {
			return nil, e
		}
		drafts = append(drafts, draft)
	}
	return drafts, rows.Err()
}

func (s *Service) DiscardContentDraft(ctx context.Context, id string, revision uint64) error {
	if !validInboxID(id) || revision == 0 {
		return ErrMetadataInvalid
	}
	done, err := s.beginWorkspaceWork()
	if err != nil {
		return err
	}
	defer done()
	s.content.mu.Lock()
	defer s.content.mu.Unlock()
	result, err := s.store.db.ExecContext(ctx, `UPDATE content_drafts SET state='discarded',revision=revision+1 WHERE id=? AND revision=? AND state='draft'`, id, revision)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return ErrMetadataConflict
	}
	s.notifyChange()
	return nil
}

type contentQueueIntent struct {
	draft         ContentDraft
	allowFallback bool
}

func (s *Service) EnqueueContent(ctx context.Context, request EnqueueContentRequest) (QueueItem, error) {
	if !validInboxID(request.RequestID) || !validInboxID(request.DraftID) || request.DraftRevision == 0 {
		return QueueItem{}, ErrMetadataInvalid
	}
	done, err := s.beginWorkspaceWork()
	if err != nil {
		return QueueItem{}, err
	}
	defer done()
	s.content.mu.Lock()
	defer s.content.mu.Unlock()
	draft, _, err := scanContentDraft(s.store.db.QueryRowContext(ctx, `SELECT `+contentDraftColumns+` FROM content_drafts WHERE id=?`, request.DraftID))
	if err != nil {
		return QueueItem{}, err
	}
	if draft.State != "draft" {
		item, e := scanQueue(s.store.db.QueryRowContext(ctx, `SELECT `+queueColumns+` FROM send_queue WHERE request_id=?`, request.RequestID))
		if e == nil {
			var prior string
			var fallback bool
			e = s.store.db.QueryRowContext(ctx, `SELECT draft_id,allow_file_fallback FROM content_queue WHERE queue_id=?`, item.ID).Scan(&prior, &fallback)
			if e == nil && prior == request.DraftID && fallback == request.AllowFileFallback && item.PeerID == request.PeerID && item.WaitForPeer == request.WaitForPeer {
				return item.QueueItem, nil
			}
		}
		return QueueItem{}, ErrMetadataConflict
	}
	if draft.Revision != request.DraftRevision {
		return QueueItem{}, ErrMetadataConflict
	}
	store, err := s.contentStoreLocked()
	if err != nil {
		return QueueItem{}, err
	}
	path, err := store.OwnedPath(ctx, draft.Snapshot.ID)
	if err != nil {
		return QueueItem{}, err
	}
	item, err := s.Enqueue(EnqueueRequest{RequestID: request.RequestID, PeerID: request.PeerID, Paths: []string{path}, WaitForPeer: request.WaitForPeer, ExpiresAt: request.ExpiresAt, content: &contentQueueIntent{draft, request.AllowFileFallback}})
	if err == nil {
		snapshot := draft.Snapshot
		item.Content = &snapshot
	}
	return item, err
}

func persistContentQueue(tx *sql.Tx, item QueueItem, intent *contentQueueIntent) error {
	var existing string
	err := tx.QueryRow(`SELECT draft_id FROM content_queue WHERE queue_id=?`, item.ID).Scan(&existing)
	if err == nil {
		if intent == nil || existing != intent.draft.ID {
			return errors.New("IDEMPOTENCY_CONFLICT")
		}
		var fallback bool
		if err = tx.QueryRow(`SELECT allow_file_fallback FROM content_queue WHERE queue_id=?`, item.ID).Scan(&fallback); err != nil {
			return err
		}
		if fallback != intent.allowFallback {
			return errors.New("IDEMPOTENCY_CONFLICT")
		}
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if intent == nil {
		return nil
	}
	data, _ := json.Marshal(intent.draft.Snapshot)
	if _, err = tx.Exec(`INSERT INTO content_queue(queue_id,draft_id,snapshot_id,snapshot,allow_file_fallback)VALUES(?,?,?,?,?)`, item.ID, intent.draft.ID, intent.draft.Snapshot.ID, data, intent.allowFallback); err != nil {
		return err
	}
	result, err := tx.Exec(`UPDATE content_drafts SET state='queued',revision=revision+1 WHERE id=? AND revision=? AND state='draft'`, intent.draft.ID, intent.draft.Revision)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return ErrMetadataConflict
	}
	return nil
}

func (s *Service) ContentSettings() (ContentSettings, error) {
	done, err := s.beginWorkspaceWork()
	if err != nil {
		return ContentSettings{}, err
	}
	defer done()
	var value string
	err = s.store.db.QueryRow(`SELECT value FROM metadata WHERE key='retain_sent_content'`).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return ContentSettings{}, nil
	}
	return ContentSettings{value == "1"}, err
}
func (s *Service) SetContentSettings(settings ContentSettings) error {
	done, err := s.beginWorkspaceWork()
	if err != nil {
		return err
	}
	defer done()
	s.content.mu.Lock()
	defer s.content.mu.Unlock()
	value := "0"
	if settings.RetainSentSnapshots {
		value = "1"
	}
	_, err = s.store.db.Exec(`INSERT INTO metadata(key,value)VALUES('retain_sent_content',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, value)
	if err == nil {
		s.notifyChange()
	}
	return err
}

// Called by the desktop host only after native actions have been wired. This
// does not request OS permissions or inspect the clipboard.
func (s *Service) EnableNativeContentActions() { s.content.enabled.Store(true) }
