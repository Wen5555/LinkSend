package nativeclipboard

import (
	"context"
	"path/filepath"
	"strings"
	"time"
)

// File paths are native metadata only. No source file bytes cross Wails IPC.
func ReadFiles(ctx context.Context, expected uint64) ([]string, uint64, error) {
	if err := ctx.Err(); err != nil {
		return nil, Generation(), err
	}
	if clipboardReadProhibited() {
		return nil, Generation(), ErrUnsupported
	}
	paths, generation, err := readClipboardFiles(ctx, expected)
	if err == nil {
		err = validateClipboardPaths(paths)
	}
	return paths, generation, err
}

func WriteFiles(paths []string, expected uint64, deadline time.Time) (uint64, error) {
	if err := validateClipboardPaths(paths); err != nil {
		return Generation(), err
	}
	if !deadline.IsZero() && !time.Now().Before(deadline) {
		return Generation(), ErrChanged
	}
	return writeClipboardFiles(paths, expected, deadline)
}

func validateClipboardPaths(paths []string) error {
	if len(paths) == 0 || len(paths) > 128 {
		return ErrUnsupported
	}
	total := 0
	for _, path := range paths {
		total += len(path)
		if !filepath.IsAbs(path) || strings.ContainsRune(path, 0) || total > 64<<10 {
			return ErrUnsupported
		}
	}
	return nil
}
