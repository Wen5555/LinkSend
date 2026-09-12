package main

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"time"

	"github.com/Wen5555/LinkSend/internal/protocol"
)

const maxPendingActivations = 64

var activationName = regexp.MustCompile(`^[0-9a-f]{32}\.json$`)
var errActivationBusy = errors.New("SYSTEM_ENTRY_BUSY: another entry is being saved; retry shortly")

type fileActivation struct {
	Version int      `json:"version"`
	Paths   []string `json:"paths"`
}

// This bounded multi-producer input journal contains only selected path metadata.
// It never opens identity/trust/SQLite. The first Wails instance is its sole
// consumer; the core profile lock remains the sole database/scheduler authority.
// Persist before Wails.New: beta.18 exits a second process even if notification
// fails. A timer in the first process also discovers committed journal entries.
func withActivationStore(dataDir string, fn func(*os.Root) error) error {
	directory := filepath.Join(dataDir, "desktop-activations-v1")
	if err := os.MkdirAll(directory, 0700); err != nil {
		return err
	}
	info, err := os.Lstat(directory)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("SYSTEM_ENTRY_STORAGE: journal must be a local directory")
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return err
	}
	defer root.Close()
	lock, err := root.OpenFile(".lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer lock.Close()
	deadline := time.Now().Add(2 * time.Second)
	for {
		err = lockActivationFile(lock)
		if err == nil {
			break
		}
		if !errors.Is(err, errActivationBusy) || time.Now().After(deadline) {
			return err
		}
		time.Sleep(10 * time.Millisecond)
	}
	return fn(root)
}

func activationEntries(root *os.Root) ([]os.DirEntry, error) {
	directory, err := root.Open(".")
	if err != nil {
		return nil, err
	}
	defer directory.Close()
	// Bound enumeration even if unrelated files were placed in the directory.
	entries, err := directory.ReadDir(maxPendingActivations*2 + 1)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	if len(entries) > maxPendingActivations*2 {
		return nil, errors.New("SYSTEM_ENTRY_TOO_LARGE: entry journal is full")
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	return entries, nil
}

func stageFileActivation(dataDir string, paths []string, workingDir string) error {
	normalized, err := normalizeNativePaths(paths, workingDir)
	if err != nil {
		return err
	}
	if len(normalized) == 0 {
		return nil
	}
	data, err := json.Marshal(fileActivation{Version: 1, Paths: normalized})
	if err != nil {
		return err
	}
	if len(data) > maxNativeEntryBytes {
		return errors.New("SYSTEM_ENTRY_TOO_LARGE: encoded paths exceed the entry limit")
	}
	return withActivationStore(dataDir, func(root *os.Root) error {
		entries, err := activationEntries(root)
		if err != nil {
			return err
		}
		count, total := 0, int64(0)
		for _, entry := range entries {
			if entry.Name() == ".lock" {
				continue
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			count++
			total += info.Size()
		}
		if count >= maxPendingActivations || total+int64(len(data)) > 16<<20 {
			return errors.New("SYSTEM_ENTRY_TOO_LARGE: pending entries are full; open the app to resolve them")
		}
		id := protocol.RandomID()
		temporary := id + ".pending"
		file, err := root.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return err
		}
		defer root.Remove(temporary)
		_, writeErr := file.Write(data)
		if writeErr == nil {
			writeErr = file.Sync()
		}
		closeErr := file.Close()
		if writeErr != nil {
			return writeErr
		}
		if closeErr != nil {
			return closeErr
		}
		if err := root.Rename(temporary, id+".json"); err != nil {
			return err
		}
		return syncActivationDirectory(root)
	})
}

// Delete only after the draft transaction commits. A crash between commit and
// delete replays through MergeDraftPaths' canonical deduplication, never send.
func consumeFileActivations(dataDir string, merge func([]string) error) (int, error) {
	count := 0
	err := withActivationStore(dataDir, func(root *os.Root) error {
		entries, err := activationEntries(root)
		if err != nil {
			return err
		}
		var firstErr error
		for _, entry := range entries {
			if !activationName.MatchString(entry.Name()) {
				continue
			}
			info, err := root.Lstat(entry.Name())
			if err != nil {
				return err
			}
			if !info.Mode().IsRegular() || info.Size() > maxNativeEntryBytes {
				firstErr = errors.New("SYSTEM_ENTRY_STORAGE: invalid entry preserved for inspection")
				continue
			}
			file, err := root.Open(entry.Name())
			if err != nil {
				return err
			}
			data, err := io.ReadAll(io.LimitReader(file, maxNativeEntryBytes+1))
			file.Close()
			if err != nil {
				return err
			}
			var activation fileActivation
			if len(data) > maxNativeEntryBytes || json.Unmarshal(data, &activation) != nil || activation.Version != 1 {
				firstErr = errors.New("SYSTEM_ENTRY_STORAGE: invalid entry preserved for inspection")
				continue
			}
			paths, err := normalizeNativePaths(activation.Paths, "")
			if err != nil {
				firstErr = err
				continue
			}
			if err := merge(paths); err != nil {
				return err
			}
			if err := root.Remove(entry.Name()); err != nil {
				return err
			}
			count++
		}
		if count > 0 {
			if err := syncActivationDirectory(root); err != nil {
				return err
			}
		}
		return firstErr
	})
	return count, err
}
