package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestDraftPreviewBoundedMetadataAndOverlap(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "中文.txt")
	if err := os.WriteFile(file, []byte("12345"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "empty"), 0700); err != nil {
		t.Fatal(err)
	}
	got := previewDraftPaths(context.Background(), SendDraft{Revision: 9, Paths: []string{file, root, file}})
	if !got.Complete || got.Files != 1 || got.Directories != 2 || got.Bytes != 5 || got.Revision != 9 {
		t.Fatalf("%+v", got)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got := previewDraftPaths(ctx, SendDraft{Paths: []string{root}}); got.Complete || got.Bytes != 0 {
		t.Fatalf("cancelled preview %+v", got)
	}
	if got := previewDraftPaths(context.Background(), SendDraft{Paths: []string{filepath.Join(root, "absent")}}); got.Complete {
		t.Fatal("missing source marked complete")
	}
}
