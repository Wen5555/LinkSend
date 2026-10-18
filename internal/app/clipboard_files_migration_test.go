package app

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func TestClipboardFilesSchema11MigrationPreservesConsentAndBackup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.sqlite")
	db, err := historyDB(path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`DROP TABLE clipboard_grants;
 CREATE TABLE clipboard_grants(peer_id TEXT NOT NULL,direction TEXT NOT NULL CHECK(direction IN ('send','receive')),kind TEXT NOT NULL CHECK(kind IN ('text','link','image')),enabled INTEGER NOT NULL CHECK(enabled IN (0,1)),revision INTEGER NOT NULL CHECK(revision>0),updated_at TEXT NOT NULL,authorization_generation INTEGER NOT NULL DEFAULT 0,PRIMARY KEY(peer_id,direction,kind));
 INSERT INTO clipboard_grants VALUES('peer','send','image',1,7,'unchanged',4);
 PRAGMA user_version=11;
 UPDATE metadata SET value='11' WHERE key='schema_version';`)
	if err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	db, err = historyDB(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var enabled bool
	var revision, generation int
	var updated string
	if err = db.QueryRow(`SELECT enabled,revision,updated_at,authorization_generation FROM clipboard_grants WHERE peer_id='peer'`).Scan(&enabled, &revision, &updated, &generation); err != nil || !enabled || revision != 7 || generation != 4 || updated != "unchanged" {
		t.Fatal("existing consent altered", err)
	}
	var count int
	if err = db.QueryRow(`SELECT count(*) FROM clipboard_grants WHERE kind='files'`).Scan(&count); err != nil || count != 0 {
		t.Fatal("migration silently enabled files", err)
	}
	if _, err = db.Exec(`INSERT INTO clipboard_grants VALUES('peer','receive','files',0,1,'explicit',4)`); err != nil {
		t.Fatal(err)
	}
	backups, err := filepath.Glob(path + ".schema-v11-*.bak")
	if err != nil || len(backups) != 1 {
		t.Fatal("missing rollback backup", backups, err)
	}
	if err = verifyHistoryBackup(backups[0], 11); err != nil {
		t.Fatal(err)
	}
	backup, err := sql.Open("sqlite", backups[0])
	if err != nil {
		t.Fatal(err)
	}
	defer backup.Close()
	if _, err = backup.Exec(`INSERT INTO clipboard_grants VALUES('peer','receive','files',0,1,'must reject',4)`); err == nil {
		t.Fatal("rollback backup no longer has old permission constraint")
	}
}
