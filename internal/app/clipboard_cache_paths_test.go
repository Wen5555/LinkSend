package app

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/Wen5555/LinkSend/internal/protocol"
)

func TestClipboardCacheDestinationRejectsUnknownExistingObject(t *testing.T) {
	profile := t.TempDir()
	id := protocol.RandomID()
	dir := filepath.Join(profile, "clipboard-files", id)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "user.txt")
	if err := os.WriteFile(path, []byte("foreign data"), 0600); err != nil {
		t.Fatal(err)
	}
	if d, err := openClipboardCacheDestination(profile, id); err == nil {
		_ = d.Close()
		t.Fatal("claimed an existing cache directory")
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != "foreign data" {
		t.Fatal("existing unknown data changed", err)
	}
	if _, err := openClipboardCacheDestination(profile, "../escape"); err == nil {
		t.Fatal("accepted escaping cache ID")
	}
}

func TestClipboardCacheDestinationRejectsLinkedCacheRoot(t *testing.T) {
	profile, outside := t.TempDir(), t.TempDir()
	link := filepath.Join(profile, "clipboard-files")
	if err := os.Symlink(outside, link); err != nil {
		if runtime.GOOS != "windows" {
			t.Fatal(err)
		}
		// NTFS junctions exercise the Windows reparse boundary without granting
		// the process symlink privileges or changing the host configuration.
		if output, junctionErr := exec.Command("cmd.exe", "/c", "mklink", "/J", link, outside).CombinedOutput(); junctionErr != nil {
			t.Fatalf("cannot create owned junction fixture: %v %s", junctionErr, output)
		}
	}
	if d, err := openClipboardCacheDestination(profile, protocol.RandomID()); err == nil {
		_ = d.Close()
		t.Fatal("followed a linked cache root")
	}
	if entries, err := os.ReadDir(outside); err != nil || len(entries) != 0 {
		t.Fatal("linked-root attack wrote outside profile", err)
	}
}

func TestClipboardCachePinnedCleanupPreservesReplacement(t *testing.T) {
	profile := t.TempDir()
	id := protocol.RandomID()
	d, err := openClipboardCacheDestination(profile, id)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if err := d.object.WriteFile("owned.txt", []byte("owned"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(profile, "clipboard-files"), filepath.Join(profile, "original-cache")); err != nil {
		if runtime.GOOS == "windows" && errors.Is(err, os.ErrPermission) {
			if err := d.visible(); err != nil {
				t.Fatal(err)
			}
			t.Log("Windows denied renaming the pinned cache; no replacement was possible")
			return
		}
		t.Fatal(err)
	}
	replacement := filepath.Join(profile, "clipboard-files", id)
	if err := os.MkdirAll(replacement, 0700); err != nil {
		t.Fatal(err)
	}
	foreign := filepath.Join(replacement, "keep.txt")
	if err := os.WriteFile(foreign, []byte("replacement user data"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := d.visible(); err == nil {
		t.Fatal("published a replacement pathname")
	}
	if data, err := d.object.ReadFile("owned.txt"); err != nil || string(data) != "owned" {
		t.Fatal("pinned receiver lost its original directory", err)
	}
	if err := d.discardUnpublished(); err == nil {
		t.Fatal("a directory handle alone authorized deletion of an unregistered file")
	}
	if data, err := d.object.ReadFile("owned.txt"); err != nil || string(data) != "owned" {
		t.Fatal("unregistered original entry was removed", err)
	}
	if data, err := os.ReadFile(foreign); err != nil || string(data) != "replacement user data" {
		t.Fatal("cleanup deleted replacement user data", err)
	}
}
