package app

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wen5555/LinkSend/internal/protocol"
	"github.com/Wen5555/LinkSend/internal/transfer"
)

func cacheSpaceManifest(bytes int64) transfer.Manifest {
	chunks := make([]string, (bytes+(8<<20)-1)/(8<<20))
	for i := range chunks {
		chunks[i] = strings.Repeat("a", 64)
	}
	return transfer.Manifest{Version: 1, TransferID: protocol.RandomID(), ChunkSize: 8 << 20,
		Files: []transfer.FileEntry{{ID: 0, Path: "file.txt", Type: "file", Size: bytes, Hash: strings.Repeat("a", 64), Chunks: chunks}}}
}

func TestClipboardCacheSpaceConcurrentReservations(t *testing.T) {
	cache := clipboardCacheSpace{available: func(string) (uint64, error) { return 256 << 20, nil }}
	// Content + metadata + guard permit exactly two of these reservations.
	var admitted atomic.Int32
	var wg sync.WaitGroup
	releases := make(chan func(), 32)
	for range 32 {
		wg.Go(func() {
			release, err := cache.reserve(t.TempDir(), cacheSpaceManifest(64<<20))
			if err == nil {
				admitted.Add(1)
				releases <- release
			} else if protocol.ErrorCode(err) != protocol.DiskFull {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	close(releases)
	if admitted.Load() != 2 {
		t.Fatalf("concurrent disk admission bypassed reservations: %d", admitted.Load())
	}
	for release := range releases {
		release()
		release()
	}
	if len(cache.reservations) != 0 {
		t.Fatal("reservations leaked")
	}
	if release, err := cache.reserve(t.TempDir(), cacheSpaceManifest(64<<20)); err != nil {
		t.Fatal("released capacity not reusable", err)
	} else {
		release()
	}
}

func TestClipboardCacheSpaceRejectsInvalidAndUnknownCapacity(t *testing.T) {
	cache := clipboardCacheSpace{available: func(string) (uint64, error) { return 0, errors.New("disk unavailable") }}
	if _, err := cache.reserve(t.TempDir(), cacheSpaceManifest(1)); protocol.ErrorCode(err) != protocol.ReceiveDirectoryUnavailable {
		t.Fatal("unknown capacity admitted", err)
	}
	invalid := cacheSpaceManifest(1)
	invalid.Files[0].Size = -1
	if _, err := cache.reserve(t.TempDir(), invalid); err == nil || len(cache.reservations) != 0 {
		t.Fatal("invalid manifest reserved space")
	}
}

func TestClipboardFilesInsufficientSpacePreservesExistingFiles(t *testing.T) {
	f := newClipboardFilesFixture(t)
	root := filepath.Join(f.b.cfg.DataDir, "clipboard-files")
	protected := filepath.Join(root, "unmanaged-current-clipboard")
	if err := os.MkdirAll(protected, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(protected, "keep.txt")
	if err := os.WriteFile(path, []byte("keep existing clipboard"), 0600); err != nil {
		t.Fatal(err)
	}
	f.b.clipboardCacheSpace.available = func(string) (uint64, error) { return 64 << 20, nil }
	f.copy(t, 2)
	waitFor(t, 3*time.Second, func() bool {
		for _, progress := range f.b.ClipboardFileProgresses() {
			if clipboardFileFinished(progress.State) {
				return true
			}
		}
		return false
	}, "low-space receive did not finish")
	for _, progress := range f.b.ClipboardFileProgresses() {
		if progress.State != "failed" || progress.Error != "file_copy_disk_full" {
			t.Fatal("disk admission failure did not expose a stable safe reason", progress)
		}
	}
	select {
	case <-f.written:
		t.Fatal("insufficient-space copy was published")
	default:
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != "keep existing clipboard" {
		t.Fatal("existing clipboard file damaged", err)
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 1 || entries[0].Name() != filepath.Base(protected) {
		t.Fatal("failed copy left partial cache or removed protected cache", err)
	}
	f.b.clipboardCacheSpace.mu.Lock()
	remaining := len(f.b.clipboardCacheSpace.reservations)
	f.b.clipboardCacheSpace.mu.Unlock()
	if remaining != 0 {
		t.Fatal("failed receive leaked capacity")
	}
	var cacheRows int
	if err := f.b.store.db.QueryRow(`SELECT count(*) FROM metadata WHERE key LIKE ?`, clipboardCacheKeyPrefix+"%").Scan(&cacheRows); err != nil || cacheRows != 0 {
		t.Fatal("removed unpublished copy retained or lost unrelated cache metadata", cacheRows, err)
	}
	if data, err := os.ReadFile(f.source); err != nil || string(data) != "clipboard file bytes" {
		t.Fatal("space rejection changed sender source", err)
	}
	// Once volume pressure ends a fresh copy may use the same authenticated
	// session. The previous disk error must clear only for this new operation.
	f.b.clipboardCacheSpace.mu.Lock()
	f.b.clipboardCacheSpace.available = func(string) (uint64, error) { return 1 << 30, nil }
	f.b.clipboardCacheSpace.mu.Unlock()
	f.copy(t, 3)
	select {
	case paths := <-f.written:
		if len(paths) != 1 {
			t.Fatal("new copy did not publish exactly its own root")
		}
		if data, err := os.ReadFile(paths[0]); err != nil || string(data) != "clipboard file bytes" {
			t.Fatal("new copy after pressure mismatch", err)
		}
	case <-f.ctx.Done():
		t.Fatal("new copy could not recover after disk pressure", f.a.ClipboardFileProgresses(), f.b.ClipboardFileProgresses(), f.a.ClipboardPeerStatuses(), f.b.ClipboardPeerStatuses())
	}
	waitFor(t, time.Second, func() bool {
		for _, status := range f.b.ClipboardPeerStatuses() {
			if clipboardFileError(status.Error) {
				return false
			}
		}
		return true
	}, "successful new copy retained stale disk error")
}

func TestClipboardFilesSpaceReservedThroughNativePublication(t *testing.T) {
	f := newClipboardFilesFixture(t)
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	f.b.clipboardAdapterMu.Lock()
	nativeWrite := f.b.clipboardAdapter.WriteFiles
	f.b.clipboardAdapter.WriteFiles = func(paths []string, expected uint64, deadline time.Time) (uint64, error) {
		close(entered)
		select {
		case <-release:
			return nativeWrite(paths, expected, deadline)
		case <-f.ctx.Done():
			return 0, f.ctx.Err()
		}
	}
	f.b.clipboardAdapterMu.Unlock()
	f.copy(t, 2)
	select {
	case <-entered:
	case <-f.ctx.Done():
		t.Fatal("native publication did not start")
	}
	f.b.clipboardCacheSpace.mu.Lock()
	reserved := len(f.b.clipboardCacheSpace.reservations)
	f.b.clipboardCacheSpace.mu.Unlock()
	if reserved != 1 {
		t.Fatal("capacity released before native publication returned")
	}
	unblock()
	waitFor(t, time.Second, func() bool {
		f.b.clipboardCacheSpace.mu.Lock()
		defer f.b.clipboardCacheSpace.mu.Unlock()
		return len(f.b.clipboardCacheSpace.reservations) == 0
	}, "successful publication leaked capacity")
}

func TestClipboardFilesCancelledIOKeepsHealthySession(t *testing.T) {
	f := newClipboardFilesFixture(t)
	f.a.directPoolMu.Lock()
	var peer *PeerSession
	for _, pooled := range f.a.directPool {
		peer = pooled.peer
		pooled.inUse = true
	}
	f.a.directPoolMu.Unlock()
	if peer == nil {
		t.Fatal("fixture has no authenticated session")
	}
	ctx, cancel := context.WithCancel(peer.Data.Conn.Context())
	cancel()
	// A canceled native/file operation can surface EOF instead of returning
	// context.Canceled from its interrupted stream. Preserve control intent.
	job := &clipboardFileJob{peer: peer, ctx: ctx}
	_ = f.a.releaseClipboardFileSession(job, classifyTaskError(io.ErrUnexpectedEOF))
	if peer.Data.Conn.Context().Err() != nil {
		t.Fatal("cancelled file I/O destroyed a healthy clipboard connection")
	}
	f.copy(t, 2)
	select {
	case <-f.written:
	case <-f.ctx.Done():
		t.Fatal("fresh copy after cancelled I/O did not transfer")
	}
}
