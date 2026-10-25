package app

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/Wen5555/LinkSend/internal/transfer"
)

const clipboardCacheInterval = 5 * time.Minute

type clipboardCacheRetry struct {
	attempts int
	next     time.Time
}

// The existing profile owner holds both runtime references and the collector.
// Context cancellation and task UI state are deliberately not references.
type clipboardCacheLifecycle struct {
	mu             sync.Mutex
	scanMu         sync.Mutex
	active         map[string]bool
	wake           chan struct{}
	wg             sync.WaitGroup
	cursor         string
	last, pressure time.Time
	retries        map[string]clipboardCacheRetry
}

func (c *clipboardCacheLifecycle) acquire(id string) (func(), error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.active[id] {
		return nil, errClipboardCacheOwnership
	}
	if c.active == nil {
		c.active = make(map[string]bool)
	}
	c.active[id] = true
	var once sync.Once
	return func() {
		once.Do(func() {
			c.mu.Lock()
			delete(c.active, id)
			if c.wake != nil {
				select {
				case c.wake <- struct{}{}:
				default:
				}
			}
			c.mu.Unlock()
		})
	}, nil
}

func (s *Service) startClipboardCacheLifecycle() {
	if s.store == nil || s.storeErr != nil {
		return
	}
	c := &s.clipboardCache
	c.wake = make(chan struct{}, 1)
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		ticker := time.NewTicker(clipboardCacheInterval)
		defer ticker.Stop()
		// Startup reconciliation is bounded and never starts a receive or native
		// write. Only a sealed, definitively unpublished record grants deletion.
		_, _ = s.collectClipboardCache(s.workCtx, false, time.Now())
		for {
			select {
			case <-s.workCtx.Done():
				return
			case <-ticker.C:
			case <-c.wake:
			}
			_, _ = s.collectClipboardCache(s.workCtx, false, time.Now())
		}
	}()
}

func (s *Service) wakeClipboardCache() {
	c := &s.clipboardCache
	c.mu.Lock()
	if c.wake != nil {
		select {
		case c.wake <- struct{}{}:
		default:
		}
	}
	c.mu.Unlock()
}

