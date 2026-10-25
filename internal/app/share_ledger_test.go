package app

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Wen5555/LinkSend/internal/protocol"
	"github.com/Wen5555/LinkSend/internal/sharegrant"
	"github.com/Wen5555/LinkSend/internal/signaling"
)

func TestTemporaryShareLedgerBudgetIncludesRetiredHistory(t *testing.T) {
	f := newTemporaryShareFixture(t)
	created, err := f.a.CreateTemporaryShare(t.Context(), []string{f.source}, nil)
	if err != nil {
		t.Fatal(err)
	}
	e, _ := f.a.temporaryEntry(created.Share.ID)
	if err = f.a.stopTemporaryShare(e, true); err != nil {
		t.Fatal(err)
	}
	template := temporaryRecordCopy(e)
	for n := 1; n < maxTemporaryShareRecords; n++ {
		r := cloneTemporaryRecord(template)
		r.ID = protocol.RandomID()
		r.Status.ID = r.ID
		r.Revision = 1
		r.Intent = nil
		r.TaskRecordID = ""
		if _, err = f.a.insertTemporaryRecord(r); err != nil {
			t.Fatal("bounded retired record rejected early", n, err)
		}
	}
	r := cloneTemporaryRecord(template)
	r.ID = protocol.RandomID()
	r.Status.ID = r.ID
	r.Revision = 1
	r.Intent = nil
	if _, err = f.a.insertTemporaryRecord(r); protocol.ErrorCode(err) != protocol.RateLimited {
		t.Fatal("local ledger budget discarded history or admitted overflow", err)
	}
	var rows int
	if err = f.a.store.db.QueryRow(`SELECT count(*) FROM metadata WHERE key LIKE ?`, temporarySharePrefix+"%").Scan(&rows); err != nil || rows != 64 {
		t.Fatal("budget check deleted retired ownership rows", rows, err)
	}
}

func TestTemporaryShareActiveOwnerLimitsAndCASRejectLostOwnership(t *testing.T) {
	f := newTemporaryShareFixture(t)
	created, err := f.a.CreateTemporaryShare(t.Context(), []string{f.source}, nil)
	if err != nil {
		t.Fatal(err)
	}
	e, _ := f.a.temporaryEntry(created.Share.ID)
	template := temporaryRecordCopy(e)
	for n := 1; n < 8; n++ {
		r := cloneTemporaryRecord(template)
		r.ID = protocol.RandomID()
		r.Status.ID = r.ID
		r.Revision = 1
		if _, err = f.a.insertTemporaryRecord(r); err != nil {
			t.Fatal(err)
		}
	}
	r := cloneTemporaryRecord(template)
	r.ID = protocol.RandomID()
	r.Status.ID = r.ID
	r.Revision = 1
	if _, err = f.a.insertTemporaryRecord(r); protocol.ErrorCode(err) != protocol.RateLimited {
		t.Fatal("sender exceeded its eight active grants", err)
	}
	// An unsigned external DB edit cannot be overwritten by a cached owner.
	if _, err = f.a.store.db.Exec(`UPDATE metadata SET value='tampered' WHERE key=?`, temporarySharePrefix+created.Share.ID); err != nil {
		t.Fatal(err)
	}
	e.mu.Lock()
	next := cloneTemporaryRecord(e.record)
	next.State = "needs_attention"
	err = f.a.saveTemporaryRecord(e, next)
	e.mu.Unlock()
	if protocol.ErrorCode(err) != protocol.ShareConflict {
		t.Fatal("CAS accepted externally replaced ownership", err)
	}
}

func TestTemporaryShareReceiverPendingLimitAndPrivateRecordBounds(t *testing.T) {
	f := newTemporaryShareFixture(t)
	created, err := f.a.CreateTemporaryShare(t.Context(), []string{f.source}, nil)
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := f.b.ClaimTemporaryShare(t.Context(), created.Code)
	if err != nil {
		t.Fatal(err)
	}
	e, _ := f.b.temporaryEntry(claimed.ID)
	template := temporaryRecordCopy(e)
	for n := 1; n < 4; n++ {
		r := cloneTemporaryRecord(template)
		r.ID = protocol.RandomID()
		r.Revision = 1
		if _, err = f.b.insertTemporaryRecord(r); err != nil {
			t.Fatal(err)
		}
	}
	r := cloneTemporaryRecord(template)
	r.ID = protocol.RandomID()
	r.Revision = 1
	if _, err = f.b.insertTemporaryRecord(r); protocol.ErrorCode(err) != protocol.RateLimited {
		t.Fatal("receiver exceeded four pending claims", err)
	}
	if _, err = f.b.decodeTemporaryRecord(template.ID, strings.Repeat(" ", temporaryShareRecordLimit+1)); err == nil {
		t.Fatal("oversized private record was accepted")
	}
	r = cloneTemporaryRecord(template)
	r.Intent = &temporaryShareIntent{ID: protocol.RandomID(), Operation: signaling.ShareClaim, Request: signaling.ShareRequest{Code: created.Code, ClaimID: r.ClaimID}}
	if _, err = f.b.encodeTemporaryRecord(r); err == nil {
		t.Fatal("plaintext code could be serialized by a future caller")
	}
	r = cloneTemporaryRecord(template)
	r.Status.State = sharegrant.Offered
	r.Status.ExpiresAt = time.Now().Add(time.Minute).Unix()
	r.Owner = f.a.identity.ID()
	if _, err = f.b.encodeTemporaryRecord(r); err == nil {
		t.Fatal("another profile acquired this ownership record")
	}
}

