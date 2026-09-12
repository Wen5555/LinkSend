package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unicode/utf8"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

const (
	maxNativeEntryPaths = 1024
	maxNativeEntryBytes = 1 << 20
)

var errNativeEntryUnsupported = errors.New("SYSTEM_ENTRY_UNSUPPORTED: this entry is not supported on this platform")

// canonicalProfilePath resolves existing directory aliases without creating a
// profile or opening its database before the Wails instance lock is acquired.
func canonicalProfilePath(dataDir string) (string, error) {
	if strings.TrimSpace(dataDir) == "" {
		return "", errors.New("PROFILE_PATH_REQUIRED")
	}
	absolute, err := filepath.Abs(dataDir)
	if err != nil {
		return "", err
	}
	ancestor := filepath.Clean(absolute)
	var missing []string
	for {
		resolved, resolveErr := filepath.EvalSymlinks(ancestor)
		if resolveErr == nil {
			resolved, resolveErr = nativeExistingDirectoryPath(resolved)
			if resolveErr != nil {
				return "", resolveErr
			}
			for i := len(missing) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, missing[i])
			}
			return canonicalNativePathKey(resolved), nil
		}
		if !errors.Is(resolveErr, os.ErrNotExist) {
			return "", fmt.Errorf("resolve profile path: %w", resolveErr)
		}
		parent := filepath.Dir(ancestor)
		if parent == ancestor {
			return "", resolveErr
		}
		missing = append(missing, filepath.Base(ancestor))
		ancestor = parent
	}
}

func canonicalNativePathKey(path string) string {
	path = filepath.Clean(path)
	if runtime.GOOS == "windows" {
		if strings.HasPrefix(path, `\\?\UNC\`) {
			path = `\\` + strings.TrimPrefix(path, `\\?\UNC\`)
		} else {
			path = strings.TrimPrefix(path, `\\?\`)
		}
		path = strings.ToLower(path)
	}
	return path
}

// The callback receives user arguments WITHOUT argv[0], and the second
// process's working directory. Both launches must use parseNativeFileArguments
// before handing paths to the visible Go-owned draft; this adapter never sends.
func profileSingleInstanceOptions(dataDir string, onSecond func([]string, string)) (*application.SingleInstanceOptions, error) {
	canonical, err := canonicalProfilePath(dataDir)
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256([]byte(canonical))
	return &application.SingleInstanceOptions{
		UniqueID: "com.linksend.desktop.p" + hex.EncodeToString(digest[:]),
		OnSecondInstanceLaunch: func(data application.SecondInstanceData) {
			if onSecond == nil {
				return
			}
			args := data.Args
			if len(args) > 0 {
				args = args[1:]
			}
			onSecond(append([]string(nil), args...), data.WorkingDir)
		},
	}, nil
}

// parseNativeFileArguments accepts either plain file arguments or the explicit
// SendTo marker "--send-files --". Unknown options fail visibly; a name starting
// with '-' is allowed after '--'. Paths are never evaluated by a shell.
func parseNativeFileArguments(args []string, workingDir string) ([]string, error) {
	paths := make([]string, 0, len(args))
	pathsOnly := false
	for _, arg := range args {
		if !pathsOnly {
			switch arg {
			case "--send-files", "--background":
				continue
			case "--":
				pathsOnly = true
				continue
			}
			// Windows COM starts toast activation servers with -Embedding;
			// this is an activation, not a file selected for the draft.
			if runtime.GOOS == "windows" && strings.EqualFold(arg, "-Embedding") {
				continue
			}
			if strings.HasPrefix(arg, "-") {
				return nil, errors.New("SYSTEM_ENTRY_ARGUMENT: unrecognised startup option")
			}
		}
		paths = append(paths, arg)
	}
	return normalizeNativePaths(paths, workingDir)
}

func normalizeNativePaths(paths []string, workingDir string) ([]string, error) {
	if len(paths) > maxNativeEntryPaths {
		return nil, errors.New("SYSTEM_ENTRY_TOO_LARGE: select at most 1024 paths per activation")
	}
	result := make([]string, 0, len(paths))
	bytes := 0
	for _, path := range paths {
		bytes += len(path)
		if bytes > maxNativeEntryBytes {
			return nil, errors.New("SYSTEM_ENTRY_TOO_LARGE: path arguments exceed the activation limit")
		}
		if path == "" || !utf8.ValidString(path) || strings.ContainsRune(path, 0) {
			return nil, errors.New("SYSTEM_ENTRY_PATH: empty or invalid path")
		}
		if !filepath.IsAbs(path) {
			if !filepath.IsAbs(workingDir) {
				return nil, errors.New("SYSTEM_ENTRY_WORKING_DIRECTORY: relative paths require the launching directory")
			}
			volume := filepath.VolumeName(path)
			if runtime.GOOS == "windows" && volume != "" {
				if !strings.EqualFold(volume, filepath.VolumeName(workingDir)) {
					return nil, errors.New("SYSTEM_ENTRY_WORKING_DIRECTORY: drive-relative path refers to a different drive")
				}
				path = filepath.Join(workingDir, strings.TrimPrefix(path, volume))
			} else if runtime.GOOS == "windows" && (strings.HasPrefix(path, `\`) || strings.HasPrefix(path, "/")) {
				path = filepath.VolumeName(workingDir) + path
			} else {
				path = filepath.Join(workingDir, path)
			}
		}
		result = append(result, filepath.Clean(path))
	}
	return result, nil
}

// EnableFileDrop belongs in WebviewWindowOptions. The frontend target also
// needs data-file-drop-target; Wails supplies native paths, never file bytes.
func attachFileDrop(window *application.WebviewWindow, onPaths func([]string, string)) func() {
	return window.OnWindowEvent(events.Common.WindowFilesDropped, func(event *application.WindowEvent) {
		if onPaths == nil {
			return
		}
		workingDir, _ := os.Getwd()
		onPaths(append([]string(nil), event.Context().DroppedFiles()...), workingDir)
	})
}
