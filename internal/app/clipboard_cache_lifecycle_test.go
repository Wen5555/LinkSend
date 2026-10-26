package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wen5555/LinkSend/internal/clipboardsync"
	"github.com/Wen5555/LinkSend/internal/protocol"
	"github.com/Wen5555/LinkSend/internal/transfer"
)

func newCacheLifecycleTestService(t *testing.T) *Service {
	t.Helper()
	s, err := New(Config{DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Shutdown)
	// Serialize past startup without depending on its goroutine scheduling.
	if _, err := s.collectClipboardCache(t.Context(), false, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	return s
}

// The fixture explicitly supplies provenance for its own created inode; product
// code obtains this only from the live receiver's closing-storage observer.
func sealedCacheFixture(t *testing.T, s *Service, id string) (*clipboardCacheDestination, *clipboardCacheReceipt) {
	t.Helper()
	d, err := openClipboardCacheDestination(s.cfg.DataDir, id)
	if err != nil {
		t.Fatal(err)
	}
	r, err := s.registerClipboardCache(d, s.identity.ID())
	if err != nil {
		_ = d.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close(); r.release() })
	if err := d.object.WriteFile("owned.txt", []byte("owned body"), 0600); err != nil {
		t.Fatal(err)
	}
	f, err := d.object.Open("owned.txt")
	if err != nil {
		t.Fatal(err)
	}
	node, err := cacheNode(f)
	_ = f.Close()
	if err != nil {
		t.Fatal(err)
	}
	next := r.record
	next.State, next.Sealed, next.Cleanup, next.Revision = "reclaimable", true, map[string]clipboardCacheNode{"owned.txt": node}, next.Revision+1
	if err := r.save(next); err != nil {
		t.Fatal(err)
	}
	return d, r
}

func collectCacheAt(t *testing.T, s *Service, pressure bool, at time.Time) int {
	t.Helper()
	removed, err := s.collectClipboardCache(t.Context(), pressure, at)
	if err != nil {
		t.Fatal(err)
	}
	return removed
}

func assertOwnedCacheBody(t *testing.T, d *clipboardCacheDestination) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(d.directory, "owned.txt"))
	if err != nil || string(data) != "owned body" {
		t.Fatal("protected body missing or changed", err)
	}
}

func TestClipboardCacheLifecycleProtectsActualReferenceUntilRelease(t *testing.T) {
	s := newCacheLifecycleTestService(t)
	d, r := sealedCacheFixture(t, s, protocol.RandomID())
	at := time.Now().Add(24 * time.Hour)
	// Simulate cancellation/terminal UI while the real owner still holds its
	// handles. Neither context state nor arbitrary elapsed time releases it.
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if ctx.Err() == nil {
		t.Fatal("fixture not cancelled")
	}
	if n := collectCacheAt(t, s, true, at); n != 0 {
		t.Fatal("active receiver reclaimed", n)
	}
	assertOwnedCacheBody(t, d)
	_ = d.Close()
	r.release()
	r.release()
	if n := collectCacheAt(t, s, true, at.Add(time.Minute)); n != 1 {
		t.Fatal("released object was not reclaimed", n)
	}
	if _, err := os.Stat(d.directory); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("reclaimed object still exists", err)
	}
	var raw string
	if err := s.store.db.QueryRow(`SELECT value FROM metadata WHERE key=?`, clipboardCacheKeyPrefix+d.id).Scan(&raw); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("removed object retained its record", err)
	}
}