// A page cursor prevents old protected rows starving later reclaimable rows.
// This is internal only: no manual cleanup command or cache settings are added.
func (s *Service) collectClipboardCache(ctx context.Context, pressure bool, now time.Time) (removed int, err error) {
	c := &s.clipboardCache
	c.scanMu.Lock()
	defer c.scanMu.Unlock()
	if pressure {
		if !c.pressure.IsZero() && now.Sub(c.pressure) < 30*time.Second {
			return 0, nil
		}
		c.pressure = now
	} else {
		if !c.last.IsZero() && now.Sub(c.last) < clipboardCacheInterval {
			return 0, nil
		}
		c.last = now
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	rows, err := s.store.db.QueryContext(ctx, `SELECT key,CASE WHEN length(CAST(value AS BLOB))<=? THEN value ELSE '' END FROM metadata WHERE key>=? AND key<? AND key>? ORDER BY key LIMIT 64`, 2*transfer.MaxMetadata, clipboardCacheKeyPrefix, clipboardCacheKeyPrefix+"\uffff", c.cursor)
	if err != nil {
		return 0, err
	}
	type row struct{ key, raw string }
	var page []row
	bytesRead, reachedEnd := 0, true
	for rows.Next() {
		var item row
		if err = rows.Scan(&item.key, &item.raw); err != nil {
			_ = rows.Close()
			return 0, err
		}
		if bytesRead+len(item.raw) > 4<<20 {
			reachedEnd = false
			break
		}
		bytesRead += len(item.raw)
		page = append(page, item)
	}
	err = rows.Err()
	_ = rows.Close() // Release shared SQLite connection before any CAS mutation.
	if err != nil {
		return 0, err
	}
	attempted := 0
	for _, item := range page {
		if err := ctx.Err(); err != nil {
			return removed, err
		}
		if attempted >= 8 {
			return removed, nil
		}
		c.cursor = item.key
		id := strings.TrimPrefix(item.key, clipboardCacheKeyPrefix)
		record, decodeErr := s.decodeCacheRecord(id, item.raw)
		if decodeErr != nil || record.Native != nil || !record.Sealed || (record.State != "reclaimable" && record.State != "deleting") {
			continue
		}
		c.mu.Lock()
		if c.active[id] {
			c.mu.Unlock()
			continue
		}
		retry := c.retries[id]
		if !pressure && (retry.attempts >= 8 || (!retry.next.IsZero() && now.Before(retry.next))) {
			c.mu.Unlock()
			continue
		}
		// Keep acquisition excluded through the final recheck, filesystem unlink
		// and metadata CAS. No worker may acquire this same object in between.
		attempted++
		cleanErr := s.collectClipboardCacheObject(ctx, record, item.raw)
		if cleanErr == nil {
			delete(c.retries, id)
			removed++
		} else {
			if c.retries == nil {
				c.retries = make(map[string]clipboardCacheRetry)
			}
			// Bounded retry memory. At the cap, untracked failures are still limited
			// by the global page/interval budget and remain on disk.
			if len(c.retries) < 256 || retry.attempts != 0 {
				retry.attempts = min(8, retry.attempts+1)
				retry.next = now.Add(min(time.Hour, clipboardCacheInterval*time.Duration(1<<uint(retry.attempts-1))))
				c.retries[id] = retry
			}
		}
		c.mu.Unlock()
	}
	if reachedEnd && len(page) < 64 {
		c.cursor = ""
	}
	return removed, nil
}

func (s *Service) collectClipboardCacheObject(ctx context.Context, record clipboardCacheRecord, raw string) error {
	// Receipt-only proves bindings, not native source lifetime. Even a valid
	// signed close claim must not reach the unpublished deletion engine.
	if record.Native != nil {
		return errClipboardCacheOwnership
	}
	var current string
	if err := s.store.db.QueryRowContext(ctx, `SELECT value FROM metadata WHERE key=?`, clipboardCacheKeyPrefix+record.ID).Scan(&current); err != nil {
		return err
	}
	if current != raw {
		return errClipboardCacheOwnership
	}
	// The row, not a potentially incomplete caller-side struct, owns the
	// deletion authority. In particular dropping Native from a copy must not
	// turn a durable receipt-only publication into an unpublished object.
	authenticated, err := s.decodeCacheRecord(record.ID, current)
	if err != nil || authenticated.Native != nil || !authenticated.Sealed || (authenticated.State != "reclaimable" && authenticated.State != "deleting") {
		return errors.Join(errClipboardCacheOwnership, err)
	}
	record = authenticated
	d, err := openRecordedClipboardCache(s.cfg.DataDir, record)
	if err != nil {
		return err
	}
	defer d.Close()
	if record.State != "deleting" {
		next := record
		next.State, next.Revision = "deleting", next.Revision+1
		if next.Revision == 0 {
			return errClipboardCacheOwnership
		}
		encoded, err := s.encodeCacheRecord(next)
		if err != nil {
			return err
		}
		result, err := s.store.db.ExecContext(ctx, `UPDATE metadata SET value=? WHERE key=? AND value=?`, encoded, clipboardCacheKeyPrefix+record.ID, raw)
		if err != nil {
			return err
		}
		if n, err := result.RowsAffected(); err != nil || n != 1 {
			return errors.Join(errClipboardCacheOwnership, err)
		}
		raw = encoded
		if err := s.store.db.QueryRowContext(ctx, `SELECT value FROM metadata WHERE key=?`, clipboardCacheKeyPrefix+record.ID).Scan(&current); err != nil {
			return err
		}
		if current != raw {
			return errClipboardCacheOwnership
		}
	}
	if d.object != nil {
		if err := removeClipboardCacheTree(ctx, d.object, record.Cleanup); err != nil {
			return err
		}
		if err := d.discardUnpublished(); err != nil {
			return err
		}
	}
	result, err := s.store.db.ExecContext(ctx, `DELETE FROM metadata WHERE key=? AND value=?`, clipboardCacheKeyPrefix+record.ID, raw)
	if err != nil {
		return err
	}
	if n, err := result.RowsAffected(); err != nil || n != 1 {
		return errors.Join(errClipboardCacheOwnership, err)
	}
	return nil
}

func openRecordedClipboardCache(profile string, record clipboardCacheRecord) (_ *clipboardCacheDestination, err error) {
	d := &clipboardCacheDestination{profilePath: profile, id: record.ID}
	defer func() {
		if err != nil {
			_ = d.Close()
		}
	}()
	d.profile, err = os.OpenRoot(profile)
	if err != nil {
		return nil, err
	}
	d.profileInfo, err = d.profile.Stat(".")
	if err != nil {
		return nil, err
	}
	info, err := d.profile.Lstat("clipboard-files")
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.Join(errClipboardCacheOwnership, err)
	}
	d.cache, err = d.profile.OpenRoot("clipboard-files")
	if err != nil {
		return nil, err
	}
	d.cacheInfo, err = d.cache.Stat(".")
	if err != nil || !os.SameFile(info, d.cacheInfo) {
		return nil, errors.Join(errClipboardCacheOwnership, err)
	}
	cacheID, err := clipboardRootIdentity(d.cache)
	if err != nil || cacheID != record.CacheRoot {
		return nil, errors.Join(errClipboardCacheOwnership, err)
	}
	info, err = d.cache.Lstat(record.ID)
	if errors.Is(err, os.ErrNotExist) {
		return d, nil
	}
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.Join(errClipboardCacheOwnership, err)
	}
	d.object, err = d.cache.OpenRoot(record.ID)
	if err != nil {
		return nil, err
	}
	d.objectInfo, err = d.object.Stat(".")
	if err != nil || !os.SameFile(info, d.objectInfo) {
		return nil, errors.Join(errClipboardCacheOwnership, err)
	}
	objectID, err := clipboardRootIdentity(d.object)
	if err != nil || objectID != record.Directory {
		return nil, errors.Join(errClipboardCacheOwnership, err)
	}
	return d, nil
}
