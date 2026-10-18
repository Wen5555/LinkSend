package app

import (
	"context"
	"errors"
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
	return previewDraftPaths(ctx, draft), nil
}

func previewDraftPaths(ctx context.Context, draft SendDraft) DraftPreview {
	result := DraftPreview{Revision: draft.Revision, Complete: true}
	if len(draft.Paths) == 0 {
		return result
	}
	preview, err := transfer.PreviewSources(ctx, draft.Paths, draft.SourceSelection)
	result.Files, result.Directories, result.Bytes = preview.Files, preview.Directories, preview.Bytes
	result.ExcludedFiles, result.ExcludedDirectories, result.ExcludedBytes = preview.ExcludedFiles, preview.ExcludedDirectories, preview.ExcludedBytes
	result.Entries, result.EntriesTruncated = preview.Entries, preview.EntriesTruncated
	if err != nil {
		result.Complete = false
		result.Problem = "部分路径无法读取或超出预览限额；加入队列前会重新核对。"
	}
	return result
}