func TestClipboardCacheLifecycleRejectsUnknownChangedAndReplacedEntries(t *testing.T) {
	for _, mode := range []string{"unknown", "modified", "replacement", "symlink"} {
		t.Run(mode, func(t *testing.T) {
			s := newCacheLifecycleTestService(t)
			d, r := sealedCacheFixture(t, s, protocol.RandomID())
			_ = d.Close()
			r.release()
			owned := filepath.Join(d.directory, "owned.txt")
			foreign := filepath.Join(d.directory, "user.txt")
			switch mode {
			case "unknown":
				if err := os.WriteFile(foreign, []byte("user data"), 0600); err != nil {
					t.Fatal(err)
				}
			case "modified":
				if err := os.WriteFile(owned, []byte("user edited body"), 0600); err != nil {
					t.Fatal(err)
				}
			case "replacement":
				if err := os.Rename(owned, foreign); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(owned, []byte("user replacement"), 0600); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				outside := filepath.Join(t.TempDir(), "outside.txt")
				if err := os.WriteFile(outside, []byte("outside data"), 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(outside, foreign); err != nil {
					t.Skip("OS did not permit owned symlink fixture", err)
				}
			}
			if n := collectCacheAt(t, s, true, time.Now().Add(24*time.Hour)); n != 0 {
				t.Fatal("unproven object removed", n)
			}
			data, err := os.ReadFile(owned)
			if err != nil || len(data) == 0 {
				t.Fatal("protected original/current entry disappeared", err)
			}
			if mode != "modified" {
				if data, err := os.ReadFile(foreign); err != nil || len(data) == 0 {
					t.Fatal("unknown or foreign file disappeared", err)
				}
			}
		})
	}
}

func TestClipboardCacheLifecycleBoundedPagesAndDeletionRetry(t *testing.T) {
	s := newCacheLifecycleTestService(t)
	for i := 0; i < 70; i++ {
		id := fmt.Sprintf("%032x", i+1)
		d, r := sealedCacheFixture(t, s, id)
		_ = d.Close()
		r.release()
		if i < 64 { // Old protected rows must not starve later eligible rows.
			next := r.record
			next.State = "abandoned"
			next.Revision++
			if err := r.save(next); err != nil {
				t.Fatal(err)
			}
		}
	}
	at := time.Now().Add(24 * time.Hour)
	if n := collectCacheAt(t, s, false, at); n != 0 {
		t.Fatal("collector exceeded its first page", n)
	}
	if n := collectCacheAt(t, s, false, at.Add(time.Minute)); n != 0 {
		t.Fatal("ordinary wake bypassed minimum interval", n)
	}
	if n := collectCacheAt(t, s, false, at.Add(5*time.Minute)); n != 6 {
		t.Fatal("cursor starved later reclaimable rows", n)
	}
	s = newCacheLifecycleTestService(t)
	// Inject a real SQLite deletion failure after unlinking. A later pressure
	// event retries the existing signed deleting intent, including absent root.
	d, r := sealedCacheFixture(t, s, protocol.RandomID())
	_ = d.Close()
	r.release()
	_, err := s.store.db.Exec(`CREATE TRIGGER fail_cache_forget BEFORE DELETE ON metadata WHEN OLD.key LIKE 'clipboard-cache/v1/%' BEGIN SELECT RAISE(ABORT,'fixture'); END`)
	if err != nil {
		t.Fatal(err)
	}
	if n := collectCacheAt(t, s, true, at.Add(10*time.Minute)); n != 0 {
		t.Fatal("failed durable deletion reported success")
	}
	if _, err := os.Stat(d.directory); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("fixture did not reach post-unlink failure", err)
	}
	if got := loadTestCacheRecord(t, s, d.id); got.State != "deleting" {
		t.Fatal("lost durable deletion intent", got.State)
	}
	if _, err := s.store.db.Exec(`DROP TRIGGER fail_cache_forget`); err != nil {
		t.Fatal(err)
	}
	if n := collectCacheAt(t, s, true, at.Add(11*time.Minute)); n != 1 {
		t.Fatal("pressure did not reconcile removed object", n)
	}
}

