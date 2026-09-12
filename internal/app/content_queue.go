package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"github.com/Wen5555/LinkSend/internal/content"
)

func (s *Service) decorateContentQueue(items []QueueItem) error {
	if len(items) == 0 {
		return nil
	}
	args := make([]any, 0, len(items))
	byID := make(map[string]int, len(items))
	for i := range items {
		args = append(args, items[i].ID)
		byID[items[i].ID] = i
	}
	rows, err := s.store.db.Query(`SELECT queue_id,snapshot FROM content_queue WHERE queue_id IN (`+strings.TrimRight(strings.Repeat("?,", len(items)), ",")+`)`, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var data []byte
		if err = rows.Scan(&id, &data); err != nil {
			return err
		}
		if len(data) > 4096 {
			return ErrMetadataInvalid
		}
		var snapshot content.Snapshot
		if err = json.Unmarshal(data, &snapshot); err != nil {
			return err
		}
		i, ok := byID[id]
		if ok {
			items[i].Content = &snapshot
		}
	}
	return rows.Err()
}

func (s *Service) loadQueueContent(ctx context.Context, id string) (*content.Snapshot, bool, error) {
	var data []byte
	var fallback bool
	err := s.store.db.QueryRowContext(ctx, `SELECT snapshot,allow_file_fallback FROM content_queue WHERE queue_id=?`, id).Scan(&data, &fallback)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if len(data) > 4096 {
		return nil, false, ErrMetadataInvalid
	}
	var snapshot content.Snapshot
	if err = json.Unmarshal(data, &snapshot); err != nil {
		return nil, false, err
	}
	store, err := s.contentStoreLocked()
	if err != nil {
		return nil, false, err
	}
	actual, err := store.Metadata(snapshot.ID)
	if err != nil {
		return nil, false, err
	}
	if actual != snapshot {
		return nil, false, content.ErrChanged
	}
	if _, err = store.OwnedPath(ctx, snapshot.ID); err != nil {
		return nil, false, err
	}
	return &snapshot, fallback, nil
}

func (s *Service) registerOutgoingContentTask(ctx context.Context, id string, snapshot *content.Snapshot, allowFallback bool) error {
	if snapshot == nil {
		return nil
	}
	data, _ := json.Marshal(snapshot)
	_, err := s.store.db.ExecContext(ctx, `INSERT INTO content_tasks(task_id,direction,snapshot_id,snapshot,mode,allow_file_fallback)VALUES(?,'send',?,?,'pending',?)`, id, snapshot.ID, data, allowFallback)
	return err
}

func (s *Service) resendContentInbox(ctx context.Context, request ResendInboxRequest, requestID string, record contentTaskRecord, peer string) (QueueItem, error) {
	if record.direction != "send" || record.snapshot == nil {
		return QueueItem{}, errors.New("CONTENT_SNAPSHOT_UNAVAILABLE")
	}
	draft, err := s.createContentDraft(ctx, requestID, "resend-content:"+record.snapshot.Digest, func(store *content.Store, owner string) (content.Snapshot, error) {
		metadata, err := store.Metadata(record.snapshotID)
		if err != nil {
			return content.Snapshot{}, errors.New("CONTENT_SNAPSHOT_UNAVAILABLE")
		}
		if metadata != *record.snapshot {
			return content.Snapshot{}, content.ErrChanged
		}
		if _, err = store.OwnedPath(ctx, record.snapshotID); err != nil {
			return content.Snapshot{}, err
		}
		if err = store.Retain(record.snapshotID, owner); err != nil {
			return content.Snapshot{}, err
		}
		return metadata, nil
	})
	if err != nil {
		return QueueItem{}, err
	}
	return s.EnqueueContent(ctx, EnqueueContentRequest{RequestID: requestID, DraftID: draft.ID, DraftRevision: draft.Revision, PeerID: peer, WaitForPeer: request.WaitForPeer, AllowFileFallback: record.allowFallback})
}
