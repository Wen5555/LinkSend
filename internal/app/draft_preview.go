package app

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"time"

	"github.com/Wen5555/LinkSend/internal/transfer"
)

type DraftPreview struct {
	ExcludedFiles       int                           `json:"excluded_files,omitempty"`
	ExcludedDirectories int                           `json:"excluded_directories,omitempty"`
	ExcludedBytes       int64                         `json:"excluded_bytes,omitempty"`
	Entries             []transfer.SourcePreviewEntry `json:"entries,omitempty"`
	EntriesTruncated    bool                          `json:"entries_truncated,omitempty"`
	Revision            uint64                        `json:"revision"`
	Files               int                           `json:"files"`
	Directories         int                           `json:"directories"`
	Bytes               int64                         `json:"bytes"`
	Complete            bool                          `json:"complete"`
	Problem             string                        `json:"problem,omitempty"`
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
	result := previewDraftPathsWithAccess(ctx, draft, s.clipboardSourceAccess)
	// Excluded entries are metadata too: a relative cache filename combined
	// with a user-selected parent can expose the physical cache namespace.
	entries := result.Entries[:0]
	for _, entry := range result.Entries {
		keep, matched := true, false
		for _, input := range draft.Paths {
			absolute, e := filepath.Abs(input)
			if e != nil {
				continue
			}
			base := filepath.ToSlash(filepath.Base(absolute))
			name := entry.Path
			candidate := ""
			if name == base {
				candidate = absolute
			} else if strings.HasPrefix(name, base+"/") {
				candidate = filepath.Join(absolute, filepath.FromSlash(strings.TrimPrefix(name, base+"/")))
			}
			if candidate != "" {
				matched = true
				if s.ValidateExternalFilePath(ctx, candidate) != nil {
					keep = false
					break
				}
			}
		}
		if matched && keep {
			entries = append(entries, entry)
		} else {
			result.EntriesTruncated = true
		}
	}
	result.Entries = entries
	return result, nil
}

func previewDraftPaths(ctx context.Context, draft SendDraft) DraftPreview {
	return previewDraftPathsWithAccess(ctx, draft, nil)
}

func previewDraftPathsWithAccess(ctx context.Context, draft SendDraft, access transfer.SourceAccessHook) DraftPreview {
	result := DraftPreview{Revision: draft.Revision, Complete: true}
	if len(draft.Paths) == 0 {
		return result
	}
	preview, err := transfer.PreviewSourcesWithAccess(ctx, draft.Paths, draft.SourceSelection, access)
	result.Files, result.Directories, result.Bytes = preview.Files, preview.Directories, preview.Bytes
	result.ExcludedFiles, result.ExcludedDirectories, result.ExcludedBytes = preview.ExcludedFiles, preview.ExcludedDirectories, preview.ExcludedBytes
	result.Entries, result.EntriesTruncated = preview.Entries, preview.EntriesTruncated
	if err != nil {
		result.Complete = false
		result.Problem = "部分路径无法读取或超出预览限额；加入队列前会重新核对。"
	}
	return result
}
