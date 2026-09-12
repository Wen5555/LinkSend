package app

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/Wen5555/LinkSend/internal/protocol"
	_ "modernc.org/sqlite"
)

const taskStoreSchema = 3

// Individual revision-guarded rows avoid read/modify/write losses between
// instances. Recovery metadata is local-only and never crosses Wails or WSS.
func historyDB(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	fail := func(e error) (*sql.DB, error) { _ = db.Close(); return nil, e }
	if _, err = db.Exec("PRAGMA busy_timeout=5000"); err != nil {
		return fail(err)
	}
	var schema int
	if err = db.QueryRow("PRAGMA user_version").Scan(&schema); err != nil {
		return fail(err)
	}
	if schema < 0 || schema > taskStoreSchema {
		return fail(errors.New("TASK_STORE_VERSION"))
	}
	if schema > 0 && schema < taskStoreSchema {
		backupPath := fmt.Sprintf("%s.schema-v%d-%s.bak", path, schema, time.Now().UTC().Format("20060102T150405.000000000Z"))
		if _, err = db.Exec("VACUUM INTO ?", backupPath); err != nil {
			return fail(fmt.Errorf("TASK_STORE_BACKUP_FAILED: %w", err))
		}
		if err = os.Chmod(backupPath, 0600); err != nil {
			_ = os.Remove(backupPath)
			return fail(fmt.Errorf("TASK_STORE_BACKUP_FAILED: %w", err))
		}
		if err = verifyHistoryBackup(backupPath, schema); err != nil {
			return fail(fmt.Errorf("TASK_STORE_BACKUP_FAILED: %w", err))
		}
		// Explicitly flush the verified rollback file before a schema commit,
		// independent of the source connection's synchronous configuration.
		backup, openErr := os.OpenFile(backupPath, os.O_RDWR, 0)
		if openErr != nil {
			return fail(fmt.Errorf("TASK_STORE_BACKUP_FAILED: %w", openErr))
		}
		syncErr := backup.Sync()
		closeErr := backup.Close()
		if err = errors.Join(syncErr, closeErr); err != nil {
			return fail(fmt.Errorf("TASK_STORE_BACKUP_FAILED: %w", err))
		}
	}
	tx, err := db.Begin()
	if err != nil {
		return fail(err)
	}
	rollback := func(e error) (*sql.DB, error) {
		_ = tx.Rollback()
		return fail(e)
	}
	if _, err = tx.Exec("CREATE TABLE IF NOT EXISTS tasks (id TEXT PRIMARY KEY, revision INTEGER NOT NULL, snapshot BLOB NOT NULL)"); err != nil {
		return rollback(err)
	}
	if schema < 2 {
		hasRecovery, columnErr := tableHasColumn(tx, "tasks", "recovery")
		if columnErr != nil {
			return rollback(columnErr)
		}
		if !hasRecovery {
			if _, err = tx.Exec("ALTER TABLE tasks ADD COLUMN recovery BLOB"); err != nil {
				return rollback(err)
			}
		}
	}
	if _, err = tx.Exec(`CREATE TABLE IF NOT EXISTS task_quarantine (
		id TEXT NOT NULL,
		revision INTEGER NOT NULL,
		snapshot BLOB,
		recovery BLOB,
		reason TEXT NOT NULL,
		isolated_at TEXT NOT NULL
	)`); err != nil {
		return rollback(err)
	}
	if _, err = tx.Exec("CREATE TABLE IF NOT EXISTS metadata (key TEXT PRIMARY KEY, value TEXT NOT NULL)"); err != nil {
		return rollback(err)
	}
	if schema < 3 {
		if err = migrateDesktopMetadata(tx); err != nil {
			return rollback(err)
		}
	}
	if _, err = tx.Exec("INSERT INTO metadata(key,value) VALUES('schema_version',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", fmt.Sprint(taskStoreSchema)); err != nil {
		return rollback(err)
	}
	if _, err = tx.Exec(fmt.Sprintf("PRAGMA user_version=%d", taskStoreSchema)); err != nil {
		return rollback(err)
	}
	if err = tx.Commit(); err != nil {
		return fail(err)
	}
	return db, nil
}

// Verify the actual rollback file before any schema mutation. VACUUM INTO
// produces a consistent SQLite snapshot, including committed WAL contents.
func verifyHistoryBackup(path string, schema int) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() == 0 {
		return errors.New("backup is not a nonempty regular file")
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
	if err = db.QueryRow("PRAGMA integrity_check").Scan(&integrity); err != nil {
		return err
	}
	if integrity != "ok" {
		return errors.New("backup integrity check failed")
	}
	var gotSchema, taskCount int
	if err = db.QueryRow("PRAGMA user_version").Scan(&gotSchema); err != nil {
		return err
	}
	if gotSchema != schema {
		return errors.New("backup schema does not match source")
	}
	return db.QueryRow("SELECT count(*) FROM tasks").Scan(&taskCount)
}

func tableHasColumn(tx *sql.Tx, table, column string) (bool, error) {
	rows, err := tx.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var cid, notNull, pk int
		var name, kind string
		var defaultValue any
		if err = rows.Scan(&cid, &name, &kind, &notNull, &defaultValue, &pk); err != nil {
			return false, err
		}
		if name == column {
			return true, nil
		}
	}
	return false, rows.Err()
}

type storedTask struct {
	id       string
	revision uint64
	snapshot []byte
	recovery []byte
}

func (m *taskManager) historyAvailable() bool {
	m.historyMu.RLock()
	defer m.historyMu.RUnlock()
	return m.historyPath != "" && m.historyErr == nil
}

