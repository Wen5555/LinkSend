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
		if saved.Version != 2 || saved.RequestID != request.RequestID || saved.PeerID != request.PeerID || saved.Source != "windows_share" || saved.WaitForPeer || len(saved.Paths) != 1 || saved.Paths[0] != path {
			t.Fatalf("wrong share activation: %+v", saved)
		}
		return nil
	})
	if err != nil || n != 1 || seen != 1 {
		t.Fatal(n, seen, err)
	}
}

func TestActivationJournalFailureDoesNotStarveLaterRequest(t *testing.T) {
	profile, source := t.TempDir(), t.TempDir()
	path := filepath.Join(source, "file.txt")
	if err := os.WriteFile(path, []byte("share"), 0600); err != nil {
		t.Fatal(err)
	}
	bad := fileActivation{RequestID: "00000000000000000000000000000000", PeerID: "revoked", Paths: []string{path}, Source: "windows_share"}
	good := fileActivation{RequestID: "11111111111111111111111111111111", PeerID: "trusted", Paths: []string{path}, Source: "windows_share"}
	if err := stageShareActivation(profile, bad, ""); err != nil {
		t.Fatal(err)
	}
	if err := stageShareActivation(profile, good, ""); err != nil {
		t.Fatal(err)
	}
	seenGood := false
	n, err := consumeActivations(profile, func(saved fileActivation) error {
		if saved.PeerID == "revoked" {
			return errors.New("PEER_BLOCKED")
		}
		seenGood = true
		return nil
	})
	if n != 1 || err == nil || !seenGood {
		t.Fatalf("count=%d err=%v seenGood=%v", n, err, seenGood)
	}
	if _, statErr := os.Stat(filepath.Join(profile, "desktop-activations-v1", bad.RequestID+".json")); statErr != nil {
		t.Fatal("failed request was not preserved", statErr)
	}
}

func TestShareActivationRetainedUntilTerminalReap(t *testing.T) {
	profile, source := t.TempDir(), t.TempDir()
	path := filepath.Join(source, "file.txt")
	if err := os.WriteFile(path, []byte("share"), 0600); err != nil {
		t.Fatal(err)
	}
	request := fileActivation{RequestID: "22222222222222222222222222222222", PeerID: "peer", Paths: []string{path}, Source: "windows_share"}
	if err := stageShareActivation(profile, request, ""); err != nil {
		t.Fatal(err)
	}
	owned := filepath.Join(profile, "share-owned-v1", request.RequestID)
	if err := os.MkdirAll(owned, 0700); err != nil {
		t.Fatal(err)
	}
	if n, err := consumeActivations(profile, func(fileActivation) error { return errActivationRetained }); err != nil || n != 1 {
		t.Fatalf("retained intake: %d %v", n, err)
	}
	entry := filepath.Join(profile, "desktop-activations-v1", request.RequestID+".json")
	if _, err := os.Stat(entry); err != nil {
		t.Fatal("source authorization journal was removed before terminal state", err)
	}
	if err := publishNativeShareReceipt(profile, request.RequestID); err != nil {
		t.Fatal(err)
	}
	if n, err := reapShareActivations(profile, map[string]bool{request.RequestID: true}); err != nil || n != 1 {
		t.Fatalf("terminal reap: %d %v", n, err)
	}
	if _, err := os.Stat(entry); !os.IsNotExist(err) {
		t.Fatal("terminal journal retained", err)
	}
	if _, err := os.Stat(owned); !os.IsNotExist(err) {
		t.Fatal("owned temporary source retained", err)
	}
	if _, err := os.Stat(filepath.Join(profile, "native-share-v1", "accepted", request.RequestID)); !os.IsNotExist(err) {
		t.Fatal("accepted receipt retained", err)
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
