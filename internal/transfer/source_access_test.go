package transfer

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestSourceAccessRejectsOpenedFileAndReleasesAfterClose(t *testing.T) {
	name := filepath.Join(t.TempDir(), "owned.txt")
	body := []byte("self-owned source admission fixture")
	if err := os.WriteFile(name, body, 0600); err != nil {
		t.Fatal(err)
	}
	denied := errors.New("owned source denied")
	var opened, released int
	hook := func(_ context.Context, access SourceAccess) (func(), error) {
		if access.Path != name || access.Root == nil || access.Relative != "owned.txt" {
			t.Fatal("missing local source/root binding")
		}
		if access.File == nil {
			return nil, nil
		}
		opened++
		info, err := access.File.Stat()
		if err != nil || !os.SameFile(info, access.Info) || !info.Mode().IsRegular() {
			t.Fatal("hook did not receive the actual source FD", err)
		}
		file := access.File
		return func() {
			if _, err := file.Stat(); err == nil || file.Fd() != ^uintptr(0) {
				t.Error("owner lease released before actual FD close", err)
			}
			released++
		}, denied
	}
	prepared, err := PrepareSelectedWithAccess(t.Context(), []string{name}, 0, nil, hook)
	if !errors.Is(err, denied) || prepared != nil || opened != 1 || released != 1 {
		t.Fatal("actual file admission rejection did not close/release", err, opened, released)
	}
	if actual, err := os.ReadFile(name); err != nil || !bytes.Equal(actual, body) {
		t.Fatal("rejected source was changed", err)
	}
}

func TestSourceAccessParentSelectionAndStickyPreparedRestriction(t *testing.T) {
	parent := filepath.Join(t.TempDir(), "parent")
	private := filepath.Join(parent, "private")
	if err := os.MkdirAll(private, 0700); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{filepath.Join(parent, "public.txt"): "public", filepath.Join(private, "hidden.txt"): "private"} {
		if err := os.WriteFile(name, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	denied := errors.New("private source denied")
	var directoryFDs, fileFDs, released int
	baseHook := func(_ context.Context, access SourceAccess) (func(), error) {
		if access.Path == private || strings.HasPrefix(access.Path, private+string(os.PathSeparator)) {
			t.Error("excluded private tree reached source admission")
			return nil, denied
		}
		if access.File == nil {
			return nil, nil
		}
		if access.Info.IsDir() {
			directoryFDs++
		} else {
			fileFDs++
		}
		file := access.File
		return func() {
			if _, err := file.Stat(); err == nil || file.Fd() != ^uintptr(0) {
				t.Error("selected source lease outlived actual FD close ordering", err)
			}
			released++
		}, nil
	}
	selection := &SourceSelection{Version: 1, Preset: "all", Exclude: []string{"private/"}}
	preview, err := PreviewSourcesWithAccess(t.Context(), []string{parent}, selection, baseHook)
	if err != nil || preview.Files != 1 || preview.ExcludedFiles != 1 {
		t.Fatal("ordinary parent with private exclusion was rejected", err, preview)
	}
	p, err := PrepareSelectedWithAccess(t.Context(), []string{parent}, 0, selection, baseHook)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	var names []string
	var publicID uint32
	for _, entry := range p.Manifest.Files {
		names = append(names, entry.Path)
		if entry.Type == "file" {
			publicID = entry.ID
		}
	}
	if !reflect.DeepEqual(names, []string{"parent", "parent/public.txt"}) || directoryFDs == 0 || fileFDs == 0 || released != directoryFDs+fileFDs {
		t.Fatal("selected actual directory/file ownership or ordering mismatch", names, directoryFDs, fileFDs, released)
	}
	buf := make([]byte, p.Manifest.ChunkSize)
	if actual, err := p.ReadChunk(t.Context(), publicID, 0, buf); err != nil || string(actual) != "public" {
		t.Fatal("allowed source could not be read", err)
	}
	denyReads := false
	extraHook := func(_ context.Context, access SourceAccess) (func(), error) {
		if denyReads && access.File != nil && !access.Info.IsDir() {
			return nil, denied
		}
		return nil, nil
	}
	if err = p.RestrictSourceAccess(t.Context(), extraHook); err != nil {
		t.Fatal("could not append and revalidate a prepared constraint", err)
	}
	denyReads = true
	if _, err = p.ReadChunk(t.Context(), publicID, 0, buf); !errors.Is(err, denied) {
		t.Fatal("prepared chunk reopening bypassed appended policy", err)
	}
	if err = p.RestrictSourceAccess(t.Context(), nil); err != nil {
		t.Fatal(err)
	}
	if err = p.RestrictSourceAccess(t.Context(), func(context.Context, SourceAccess) (func(), error) { return nil, nil }); !errors.Is(err, denied) {
		t.Fatal("later permissive policy replaced an existing restriction", err)
	}
	if _, err = p.ReadChunk(t.Context(), publicID, 0, buf); !errors.Is(err, denied) {
		t.Fatal("rejected restriction update removed the existing rule", err)
	}
	if released != directoryFDs+fileFDs {
		t.Fatal("source rejection left an actually closed FD lease unreleased")
	}
}

func TestSourceAccessCancellationClosesBeforeReleasing(t *testing.T) {
	name := filepath.Join(t.TempDir(), "owned.txt")
	if err := os.WriteFile(name, []byte("owned"), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	var released int
	hook := func(_ context.Context, access SourceAccess) (func(), error) {
		if access.File == nil {
			return nil, nil
		}
		file := access.File
		cancel()
		return func() {
			if _, err := file.Stat(); err == nil || file.Fd() != ^uintptr(0) {
				t.Error("cancellation released before FD close", err)
			}
			released++
		}, nil
	}
	if p, err := PrepareSelectedWithAccess(ctx, []string{name}, 0, nil, hook); !errors.Is(err, context.Canceled) || p != nil || released != 1 {
		t.Fatal("canceled admission did not close then release", err, released)
	}
}
