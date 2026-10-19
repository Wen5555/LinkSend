package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"time"

	"github.com/Wen5555/LinkSend/internal/identity"
	"github.com/Wen5555/LinkSend/internal/sharegrant"
)

const shareSchemaSQL = `CREATE TABLE share_grants_v1(
 id TEXT PRIMARY KEY, code_hash BLOB NOT NULL UNIQUE, sender_id TEXT NOT NULL,
 receiver_id TEXT NOT NULL, state TEXT NOT NULL, revision INTEGER NOT NULL,
 expires_at INTEGER NOT NULL, claim_until INTEGER NOT NULL, resume_until INTEGER NOT NULL,
 finished_at INTEGER NOT NULL, record BLOB NOT NULL CHECK(length(record)<=8192));
 CREATE INDEX share_grants_sender ON share_grants_v1(sender_id,state);
 CREATE INDEX share_grants_receiver ON share_grants_v1(receiver_id,state,claim_until);
 CREATE INDEX share_grants_expiry ON share_grants_v1(state,expires_at,claim_until,resume_until,finished_at);`

// EnableShareGrants is an explicit staged activation. No current server route
// calls it or advertises the capability. It borrows the existing connection.
// A v4 service refuses schema5; rolling a program back must not roll data back.
func (s *Control) EnableShareGrants(ctx context.Context) (string, error) {
	var version int
	if err := s.db.QueryRowContext(ctx, "SELECT version FROM schema_version").Scan(&version); err != nil {
		return "", err
	}
	if version == 5 {
		return "", verifyShareSchema(s.db)
	}
	if version != 4 {
		return "", sharegrant.ErrInvalid
	}
	var sequence int
	var name, path string
	if err := s.db.QueryRowContext(ctx, "PRAGMA database_list").Scan(&sequence, &name, &path); err != nil {
		return "", err
	}
	backup := ""
	if path != "" {
		nonce, err := randomIncarnation()
		if err != nil {
			return "", err
		}
		backup = path + ".pre-share-v5-" + nonce + ".sqlite"
		file, err := os.OpenFile(backup, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return "", err
		}
		if err = file.Close(); err != nil {
			return backup, err
		}
		if _, err = s.db.ExecContext(ctx, "VACUUM INTO ?", backup); err != nil {
			return backup, err
		}
		if err = verifyControlBackup(backup); err != nil {
			return backup, err
		}
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return backup, err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, shareSchemaSQL); err != nil {
		return backup, err
	}
	result, err := tx.ExecContext(ctx, "UPDATE schema_version SET version=5 WHERE version=4")
	if err != nil {
		return backup, err
	}
	if count, e := result.RowsAffected(); e != nil || count != 1 {
		return backup, sharegrant.ErrConflict
	}
	if err = tx.Commit(); err != nil {
		return backup, err
	}
	return backup, verifyShareSchema(s.db)
}
func verifyControlBackup(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() == 0 {
		return sharegrant.ErrInvalid
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if _, err = db.Exec("PRAGMA query_only=ON"); err != nil {
		return err
	}
	var integrity string
	var version int
	if err = db.QueryRow("PRAGMA integrity_check").Scan(&integrity); err != nil {
		return err
	}
	if integrity != "ok" {
		return sharegrant.ErrInvalid
	}
	if err = db.QueryRow("SELECT version FROM schema_version").Scan(&version); err != nil {
		return err
	}
	if version != 4 {
		return sharegrant.ErrInvalid
	}
	var count int
	return db.QueryRow("SELECT count(*) FROM devices").Scan(&count)
}
func verifyShareSchema(db *sql.DB) error {
	var version int
	var integrity string
	if err := db.QueryRow("SELECT version FROM schema_version").Scan(&version); err != nil {
		return err
	}
	if version != 5 {
		return sharegrant.ErrInvalid
	}
	if err := db.QueryRow("PRAGMA quick_check").Scan(&integrity); err != nil {
		return err
	}
	if integrity != "ok" {
		return sharegrant.ErrInvalid
	}
	rows, err := db.Query("SELECT id,code_hash,sender_id,receiver_id,state,revision,expires_at,claim_until,resume_until,finished_at,record FROM share_grants_v1 LIMIT 0")
	if err != nil {
		return err
	}
	return rows.Close()
}

func keyID(key sharegrant.Key) string {
	if key == (sharegrant.Key{}) {
		return ""
	}
	return identity.DeviceID(key[:])
}

const shareSelect = "SELECT id,code_hash,sender_id,receiver_id,state,revision,expires_at,claim_until,resume_until,finished_at,CASE WHEN length(record)<=8192 THEN record ELSE NULL END FROM share_grants_v1 "

func scanShare(row rowScanner) (sharegrant.Record, error) {
	var id, sender, receiver, state string
	var code, raw []byte
	var revision uint64
	var expires, claim, resume, finished int64
	if err := row.Scan(&id, &code, &sender, &receiver, &state, &revision, &expires, &claim, &resume, &finished, &raw); err != nil {
		return sharegrant.Record{}, err
	}
	r, err := sharegrant.Decode(raw)
	if err != nil {
		return r, err
	}
	if id != r.ID || string(code) != string(r.CodeDigest[:]) || sender != keyID(r.Sender) || receiver != keyID(r.Receiver) || state != string(r.State) || revision != r.Revision || expires != r.ExpiresAt || claim != r.ClaimUntil || resume != r.ResumeUntil || finished != r.FinishedAt {
		return sharegrant.Record{}, sharegrant.ErrInvalid
	}
	return r, nil
}

// CreateShare takes only the client's verifier. It never receives the code.
// The sender key is a verified identity supplied by a future signed handler.
func (s *Control) CreateShare(ctx context.Context, r sharegrant.Record, now time.Time) (sharegrant.Record, error) {
	if err := r.Validate(); err != nil {
		return sharegrant.Record{}, err
	}
	if r.State != sharegrant.Offered || r.Revision != 1 || r.Generation != 1 {
		return sharegrant.Record{}, sharegrant.ErrInvalid
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return sharegrant.Record{}, err
	}
	defer tx.Rollback()
	old, err := scanShare(tx.QueryRowContext(ctx, shareSelect+"WHERE id=?", r.ID))
	if err == nil {
		if old.Sender != r.Sender || old.Manifest != r.Manifest || old.CodeDigest != r.CodeDigest {
			return sharegrant.Record{}, sharegrant.ErrConflict
		}
		return old, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return sharegrant.Record{}, err
	}
	if r.CreatedAt != now.Unix() {
		return sharegrant.Record{}, sharegrant.ErrInvalid
	}
	var total, active int
	if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM share_grants_v1").Scan(&total); err != nil {
		return sharegrant.Record{}, err
	}
	if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM share_grants_v1 WHERE sender_id=? AND state NOT IN ('completed','revoked','expired')`, keyID(r.Sender)).Scan(&active); err != nil {
		return sharegrant.Record{}, err
	}
	if total >= 4096 || active >= 8 {
		return sharegrant.Record{}, sharegrant.ErrConflict
	}
	raw, err := json.Marshal(r)
	if err != nil {
		return sharegrant.Record{}, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO share_grants_v1(id,code_hash,sender_id,receiver_id,state,revision,expires_at,claim_until,resume_until,finished_at,record) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, r.ID, r.CodeDigest[:], keyID(r.Sender), "", string(r.State), r.Revision, r.ExpiresAt, 0, 0, 0, raw)
	if err != nil {
		return sharegrant.Record{}, err
	}
	return r, tx.Commit()
}

func updateShare(ctx context.Context, tx *sql.Tx, old, next sharegrant.Record) error {
	if err := next.Validate(); err != nil {
		return err
	}
	if old == next {
		return nil
	}
	if next.ID != old.ID || next.Sender != old.Sender || next.CodeDigest != old.CodeDigest || next.Manifest != old.Manifest || next.CreatedAt != old.CreatedAt || next.ExpiresAt != old.ExpiresAt || old.Revision == ^uint64(0) || next.Revision != old.Revision+1 {
		return sharegrant.ErrInvalid
	}
	raw, err := json.Marshal(next)
	if err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `UPDATE share_grants_v1 SET receiver_id=?,state=?,revision=?,claim_until=?,resume_until=?,finished_at=?,record=? WHERE id=? AND revision=?`, keyID(next.Receiver), string(next.State), next.Revision, next.ClaimUntil, next.ResumeUntil, next.FinishedAt, raw, old.ID, old.Revision)
	if err != nil {
		return err
	}
	if count, e := result.RowsAffected(); e != nil || count != 1 {
		return sharegrant.ErrConflict
	}
	return nil
}

func (s *Control) ClaimShare(ctx context.Context, actor sharegrant.Key, code, claimID string, now time.Time) (sharegrant.Record, error) {
	digest, err := sharegrant.CodeDigest(code)
	if err != nil {
		return sharegrant.Record{}, sharegrant.ErrRejected
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return sharegrant.Record{}, err
	}
	defer tx.Rollback()
	old, err := scanShare(tx.QueryRowContext(ctx, shareSelect+"WHERE code_hash=?", digest[:]))
	if errors.Is(err, sql.ErrNoRows) {
		return sharegrant.Record{}, sharegrant.ErrRejected
	}
	if err != nil {
		return sharegrant.Record{}, err
	}
	next, err := old.Claim(actor, code, claimID, now)
	if err != nil {
		return sharegrant.Record{}, err
	}
	if next != old {
		var count int
		if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM share_grants_v1 WHERE receiver_id=? AND state='pending' AND claim_until>? AND expires_at>?`, keyID(actor), now.Unix(), now.Unix()).Scan(&count); err != nil {
			return sharegrant.Record{}, err
		}
		if count >= 4 {
			return sharegrant.Record{}, sharegrant.ErrConflict
		}
	}
	if err = updateShare(ctx, tx, old, next); err != nil {
		return sharegrant.Record{}, err
	}
	return next, tx.Commit()
}

