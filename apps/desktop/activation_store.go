package main

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/Wen5555/LinkSend/internal/protocol"
)

const maxPendingActivations = 64

var activationName = regexp.MustCompile(`^[0-9a-f]{32}\.json$`)
var errActivationBusy = errors.New("SYSTEM_ENTRY_BUSY: another entry is being saved; retry shortly")
var errActivationRetained = errors.New("SYSTEM_SHARE_RETAINED")

type fileActivation struct {
	Version     int      `json:"version"`
	RequestID   string   `json:"request_id,omitempty"`
	PeerID      string   `json:"peer_id,omitempty"`
	Paths       []string `json:"paths"`
	Bookmarks   []string `json:"bookmarks,omitempty"`
	WaitForPeer bool     `json:"wait_for_peer,omitempty"`
	Source      string   `json:"source,omitempty"`
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
	var lock *os.File
	// Concurrent first creation returned ENOENT in the native APFS probe.
	// Retry only absence, through the same Root;
	// permission, capacity and persistent failures still reject intake visibly.
	for attempt := 0; attempt < 5; attempt++ {
		lock, err = root.OpenFile(".lock", os.O_CREATE|os.O_RDWR, 0600)
		if !errors.Is(err, os.ErrNotExist) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
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
	return persistActivation(dataDir, fileActivation{Version: 1, Paths: normalized})
}

func stageShareActivation(dataDir string, activation fileActivation, workingDir string) error {
	if activation.RequestID != strings.ToLower(activation.RequestID) || activation.RequestID != strings.TrimSpace(activation.RequestID) || len(activation.RequestID) != 32 || !activationName.MatchString(activation.RequestID+".json") {
		return errors.New("SYSTEM_SHARE_INVALID: request ID must be 32 lowercase hex characters")
	}
	if activation.PeerID == "" || len(activation.PeerID) > 128 || strings.ContainsAny(activation.PeerID, "\x00\r\n") {
		return errors.New("SYSTEM_SHARE_INVALID: destination required")
	}
	if len(activation.Bookmarks) != 0 && len(activation.Bookmarks) != len(activation.Paths) {
		return errors.New("SYSTEM_SHARE_INVALID: bookmark count")
	}
	normalized, err := normalizeNativePaths(activation.Paths, workingDir)
	if err != nil || len(normalized) == 0 {
		return errors.New("SYSTEM_SHARE_INVALID: source paths required")
	}
	activation.Version = 2
	activation.Paths = normalized
	if activation.Source != "windows_share" && activation.Source != "macos_share" {
		return errors.New("SYSTEM_SHARE_INVALID: source kind")
	}
	return persistActivation(dataDir, activation)
}

func persistActivation(dataDir string, activation fileActivation) error {
	data, err := json.Marshal(activation)
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
		id := activation.RequestID
		if id == "" {
			id = protocol.RandomID()
		}
		final := id + ".json"
		if existing, openErr := root.Open(final); openErr == nil {
			saved, readErr := io.ReadAll(io.LimitReader(existing, maxNativeEntryBytes+1))
			existing.Close()
			if readErr != nil {
				return readErr
			}
			if string(saved) == string(data) {
				return nil
			}
			return errors.New("SYSTEM_SHARE_CONFLICT: request ID was already used")
		} else if !errors.Is(openErr, os.ErrNotExist) {
			return openErr
		}
		if count >= maxPendingActivations || total+int64(len(data)) > 16<<20 {
			return errors.New("SYSTEM_ENTRY_TOO_LARGE: pending entries are full; open the app to resolve them")
		}
		temporary := protocol.RandomID() + ".pending"
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
		if err := root.Rename(temporary, final); err != nil {
			return err
		}
		return syncActivationDirectory(root)
	})
}

// Delete only after the draft transaction commits. A crash between commit and
// delete replays through MergeDraftPaths' canonical deduplication, never send.
func consumeActivations(dataDir string, consume func(fileActivation) error) (int, error) {
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
			if len(data) > maxNativeEntryBytes || json.Unmarshal(data, &activation) != nil || activation.Version != 1 && activation.Version != 2 {
				firstErr = errors.New("SYSTEM_ENTRY_STORAGE: invalid entry preserved for inspection")
				continue
			}
			if activation.Version == 2 && (activation.RequestID != strings.TrimSuffix(entry.Name(), ".json") || activation.PeerID == "" || activation.Source != "windows_share" && activation.Source != "macos_share" || len(activation.Bookmarks) != 0 && len(activation.Bookmarks) != len(activation.Paths)) {
				firstErr = errors.New("SYSTEM_ENTRY_STORAGE: invalid share entry preserved for inspection")
				continue
			}
			paths, err := normalizeNativePaths(activation.Paths, "")
			if err != nil {
				firstErr = err
				continue
			}
			activation.Paths = paths
			if err := consume(activation); err != nil {
				if errors.Is(err, errActivationRetained) {
					count++
					continue
				}
				if firstErr == nil {
					firstErr = err
				}
				continue
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

func reapShareActivations(dataDir string, terminal map[string]bool) (int, error) {
	removed := 0
	err := withActivationStore(dataDir, func(root *os.Root) error {
		entries, err := activationEntries(root)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			requestID := strings.TrimSuffix(entry.Name(), ".json")
			if !activationName.MatchString(entry.Name()) || !terminal[requestID] {
				continue
			}
			file, err := root.Open(entry.Name())
			if err != nil {
				return err
			}
			data, readErr := io.ReadAll(io.LimitReader(file, maxNativeEntryBytes+1))
			file.Close()
			var activation fileActivation
			if readErr != nil || json.Unmarshal(data, &activation) != nil || activation.Version != 2 || activation.RequestID != requestID {
				continue
			}
			owned := filepath.Join(dataDir, "share-owned-v1", requestID)
			if info, statErr := os.Lstat(owned); statErr == nil {
				if info.Mode()&os.ModeSymlink != 0 {
					return errors.New("SYSTEM_SHARE_STORAGE: owned source is a symlink")
				}
				if err := os.RemoveAll(owned); err != nil {
					return err
				}
			} else if !errors.Is(statErr, os.ErrNotExist) {
				return statErr
			}
			releaseNativeShareActivation(requestID)
			if err := root.Remove(entry.Name()); err != nil {
				return err
			}
			removeNativeShareReceipt(dataDir, requestID)
			removed++
		}
		if removed > 0 {
			return syncActivationDirectory(root)
		}
		return nil
	})
	return removed, err
}

func consumeFileActivations(dataDir string, merge func([]string) error) (int, error) {
	return consumeActivations(dataDir, func(activation fileActivation) error {
		if activation.Version != 1 {
			return errors.New("SYSTEM_ENTRY_STORAGE: share activation requires the native share consumer")
		}
		return merge(activation.Paths)
	})
}
