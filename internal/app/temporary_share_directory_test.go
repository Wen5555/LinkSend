package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Wen5555/LinkSend/internal/protocol"
	"github.com/Wen5555/LinkSend/internal/transfer"
)

func temporaryDirectoryFixture(t *testing.T) temporaryShareFixture {
	t.Helper()
	f := newTemporaryShareFixture(t)
	f.source = filepath.Join(t.TempDir(), "fixture-一次性分享")
	if err := os.MkdirAll(filepath.Join(f.source, "嵌套", "空目录"), 0700); err != nil {
		t.Fatal(err)
	}
	payload := make([]byte, 256<<10)
	for i := range payload {
		payload[i] = byte((i*31 + 17) % 251)
	}
	for _, file := range []struct {
		name string
		body []byte
	}{{"说明.txt", []byte("一次性分享目录内容\n")}, {"零字节.bin", nil}, {"payload.bin", payload}} {
		if err := os.WriteFile(filepath.Join(f.source, file.name), file.body, 0600); err != nil {
			t.Fatal(err)
		}
	}
	return f
}

func TestTemporaryShareDirectoryFrozenManifestSurvivesPreparation(t *testing.T) {
	f := temporaryDirectoryFixture(t)
	created, claimed := createApprovedTemporaryShare(t, f)
	r := temporaryRecordCopy(f.a.temporaryShares.entries[created.Share.ID])
	base := filepath.Base(f.source)
	wantEntries := map[string]string{
		base: "directory", base + "/嵌套": "directory", base + "/嵌套/空目录": "directory",
		base + "/payload.bin": "file", base + "/说明.txt": "file", base + "/零字节.bin": "file",
	}
	if len(r.Manifest.Files) != len(wantEntries) {
		t.Fatal("frozen directory manifest lost expected files or empty directories")
	}
	for _, entry := range r.Manifest.Files {
		if wantEntries[entry.Path] != entry.Type {
			t.Fatal("frozen directory manifest changed an expected path or type")
		}
	}
	// Windows can expose a directory's final timestamp after its children were
	// created and closed. Make that metadata-only change deterministic; neither
	// the path set nor any regular file's bytes or timestamp changes here.
	for _, name := range []string{f.source, filepath.Join(f.source, "嵌套")} {
		info, err := os.Stat(name)
		if err != nil {
			t.Fatal(err)
		}
		changed := info.ModTime().Add(2 * time.Second)
		if err = os.Chtimes(name, changed, changed); err != nil {
			t.Fatal(err)
		}
	}
	destination := t.TempDir()
	cfg := DirectConfig{AllowLoopback: true, CheckTimeout: 5 * time.Second, WaitTimeout: 8 * time.Second}
	receiver, err := f.b.StartTemporaryShare(t.Context(), claimed.ID, cfg)
	if err != nil {
		t.Fatal(err)
	}
	sender, err := f.a.StartTemporaryShare(t.Context(), created.Share.ID, cfg)
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, 8*time.Second, func() bool {
		a, _ := f.a.Task(sender.ID)
		return onlyTemporary(t, f.b).State == "awaiting_save" || a.State == "failed"
	}, "directory preview never reached explicit save")
	if a, _ := f.a.Task(sender.ID); a.State == "failed" {
		t.Fatalf("directory timestamp alone rejected the original task before preview: phase=%s code=%s", a.Phase, a.ErrorCode)
	}
	if _, err = f.b.ConfirmTemporaryShareSave(t.Context(), claimed.ID, destination, transfer.PlanRequest{ConflictPolicy: transfer.ConflictKeepBoth}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, 10*time.Second, func() bool {
		a, _ := f.a.Task(sender.ID)
		b, _ := f.b.Task(receiver.ID)
		return a.State == "completed" && b.State == "completed"
	}, "directory share did not finish original file engine")
	for _, entry := range r.Manifest.Files {
		name := filepath.Join(destination, filepath.FromSlash(entry.Path))
		info, err := os.Stat(name)
		if err != nil || info.IsDir() != (entry.Type == "directory") {
			t.Fatalf("received entry type differs: id=%d err=%v", entry.ID, err)
		}
		if entry.Type == "file" {
			body, err := os.ReadFile(name)
			if err != nil || int64(len(body)) != entry.Size || transfer.Sum(body) != entry.Hash {
				t.Fatalf("received entry content differs: id=%d err=%v", entry.ID, err)
			}
		}
	}
	for _, s := range []*Service{f.a, f.b} {
		all, err := s.TemporaryShares()
		if err != nil || len(all) != 1 || all[0].Manifest == nil || all[0].Manifest.Digest() != r.Manifest.Digest() {
			t.Fatal("directory transfer changed the frozen manifest or created another share", err)
		}
	}
}

func TestTemporaryShareDirectoryReprepareRejectsChangedSources(t *testing.T) {
	for _, mutation := range []struct {
		name   string
		change func(*testing.T, string)
	}{
		{"added_directory", func(t *testing.T, root string) {
			if err := os.Mkdir(filepath.Join(root, "新增目录"), 0700); err != nil {
				t.Fatal(err)
			}
		}},
		{"removed_directory", func(t *testing.T, root string) {
			if err := os.Remove(filepath.Join(root, "嵌套", "空目录")); err != nil {
				t.Fatal(err)
			}
		}},
		{"renamed_directory", func(t *testing.T, root string) {
			if err := os.Rename(filepath.Join(root, "嵌套", "空目录"), filepath.Join(root, "嵌套", "改名目录")); err != nil {
				t.Fatal(err)
			}
		}},
		{"same_size_and_mtime_changed_content", func(t *testing.T, root string) {
			name := filepath.Join(root, "payload.bin")
			info, err := os.Stat(name)
			if err != nil {
				t.Fatal(err)
			}
			body, err := os.ReadFile(name)
			if err != nil {
				t.Fatal(err)
			}
			body[0] ^= 0xff
			if err = os.WriteFile(name, body, 0600); err != nil {
				t.Fatal(err)
			}
			if err = os.Chtimes(name, info.ModTime(), info.ModTime()); err != nil {
				t.Fatal(err)
			}
			after, err := os.Stat(name)
			if err != nil || after.Size() != info.Size() || !after.ModTime().Equal(info.ModTime()) {
				t.Fatal("fixture did not retain original file metadata", err)
			}
		}},
		{"file_mtime_only", func(t *testing.T, root string) {
			name := filepath.Join(root, "payload.bin")
			info, err := os.Stat(name)
			if err != nil {
				t.Fatal(err)
			}
			changed := info.ModTime().Add(2 * time.Second)
			if err = os.Chtimes(name, changed, changed); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(mutation.name, func(t *testing.T) {
			f := temporaryDirectoryFixture(t)
			created, _ := createApprovedTemporaryShare(t, f)
			mutation.change(t, f.source)
			task, err := f.a.StartTemporaryShare(t.Context(), created.Share.ID, DirectConfig{AllowLoopback: true})
			if err != nil {
				t.Fatal(err)
			}
			waitFor(t, time.Second, func() bool { v, _ := f.a.Task(task.ID); return v.State == "failed" }, "changed source was not rejected before connecting")
			got, _ := f.a.Task(task.ID)
			if got.ErrorCode != string(protocol.SourceChanged) || got.SentBytes != 0 || got.SessionID != "" || got.CanRetry {
				t.Fatal("changed source entered a new connection or ordinary retry", got.ErrorCode)
			}
			if owner := onlyTemporary(t, f.a); owner.State != "stopped" {
				t.Fatal("changed source did not stop original grant")
			}
		})
	}
}
