package app

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Wen5555/LinkSend/internal/protocol"
	"github.com/Wen5555/LinkSend/internal/transfer"
)

func loadTestCacheRecord(t *testing.T, s *Service, id string) clipboardCacheRecord {
	t.Helper()
	var raw string
	if err := s.store.db.QueryRow(`SELECT value FROM metadata WHERE key=?`, clipboardCacheKeyPrefix+id).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	record, err := s.decodeCacheRecord(id, raw)
	if err != nil {
		t.Fatal(err)
	}
	return record
}

func TestClipboardCacheLedgerRejectsTamperAndStaleMutation(t *testing.T) {
	s, err := New(Config{DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Shutdown)
	d, err := openClipboardCacheDestination(s.cfg.DataDir, protocol.RandomID())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	r, err := s.registerClipboardCache(d, s.identity.ID())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.registerClipboardCache(d, s.identity.ID()); !errors.Is(err, errClipboardCacheOwnership) {
		t.Fatal("duplicate object overwrote its ownership record", err)
	}
	if err := r.advance("verified"); !errors.Is(err, errClipboardCacheOwnership) {
		t.Fatal("unreceived object acquired verified authority", err)
	}
	if _, err := s.decodeCacheRecord(r.record.ID, r.raw+"{}"); err == nil {
		t.Fatal("trailing metadata acquired ownership")
	}
	future := r.record
	future.Version = 2
	if raw, err := s.encodeCacheRecord(future); err != nil {
		t.Fatal(err)
	} else if _, err := s.decodeCacheRecord(r.record.ID, raw); err == nil {
		t.Fatal("future ownership version accepted")
	}
	var changed clipboardCacheRecord
	if err := json.Unmarshal([]byte(r.raw), &changed); err != nil {
		t.Fatal(err)
	}
	changed.Created++
	unsignedChange, _ := json.Marshal(changed)
	if _, err := s.decodeCacheRecord(r.record.ID, string(unsignedChange)); err == nil {
		t.Fatal("modified metadata passed signature verification")
	}
	changed.State = "published"
	tampered, _ := json.Marshal(changed)
	if _, err := s.decodeCacheRecord(r.record.ID, string(tampered)); err == nil {
		t.Fatal("tampered publication record authenticated")
	}
	if _, err := s.store.db.Exec(`UPDATE metadata SET value=? WHERE key=?`, string(tampered), clipboardCacheKeyPrefix+r.record.ID); err != nil {
		t.Fatal(err)
	}
	if err := r.advance("abandoned"); !errors.Is(err, errClipboardCacheOwnership) {
		t.Fatal("stale owner replaced newer/corrupt evidence", err)
	}
	if err := d.discardUnpublished(); err != nil {
		t.Fatal(err)
	}
	if err := r.forgetRemoved(); !errors.Is(err, errClipboardCacheOwnership) {
		t.Fatal("stale owner erased altered metadata", err)
	}
	var raw string
	if err := s.store.db.QueryRow(`SELECT value FROM metadata WHERE key=?`, clipboardCacheKeyPrefix+r.record.ID).Scan(&raw); err != nil || raw != string(tampered) {
		t.Fatal("tampered evidence was silently rewritten", err)
	}
}

func TestClipboardCacheLedgerRejectsForeignStagePart(t *testing.T) {
	s, err := New(Config{DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Shutdown)
	id := protocol.RandomID()
	d, err := openClipboardCacheDestination(s.cfg.DataDir, id)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	r, err := s.registerClipboardCache(d, s.identity.ID())
	if err != nil {
		t.Fatal(err)
	}
	m := cacheSpaceManifest(1)
	m.TransferID = id
	if err := r.bindManifest(m); err != nil {
		t.Fatal(err)
	}
	if err := d.object.Mkdir(".linksend-"+id, 0700); err != nil {
		t.Fatal(err)
	}
	stage, err := d.object.OpenRoot(".linksend-" + id)
	if err != nil {
		t.Fatal(err)
	}
	defer stage.Close()
	if err := stage.WriteFile("0.part", []byte("owned staging"), 0600); err != nil {
		t.Fatal(err)
	}
	plan := transfer.FullReceivePlan(m)
	state := transfer.ResumeState{Peer: s.identity.ID(), Manifest: m, Digest: m.Digest(), Plan: &plan, PlanDigest: plan.Digest()}
	raw, _ := json.Marshal(state)
	if err := stage.WriteFile("state.json", raw, 0600); err != nil {
		t.Fatal(err)
	}
	foreignPath := filepath.Join(t.TempDir(), "foreign.txt")
	if err := os.WriteFile(foreignPath, []byte("user data"), 0600); err != nil {
		t.Fatal(err)
	}
	foreign, err := os.Open(foreignPath)
	if err != nil {
		t.Fatal(err)
	}
	defer foreign.Close()
	err = r.bindStage(transfer.ReceiveStorage{Root: d.object, Stage: stage, Files: map[uint32]*os.File{0: foreign}, Manifest: m, Plan: plan, Peer: s.identity.ID()})
	if !errors.Is(err, errClipboardCacheOwnership) {
		t.Fatal("a pathname acquired ownership of another open inode", err)
	}
	if got := loadTestCacheRecord(t, s, id); got.Stage != "" || len(got.Parts) != 0 {
		t.Fatal("foreign provenance was persisted")
	}
	if data, err := os.ReadFile(foreignPath); err != nil || string(data) != "user data" {
		t.Fatal("foreign file modified", err)
	}
}

func TestClipboardCacheLedgerIntentFailureDoesNotPublish(t *testing.T) {
	f := newClipboardFilesFixture(t)
	_, err := f.b.store.db.Exec(`CREATE TRIGGER fail_cache_publish BEFORE UPDATE ON metadata
		WHEN NEW.key LIKE 'clipboard-cache/v1/%' AND json_extract(NEW.value,'$.state')='publishing'
		BEGIN SELECT RAISE(ABORT,'fixture cache metadata failure'); END`)
	if err != nil {
		t.Fatal(err)
	}
	f.copy(t, 2)
	waitFor(t, 3*time.Second, func() bool {
		for _, p := range f.b.ClipboardFileProgresses() {
			if clipboardFileFinished(p.State) {
				return true
			}
		}
		return false
	}, "failed publication intent did not terminate")
	select {
	case <-f.written:
		t.Fatal("OS received paths without a durable publication intent")
	default:
	}
	entries, err := os.ReadDir(filepath.Join(f.b.cfg.DataDir, "clipboard-files"))
	if err != nil || len(entries) != 0 {
		t.Fatal("unpublished intent failure left unsafe body files", err)
	}
	var rows int
	if err := f.b.store.db.QueryRow(`SELECT count(*) FROM metadata WHERE key LIKE ?`, clipboardCacheKeyPrefix+"%").Scan(&rows); err != nil || rows != 0 {
		t.Fatal("removed unpublished object retained a misleading record", rows, err)
	}
}

func TestClipboardCacheLedgerPersistsIntentBeforeNativeWrite(t *testing.T) {
	f := newClipboardFilesFixture(t)
	f.b.clipboardAdapterMu.Lock()
	write := f.b.clipboardAdapter.WriteFiles
	intent := make(chan clipboardCacheRecord, 1)
	f.b.clipboardAdapter.WriteFiles = func(paths []string, expected uint64, deadline time.Time) (uint64, error) {
		id := filepath.Base(filepath.Dir(paths[0]))
		intent <- loadTestCacheRecord(t, f.b, id)
		return write(paths, expected, deadline)
	}
	f.b.clipboardAdapterMu.Unlock()
	f.copy(t, 2)
	var record clipboardCacheRecord
	select {
	case record = <-intent:
	case <-f.ctx.Done():
		t.Fatal("native publication did not see a persisted intent")
	}
	if record.State != "publishing" || record.Manifest == nil || record.Digest != record.Manifest.Digest() || record.Stage == "" || record.PlanDigest == "" || len(record.Parts) != 1 {
		t.Fatal("native write began before ownership and actual staging were durable", record.State)
	}
	waitFor(t, time.Second, func() bool {
		for _, p := range f.b.ClipboardFileProgresses() {
			if p.ID == record.ID && p.State == "completed" {
				return true
			}
		}
		return false
	}, "published copy did not complete")
	if got := loadTestCacheRecord(t, f.b, record.ID); got.State != "published" || got.Revision <= record.Revision {
		t.Fatal("publication did not persist its own newer terminal record", got.State)
	}
	if err := f.b.ShutdownContext(f.ctx); err != nil {
		t.Fatal(err)
	}
	reopened, err := New(Config{DataDir: f.b.cfg.DataDir, Identity: f.bID})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(reopened.Shutdown)
	if got := loadTestCacheRecord(t, reopened, record.ID); got.State != "published" {
		t.Fatal("restart lost the protected publication record", got.State)
	}
	// The earlier, authentic publishing snapshot must not become a fresh local
	// authority just because a process lost the RAM-only native-start flag.
	oldIntent := &clipboardCacheReceipt{service: reopened, record: record}
	if err := oldIntent.beginNative(); !errors.Is(err, errClipboardCacheOwnership) {
		t.Fatal("restart reactivated the old publishing intent", err)
	}
	if err := oldIntent.forgetRemoved(); !errors.Is(err, errClipboardCacheOwnership) {
		t.Fatal("restart inferred that the old publishing intent was unattempted", err)
	}
	if len(reopened.ClipboardFileProgresses()) != 0 {
		t.Fatal("restart recreated or replayed an old clipboard job")
	}
}

func TestClipboardCacheLedgerSurvivesRealProfileKeyReload(t *testing.T) {
	profile := t.TempDir()
	s, err := New(Config{DataDir: profile})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Shutdown)
	id := protocol.RandomID()
	d, err := openClipboardCacheDestination(profile, id)
	if err != nil {
		t.Fatal(err)
	}
	r, err := s.registerClipboardCache(d, s.identity.ID())
	if err != nil {
		t.Fatal(err)
	}
	owner, directory := s.identity.ID(), r.record.Directory
	_ = d.Close()
	s.Shutdown()
	// No injected Identity: reopen the real restricted/DPAPI profile key.
	reopened, err := New(Config{DataDir: profile})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(reopened.Shutdown)
	got := loadTestCacheRecord(t, reopened, id)
	if got.Owner != owner || got.Directory != directory || got.State != "receiving" {
		t.Fatal("restart changed ownership or fabricated completion", got.State)
	}
	stale := &clipboardCacheReceipt{service: reopened, record: got}
	if err := stale.beginNative(); !errors.Is(err, errClipboardCacheOwnership) {
		t.Fatal("restart reactivated an old native intent", err)
	}
	if err := stale.forgetRemoved(); !errors.Is(err, errClipboardCacheOwnership) {
		t.Fatal("restart inferred that native reading had never begun", err)
	}
	if len(reopened.ClipboardFileProgresses()) != 0 {
		t.Fatal("reloading private cache records replayed a clipboard job")
	}
}
