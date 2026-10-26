package app

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Wen5555/LinkSend/internal/protocol"
	"github.com/Wen5555/LinkSend/internal/transfer"
)

func clipboardExportFixture(t *testing.T) (*Service, string, string, []byte) {
	t.Helper()
	parent := t.TempDir()
	s, err := New(Config{DataDir: filepath.Join(parent, "profile"), AllowInsecureLoopback: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Shutdown)
	private := filepath.Join(s.cfg.DataDir, "clipboard-files", protocol.RandomID(), "owned.txt")
	if err = os.MkdirAll(filepath.Dir(private), 0700); err != nil {
		t.Fatal(err)
	}
	body := []byte("self-owned private cache source fixture")
	if err = os.WriteFile(private, body, 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(parent, "public.txt"), []byte("public source"), 0600); err != nil {
		t.Fatal(err)
	}
	return s, parent, private, body
}

func TestClipboardCacheExportsActualSourcesAndExcludedParent(t *testing.T) {
	s, parent, private, body := clipboardExportFixture(t)
	if p, err := s.prepareUserSources(t.Context(), []string{private}, 0, nil); p != nil || !errors.Is(err, errClipboardCachePrivate) {
		if p != nil {
			_ = p.Close()
		}
		t.Fatal("private source reached preparation", err)
	}
	selection := &transfer.SourceSelection{Version: 1, Preset: "all", Exclude: []string{"profile/"}}
	p, err := s.prepareUserSources(t.Context(), []string{parent}, 0, selection)
	if err != nil {
		t.Fatal("ordinary parent with private subtree excluded was rejected", err)
	}
	defer p.Close()
	files := 0
	for _, entry := range p.Manifest.Files {
		if entry.Type == "file" {
			files++
			chunk, e := p.ReadChunk(t.Context(), entry.ID, 0, make([]byte, p.Manifest.ChunkSize))
			if e != nil || string(chunk) != "public source" {
				t.Fatal("ordinary selected source failed", e)
			}
		}
	}
	if files != 1 {
		t.Fatal("private/excluded sources entered manifest")
	}
	legacy, err := transfer.Prepare(t.Context(), []string{private}, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer legacy.Close()
	if err = legacy.RestrictSourceAccess(t.Context(), s.clipboardSourceAccess); !errors.Is(err, errClipboardCachePrivate) {
		t.Fatal("caller-prepared private source bypassed app restriction", err)
	}
	if _, err = legacy.ReadChunk(t.Context(), 0, 0, make([]byte, legacy.Manifest.ChunkSize)); !errors.Is(err, errClipboardCachePrivate) {
		t.Fatal("later chunk reopening bypassed sticky restriction", err)
	}
	outside := filepath.Join(t.TempDir(), "independent-user-name.txt")
	if err = os.Link(private, outside); err != nil {
		t.Fatal(err)
	}
	copySource, err := s.prepareUserSources(t.Context(), []string{outside}, 0, nil)
	if err != nil {
		t.Fatal("external user hardlink name was incorrectly claimed as private cache", err)
	}
	_ = copySource.Close()
	if data, e := os.ReadFile(private); e != nil || !bytes.Equal(data, body) {
		t.Fatal("private cache bytes changed", e)
	}
}

func TestClipboardCacheExportsOldHistoryAndPreviewMetadata(t *testing.T) {
	s, parent, private, body := clipboardExportFixture(t)
	task, _ := inboxIndexedFixture(t, s, []string{private}, "send")
	task.finish("completed", nil)
	if _, err := s.ResolveInboxFile(t.Context(), task.snap.ID, 0); !errors.Is(err, errClipboardCachePrivate) {
		t.Fatal("old history resolved a private cache path for Open/Reveal", err)
	}
	if _, err := s.ResendInbox(t.Context(), ResendInboxRequest{TaskID: task.snap.ID, RequestID: "private-cache-resend", WaitForPeer: true}); !errors.Is(err, errClipboardCachePrivate) {
		t.Fatal("old persisted source was prepared for resend", err)
	}
	var queued int
	if err := s.store.db.QueryRow(`SELECT count(*) FROM send_queue`).Scan(&queued); err != nil || queued != 0 {
		t.Fatal("private resend created durable queue", err)
	}
	_, err := s.SaveDraft(SendDraft{Paths: []string{parent}, SourceSelection: &transfer.SourceSelection{Version: 1, Preset: "all", Exclude: []string{"profile/"}}}, parent)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := s.PreviewDraft()
	if err != nil || !preview.Complete || preview.Files != 1 {
		t.Fatal("normal small parent preview regressed", err, preview.Problem)
	}
	publicVisible := false
	for _, entry := range preview.Entries {
		if strings.HasSuffix(entry.Path, "/public.txt") {
			publicVisible = true
		}
		if strings.Contains(entry.Path, "clipboard-files") || strings.Contains(entry.Path, filepath.Base(filepath.Dir(private))) {
			t.Fatal("excluded preview metadata exposed cache names")
		}
	}
	if !publicVisible {
		t.Fatal("normal small preview lost its public file entry")
	}
	if data, e := os.ReadFile(private); e != nil || !bytes.Equal(data, body) {
		t.Fatal("history/preview touched private source", e)
	}
}