func (s *Control) mutateShare(ctx context.Context, id string, transition func(sharegrant.Record) (sharegrant.Record, error)) (sharegrant.Record, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return sharegrant.Record{}, err
	}
	defer tx.Rollback()
	old, err := scanShare(tx.QueryRowContext(ctx, shareSelect+"WHERE id=?", id))
	if err != nil {
		return sharegrant.Record{}, err
	}
	next, err := transition(old)
	if err != nil {
		return sharegrant.Record{}, err
	}
	if err = updateShare(ctx, tx, old, next); err != nil {
		return sharegrant.Record{}, err
	}
	return next, tx.Commit()
}
func (s *Control) ApproveShare(ctx context.Context, id string, actor sharegrant.Key, claim, approval, task string, generation uint64, now time.Time) (sharegrant.Record, error) {
	return s.mutateShare(ctx, id, func(r sharegrant.Record) (sharegrant.Record, error) {
		return r.Approve(actor, claim, approval, task, generation, now)
	})
}
func (s *Control) DenyShare(ctx context.Context, id string, actor sharegrant.Key, claim string, generation uint64, now time.Time) (sharegrant.Record, error) {
	return s.mutateShare(ctx, id, func(r sharegrant.Record) (sharegrant.Record, error) { return r.Deny(actor, claim, generation, now) })
}
func (s *Control) BindShare(ctx context.Context, id string, actor sharegrant.Key, task string, manifest, plan sharegrant.Digest, generation uint64, now time.Time) (sharegrant.Record, error) {
	return s.mutateShare(ctx, id, func(r sharegrant.Record) (sharegrant.Record, error) {
		return r.Bind(actor, task, manifest, plan, generation, now)
	})
}
func (s *Control) CompleteShare(ctx context.Context, id string, actor sharegrant.Key, task string, manifest, plan sharegrant.Digest, generation uint64, now time.Time) (sharegrant.Record, error) {
	return s.mutateShare(ctx, id, func(r sharegrant.Record) (sharegrant.Record, error) {
		return r.Complete(actor, task, manifest, plan, generation, now)
	})
}
func (s *Control) RevokeShare(ctx context.Context, id string, actor sharegrant.Key, now time.Time) (sharegrant.Record, error) {
	return s.mutateShare(ctx, id, func(r sharegrant.Record) (sharegrant.Record, error) { return r.Revoke(actor, now) })
}
func (s *Control) AuthorizeShare(ctx context.Context, id string, actor sharegrant.Key, task string, manifest, plan sharegrant.Digest, generation uint64, now time.Time) (sharegrant.Record, error) {
	r, err := scanShare(s.db.QueryRowContext(ctx, shareSelect+"WHERE id=?", id))
	if err != nil {
		return sharegrant.Record{}, err
	}
	if err = r.Authorize(actor, task, manifest, plan, generation, now); err != nil {
		return sharegrant.Record{}, err
	}
	return r, nil
}

