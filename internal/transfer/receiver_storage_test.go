package transfer

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestReceiveClosingStorageBorrowsActualHandlesAndClosesBeforeReturn(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	source := filepath.Join(t.TempDir(), "目录")
	if err := os.MkdirAll(filepath.Join(source, "空目录"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "file.txt"), []byte("closing observer bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	p, err := Prepare(ctx, []string{source}, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	destination := t.TempDir()
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()
	borrowed := make(chan ReceiveStorage, 1)
	done := make(chan error, 1)
	go func() {
		_, err := ReceiveWithOptions(ctx, b, ReceiveOptions{Directory: destination, Peer: "peer", Accept: func(Manifest) bool { return true }, StorageClosing: func(storage ReceiveStorage) error {
			borrowed <- storage
			if len(storage.Directories) != 2 || len(storage.Files) != 1 || storage.Checkpoint == nil {
				return errors.New("missing live owned storage")
			}
			for _, file := range storage.Files {
				if _, err := file.Stat(); err != nil {
					return err
				}
			}
			for _, file := range storage.Directories {
				if _, err := file.Stat(); err != nil {
					return err
				}
			}
			return nil
		}})
		done <- err
	}()
	if _, err := Send(ctx, a, p, nil); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	storage := <-borrowed
	for _, file := range storage.Files {
		if _, err := file.Stat(); err == nil {
			t.Fatal("engine file outlived receive return", err)
		}
	}
	for _, file := range storage.Directories {
		if _, err := file.Stat(); err == nil {
			t.Fatal("borrowed directory outlived observer", err)
		}
	}
	if _, err := storage.Checkpoint.Stat(); err == nil {
		t.Fatal("checkpoint outlived observer", err)
	}
	if _, err := storage.Root.Stat("."); err == nil {
		t.Fatal("receiver root remained open")
	}
	if _, err := storage.Stage.Stat("."); err == nil {
		t.Fatal("receiver stage remained open")
	}
	if data, err := os.ReadFile(filepath.Join(destination, "目录", "file.txt")); err != nil || string(data) != "closing observer bytes" {
		t.Fatal("received body changed", err)
	}
}

func TestReceiverClosingStorageRejectsForeignCheckpointAndDirectory(t *testing.T) {
	for _, mode := range []string{"checkpoint", "directory"} {
		t.Run(mode, func(t *testing.T) {
			ctx := t.Context()
			source := filepath.Join(t.TempDir(), "empty")
			if err := os.Mkdir(source, 0700); err != nil {
				t.Fatal(err)
			}
			p, err := Prepare(ctx, []string{source}, 0)
			if err != nil {
				t.Fatal(err)
			}
			defer p.Close()
			destination := t.TempDir()
			r, err := OpenReceiver(ctx, destination, "peer", p.Manifest)
			if err != nil {
				t.Fatal(err)
			}
			defer r.Close()
			if err := r.Finish(ctx); err != nil {
				t.Fatal(err)
			}
			var foreign string
			if mode == "checkpoint" {
				foreign = filepath.Join(destination, ".linksend-"+p.Manifest.TransferID, "state.json")
				if err := os.WriteFile(foreign, []byte("foreign checkpoint"), 0600); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := os.Rename(filepath.Join(destination, "empty"), filepath.Join(destination, "original")); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(filepath.Join(destination, "empty"), 0700); err != nil {
					t.Fatal(err)
				}
				foreign = filepath.Join(destination, "empty", "user.txt")
				if err := os.WriteFile(foreign, []byte("foreign directory"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			called := false
			if err := r.observeClosingStorage(func(ReceiveStorage) error { called = true; return nil }); err == nil || called {
				t.Fatal("foreign path was granted live provenance", err)
			}
			if data, err := os.ReadFile(foreign); err != nil || len(data) == 0 {
				t.Fatal("foreign fixture changed", err)
			}
		})
	}
}
