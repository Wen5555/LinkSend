package transfer

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestReceivePinnedRootIgnoresReplacedDirectoryAndCheckpoint(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	source := filepath.Join(t.TempDir(), "中文目录")
	if err := os.MkdirAll(filepath.Join(source, "空目录"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "payload.txt"), []byte("pinned destination bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	p, err := Prepare(ctx, []string{source}, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	owned, replacement := t.TempDir(), t.TempDir()
	root, err := os.OpenRoot(owned)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	// A directory pathname is no longer storage authority after its owner has
	// pinned a handle. This unrelated pathname even has a forged checkpoint.
	stage := filepath.Join(replacement, ".linksend-"+p.Manifest.TransferID)
	if err := os.Mkdir(stage, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stage, "state.json"), []byte("foreign checkpoint"), 0600); err != nil {
		t.Fatal(err)
	}
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()
	done := make(chan error, 1)
	go func() {
		_, err := ReceiveWithOptions(ctx, b, ReceiveOptions{Directory: replacement, Root: root, Peer: "pinned-peer", Plan: func(context.Context, Offer) (ReceivePlan, error) { return FullReceivePlan(p.Manifest), nil }})
		done <- err
	}()
	if _, err := SendWithOptions(ctx, a, p, SendOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	got, err := root.ReadFile("中文目录/payload.txt")
	if err != nil || string(got) != "pinned destination bytes" {
		t.Fatal("receiver did not use borrowed pinned root", err)
	}
	if info, err := root.Stat("中文目录/空目录"); err != nil || !info.IsDir() {
		t.Fatal("pinned-root empty directory missing", err)
	}
	if _, err := os.Stat(filepath.Join(replacement, "中文目录")); !os.IsNotExist(err) {
		t.Fatal("receiver wrote outside pinned destination", err)
	}
	if got, err := os.ReadFile(filepath.Join(stage, "state.json")); err != nil || string(got) != "foreign checkpoint" {
		t.Fatal("foreign checkpoint was read or replaced", err)
	}
}

func TestReceiveClosedPinnedRootDoesNotFallbackToPath(t *testing.T) {
	dest := t.TempDir()
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	_ = root.Close()
	m := Manifest{Version: 1, TransferID: "0123456789abcdef0123456789abcdef", ChunkSize: DefaultChunkSize,
		Files: []FileEntry{{ID: 0, Path: "empty", Type: "directory"}}}
	if _, err := openReceiverContentRoot(t.Context(), dest, "peer", m, nil, nil, nil, root); err == nil {
		t.Fatal("closed destination fell back to an unrelated pathname")
	}
	if entries, err := os.ReadDir(dest); err != nil || len(entries) != 0 {
		t.Fatal("closed root failure modified pathname destination", err)
	}
}