// ReconcileShareGrants has no independent worker. The existing server owner
// will call it when the feature is enabled. It never deletes a live grant.
func (s *Control) ReconcileShareGrants(ctx context.Context, now time.Time, limit int) (int, error) {
	if limit < 1 || limit > 64 {
		return 0, sharegrant.ErrInvalid
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, shareSelect+`WHERE (state IN ('offered','pending','approved') AND expires_at<=?) OR (state='pending' AND claim_until<=?) OR (state='bound' AND resume_until<=?) LIMIT ?`, now.Unix(), now.Unix(), now.Unix(), limit)
	if err != nil {
		return 0, err
	}
	var records []sharegrant.Record
	for rows.Next() {
		r, e := scanShare(rows)
		if e != nil {
			rows.Close()
			return 0, e
		}
		records = append(records, r)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	if err = rows.Close(); err != nil {
		return 0, err
	}
	for _, old := range records {
		next, e := old.Reconcile(now)
		if e != nil {
			return 0, e
		}
		if e = updateShare(ctx, tx, old, next); e != nil {
			return 0, e
		}
	}
	// Completion/revocation tombstones are retained for seven full days. No
	// allocation pressure can delete an active or freshly terminal record.
	_, err = tx.ExecContext(ctx, `DELETE FROM share_grants_v1 WHERE id IN (SELECT id FROM share_grants_v1 WHERE state IN ('completed','revoked','expired') AND finished_at>0 AND finished_at<=? LIMIT ?)`, now.Add(-7*24*time.Hour).Unix(), limit-len(records))
	if err != nil {
		return 0, err
	}
	return len(records), tx.Commit()
}