func TestClipboardCacheLifecycleDeletionCapBackoffAndClockChanges(t *testing.T) {
	s := newCacheLifecycleTestService(t)
	for i := 0; i < 10; i++ {
		d, r := sealedCacheFixture(t, s, fmt.Sprintf("%032x", i+1))
		_ = d.Close()
		r.release()
	}
	at := time.Now().Add(24 * time.Hour)
	if n := collectCacheAt(t, s, false, at); n != 8 {
		t.Fatal("per-pass deletion budget not respected", n)
	}
	if n := collectCacheAt(t, s, false, at.Add(5*time.Minute)); n != 2 {
		t.Fatal("remaining objects starved", n)
	}
	d, r := sealedCacheFixture(t, s, protocol.RandomID())
	_ = d.Close()
	r.release()
	if err := os.WriteFile(filepath.Join(d.directory, "unknown.txt"), []byte("protected"), 0600); err != nil {
		t.Fatal(err)
	}
	collectCacheAt(t, s, true, at.Add(10*time.Minute))
	c := &s.clipboardCache
	c.scanMu.Lock()
	first := c.retries[d.id]
	c.scanMu.Unlock()
	if first.attempts != 1 || !first.next.Equal(at.Add(15*time.Minute)) {
		t.Fatal("unexpected first retry", first)
	}
	collectCacheAt(t, s, false, at.Add(11*time.Minute))
	c.scanMu.Lock()
	second := c.retries[d.id]
	c.scanMu.Unlock()
	if second != first {
		t.Fatal("ordinary scan ignored object backoff")
	}
	collectCacheAt(t, s, false, at.Add(-time.Hour))
	assertOwnedCacheBody(t, d)
	for i := 0; i < 9; i++ {
		collectCacheAt(t, s, false, at.Add(time.Duration(i+1)*2*time.Hour))
	}
	c.scanMu.Lock()
	exhausted := c.retries[d.id]
	c.scanMu.Unlock()
	if exhausted.attempts != 8 {
		t.Fatal("retry exhaustion is not bounded", exhausted)
	}
	collectCacheAt(t, s, false, at.Add(100*24*time.Hour))
	assertOwnedCacheBody(t, d)
	if data, err := os.ReadFile(filepath.Join(d.directory, "unknown.txt")); err != nil || string(data) != "protected" {
		t.Fatal("wall clock jump deleted unknown data", err)
	}
}

func TestClipboardCacheLifecycleRestartContinuesOnlySignedUnpublishedIntent(t *testing.T) {
	s := newCacheLifecycleTestService(t)
	d, r := sealedCacheFixture(t, s, protocol.RandomID())
	_ = d.Close()
	r.release()
	s.Shutdown()
	reopened, err := New(Config{DataDir: s.cfg.DataDir})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(reopened.Shutdown)
	waitFor(t, 2*time.Second, func() bool { _, err := os.Stat(d.directory); return errors.Is(err, os.ErrNotExist) }, "startup did not reclaim signed unpublished intent")
	if len(reopened.ClipboardFileProgresses()) != 0 {
		t.Fatal("startup replayed clipboard transfer")
	}
}

func TestClipboardCacheLifecycleFailedBeforeNativePreservesUnknownThenReclaims(t *testing.T) {
	f := newClipboardFilesFixture(t)
	var unknown string
	var once sync.Once
	f.b.clipboardAdapterMu.Lock()
	f.b.clipboardAdapter.Generation = func() uint64 {
		entries, _ := os.ReadDir(filepath.Join(f.b.cfg.DataDir, "clipboard-files"))
		for _, entry := range entries {
			directory := filepath.Join(f.b.cfg.DataDir, "clipboard-files", entry.Name())
			if _, err := os.Stat(filepath.Join(directory, "copied.txt")); err == nil {
				once.Do(func() {
					unknown = filepath.Join(directory, "foreign.txt")
					_ = os.WriteFile(unknown, []byte("user-owned intrusion"), 0600)
				})
				return f.bGen.Load() + 1 // Fail CAS before any native mutation.
			}
		}
		return f.bGen.Load()
	}
	f.b.clipboardAdapterMu.Unlock()
	f.copy(t, 2)
	waitFor(t, 3*time.Second, func() bool {
		for _, p := range f.b.ClipboardFileProgresses() {
			if clipboardFileFinished(p.State) {
				return true
			}
		}
		return false
	}, "pre-native generation conflict did not finish")
	if unknown == "" {
		t.Fatal("fixture did not add unknown file")
	}
	select {
	case <-f.written:
		t.Fatal("conflicted copy reached native write")
	default:
	}
	if data, err := os.ReadFile(unknown); err != nil || string(data) != "user-owned intrusion" {
		t.Fatal("unpublished cleanup removed foreign data", err)
	}
	id := filepath.Base(filepath.Dir(unknown))
	got := loadTestCacheRecord(t, f.b, id)
	if got.State != "deleting" || !got.Sealed || len(got.Cleanup) < 3 {
		t.Fatal("failed cleanup lost its authenticated inventory", got.State)
	}
	owned := filepath.Join(filepath.Dir(unknown), "copied.txt")
	if data, err := os.ReadFile(owned); err != nil || string(data) != "clipboard file bytes" {
		t.Fatal("collector partially deleted before checking unknown file", err)
	}
	// Remove only the fixture's own intrusion to exercise the automatic retry;
	// this is not a product cleanup action or a user-facing recovery workflow.
	if err := os.Remove(unknown); err != nil {
		t.Fatal(err)
	}
	if n := collectCacheAt(t, f.b, true, time.Now().Add(24*time.Hour)); n != 1 {
		t.Fatal("safe leftover did not automatically reclaim", n)
	}
	if _, err := os.Stat(owned); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("owned body still consumes space", err)
	}
}

