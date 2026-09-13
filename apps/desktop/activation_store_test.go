package main

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestActivationJournalConcurrentProducersAndRestart(t *testing.T) {
	profile, source := t.TempDir(), t.TempDir()
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := stageFileActivation(profile, []string{"中文 文件.txt"}, source); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	// A failed draft transaction must not acknowledge/delete any activation.
	failed := errors.New("disk full")
	if n, err := consumeFileActivations(profile, func([]string) error { return failed }); n != 0 || !errors.Is(err, failed) {
		t.Fatalf("failure: %d %v", n, err)
	}
	seen := 0
	n, err := consumeFileActivations(profile, func(paths []string) error {
		if len(paths) != 1 || paths[0] != filepath.Join(source, "中文 文件.txt") {
			t.Fatalf("wrong origin: %v", paths)
		}
		seen++
		return nil
	})
	if err != nil || n != 20 || seen != 20 {
		t.Fatalf("restart: %d %d %v", n, seen, err)
	}
	if n, err = consumeFileActivations(profile, func([]string) error { t.Fatal("replayed removed entry"); return nil }); n != 0 || err != nil {
		t.Fatal(n, err)
	}
}

func TestActivationJournalBoundAndCorruptionPreservation(t *testing.T) {
	profile, source := t.TempDir(), t.TempDir()
	for i := 0; i < maxPendingActivations; i++ {
		if err := stageFileActivation(profile, []string{"file"}, source); err != nil {
			t.Fatal(err)
		}
	}
	if err := stageFileActivation(profile, []string{"overflow"}, source); err == nil {
		t.Fatal("unbounded journal")
	}
	n, err := consumeFileActivations(profile, func([]string) error { return nil })
	if err != nil || n != maxPendingActivations {
		t.Fatal(n, err)
	}
	path := filepath.Join(profile, "desktop-activations-v1", "00000000000000000000000000000000.json")
	if err := os.WriteFile(path, []byte(`{"version":2,"paths":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	if n, err := consumeFileActivations(profile, func([]string) error { t.Fatal("future schema used"); return nil }); n != 0 || err == nil {
		t.Fatal(n, err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("invalid source deleted", err)
	}
}

func TestActivationJournalPreservesOutsideSymlink(t *testing.T) {
	profile, outside := t.TempDir(), t.TempDir()
	dir := filepath.Join(profile, "desktop-activations-v1")
	if err := os.Symlink(outside, dir); err != nil {
		t.Skip("symlink privilege unavailable:", err)
	}
	if err := stageFileActivation(profile, []string{filepath.Join(outside, "file")}, ""); err == nil {
		t.Fatal("accepted symlink journal")
	}
	entries, err := os.ReadDir(outside)
	if err != nil || len(entries) != 0 {
		t.Fatal("changed outside directory", entries, err)
	}
}

func TestShareActivationJournalIsPersistentAndIdempotent(t *testing.T) {
	profile, source := t.TempDir(), t.TempDir()
	path := filepath.Join(source, "共享 文件.txt")
	if err := os.WriteFile(path, []byte("share"), 0600); err != nil {
		t.Fatal(err)
	}
	request := fileActivation{RequestID: "0123456789abcdef0123456789abcdef", PeerID: "peer-device", Paths: []string{path}, Source: "windows_share"}
	badBookmarks := request
	badBookmarks.Bookmarks = []string{"one", "extra"}
	if err := stageShareActivation(profile, badBookmarks, ""); err == nil {
		t.Fatal("accepted misaligned bookmarks")
	}
	if err := stageShareActivation(profile, request, ""); err != nil {
		t.Fatal(err)
	}
	if err := stageShareActivation(profile, request, ""); err != nil {
		t.Fatal("idempotent replay failed:", err)
	}
	conflict := request
	conflict.PeerID = "other-device"
	if err := stageShareActivation(profile, conflict, ""); err == nil {
		t.Fatal("accepted conflicting replay")
	}
	seen := 0
	n, err := consumeActivations(profile, func(saved fileActivation) error {
		seen++
		if saved.Version != 2 || saved.RequestID != request.RequestID || saved.PeerID != request.PeerID || saved.Source != "windows_share" || !saved.WaitForPeer || len(saved.Paths) != 1 || saved.Paths[0] != path {
			t.Fatalf("wrong share activation: %+v", saved)
		}
		return nil
	})
	if err != nil || n != 1 || seen != 1 {
		t.Fatal(n, seen, err)
	}
}

func TestShareActivationJournalPreservesInvalidAndFailedRequests(t *testing.T) {
	profile, source := t.TempDir(), t.TempDir()
	path := filepath.Join(source, "file.txt")
	if err := os.WriteFile(path, []byte("share"), 0600); err != nil {
		t.Fatal(err)
	}
	request := fileActivation{RequestID: "abcdefabcdefabcdefabcdefabcdefab", PeerID: "peer", Paths: []string{path}, Source: "macos_share"}
	if err := stageShareActivation(profile, request, ""); err != nil {
		t.Fatal(err)
	}
	failure := errors.New("queue unavailable")
	if n, err := consumeActivations(profile, func(fileActivation) error { return failure }); n != 0 || !errors.Is(err, failure) {
		t.Fatal(n, err)
	}
	entry := filepath.Join(profile, "desktop-activations-v1", request.RequestID+".json")
	if _, err := os.Stat(entry); err != nil {
		t.Fatal("failed request was deleted:", err)
	}
	bad := filepath.Join(profile, "desktop-activations-v1", "00000000000000000000000000000000.json")
	if err := os.WriteFile(bad, []byte(`{"version":2,"request_id":"different","peer_id":"peer","paths":["`+filepath.ToSlash(path)+`"],"source":"macos_share"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := consumeActivations(profile, func(fileActivation) error { return nil }); err == nil {
		t.Fatal("invalid share record was accepted")
	}
	if _, err := os.Stat(bad); err != nil {
		t.Fatal("invalid request was deleted:", err)
	}
}
