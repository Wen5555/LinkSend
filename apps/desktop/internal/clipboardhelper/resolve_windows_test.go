package clipboardhelper

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveFixedPathRejectsMissingChangedAndReparse(t *testing.T) {
	root := t.TempDir()
	fixture, a := testPE(t, root)
	executable := filepath.Join(root, "LinkSend.exe")
	if _, _, err := Resolve(executable, a); err == nil {
		t.Fatal("missing fixed-path helper accepted")
	}
	destination := filepath.Join(root, a.RelativePath())
	if err := os.MkdirAll(filepath.Dir(destination), 0700); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destination, data, 0600); err != nil {
		t.Fatal(err)
	}
	path, digest, err := Resolve(executable, a)
	if err != nil || path != destination || digest != a.HelperSHA256 {
		t.Fatalf("fixed candidate: %q %q %v", path, digest, err)
	}
	if err := os.WriteFile(destination, []byte("replacement"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Resolve(executable, a); err == nil {
		t.Fatal("changed helper accepted")
	}
	t.Run("reparse ancestor", func(t *testing.T) {
		if err := os.WriteFile(destination, data, 0600); err != nil {
			t.Fatal(err)
		}
		other := t.TempDir()
		if err := os.Symlink(filepath.Join(root, "native-clipboard"), filepath.Join(other, "native-clipboard")); err != nil {
			t.Skipf("test-only symlink unavailable: %v", err)
		}
		if _, _, err := Resolve(filepath.Join(other, "LinkSend.exe"), a); err == nil {
			t.Fatal("reparse ancestor accepted")
		}
	})
}
