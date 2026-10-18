package transfer

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"
)

func TestSourceSelectionRulesArePortableAndBounded(t *testing.T) {
	selection, err := NormalizeSourceSelection(&SourceSelection{Version: 1, Preset: "source", Exclude: []string{" src\\**\\*.tmp ", "缓存/", "src/**/*.tmp"}})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(selection.Exclude, []string{"src/**/*.tmp", "缓存/"}) {
		t.Fatalf("non-canonical rules: %+v", selection)
	}
	for _, test := range []struct {
		path                string
		directory, excluded bool
	}{
		{"", true, false}, {".git", true, true}, {"nested/node_modules/a.js", false, true}, {"node_modules", false, false},
		{"src/a.tmp", false, true}, {"src/深层/a.tmp", false, true}, {"else/a.tmp", false, false}, {"src/A.TMP", false, false},
		{"DIST/a.js", false, false}, {"dist/a.js", false, true}, {"缓存/中文.txt", false, true}, {"空目录", true, false},
	} {
		if got := sourceExcluded(selection, test.path, test.directory); got != test.excluded {
			t.Errorf("%q excluded=%t, want %t", test.path, got, test.excluded)
		}
	}
	for _, pattern := range []string{"../secret", "/abs", "C:/outside", "!keep", "[ab]", "a//b", "a/**b", "a/./b", "a\x00b"} {
		if _, err := NormalizeSourceSelection(&SourceSelection{Version: 1, Preset: "all", Exclude: []string{pattern}}); err == nil {
			t.Errorf("accepted unsafe/unsupported pattern %q", pattern)
		}
	}
	for _, selection := range []*SourceSelection{{Version: 2, Preset: "source"}, {Version: 1, Preset: "gitignore"}, {Version: 1, Preset: "all", Exclude: make([]string, 33)}} {
		if _, err := NormalizeSourceSelection(selection); err == nil {
			t.Fatal("accepted unsupported rule version or limits")
		}
	}
}

func TestSourceSelectionPreviewPrepareAndResumeAgree(t *testing.T) {
	project := filepath.Join(t.TempDir(), "工程")
	write := func(relative, content string) {
		t.Helper()
		file := filepath.Join(project, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("main.go", "package main")
	write("src/中文.txt", "正文")
	write("src/ignore.tmp", "excluded")
	write(".git/config", "not transferred")
	write("node_modules/a.js", "excluded dependency")
	if err := os.Mkdir(filepath.Join(project, "空目录"), 0700); err != nil {
		t.Fatal(err)
	}
	selection := &SourceSelection{Version: 1, Preset: "source", Exclude: []string{"*.tmp"}}
	inputs := []string{filepath.Join(project, "src"), project, project, filepath.Join(project, "main.go")}
	preview, err := PreviewSources(t.Context(), inputs, selection)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Files != 2 || preview.ExcludedFiles != 3 || preview.Directories != 3 || preview.ExcludedDirectories != 2 {
		t.Fatalf("wrong selection counts: %+v", preview)
	}
	prepared, err := PrepareSelected(t.Context(), inputs, 0, selection)
	if err != nil {
		t.Fatal(err)
	}
	defer prepared.Close()
	var sent []string
	for _, entry := range prepared.Manifest.Files {
		sent = append(sent, entry.Path)
	}
	want := []string{"工程", "工程/main.go", "工程/src", "工程/src/中文.txt", "工程/空目录"}
	if !reflect.DeepEqual(sent, want) || preview.Bytes != prepared.Manifest.TotalBytes() {
		t.Fatalf("preview/manifest disagreement: sent=%v bytes=%d/%d", sent, preview.Bytes, prepared.Manifest.TotalBytes())
	}
	write(".git/config", "excluded file changed after preview")
	resumed, err := PrepareSelectedForResume(t.Context(), inputs, prepared.Manifest.ChunkSize, prepared.Manifest.TransferID, selection)
	if err != nil {
		t.Fatal(err)
	}
	defer resumed.Close()
	if resumed.Manifest.Digest() != prepared.Manifest.Digest() {
		t.Fatal("resume reintroduced excluded content")
	}
	full, err := Prepare(t.Context(), []string{project}, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer full.Close()
	if len(full.Manifest.Files) != len(prepared.Manifest.Files)+5 {
		t.Fatal("reverting selection did not restore complete contents")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := PreviewSources(ctx, inputs, selection); !errors.Is(err, context.Canceled) {
		t.Fatalf("preview ignored cancellation: %v", err)
	}
}

func TestSourceSelectionNeverFollowsSymlink(t *testing.T) {
	project := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(filepath.Join(project, "node_modules"), 0700); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("outside"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(project, "node_modules", "escape")); err != nil {
		t.Skipf("OS symlink fixture unavailable: %v", err)
	}
	selection := &SourceSelection{Version: 1, Preset: "source"}
	preview, err := PreviewSources(t.Context(), []string{project}, selection)
	if err != nil || preview.ExcludedBytes != 0 || preview.ExcludedFiles != 1 {
		t.Fatalf("excluded link was followed: %+v err=%v", preview, err)
	}
	prepared, err := PrepareSelected(t.Context(), []string{project}, 0, selection)
	if err != nil {
		t.Fatal(err)
	}
	prepared.Close()
	if _, err := Prepare(t.Context(), []string{project}, 0); !errors.Is(err, ErrPath) {
		t.Fatalf("included symlink accepted: %v", err)
	}
}

func TestSourceSelectionPreviewTruncatesEntriesWithoutLosingTotals(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "many")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 205; i++ {
		if err := os.WriteFile(filepath.Join(dir, strconv.Itoa(i)+".txt"), []byte("one"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	preview, err := PreviewSources(t.Context(), []string{dir}, nil)
	if err != nil || preview.Files != 205 || preview.Bytes != 615 || !preview.EntriesTruncated || len(preview.Entries) != 200 {
		t.Fatalf("bounded preview lost counts: %+v %v", preview, err)
	}
}
