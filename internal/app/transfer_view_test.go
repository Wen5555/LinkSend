package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestClearTransferViewKeepsHistoryFilesAndReopenedAttempts(t *testing.T) {
	s := inboxTestService(t)
	filename := filepath.Join(t.TempDir(), "中文 source.txt")
	if err := os.WriteFile(filename, []byte("retained"), 0600); err != nil {
		t.Fatal(err)
	}
	task, _ := inboxIndexedFixture(t, s, []string{filename}, "send")
	task.finish("completed", nil)
	if err := s.ClearTransferQueue(false); err != nil {
		t.Fatal(err)
	}
	w, err := s.Workspace()
	if err != nil || len(w.Tasks) != 0 {
		t.Fatal(w.Tasks, err)
	}
	history, err := s.Inbox(t.Context(), InboxQuery{})
	if err != nil || len(history.Items) != 1 {
		t.Fatal("view cleanup erased history", history, err)
	}
	location, err := s.ResolveInboxFile(t.Context(), task.snap.ID, 0)
	if err != nil || location.Path != filename {
		t.Fatal("send file cannot be opened", location, err)
	}
	if _, err = os.Stat(filename); err != nil {
		t.Fatal("cleanup deleted user file", err)
	}
	task.mu.Lock()
	task.snap.AttemptID, task.snap.State = "new-attempt", "transferring"
	task.mu.Unlock()
	w, err = s.Workspace()
	if err != nil || len(w.Tasks) != 1 {
		t.Fatal("retry invisible", w.Tasks, err)
	}
	if err = s.ClearTransferQueue(true); err != nil {
		t.Fatal(err)
	}
	if snap, _ := s.Task(task.snap.ID); snap.State != "cancelled" {
		t.Fatal("active task not cancelled", snap.State)
	}
	task.finish("cancelled", nil)
	w, err = s.Workspace()
	if err != nil || len(w.Tasks) != 0 {
		t.Fatal("cancelled task stayed visible", w.Tasks, err)
	}
}

func TestResolveSentFolderRejectsMovedAndChangedSources(t *testing.T) {
	s := inboxTestService(t)
	dir := filepath.Join(t.TempDir(), "source")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	filename := filepath.Join(dir, "report.txt")
	if err := os.WriteFile(filename, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	task, prepared := inboxIndexedFixture(t, s, []string{dir}, "send")
	for _, file := range prepared.Manifest.Files {
		location, err := s.ResolveInboxFile(t.Context(), task.snap.ID, file.ID)
		if err != nil {
			t.Fatal(err)
		}
		if file.Type != "file" {
			continue
		}
		if location.Path != filename {
			t.Fatal(location)
		}
		if err = os.WriteFile(filename, []byte("changed-size"), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err = s.ResolveInboxFile(t.Context(), task.snap.ID, file.ID); err != ErrInboxFileChanged {
			t.Fatal(err)
		}
		if err = os.Rename(filename, filename+".moved"); err != nil {
			t.Fatal(err)
		}
		if _, err = s.ResolveInboxFile(t.Context(), task.snap.ID, file.ID); err != ErrInboxFileMissing {
			t.Fatal(err)
		}
	}
}

func TestForgetDeviceWorksOfflineAndClearsItsPrivileges(t *testing.T) {
	s := inboxTestService(t)
	peer := authorizationPeer(t, s)
	if err := s.SetAlwaysAccept(peer.ID(), true); err != nil {
		t.Fatal(err)
	}
	if err := s.ForgetPeer(peer.ID()); err != nil {
		t.Fatal(err)
	}
	if err := s.checkPeerAllowed(peer.ID()); err != nil {
		t.Fatal("deleted device blacklisted", err)
	}
	devices, _ := s.Devices(t.Context())
	for _, device := range devices {
		if device.ID == peer.ID() {
			t.Fatal("deleted device remained in directory", device)
		}
	}
	if s.alwaysAccept(peer.ID()) {
		t.Fatal("deleted peer retained auto-accept")
	}
}
