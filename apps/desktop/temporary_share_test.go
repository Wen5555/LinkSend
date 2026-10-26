package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	coreapp "github.com/Wen5555/LinkSend/internal/app"
	"github.com/Wen5555/LinkSend/internal/clipboardsync"
	"github.com/Wen5555/LinkSend/internal/protocol"
	"github.com/Wen5555/LinkSend/internal/server"
	"github.com/Wen5555/LinkSend/internal/transfer"
)

func temporaryDesktopFixture(t *testing.T) (*App, coreapp.SendDraft, *atomic.Int32) {
	t.Helper()
	s, err := server.New(server.Config{Listen: "127.0.0.1:0", Database: filepath.Join(t.TempDir(), "control.sqlite"), AllowInsecureLoopback: true, ExperimentalShareControl: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	requests := &atomic.Int32{}
	h := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/shares/create" {
			requests.Add(1)
		}
		s.Handler().ServeHTTP(w, r)
	}))
	t.Cleanup(h.Close)
	core, err := coreapp.New(coreapp.Config{DataDir: t.TempDir(), ServerURL: h.URL, AllowInsecureLoopback: true, Name: "desktop-share-fixture"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(core.Shutdown)
	source := filepath.Join(t.TempDir(), "所选目录")
	if err := os.MkdirAll(filepath.Join(source, "exclude"), 0700); err != nil {
		t.Fatal(err)
	}
	for path, body := range map[string]string{"保留.txt": "selected fixture", "exclude/omit.txt": "excluded fixture"} {
		if err := os.WriteFile(filepath.Join(source, filepath.FromSlash(path)), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	draft, err := core.SaveDraft(coreapp.SendDraft{ID: "main", Paths: []string{source}, SourceSelection: &transfer.SourceSelection{Version: 1, Preset: "all", Exclude: []string{"exclude/"}}}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	a := NewApp()
	a.ctx, a.core = context.Background(), core
	return a, draft, requests
}

func TestDesktopTemporaryShareUsesSavedDraftAndRejectsStaleRevisionBeforeRequest(t *testing.T) {
	a, draft, requests := temporaryDesktopFixture(t)
	if _, err := a.CreateTemporaryShare(draft.Revision + 1); !errors.Is(err, coreapp.ErrMetadataConflict) || requests.Load() != 0 {
		t.Fatalf("stale draft reached create: count=%d err=%v", requests.Load(), err)
	}
	created, err := a.CreateTemporaryShare(draft.Revision)
	if err != nil || created.Code == "" || created.Share.Manifest == nil || requests.Load() != 1 {
		t.Fatalf("saved draft create failed: %v", err)
	}
	for _, entry := range created.Share.Manifest.Files {
		if strings.Contains(entry.Path, "omit.txt") || strings.Contains(entry.Path, "/exclude") {
			t.Fatal("desktop did not use saved source selection")
		}
	}
	still, err := a.core.SendDraft("main")
	if err != nil || still.Revision != draft.Revision || len(still.Paths) != 1 {
		t.Fatal("create consumed or rewrote the ordinary main draft")
	}
	snapshots, err := a.TemporaryShares()
	if err != nil || len(snapshots) != 1 {
		t.Fatal("missing local temporary owner")
	}
	raw, _ := json.Marshal(snapshots)
	workspace, _ := a.Workspace()
	tasks, _ := json.Marshal(workspace.Tasks)
	diagnostics, _ := json.Marshal(a.Diagnostics())
	for _, output := range [][]byte{raw, tasks, diagnostics} {
		if strings.Contains(string(output), created.Code) {
			t.Fatal("code escaped the explicit creation result")
		}
	}
}

func TestDesktopTemporaryCodeCopyChecksOwnershipBeforeNativeWriteAndConsumesExactGeneration(t *testing.T) {
	a, draft, _ := temporaryDesktopFixture(t)
	created, err := a.CreateTemporaryShare(draft.Revision)
	if err != nil {
		t.Fatal(err)
	}
	var writes atomic.Int32
	write := func(string) bool { writes.Add(1); return true }
	a.core.ResetClipboard(false, 1)
	wrong := "A" + created.Code[1:]
	if wrong == created.Code {
		wrong = "B" + created.Code[1:]
	}
	if err := a.copyTemporaryShareCode(created.Share.ID, wrong, write, func() uint64 { return 7 }); err == nil || writes.Load() != 0 {
		t.Fatal("wrong code wrote clipboard or altered observation")
	}
	if err := a.copyTemporaryShareCode(created.Share.ID, created.Code, write, func() uint64 { return 7 }); err != nil || writes.Load() != 1 {
		t.Fatalf("owned code copy failed: %v", err)
	}
	if err := a.core.ClipboardChanged(t.Context(), coreapp.ClipboardChange{Generation: 7, Kinds: []clipboardsync.Kind{clipboardsync.Text}}); !errors.Is(err, clipboardsync.ErrStale) {
		t.Fatalf("watch delivery could reuse the protected generation: %v", err)
	}
	if _, err := a.RevokeTemporaryShare(created.Share.ID); err != nil {
		t.Fatal(err)
	}
	if err := a.copyTemporaryShareCode(created.Share.ID, created.Code, write, func() uint64 { return 8 }); protocol.ErrorCode(err) != protocol.ShareRejected || writes.Load() != 1 {
		t.Fatalf("revoked code wrote clipboard: count=%d err=%v", writes.Load(), err)
	}
}
