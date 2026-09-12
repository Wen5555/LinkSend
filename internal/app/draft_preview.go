package app

import (
	"context"
	"errors"
	"io/fs"
	"math"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type DraftPreview struct {
	Revision    uint64 `json:"revision"`
	Files       int    `json:"files"`
	Directories int    `json:"directories"`
	Bytes       int64  `json:"bytes"`
	Complete    bool   `json:"complete"`
	Problem     string `json:"problem,omitempty"`
}

// Metadata only, with bounded work. Prepare still performs authoritative path,
// content and source-change validation when explicitly enqueued and dispatched.
func (s *Service) PreviewDraft() (DraftPreview, error) {
	done, err := s.beginWorkspaceWork()
	if err != nil {
		return DraftPreview{}, err
	}
	defer done()
	draft, err := s.store.Draft("main")
	if errors.Is(err, ErrMetadataNotFound) {
		return DraftPreview{Complete: true}, nil
	}
	if err != nil {
		return DraftPreview{}, err
	}
	ctx, cancel := context.WithTimeout(s.workCtx, 2*time.Second)
	defer cancel()
	return previewDraftPaths(ctx, draft), nil
}

func previewDraftPaths(ctx context.Context, draft SendDraft) DraftPreview {
	result := DraftPreview{Revision: draft.Revision, Complete: true}
	seen := make(map[string]bool)
	for _, root := range draft.Paths {
		err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			key := filepath.Clean(path)
			if runtime.GOOS == "windows" {
				key = strings.ToLower(key)
			}
			if seen[key] {
				if entry.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if len(seen) >= 100000 {
				return errors.New("preview bound exceeded")
			}
			seen[key] = true
			if entry.Type()&fs.ModeSymlink != 0 {
				return errors.New("unsupported symbolic link")
			}
			if entry.IsDir() {
				result.Directories++
				return nil
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			if !info.Mode().IsRegular() || info.Size() < 0 || info.Size() > math.MaxInt64-result.Bytes {
				return errors.New("unsupported file")
			}
			result.Files++
			result.Bytes += info.Size()
			return nil
		})
		if err != nil {
			result.Complete = false
			result.Problem = "部分路径无法读取或超出预览限额；加入队列前会重新核对。"
		}
		if ctx.Err() != nil || len(seen) >= 100000 {
			break
		}
	}
	return result
}