func TestClipboardCacheLifecycleNeverReclaimsPossiblyPublishedOrUnknown(t *testing.T) {
	f := newClipboardFilesFixture(t)
	// The shared fixture's SendReady is a cached status bit. This lifecycle
	// assertion starts only with the current healthy session's real files lease.
	var sender, receiver *PeerSession
	var initialLease clipboardsync.Lease
	waitFor(t, 2*time.Second, func() bool {
		sender, receiver = nil, nil
		f.a.directPoolMu.Lock()
		for _, pooled := range f.a.directPool {
			p := pooled.peer
			if p != nil && p.PeerID == f.bID.ID() && p.ClipboardFiles && p.Data != nil && p.Data.Conn != nil && p.Data.Conn.Context().Err() == nil {
				sender = p
				break
			}
		}
		f.a.directPoolMu.Unlock()
		if sender == nil {
			return false
		}
		f.b.directPoolMu.Lock()
		for _, pooled := range f.b.directPool {
			p := pooled.peer
			if p != nil && p.PeerID == f.aID.ID() && p.SessionID == sender.SessionID && p.ClipboardFiles && p.Data != nil && p.Data.Conn != nil && p.Data.Conn.Context().Err() == nil {
				receiver = p
				break
			}
		}
		f.b.directPoolMu.Unlock()
		if receiver == nil || sender.AuthorizationGeneration != receiver.RemoteAuthorizationGeneration || sender.RemoteAuthorizationGeneration != receiver.AuthorizationGeneration {
			return false
		}
		grant, permitted := f.b.clipboardGrant(f.aID.ID(), "receive", clipboardsync.Files, receiver.AuthorizationGeneration)
		var leased bool
		initialLease, leased = f.a.clipboardSync.Outbound(sender.PeerID, sender.SessionID, sender.RemoteAuthorizationGeneration, clipboardsync.Files)
		return permitted && leased && initialLease.Revision(clipboardsync.Files) == grant.Revision
	}, "current authenticated files lease was not ready for cache lifecycle fixture")
	t.Logf("cache publication start: session=%s generation=%d/%d lease=%s expires=%s", sender.SessionID, sender.AuthorizationGeneration, sender.RemoteAuthorizationGeneration, initialLease.ID, initialLease.ExpiresAt().Format(time.RFC3339Nano))
	var reads, writes atomic.Int32
	f.a.clipboardAdapterMu.Lock()
	readFiles := f.a.clipboardAdapter.ReadFiles
	f.a.clipboardAdapter.ReadFiles = func(ctx context.Context, generation uint64) ([]string, uint64, error) {
		reads.Add(1)
		return readFiles(ctx, generation)
	}
	f.a.clipboardAdapterMu.Unlock()
	f.b.clipboardAdapterMu.Lock()
	writeFiles := f.b.clipboardAdapter.WriteFiles
	f.b.clipboardAdapter.WriteFiles = func(paths []string, generation uint64, deadline time.Time) (uint64, error) {
		writes.Add(1)
		return writeFiles(paths, generation, deadline)
	}
	f.b.clipboardAdapterMu.Unlock()
	f.copy(t, 2)
	var paths []string
	select {
	case paths = <-f.written:
	case <-f.ctx.Done():
		t.Fatalf("copy did not publish: reads=%d writes=%d aGen=%d bGen=%d sender=%+v receiver=%+v sent=%+v received=%+v", reads.Load(), writes.Load(), f.aGen.Load(), f.bGen.Load(), f.a.ClipboardPeerStatuses(), f.b.ClipboardPeerStatuses(), f.a.ClipboardFileProgresses(), f.b.ClipboardFileProgresses())
	}
	id := filepath.Base(filepath.Dir(paths[0]))
	waitFor(t, time.Second, func() bool {
		for _, p := range f.b.ClipboardFileProgresses() {
			if p.ID == id && clipboardFileFinished(p.State) {
				return true
			}
		}
		return false
	}, "publication did not settle")
	for i, state := range []string{"publishing", "published", "protected"} {
		record := loadTestCacheRecord(t, f.b, id)
		record.State = state
		record.Revision++
		raw, err := f.b.encodeCacheRecord(record)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.b.store.db.Exec(`UPDATE metadata SET value=? WHERE key=?`, raw, clipboardCacheKeyPrefix+id); err != nil {
			t.Fatal(err)
		}
		collectCacheAt(t, f.b, true, time.Now().Add(time.Duration(100+i)*365*24*time.Hour))
		if data, err := os.ReadFile(paths[0]); err != nil || string(data) != "clipboard file bytes" {
			t.Fatal("elapsed time removed possibly published files", state, err)
		}
	}
	unknown := filepath.Join(f.b.cfg.DataDir, "clipboard-files", protocol.RandomID())
	if err := os.Mkdir(unknown, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(unknown, "keep.txt"), []byte("unknown"), 0600); err != nil {
		t.Fatal(err)
	}
	collectCacheAt(t, f.b, true, time.Now().Add(150*365*24*time.Hour))
	if data, err := os.ReadFile(filepath.Join(unknown, "keep.txt")); err != nil || string(data) != "unknown" {
		t.Fatal("hex directory name acquired delete authority", err)
	}
}

