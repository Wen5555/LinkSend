package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
)

type ContentCleanupResult struct {
	Removed   int `json:"removed"`
	Protected int `json:"protected"`
}

func (s *Service) contentObjectPath(id string) string {
	return filepath.Join(s.cfg.DataDir, "content-snapshots", id)
}

// CleanupContentSnapshots never removes received user files. The content store
// verifies object ownership, persistent OS file identity, hash and allowed names
// again before unlinking. Reference failures stop cleanup conservatively.
func (s *Service) CleanupContentSnapshots(ctx context.Context) (ContentCleanupResult, error) {
	done, err := s.beginWorkspaceWork()
	if err != nil {
		return ContentCleanupResult{}, err
	}
	defer done()
	s.content.mu.Lock()
	defer s.content.mu.Unlock()
	s.queue.mu.Lock()
	defer s.queue.mu.Unlock()
	s.operationMu.Lock()
	defer s.operationMu.Unlock()
	store, err := s.contentStoreLocked()
	if err != nil {
		return ContentCleanupResult{}, err
	}
	objects, err := store.OwnedObjects(ctx)
	if err != nil {
		return ContentCleanupResult{}, err
	}
	var retention string
	err = s.store.db.QueryRowContext(ctx, `SELECT value FROM metadata WHERE key='retain_sent_content'`).Scan(&retention)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return ContentCleanupResult{}, err
	}
	result := ContentCleanupResult{}
	for _, object := range objects {
		protected, err := s.contentSnapshotProtected(ctx, object.Snapshot.ID, retention == "1")
		if err != nil {
			return result, err
		}
		if protected {
			result.Protected++
			continue
		}
		for _, ref := range object.References {
			if !strings.HasPrefix(ref, "app-content:") {
				continue
			}
			if err = store.Release(object.Snapshot.ID, ref); err != nil {
				return result, err
			}
		}
	}
	removed, err := store.Cleanup(ctx)
	result.Removed = len(removed)
	if len(removed) > 0 {
		s.notifyChange()
	}
	return result, err
}

func (s *Service) contentSnapshotProtected(ctx context.Context, id string, retainHistory bool) (bool, error) {
	var count int
	err := s.store.db.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM content_drafts WHERE snapshot_id=? AND state='draft')+(SELECT count(*) FROM content_queue c JOIN send_queue q ON q.id=c.queue_id WHERE c.snapshot_id=? AND q.state NOT IN ('completed','cancelled','expired'))`, id, id).Scan(&count)
	if err != nil || count > 0 {
		return count > 0, err
	}
	rows, err := s.store.db.QueryContext(ctx, `SELECT t.snapshot,t.recovery FROM content_tasks c JOIN tasks t ON t.id=c.task_id WHERE c.snapshot_id=?`, id)
	if err != nil {
		return false, err
	}
	for rows.Next() {
		var snapData, recoveryData []byte
		if err = rows.Scan(&snapData, &recoveryData); err != nil {
			_ = rows.Close()
			return false, err
		}
		var snap TaskSnapshot
		var recovery taskRecovery
		if len(snapData) > 1<<20 || len(recoveryData) > 2<<20 || json.Unmarshal(snapData, &snap) != nil || json.Unmarshal(recoveryData, &recovery) != nil {
			_ = rows.Close()
			return false, ErrMetadataInvalid
		}
		if retainHistory || inboxTaskProtected(snap, recovery) {
			_ = rows.Close()
			return true, nil
		}
	}
	err = rows.Err()
	_ = rows.Close()
	if err != nil {
		return false, err
	}
	// An ordinary file draft/queue can explicitly name this object too. It must
	// remain protected even though that request did not negotiate content_v1.
	queries := []string{
		`SELECT j.value FROM send_drafts d,json_each(d.source_paths) j`,
		`SELECT j.value FROM send_queue q,json_each(q.source_paths) j WHERE q.state NOT IN ('completed','cancelled','expired')`,
		`SELECT j.value FROM tasks t,json_each(t.recovery,'$.source_paths') j WHERE json_valid(t.recovery) AND (t.inbox_state NOT IN ('completed','cancelled','rejected','no_content') OR json_extract(t.snapshot,'$.can_resume')=1)`,
	}
	for _, query := range queries {
		rows, err := s.store.db.QueryContext(ctx, query)
		if err != nil {
			return false, err
		}
		count := 0
		for rows.Next() {
			var source string
			if err = rows.Scan(&source); err != nil {
				_ = rows.Close()
				return false, err
			}
			count++
			if count > 100000 {
				_ = rows.Close()
				return false, errors.New("CONTENT_REFERENCE_SCAN_LIMIT")
			}
			if pathsOverlapForCleanup(s.contentObjectPath(id), source) {
				_ = rows.Close()
				return true, nil
			}
		}
		err = rows.Err()
		_ = rows.Close()
		if err != nil {
			return false, err
		}
	}
	return false, nil
}
