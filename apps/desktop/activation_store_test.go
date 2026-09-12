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