func (m *taskManager) historyError() error {
	m.historyMu.RLock()
	defer m.historyMu.RUnlock()
	return m.historyErr
}

func (m *taskManager) setHistoryError(err error) {
	if err == nil {
		return
	}
	m.historyMu.Lock()
	if m.historyErr == nil {
		m.historyErr = err
	}
	m.historyMu.Unlock()
}

func (m *taskManager) configureHistory(path string) {
	m.historyPath = path
	db, err := historyDB(path)
	if err != nil {
		m.setHistoryError(err)
		return
	}
	defer db.Close()
	rows, err := db.Query("SELECT id,revision,snapshot,recovery FROM tasks ORDER BY id")
	if err != nil {
		m.setHistoryError(err)
		return
	}
	var stored []storedTask
	for rows.Next() {
		var item storedTask
		if scanErr := rows.Scan(&item.id, &item.revision, &item.snapshot, &item.recovery); scanErr != nil {
			continue
		}
		stored = append(stored, item)
	}
	rowsErr := rows.Err()
	_ = rows.Close()
	if rowsErr != nil {
		m.setHistoryError(rowsErr)
		return
	}
	for _, item := range stored {
		var snap TaskSnapshot
		var recovery taskRecovery
		if len(item.snapshot) > 1<<20 || json.Unmarshal(item.snapshot, &snap) != nil || snap.ID == "" || snap.ID != item.id || snap.Revision != item.revision {
			if err = quarantineTask(db, item, "INVALID_SNAPSHOT"); err != nil {
				m.setHistoryError(err)
				return
			}
			continue
		}
		if len(item.recovery) > 2<<20 || (len(item.recovery) != 0 && json.Unmarshal(item.recovery, &recovery) != nil) {
			if err = quarantineTask(db, item, "INVALID_RECOVERY_METADATA"); err != nil {
				m.setHistoryError(err)
				return
			}
			continue
		}
		if snap.TaskID == "" {
			snap.TaskID = snap.ID
		}
		if snap.AttemptID == "" {
			snap.AttemptID = protocol.RandomID()
		}
		recoverable := recoveryUsable(recovery)
		if !isTerminal(snap.State) {
			snap.Revision++
			if recoverable {
				snap.State = "recovering"
				snap.Phase = "restart_recovery_available"
				snap.ErrorCode = string(protocol.ConnectionInterrupted)
				snap.ErrorMessage = "上次传输因应用退出而中断；请核对双方身份和文件后明确恢复。"
				snap.EndedAt = ""
			} else {
				snap.State = "failed"
				snap.Phase = "interrupted"
				snap.ErrorCode = "TASK_INTERRUPTED"
				snap.ErrorMessage = "上次传输因应用退出而中断，且没有完整恢复元数据。"
				snap.EndedAt = time.Now().UTC().Format(time.RFC3339Nano)
			}
		}
		// A historical session is never a live connectivity claim.
		snap.CanCancel = recoverable && snap.State == "recovering"
		snap.CanRetry = false
		snap.CanPause = false
		snap.CanResume = recoverable && snap.State == "recovering"
		clearDirectEvidence(&snap)
		snap.SessionID = ""
		snap.ICEGeneration = 0
		snap.RateBytesPerSecond = nil
		snap.HistoryPersisted = true
		snap.RestartRecoverySupported = recoverable
		snap.ByteResumeSupported = recoverable
		record := &taskRecord{snap: snap, recovery: recovery, peerID: recovery.PeerID, paths: append([]string(nil), recovery.SourcePaths...), persist: m.persistRecordTracked}
		m.tasks[snap.ID] = record
		if err = upsertTask(db, snap, recovery); err != nil {
			m.setHistoryError(err)
			return
		}
	}
}

func quarantineTask(db *sql.DB, item storedTask, reason string) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	if _, err = tx.Exec("INSERT INTO task_quarantine(id,revision,snapshot,recovery,reason,isolated_at) VALUES(?,?,?,?,?,?)", item.id, item.revision, item.snapshot, item.recovery, reason, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		_ = tx.Rollback()
		return err
	}
	if _, err = tx.Exec("DELETE FROM tasks WHERE id=? AND revision=?", item.id, item.revision); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func upsertTask(db *sql.DB, snap TaskSnapshot, recovery taskRecovery) error {
	snapshotData, err := json.Marshal(snap)
	if err != nil {
		return err
	}
	recoveryData, err := json.Marshal(recovery)
	if err != nil {
		return err
	}
	_, err = db.Exec(`INSERT INTO tasks(id,revision,snapshot,recovery) VALUES(?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET revision=excluded.revision,snapshot=excluded.snapshot,recovery=excluded.recovery
		WHERE excluded.revision>tasks.revision`, snap.ID, snap.Revision, snapshotData, recoveryData)
	return err
}

// persistSnapshot remains for revision-guard compatibility tests. New task
// updates persist the snapshot and private recovery record together.
func (m *taskManager) persistSnapshot(snap TaskSnapshot) error {
	return m.persistRecord(snap, taskRecovery{})
}

func (m *taskManager) persistRecordTracked(snap TaskSnapshot, recovery taskRecovery) error {
	err := m.persistRecord(snap, recovery)
	if err != nil {
		m.setHistoryError(err)
	}
	return err
}

func (m *taskManager) persistRecord(snap TaskSnapshot, recovery taskRecovery) error {
	if m.historyPath == "" {
		return nil
	}
	if historyErr := m.historyError(); historyErr != nil {
		return historyErr
	}
	db, err := historyDB(m.historyPath)
	if err != nil {
		return err
	}
	defer db.Close()
	return upsertTask(db, snap, recovery)
}