func TestTemporaryShareRetiresOnlyConfirmedTerminalMetadataAfterRetention(t *testing.T) {
	f := newTemporaryShareFixture(t)
	clock := time.Now()
	f.a.temporaryShares.now = func() time.Time { return clock }
	created, err := f.a.CreateTemporaryShare(t.Context(), []string{f.source}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.a.RevokeTemporaryShare(t.Context(), created.Share.ID); err != nil {
		t.Fatal(err)
	}
	first, _ := f.a.temporaryEntry(created.Share.ID)
	template := temporaryRecordCopy(first)
	if template.TerminalObservedAt != clock.Unix() {
		t.Fatal("first real terminal observation was not persisted")
	}
	var historyBefore int
	if err = f.a.store.db.QueryRow(`SELECT count(*) FROM tasks`).Scan(&historyBefore); err != nil {
		t.Fatal(err)
	}
	ids := []string{template.ID}
	for n := 1; n < maxTemporaryShareRecords; n++ {
		r := cloneTemporaryRecord(template)
		r.ID = protocol.RandomID()
		r.Status.ID = r.ID
		r.Revision = 1
		if _, err = f.a.insertTemporaryRecord(r); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, r.ID)
	}
	mark := func(id string, change func(*temporaryShareEntry, *temporaryShareRecord)) {
		e, _ := f.a.temporaryEntry(id)
		e.mu.Lock()
		r := cloneTemporaryRecord(e.record)
		change(e, &r)
		if err := f.a.saveTemporaryRecord(e, r); err != nil {
			t.Fatal(err)
		}
		e.mu.Unlock()
	}
	mark(ids[0], func(e *temporaryShareEntry, _ *temporaryShareRecord) { e.active = true })
	mark(ids[1], func(_ *temporaryShareEntry, r *temporaryShareRecord) {
		r.Intent = temporaryIntent(signaling.ShareRevoke, signaling.ShareRequest{ID: r.Status.ID})
	})
	mark(ids[2], func(_ *temporaryShareEntry, r *temporaryShareRecord) { r.TerminalObservedAt = 0 })
	mark(ids[3], func(e *temporaryShareEntry, _ *temporaryShareRecord) { e.controlBusy = 1 })
	casID := ids[4]
	var replacement string
	f.a.temporaryShares.beforeRetire = func(id string) {
		if id == casID {
			r := cloneTemporaryRecord(f.a.temporaryShares.entries[id].record)
			r.Revision++
			replacement, err = f.a.encodeTemporaryRecord(r)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = f.a.store.db.Exec(`UPDATE metadata SET value=? WHERE key=?`, replacement, temporarySharePrefix+id); err != nil {
				t.Fatal(err)
			}
		}
	}
	clock = clock.Add(7*24*time.Hour - time.Second)
	r := cloneTemporaryRecord(template)
	r.ID = protocol.RandomID()
	r.Status.ID = r.ID
	r.Revision = 1
	if _, err = f.a.insertTemporaryRecord(r); protocol.ErrorCode(err) != protocol.RateLimited {
		t.Fatal("retention window was shortened", err)
	}
	clock = clock.Add(2 * time.Second)
	if _, err = f.a.insertTemporaryRecord(r); err != nil {
		t.Fatal("safe old terminal capacity was not reclaimed", err)
	}
	var rows, historyAfter int
	if err = f.a.store.db.QueryRow(`SELECT count(*) FROM metadata WHERE key LIKE ?`, temporarySharePrefix+"%").Scan(&rows); err != nil || rows != 6 {
		t.Fatal("retirement touched protected or missed safe private records", rows, err)
	}
	if err = f.a.store.db.QueryRow(`SELECT count(*) FROM tasks`).Scan(&historyAfter); err != nil || historyAfter != historyBefore {
		t.Fatal("private metadata retirement changed task history", err)
	}
	for _, id := range ids[:5] {
		var raw string
		if err = f.a.store.db.QueryRow(`SELECT value FROM metadata WHERE key=?`, temporarySharePrefix+id).Scan(&raw); err != nil {
			t.Fatal("protected private record was removed", id, err)
		}
		if id == casID && raw != replacement {
			t.Fatal("late CAS replacement was overwritten")
		}
	}
	if data, err := os.ReadFile(f.source); err != nil || string(data) != "one-time authenticated content" {
		t.Fatal("private metadata retirement changed user source", err)
	}
	first.mu.Lock()
	first.active = false
	first.mu.Unlock()
}
