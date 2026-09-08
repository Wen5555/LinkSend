package store

import (
	"path/filepath"
	"testing"
)

func TestOpenControlRejectsUnknownSchemaVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "control.db")
	control, err := OpenControl(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = control.db.Exec("INSERT INTO schema_version(version) VALUES(2)"); err != nil {
		t.Fatal(err)
	}
	if err = control.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = OpenControl(path); err == nil {
		t.Fatal("accepted unknown schema version")
	}
}