func TestClipboardCacheLifecycleObserverBudgetDoesNotRejectLargeDirectorySet(t *testing.T) {
	f := newClipboardFilesFixture(t)
	source := filepath.Dir(f.source)
	for i := 0; i < 70; i++ {
		if err := os.Mkdir(filepath.Join(source, fmt.Sprintf("empty-%03d", i)), 0700); err != nil {
			t.Fatal(err)
		}
	}
	f.a.clipboardAdapterMu.Lock()
	f.a.clipboardAdapter.ReadFiles = func(context.Context, uint64) ([]string, uint64, error) { return []string{source}, f.aGen.Load(), nil }
	f.a.clipboardAdapterMu.Unlock()
	f.copy(t, 2)
	var paths []string
	select {
	case paths = <-f.written:
	case <-f.ctx.Done():
		t.Fatal("optional inventory limit rejected a valid directory transfer")
	}
	if len(paths) != 1 {
		t.Fatal("unexpected roots", paths)
	}
	if data, err := os.ReadFile(filepath.Join(paths[0], filepath.Base(f.source))); err != nil || string(data) != "clipboard file bytes" {
		t.Fatal("bounded observer lost file data", err)
	}
	for i := 0; i < 70; i++ {
		if info, err := os.Stat(filepath.Join(paths[0], fmt.Sprintf("empty-%03d", i))); err != nil || !info.IsDir() {
			t.Fatal("empty directory missing", i, err)
		}
	}
}

func TestClipboardCacheLifecycleOversizedAndTamperedRowsDoNotAuthorizeDeletion(t *testing.T) {
	s := newCacheLifecycleTestService(t)
	d, r := sealedCacheFixture(t, s, "ffffffffffffffffffffffffffffffff")
	_ = d.Close()
	r.release()
	// Unicode byte length must be checked in SQLite before reading oversized
	// metadata; it must not wedge the bounded page cursor ahead of valid rows.
	invalid := strings.Repeat("文", 2*transfer.MaxMetadata/3+1)
	if _, err := s.store.db.Exec(`INSERT INTO metadata(key,value) VALUES(?,?)`, clipboardCacheKeyPrefix+"00000000000000000000000000000001", invalid); err != nil {
		t.Fatal(err)
	}
	if n := collectCacheAt(t, s, true, time.Now().Add(24*time.Hour)); n != 1 {
		t.Fatal("oversized row starved valid sealed object", n)
	}
	d, r = sealedCacheFixture(t, s, protocol.RandomID())
	_ = d.Close()
	r.release()
	if _, err := s.store.db.Exec(`UPDATE metadata SET value=replace(value,'owned.txt','other.txt') WHERE key=?`, clipboardCacheKeyPrefix+d.id); err != nil {
		t.Fatal(err)
	}
	if n := collectCacheAt(t, s, true, time.Now().Add(25*time.Hour)); n != 0 {
		t.Fatal("tampered deletion inventory authorized unlink", n)
	}
	assertOwnedCacheBody(t, d)
}
