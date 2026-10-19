package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wen5555/LinkSend/internal/identity"
	"github.com/Wen5555/LinkSend/internal/sharegrant"
)

var shareNow = time.Unix(1792368000, 0)

func grantKey(t *testing.T) sharegrant.Key {
	t.Helper()
	id, err := identity.Generate()
	if err != nil {
		t.Fatal(err)
	}
	return sharegrant.Key(id.PublicKey())
}
func newShare(t *testing.T, id string, sender sharegrant.Key) (sharegrant.Record, string) {
	t.Helper()
	code, digest, err := sharegrant.GenerateCode()
	if err != nil {
		t.Fatal(err)
	}
	r, err := sharegrant.New(id, sender, sharegrant.Digest{8}, digest, shareNow)
	if err != nil {
		t.Fatal(err)
	}
	return r, code
}
func shareStore(t *testing.T) (*Control, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "control.sqlite")
	s, err := OpenControl(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.EnableShareGrants(context.Background()); err != nil {
		s.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s, path
}
func readStoredShare(t *testing.T, s *Control, id string) sharegrant.Record {
	t.Helper()
	r, err := scanShare(s.db.QueryRow(shareSelect+"WHERE id=?", id))
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func preparedBound(t *testing.T, s *Control, id string) (sharegrant.Record, sharegrant.Key, sharegrant.Key) {
	t.Helper()
	ctx := context.Background()
	sender, receiver := grantKey(t), grantKey(t)
	r, code := newShare(t, id, sender)
	r, err := s.CreateShare(ctx, r, shareNow)
	if err != nil {
		t.Fatal(err)
	}
	r, err = s.ClaimShare(ctx, receiver, code, "22222222222222222222222222222222", shareNow.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	r, err = s.ApproveShare(ctx, id, sender, r.ClaimID, "33333333333333333333333333333333", "44444444444444444444444444444444", r.Generation, shareNow.Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	r, err = s.BindShare(ctx, id, receiver, r.TaskID, r.Manifest, sharegrant.Digest{9}, r.Generation, shareNow.Add(3*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	return r, sender, receiver
}

func TestShareSchemaBackupContainsCommittedMembershipAndReopens(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "control.sqlite")
	s, err := OpenControl(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	a, b := grantKey(t), grantKey(t)
	left, err := s.Initialize(ctx, "sender", a[:])
	if err != nil {
		t.Fatal(err)
	}
	right, err := s.Initialize(ctx, "receiver", b[:])
	if err != nil {
		t.Fatal(err)
	}
	if left.GroupID == right.GroupID {
		t.Fatal("fixture does not have independent groups")
	}
	backup, err := s.EnableShareGrants(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = verifyControlBackup(backup); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(backup)
	if err != nil {
		t.Fatal(err)
	}
	old, err := sql.Open("sqlite", backup)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = old.Exec("PRAGMA query_only=ON"); err != nil {
		t.Fatal(err)
	}
	var version, count int
	if err = old.QueryRow("SELECT version FROM schema_version").Scan(&version); err != nil || version != 4 {
		t.Fatalf("backup schema: %d %v", version, err)
	}
	if err = old.QueryRow("SELECT count(*) FROM devices").Scan(&count); err != nil || count != 2 {
		t.Fatalf("backup missing WAL data: %d %v", count, err)
	}
	old.Close()
	if again, e := s.EnableShareGrants(ctx); e != nil || again != "" {
		t.Fatal("repeat migration created another backup", e)
	}
	after, err := os.ReadFile(backup)
	if err != nil || string(before) != string(after) {
		t.Fatal("backup overwritten")
	}
	r, code := newShare(t, "11111111111111111111111111111111", a)
	if _, err = s.CreateShare(ctx, r, shareNow); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ClaimShare(ctx, b, code, "22222222222222222222222222222222", shareNow.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	for _, device := range []Device{left, right} {
		got, e := s.Device(ctx, device.ID)
		if e != nil || got.GroupID != device.GroupID || got.MembershipRevision != device.MembershipRevision {
			t.Fatal("temporary claimant changed group", e)
		}
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = OpenControl(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	stored := readStoredShare(t, s, r.ID)
	if stored.State != sharegrant.Pending || stored.Receiver != b {
		t.Fatal("persistent claim lost")
	}
	for _, suffix := range []string{"", "-wal"} {
		raw, e := os.ReadFile(path + suffix)
		if e == nil && strings.Contains(string(raw), code) {
			t.Fatal("plaintext share code in database")
		}
	}
}

func TestShareMigrationCancellationAndFutureSchemaFailClosed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "control.sqlite")
	s, err := OpenControl(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = s.EnableShareGrants(ctx); err == nil {
		t.Fatal("cancelled migration succeeded")
	}
	var version int
	if err = s.db.QueryRow("SELECT version FROM schema_version").Scan(&version); err != nil || version != 4 {
		t.Fatal("cancelled migration changed schema")
	}
	if _, err = s.db.Exec("UPDATE schema_version SET version=5"); err != nil {
		t.Fatal(err)
	}
	s.Close()
	if opened, e := OpenControl(path); e == nil {
		opened.Close()
		t.Fatal("accepted schema5 without grant table")
	}
}

func TestShareAtomicClaimHasOneWinnerAndNoMembershipWrites(t *testing.T) {
	s, _ := shareStore(t)
	ctx := context.Background()
	r, code := newShare(t, "11111111111111111111111111111111", grantKey(t))
	if _, err := s.CreateShare(ctx, r, shareNow); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	winners := make(chan sharegrant.Record, 32)
	failures := make(chan error, 32)
	for i := 0; i < 32; i++ {
		key := grantKey(t)
		wg.Add(1)
		go func(i int, key sharegrant.Key) {
			defer wg.Done()
			got, e := s.ClaimShare(ctx, key, code, fmt.Sprintf("%032x", i+1), shareNow.Add(time.Second))
			if e == nil {
				winners <- got
			} else {
				failures <- e
			}
		}(i, key)
	}
	wg.Wait()
	close(winners)
	close(failures)
	if len(winners) != 1 || len(failures) != 31 {
		t.Fatalf("claims: winners=%d failures=%d", len(winners), len(failures))
	}
	winner := <-winners
	stored := readStoredShare(t, s, r.ID)
	if stored != winner || stored.Revision != 2 {
		t.Fatal("loser mutated winner")
	}
	for err := range failures {
		if !errors.Is(err, sharegrant.ErrConflict) {
			t.Fatalf("unexpected competing claim failure: %v", err)
		}
	}
	var devices, groups int
	if err := s.db.QueryRow("SELECT count(*) FROM devices").Scan(&devices); err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRow("SELECT count(*) FROM groups").Scan(&groups); err != nil {
		t.Fatal(err)
	}
	if devices != 0 || groups != 0 {
		t.Fatal("temporary share manufactured membership")
	}
}

func TestShareCreateRetryCannotExtendOrReplaceOffer(t *testing.T) {
	s, _ := shareStore(t)
	ctx := context.Background()
	r, _ := newShare(t, "11111111111111111111111111111111", grantKey(t))
	if _, err := s.CreateShare(ctx, r, shareNow); err != nil {
		t.Fatal(err)
	}
	got, err := s.CreateShare(ctx, r, shareNow.Add(time.Minute))
	if err != nil || got != r {
		t.Fatal("create retry not idempotent", err)
	}
	for _, change := range []func(*sharegrant.Record){func(r *sharegrant.Record) { r.Sender = grantKey(t) }, func(r *sharegrant.Record) { r.Manifest = sharegrant.Digest{99} }, func(r *sharegrant.Record) { r.CodeDigest = sharegrant.Digest{99} }} {
		altered := r
		change(&altered)
		if _, e := s.CreateShare(ctx, altered, shareNow); e == nil {
			t.Fatal("same ID replaced grant")
		}
		if readStoredShare(t, s, r.ID) != r {
			t.Fatal("failed create altered grant")
		}
	}
}

func TestSharePersistentResumePinsAndTerminalRace(t *testing.T) {
	s, path := shareStore(t)
	ctx := context.Background()
	r, sender, receiver := preparedBound(t, s, "11111111111111111111111111111111")
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	var err error
	s, err = OpenControl(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	now := shareNow.Add(time.Hour)
	if _, err = s.AuthorizeShare(ctx, r.ID, receiver, r.TaskID, r.Manifest, r.Plan, r.Generation, now); err != nil {
		t.Fatal("store restart lost valid original resume", err)
	}
	if _, err = s.AuthorizeShare(ctx, r.ID, grantKey(t), r.TaskID, r.Manifest, r.Plan, r.Generation, now); err == nil {
		t.Fatal("wrong public key authorized")
	}
	if _, err = s.AuthorizeShare(ctx, r.ID, receiver, r.TaskID, sharegrant.Digest{99}, r.Plan, r.Generation, now); err == nil {
		t.Fatal("changed manifest authorized")
	}
	if _, err = s.AuthorizeShare(ctx, r.ID, receiver, r.TaskID, r.Manifest, r.Plan, r.Generation, time.Unix(r.ResumeUntil, 0)); err == nil {
		t.Fatal("expired resume authorized")
	}
	if readStoredShare(t, s, r.ID) != r {
		t.Fatal("failed resume altered grant")
	}
	results := make(chan error, 2)
	go func() { _, e := s.RevokeShare(ctx, r.ID, sender, now); results <- e }()
	go func() {
		_, e := s.CompleteShare(ctx, r.ID, receiver, r.TaskID, r.Manifest, r.Plan, r.Generation, now)
		results <- e
	}()
	passed := 0
	for i := 0; i < 2; i++ {
		if e := <-results; e == nil {
			passed++
		} else if !errors.Is(e, sharegrant.ErrTerminal) {
			t.Fatal(e)
		}
	}
	if passed != 1 {
		t.Fatalf("terminal race winners=%d", passed)
	}
	final := readStoredShare(t, s, r.ID)
	if !final.Terminal() || final.Revision != r.Revision+1 {
		t.Fatal("terminal race lost atomicity")
	}
	if _, err = s.AuthorizeShare(ctx, r.ID, receiver, r.TaskID, r.Manifest, r.Plan, r.Generation, now); err == nil {
		t.Fatal("terminal record authorized new file action")
	}
}

func TestShareQuotasAndBoundedReconciliationProtectLiveRecords(t *testing.T) {
	s, _ := shareStore(t)
	ctx := context.Background()
	sender, receiver := grantKey(t), grantKey(t)
	records := make([]sharegrant.Record, 8)
	codes := make([]string, 8)
	for i := range records {
		r, code := newShare(t, fmt.Sprintf("%032x", i+1), sender)
		if _, e := s.CreateShare(ctx, r, shareNow); e != nil {
			t.Fatal(e)
		}
		records[i] = r
		codes[i] = code
	}
	extra, _ := newShare(t, "99999999999999999999999999999999", sender)
	if _, e := s.CreateShare(ctx, extra, shareNow); !errors.Is(e, sharegrant.ErrConflict) {
		t.Fatal("sender quota bypass", e)
	}
	for i := 0; i < 4; i++ {
		if _, e := s.ClaimShare(ctx, receiver, codes[i], fmt.Sprintf("%032x", i+20), shareNow.Add(time.Second)); e != nil {
			t.Fatal(e)
		}
	}
	before := readStoredShare(t, s, records[4].ID)
	if _, e := s.ClaimShare(ctx, receiver, codes[4], "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", shareNow.Add(time.Second)); !errors.Is(e, sharegrant.ErrConflict) {
		t.Fatal("pending identity quota bypass", e)
	}
	if readStoredShare(t, s, before.ID) != before {
		t.Fatal("rejected quota consumed offer")
	}
	count, err := s.ReconcileShareGrants(ctx, shareNow.Add(3*time.Minute), 1)
	if err != nil || count != 1 {
		t.Fatalf("scan budget count=%d err=%v", count, err)
	}
	var pending int
	if err = s.db.QueryRow("SELECT count(*) FROM share_grants_v1 WHERE state='pending'").Scan(&pending); err != nil || pending != 3 {
		t.Fatal("reconciler exceeded budget")
	}
	if _, err = s.ReconcileShareGrants(ctx, shareNow.Add(3*time.Minute), 65); err == nil {
		t.Fatal("unbounded reconciliation accepted")
	}
	for i := 4; i < 8; i++ {
		if readStoredShare(t, s, records[i].ID) != records[i] {
			t.Fatal("reconciliation changed unrelated live offer")
		}
	}
	if _, err = s.ReconcileShareGrants(ctx, shareNow.Add(11*time.Minute), 64); err != nil {
		t.Fatal(err)
	}
	var total int
	if err = s.db.QueryRow("SELECT count(*) FROM share_grants_v1").Scan(&total); err != nil || total != 8 {
		t.Fatal("fresh terminal tombstones removed")
	}
	if _, err = s.ReconcileShareGrants(ctx, shareNow.Add(8*24*time.Hour), 64); err != nil {
		t.Fatal(err)
	}
	if err = s.db.QueryRow("SELECT count(*) FROM share_grants_v1").Scan(&total); err != nil || total != 0 {
		t.Fatal("old terminal tombstones not reclaimed")
	}
}

func TestShareCorruptedStorageCannotGrantAccess(t *testing.T) {
	s, _ := shareStore(t)
	r, _, receiver := preparedBound(t, s, "11111111111111111111111111111111")
	ctx := context.Background()
	if _, err := s.db.Exec("UPDATE share_grants_v1 SET state='offered' WHERE id=?", r.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AuthorizeShare(ctx, r.ID, receiver, r.TaskID, r.Manifest, r.Plan, r.Generation, shareNow.Add(time.Minute)); !errors.Is(err, sharegrant.ErrInvalid) {
		t.Fatal("corrupt index authorized", err)
	}
	raw, _ := json.Marshal(r)
	raw = []byte(strings.Replace(string(raw), `"version":1`, `"version":999`, 1))
	if _, err := s.db.Exec("UPDATE share_grants_v1 SET state=?,record=? WHERE id=?", string(r.State), raw, r.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RevokeShare(ctx, r.ID, r.Sender, shareNow.Add(time.Minute)); !errors.Is(err, sharegrant.ErrInvalid) {
		t.Fatal("unknown stored version rewritten", err)
	}
	var actual []byte
	if err := s.db.QueryRow("SELECT record FROM share_grants_v1 WHERE id=?", r.ID).Scan(&actual); err != nil || string(actual) != string(raw) {
		t.Fatal("unknown record not preserved")
	}
}

// Keep a direct old-format fixture: migration must not require a v5-created
// database, and malformed/future versions must fail before granting anything.
func TestShareV4OptInDoesNotAffectOrdinaryOpen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v4.sqlite")
	s, err := OpenControl(path)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	var version int
	err = db.QueryRow("SELECT version FROM schema_version").Scan(&version)
	db.Close()
	if err != nil || version != 4 {
		t.Fatal("ordinary server silently activated shares")
	}
}
