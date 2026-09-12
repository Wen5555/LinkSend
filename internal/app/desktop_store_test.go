package app

import (
	"bytes"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const metadataTestPeer = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func newTestDesktopStore(t *testing.T) (*desktopStore, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "task-history.sqlite")
	store, err := openDesktopStore(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store, path
}

func createSchema2MetadataFixture(t *testing.T, path string) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = db.Exec(`CREATE TABLE tasks(id TEXT PRIMARY KEY,revision INTEGER NOT NULL,snapshot BLOB NOT NULL,recovery BLOB);
		CREATE TABLE metadata(key TEXT PRIMARY KEY,value TEXT NOT NULL);
		INSERT INTO metadata(key,value) VALUES('schema_version','2');
		PRAGMA user_version=2`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO tasks(id,revision,snapshot,recovery) VALUES('retained',7,?,?)`, []byte(`{"state":"paused","revision":7}`), []byte(`{"peer_id":"old","chunk_size":4194304}`))
	if err != nil {
		t.Fatal(err)
	}
}

func TestDesktopStoreSchema2MigrationPreservesHistoryAndBackup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "task-history.sqlite")
	createSchema2MetadataFixture(t, path)
	store, err := openDesktopStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	var version int
	var metadataVersion string
	if err = store.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != taskStoreSchema {
		t.Fatalf("version=%d err=%v", version, err)
	}
	if err = store.db.QueryRow("SELECT value FROM metadata WHERE key='schema_version'").Scan(&metadataVersion); err != nil || metadataVersion != fmt.Sprint(taskStoreSchema) {
		t.Fatalf("metadata version=%s err=%v", metadataVersion, err)
	}
	backups, err := filepath.Glob(path + ".schema-v2-*.bak")
	if err != nil || len(backups) != 1 {
		t.Fatalf("backups=%v err=%v", backups, err)
	}
	if err = verifyHistoryBackup(backups[0], 2); err != nil {
		t.Fatalf("rollback file cannot be verified: %v", err)
	}
	backup, err := sql.Open("sqlite", backups[0])
	if err != nil {
		t.Fatal(err)
	}
	defer backup.Close()
	var snapshot, recovery, backupSnapshot, backupRecovery []byte
	if err = store.db.QueryRow("SELECT snapshot,recovery FROM tasks WHERE id='retained'").Scan(&snapshot, &recovery); err != nil {
		t.Fatal(err)
	}
	if err = backup.QueryRow("SELECT snapshot,recovery FROM tasks WHERE id='retained'").Scan(&backupSnapshot, &backupRecovery); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(snapshot, backupSnapshot) || !bytes.Equal(recovery, backupRecovery) {
		t.Fatal("metadata migration rewrote task recovery content")
	}
	var backupTables int
	if err = backup.QueryRow("SELECT count(*) FROM sqlite_master WHERE name IN ('device_profiles','send_drafts','send_queue')").Scan(&backupTables); err != nil || backupTables != 0 {
		t.Fatalf("rollback backup was mutated: count=%d err=%v", backupTables, err)
	}
	// Reopening schema3 must neither rerun DDL nor produce another backup.
	reopened, err := openDesktopStore(path)
	if err != nil {
		t.Fatal(err)
	}
	_ = reopened.Close()
	backups, _ = filepath.Glob(path + ".schema-v2-*.bak")
	if len(backups) != 1 {
		t.Fatal("reopen unexpectedly migrated again")
	}
}

func TestDesktopStoreMigrationFailureRollsBackAllDDL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "task-history.sqlite")
	createSchema2MetadataFixture(t, path)
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	// A conflicting object forces failure after the device/draft DDL ran.
	if _, err = db.Exec("CREATE VIEW send_queue AS SELECT id FROM tasks"); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	if store, err := openDesktopStore(path); err == nil {
		_ = store.Close()
		t.Fatal("incompatible queue object did not fail migration")
	}
	db, err = sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var version, partialTables, tasks int
	if err = db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow("SELECT count(*) FROM sqlite_master WHERE name IN ('device_profiles','send_drafts')").Scan(&partialTables); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow("SELECT count(*) FROM tasks WHERE id='retained' AND revision=7").Scan(&tasks); err != nil {
		t.Fatal(err)
	}
	if version != 2 || partialTables != 0 || tasks != 1 {
		t.Fatalf("migration left partial state: schema=%d tables=%d tasks=%d", version, partialTables, tasks)
	}
	backups, _ := filepath.Glob(path + ".schema-v2-*.bak")
	if len(backups) != 1 || verifyHistoryBackup(backups[0], 2) != nil {
		t.Fatal("failed migration did not retain a readable rollback backup")
	}
}

func TestDesktopStoreBackupVerificationRejectsInvalidSource(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad-backup.sqlite")
	if err := os.WriteFile(path, []byte("broken sqlite"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := verifyHistoryBackup(path, 2); err == nil {
		t.Fatal("corrupt backup passed verification")
	}
	valid := filepath.Join(t.TempDir(), "schema2.sqlite")
	createSchema2MetadataFixture(t, valid)
	if err := verifyHistoryBackup(valid, 1); err == nil {
		t.Fatal("wrong schema backup passed verification")
	}
}

func TestDesktopStoreRejectsFutureSchemaWithoutMutation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "future.sqlite")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(fmt.Sprintf("PRAGMA user_version=%d", taskStoreSchema+1)); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if store, err := openDesktopStore(path); err == nil || err.Error() != "TASK_STORE_VERSION" {
		if store != nil {
			_ = store.Close()
		}
		t.Fatalf("unexpected future schema error: %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("opening a future schema modified its file")
	}
}

func TestDesktopDeviceProfilePersistsAndCASProtectsLastUse(t *testing.T) {
	store, path := newTestDesktopStore(t)
	profile, err := store.SaveDeviceProfile(DeviceProfile{PeerID: metadataTestPeer, Alias: "  我的笔记本  ", MyDevice: true, Pinned: true, Position: 2, ReceiveDirectory: t.TempDir(), LastUsedAt: "forged"})
	if err != nil {
		t.Fatal(err)
	}
	if profile.Revision != 1 || profile.Alias != "我的笔记本" || profile.LastUsedAt != "" {
		t.Fatalf("bad initial profile: %+v", profile)
	}
	if _, err = store.SaveDeviceProfile(DeviceProfile{PeerID: metadataTestPeer}); !errors.Is(err, ErrMetadataConflict) {
		t.Fatalf("insert overwrote an existing peer: %v", err)
	}
	when := time.Date(2026, 9, 12, 5, 0, 0, 123, time.UTC)
	if err = store.TouchDevice(metadataTestPeer, when); err != nil {
		t.Fatal(err)
	}
	if _, err = store.SaveDeviceProfile(profile); !errors.Is(err, ErrMetadataConflict) {
		t.Fatalf("stale preference revision accepted: %v", err)
	}
	profile, err = store.DeviceProfile(metadataTestPeer)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.TouchDevice(metadataTestPeer, when.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	profile.Alias = "新别名"
	profile.LastUsedAt = "2030-01-01T00:00:00Z"
	profile, err = store.SaveDeviceProfile(profile)
	if err != nil {
		t.Fatal(err)
	}
	if profile.LastUsedAt != when.Format(desktopTimestamp) || profile.Revision != 3 {
		t.Fatalf("preferences forged last-use or old event advanced revision: %+v", profile)
	}
	_ = store.Close()
	reopened, err := openDesktopStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	got, err := reopened.DeviceProfile(metadataTestPeer)
	if err != nil || got != profile {
		t.Fatalf("profile did not survive reopening: got=%+v err=%v", got, err)
	}
	other := strings.Repeat("b", 64)
	if _, err = reopened.DeviceProfile(other); !errors.Is(err, ErrMetadataNotFound) {
		t.Fatal("different identity inherited profile")
	}
	if _, err = reopened.SaveDeviceProfile(DeviceProfile{PeerID: other, Alias: profile.Alias}); err != nil {
		t.Fatal(err)
	}
	profiles, err := reopened.DeviceProfiles()
	if err != nil || len(profiles) != 2 || profiles[0].PeerID != metadataTestPeer || profiles[1].MyDevice || profiles[1].Pinned || profiles[1].ReceiveDirectory != "" {
		t.Fatalf("identity/sort isolation failed: %+v err=%v", profiles, err)
	}
}

func TestDesktopDeviceProfileConcurrentCASAcrossConnections(t *testing.T) {
	store, path := newTestDesktopStore(t)
	profile, err := store.SaveDeviceProfile(DeviceProfile{PeerID: metadataTestPeer})
	if err != nil {
		t.Fatal(err)
	}
	other, err := openDesktopStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	var successes atomic.Int32
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := range 12 {
		wg.Go(func() {
			<-start
			candidate := profile
			candidate.Position = i
			writer := store
			if i%2 == 1 {
				writer = other
			}
			_, err := writer.SaveDeviceProfile(candidate)
			if err == nil {
				successes.Add(1)
			} else if !errors.Is(err, ErrMetadataConflict) {
				t.Errorf("unexpected write error: %v", err)
			}
		})
	}
	close(start)
	wg.Wait()
	if successes.Load() != 1 {
		t.Fatalf("expected one committed CAS winner, got %d", successes.Load())
	}
}

func TestDesktopDeviceProfileValidation(t *testing.T) {
	store, _ := newTestDesktopStore(t)
	file := filepath.Join(t.TempDir(), "file.txt")
	if err := os.WriteFile(file, nil, 0600); err != nil {
		t.Fatal(err)
	}
	for name, change := range map[string]func(*DeviceProfile){
		"identity":           func(p *DeviceProfile) { p.PeerID = "device name" },
		"uppercase_identity": func(p *DeviceProfile) { p.PeerID = strings.ToUpper(metadataTestPeer) },
		"control_alias":      func(p *DeviceProfile) { p.Alias = "line\nname" },
		"leading_control":    func(p *DeviceProfile) { p.Alias = "\nname" },
		"long_alias":         func(p *DeviceProfile) { p.Alias = strings.Repeat("字", 129) },
		"invalid_utf8":       func(p *DeviceProfile) { p.Alias = string([]byte{0xff}) },
		"negative_position":  func(p *DeviceProfile) { p.Position = -1 },
		"relative_directory": func(p *DeviceProfile) { p.ReceiveDirectory = "relative" },
		"missing_directory":  func(p *DeviceProfile) { p.ReceiveDirectory = filepath.Join(t.TempDir(), "missing") },
		"file_directory":     func(p *DeviceProfile) { p.ReceiveDirectory = file },
		"revision_overflow":  func(p *DeviceProfile) { p.Revision = math.MaxUint64 },
	} {
		t.Run(name, func(t *testing.T) {
			p := DeviceProfile{PeerID: metadataTestPeer}
			change(&p)
			if _, err := store.SaveDeviceProfile(p); !errors.Is(err, ErrMetadataInvalid) {
				t.Fatalf("expected invalid metadata, got %v", err)
			}
		})
	}
}

func TestDesktopDraftPathsPersistWithoutReadingSourceBodies(t *testing.T) {
	store, path := newTestDesktopStore(t)
	wd := t.TempDir()
	filename := "未创建 中文 空格.txt"
	abs := filepath.Join(wd, filename)
	draft, err := store.SaveDraft(SendDraft{Paths: []string{filename, filepath.Join("nested", "..", filename), abs}, PeerID: metadataTestPeer}, wd)
	if err != nil {
		t.Fatal(err)
	}
	if draft.ID != "main" || draft.Revision != 1 || !reflect.DeepEqual(draft.Paths, []string{abs}) {
		t.Fatalf("unexpected normalized draft: %+v", draft)
	}
	if _, err = os.Stat(abs); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("saving metadata accessed/created source content: %v", err)
	}
	stale := draft
	draft.PeerID = strings.Repeat("b", 64)
	draft, err = store.SaveDraft(draft, "")
	if err != nil || draft.Paths[0] != abs || draft.Revision != 2 {
		t.Fatalf("switching target lost draft: %+v err=%v", draft, err)
	}
	if _, err = store.SaveDraft(stale, ""); !errors.Is(err, ErrMetadataConflict) {
		t.Fatalf("stale draft overwrote current: %v", err)
	}
	_ = store.Close()
	reopened, err := openDesktopStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	got, err := reopened.Draft("")
	if err != nil || !reflect.DeepEqual(got, draft) {
		t.Fatalf("draft not restored: got=%+v err=%v", got, err)
	}
	drafts, err := reopened.Drafts()
	if err != nil || len(drafts) != 1 {
		t.Fatalf("draft list: %v err=%v", drafts, err)
	}
	got.Paths = nil
	cleared, err := reopened.SaveDraft(got, "")
	if err != nil || len(cleared.Paths) != 0 || cleared.Paths == nil {
		t.Fatalf("empty draft did not remain a JSON array: %+v err=%v", cleared, err)
	}
}

func TestDesktopDraftBoundsAndRelativePathPolicy(t *testing.T) {
	store, _ := newTestDesktopStore(t)
	wd := t.TempDir()
	tooMany := make([]string, maxDraftPaths+1)
	for i := range tooMany {
		tooMany[i] = "same.txt"
	}
	var tooMuchJSON []string
	for i := range 40 {
		tooMuchJSON = append(tooMuchJSON, strings.Repeat(string(rune('一'+i)), 10000))
	}
	for name, paths := range map[string][]string{
		"too_many":     tooMany,
		"empty":        {""},
		"null_byte":    {"bad\x00.txt"},
		"invalid_utf8": {string([]byte{0xff})},
		"overlong":     {strings.Repeat("a", maxDesktopPathBytes+1)},
		"json_budget":  tooMuchJSON,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := store.SaveDraft(SendDraft{Paths: paths}, wd); !errors.Is(err, ErrMetadataInvalid) {
				t.Fatalf("invalid draft accepted: %v", err)
			} else if name == "json_budget" && !strings.Contains(err.Error(), "metadata limit") {
				t.Fatalf("JSON-budget fixture failed for a different reason: %v", err)
			}
		})
	}
	if _, err := store.SaveDraft(SendDraft{Paths: []string{"relative.txt"}}, ""); !errors.Is(err, ErrMetadataInvalid) {
		t.Fatal("relative path lost its sender working directory")
	}
	if runtime.GOOS == "windows" {
		for _, path := range []string{`C:relative.txt`, `\ambiguous.txt`} {
			if _, err := store.SaveDraft(SendDraft{Paths: []string{path}}, wd); !errors.Is(err, ErrMetadataInvalid) {
				t.Fatalf("drive/root relative path accepted: %s %v", path, err)
			}
		}
		paths, err := normalizeDraftPaths([]string{filepath.Join(wd, "Name.txt"), filepath.Join(wd, "NAME.TXT")}, "")
		if err != nil || len(paths) != 1 {
			t.Fatalf("Windows duplicate path not folded: %v %v", paths, err)
		}
	}
}

func TestDesktopDeviceLastUseRevisionExhaustionIsReported(t *testing.T) {
	store, _ := newTestDesktopStore(t)
	if _, err := store.db.Exec(`INSERT INTO device_profiles(peer_id,revision) VALUES(?,?)`, metadataTestPeer, int64(math.MaxInt64)); err != nil {
		t.Fatal(err)
	}
	if err := store.TouchDevice(metadataTestPeer, time.Now()); !errors.Is(err, ErrMetadataConflict) {
		t.Fatalf("exhausted revision claimed a last-use write: %v", err)
	}
}

func TestDesktopMetadataWriteFailureNeverClaimsCommit(t *testing.T) {
	store, _ := newTestDesktopStore(t)
	profile, err := store.SaveDeviceProfile(DeviceProfile{PeerID: metadataTestPeer, Alias: "before"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.db.Exec("PRAGMA query_only=ON"); err != nil {
		t.Fatal(err)
	}
	profile.Alias = "must not persist"
	if saved, err := store.SaveDeviceProfile(profile); err == nil || saved.Revision != 0 {
		t.Fatalf("readonly DB claimed saved profile: %+v err=%v", saved, err)
	}
	if saved, err := store.SaveDraft(SendDraft{}, ""); err == nil || saved.Revision != 0 {
		t.Fatalf("readonly DB claimed saved draft: %+v err=%v", saved, err)
	}
	if err = store.TouchDevice(metadataTestPeer, time.Now()); err == nil {
		t.Fatal("readonly DB claimed real-use update")
	}
	got, err := store.DeviceProfile(metadataTestPeer)
	if err != nil || got.Alias != "before" || got.Revision != 1 {
		t.Fatalf("failed transaction changed stored profile: %+v err=%v", got, err)
	}
}

func TestDesktopQueueSchemaEnforcesRequestIdentityAndJSON(t *testing.T) {
	store, _ := newTestDesktopStore(t)
	insert := `INSERT INTO send_queue(id,request_id,peer_id,source_paths,state,position,revision,created_at,updated_at) VALUES(?,?,?,?,'needs_attention',0,1,?,?)`
	now := time.Now().UTC().Format(desktopTimestamp)
	if _, err := store.db.Exec(insert, "one", "request-one", metadataTestPeer, `[]`, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(insert, "two", "request-one", metadataTestPeer, `[]`, now, now); err == nil {
		t.Fatal("duplicate queue idempotency key accepted")
	}
	if _, err := store.db.Exec(insert, "bad", "request-bad", metadataTestPeer, `{}`, now, now); err == nil {
		t.Fatal("non-array source metadata accepted")
	}
	var count, waiting int
	var reason string
	if err := store.db.QueryRow("SELECT count(*),wait_for_peer,last_error FROM send_queue").Scan(&count, &waiting, &reason); err != nil || count != 1 || waiting != 0 || reason != "" {
		t.Fatalf("queue defaults/count mismatch: %d %d %q err=%v", count, waiting, reason, err)
	}
	rows, err := store.db.Query("PRAGMA foreign_key_list(send_queue)")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	if rows.Next() {
		t.Fatal("queue unexpectedly coupled to deletable task history")
	}
}
