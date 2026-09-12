package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Wen5555/LinkSend/internal/identity"
)

func TestDeviceLabelsDoNotGrantTrustAndRemoteRenameKeepsAlias(t *testing.T) {
	s, err := New(Config{DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Shutdown)
	peer, err := identity.Generate()
	if err != nil {
		t.Fatal(err)
	}
	profile, err := s.SaveDeviceProfile(DeviceProfile{PeerID: peer.ID(), Alias: "我的工作电脑", Pinned: true, MyDevice: true})
	if err != nil {
		t.Fatal(err)
	}
	devices := s.decorateDevices([]DeviceInfo{{ID: peer.ID(), Name: "远端的新名称"}})
	if len(devices) != 1 || devices[0].Trusted || devices[0].AlwaysAccept || devices[0].Profile.Alias != profile.Alias || devices[0].Name != "远端的新名称" {
		t.Fatalf("policy mixed with display: %+v", devices)
	}
	if err = s.SetAlwaysAccept(peer.ID(), true); err == nil {
		t.Fatal("label granted receive authorization")
	}
}

func TestLostDeviceDirectoryDoesNotFallBackToGlobalDirectory(t *testing.T) {
	s, err := New(Config{DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Shutdown)
	peer, err := identity.Generate()
	if err != nil {
		t.Fatal(err)
	}
	directory := filepath.Join(t.TempDir(), "external-receiver")
	if err = os.Mkdir(directory, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err = s.SaveDeviceProfile(DeviceProfile{PeerID: peer.ID(), ReceiveDirectory: directory}); err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(directory); err != nil {
		t.Fatal(err)
	}
	if target, err := s.receiveDirectory(peer.ID(), t.TempDir()); err == nil || target != "" {
		t.Fatalf("lost device directory was redirected: target=%q err=%v", target, err)
	}
}

func TestDeviceDirectoryChangeDoesNotRedirectActiveQUICReceive(t *testing.T) {
	f := newDirectFixtureServices(t)
	first, second := t.TempDir(), t.TempDir()
	profile, err := f.b.SaveDeviceProfile(DeviceProfile{PeerID: f.aID.ID(), ReceiveDirectory: first})
	if err != nil {
		t.Fatal(err)
	}
	cfg := DirectConfig{AllowLoopback: true, CheckTimeout: 5 * time.Second, WaitTimeout: 10 * time.Second}
	if err = f.b.StartInbox(t.TempDir(), cfg); err != nil {
		t.Fatal(err)
	}
	waitFor(t, 5*time.Second, func() bool { return f.b.InboxStatus().Listening }, "receiver offline")
	source := filepath.Join(t.TempDir(), "destination.txt")
	if err = os.WriteFile(source, []byte("immutable destination"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = f.a.StartSend(f.bID.ID(), []string{source}, cfg); err != nil {
		t.Fatal(err)
	}
	var incoming TaskSnapshot
	waitFor(t, 10*time.Second, func() bool {
		for _, task := range f.b.Tasks() {
			if task.State == "awaiting_acceptance" {
				incoming = task
				return true
			}
		}
		return false
	}, "no request")
	if incoming.TargetDirectory != first {
		t.Fatalf("peer policy not selected: %+v", incoming)
	}
	profile.ReceiveDirectory = second
	if _, err = f.b.SaveDeviceProfile(profile); err != nil {
		t.Fatal(err)
	}
	if err = f.b.AcceptTask(incoming.ID); err != nil {
		t.Fatal(err)
	}
	waitTask(t, f.b, incoming.ID, func(task TaskSnapshot) bool { return task.State == "completed" })
	if _, err = os.Stat(filepath.Join(first, "destination.txt")); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(second, "destination.txt")); !os.IsNotExist(err) {
		t.Fatal("active receive changed destination")
	}
}

func TestQueueWriteFailureNeverReportsJoinedOrDispatches(t *testing.T) {
	f := newDirectFixtureServices(t)
	source := filepath.Join(t.TempDir(), "readonly.txt")
	if err := os.WriteFile(source, []byte("read-only queue"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := f.a.store.db.Exec("PRAGMA query_only=ON"); err != nil {
		t.Fatal(err)
	}
	item, err := f.a.Enqueue(EnqueueRequest{RequestID: "write-fails", PeerID: f.bID.ID(), Paths: []string{source}, WaitForPeer: true})
	if err == nil || item.ID != "" {
		t.Fatalf("failed commit reported enqueue: %+v %v", item, err)
	}
	if len(f.a.Tasks()) != 0 {
		t.Fatal("failed queue commit dispatched")
	}
	var count int
	if err = f.a.store.db.QueryRow("SELECT count(*) FROM send_queue").Scan(&count); err != nil || count != 0 {
		t.Fatalf("partial queue row: %d %v", count, err)
	}
}
