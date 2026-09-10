package app

import (
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	_ "modernc.org/sqlite"
)

// Individual revision-guarded rows avoid read/modify/write losses between
// instances. No local paths beyond the explicitly selected receive directory,
// credentials, identity keys or file bodies are added to these snapshots.
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
	if schema != 0 && schema != 1 {
		return fail(errors.New("TASK_STORE_VERSION"))
	}
	if _, err = db.Exec("CREATE TABLE IF NOT EXISTS tasks (id TEXT PRIMARY KEY, revision INTEGER NOT NULL, snapshot BLOB NOT NULL)"); err != nil {
		return fail(err)
	}
	if _, err = db.Exec("CREATE TABLE IF NOT EXISTS metadata (key TEXT PRIMARY KEY, value TEXT NOT NULL)"); err != nil {
		return fail(err)
	}
	if _, err = db.Exec("INSERT INTO metadata(key,value) VALUES('schema_version','1') ON CONFLICT(key) DO NOTHING"); err != nil {
		return fail(err)
	}
	if _, err = db.Exec("PRAGMA user_version=1"); err != nil {
		return fail(err)
	}
	return db, nil
}

func (m *taskManager) configureHistory(path string) {
	m.historyPath = path
	db, err := historyDB(path)
	if err != nil {
		m.historyErr = err
		return
	}
	defer db.Close()
	rows, err := db.Query("SELECT snapshot FROM tasks ORDER BY id")
	if err != nil {
		m.historyErr = err
		return
	}
	defer rows.Close()
	for rows.Next() {
		var data []byte
		if rows.Scan(&data) != nil {
			continue
		}
		var snap TaskSnapshot
		if len(data) > 1<<20 || json.Unmarshal(data, &snap) != nil || snap.ID == "" {
			continue
		}
		if !isTerminal(snap.State) {
			snap.State = "failed"
			snap.Phase = "interrupted"
			snap.ErrorCode = "TASK_INTERRUPTED"
			snap.ErrorMessage = "上次传输因应用退出而中断。自动恢复尚未支持，请核对接收结果后重新选择内容。"
			snap.EndedAt = time.Now().UTC().Format(time.RFC3339Nano)
			snap.Revision++
		}
		// A historical session is never a live connectivity claim.
		snap.CanCancel = false
		snap.CanRetry = false
		snap.CanPause = false
		snap.CanResume = false
		snap.ConnectionMethod = ""
		snap.TransportProtocol = ""
		snap.SessionID = ""
		snap.RateBytesPerSecond = nil
		snap.HistoryPersisted = true
		snap.RestartRecoverySupported = false
		snap.ByteResumeSupported = false
		m.tasks[snap.ID] = &taskRecord{snap: snap}
	}
	if err = rows.Err(); err != nil {
		m.historyErr = err
	}
}

func (m *taskManager) persistSnapshot(snap TaskSnapshot) error {
	if m.historyPath == "" {
		return nil
	}
	if m.historyErr != nil {
		return m.historyErr
	}
	db, err := historyDB(m.historyPath)
	if err != nil {
		return err
	}
	defer db.Close()
	data, err := json.Marshal(snap)
	if err != nil {
		return err
	}
	_, err = db.Exec(`INSERT INTO tasks(id,revision,snapshot) VALUES(?,?,?)
		ON CONFLICT(id) DO UPDATE SET revision=excluded.revision,snapshot=excluded.snapshot
		WHERE excluded.revision>tasks.revision`, snap.ID, snap.Revision, data)
	return err
}
