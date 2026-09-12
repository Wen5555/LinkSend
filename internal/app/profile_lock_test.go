package app

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestProfileLockServiceRejectsSecondWriterAndReopens(t *testing.T) {
	dir := t.TempDir()
	first, err := New(Config{DataDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(first.Shutdown)
	identityID := first.Identity().ID
	duplicate, err := New(Config{DataDir: dir})
	if duplicate != nil || !errors.Is(err, ErrProfileInUse) {
		if duplicate != nil {
			duplicate.Shutdown()
		}
		t.Fatalf("second service writer accepted: %v", err)
	}
	first.Shutdown()
	reopened, err := New(Config{DataDir: dir})
	if err != nil {
		t.Fatalf("service shutdown retained ownership: %v", err)
	}
	t.Cleanup(reopened.Shutdown)
	if got := reopened.Identity().ID; got != identityID {
		t.Fatalf("reopen changed profile identity: %s != %s", got, identityID)
	}
}

func TestProfileLockExclusiveAndIndependent(t *testing.T) {
	dir := t.TempDir()
	first, err := acquireProfileLock(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = first.Close() })
	second, err := acquireProfileLock(filepath.Join(dir, "."))
	if second != nil || !errors.Is(err, ErrProfileInUse) {
		if second != nil {
			_ = second.Close()
		}
		t.Fatalf("duplicate writer accepted: lock=%v err=%v", second, err)
	}
	other, err := acquireProfileLock(t.TempDir())
	if err != nil {
		t.Fatalf("independent profile refused: %v", err)
	}
	if err := other.Close(); err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("repeat close: %v", err)
	}
	reopened, err := acquireProfileLock(dir)
	if err != nil {
		t.Fatalf("profile not released: %v", err)
	}
	_ = reopened.Close()
	if _, err := os.Stat(filepath.Join(dir, ".profile.lock")); err != nil {
		t.Fatalf("stable lock inode was removed: %v", err)
	}
}

func TestProfileLockPreservesExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".profile.lock")
	const marker = "existing marker"
	if err := os.WriteFile(path, []byte(marker), 0600); err != nil {
		t.Fatal(err)
	}
	lock, err := acquireProfileLock(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := lock.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != marker {
		t.Fatalf("existing contents altered: %q, %v", data, err)
	}
}

func TestProfileLockSubprocessHelper(t *testing.T) {
	if os.Getenv("LINKSEND_TEST_PROFILE_LOCK_CHILD") != "1" {
		return
	}
	lock, err := acquireProfileLock(os.Getenv("LINKSEND_TEST_PROFILE_LOCK_DIR"))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	// Intentionally never close the handle: the parent tests OS cleanup, rather
	// than a Go defer that would only run on graceful application shutdown.
	_ = lock
	fmt.Fprintln(os.Stdout, "LOCKED")
	var b [1]byte
	_, _ = os.Stdin.Read(b[:])
	runtime.KeepAlive(lock)
	os.Exit(0)
}

func TestProfileLockProcessExitAndKillReleaseOwnership(t *testing.T) {
	for _, kill := range []bool{false, true} {
		name := "exit"
		if kill {
			name = "kill"
		}
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestProfileLockSubprocessHelper$")
			cmd.Env = append(os.Environ(), "LINKSEND_TEST_PROFILE_LOCK_CHILD=1", "LINKSEND_TEST_PROFILE_LOCK_DIR="+dir)
			stdout, err := cmd.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			stdin, err := cmd.StdinPipe()
			if err != nil {
				t.Fatal(err)
			}
			cmd.Stderr = os.Stderr
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			defer func() { _ = cmd.Process.Kill() }()
			line, err := bufio.NewReader(stdout).ReadString('\n')
			if err != nil || line != "LOCKED\n" {
				_ = cmd.Process.Kill()
				_ = cmd.Wait()
				t.Fatalf("child failed to lock: %q %v", line, err)
			}
			duplicate, err := acquireProfileLock(dir)
			if duplicate != nil || !errors.Is(err, ErrProfileInUse) {
				if duplicate != nil {
					_ = duplicate.Close()
				}
				_ = cmd.Process.Kill()
				_ = cmd.Wait()
				t.Fatalf("child ownership not exclusive: %v", err)
			}
			if kill {
				if err := cmd.Process.Kill(); err != nil {
					t.Fatal(err)
				}
			} else if _, err := io.WriteString(stdin, "q"); err != nil {
				t.Fatal(err)
			}
			_ = stdin.Close()
			err = cmd.Wait()
			if !kill && err != nil {
				t.Fatalf("normal child exit failed: %v", err)
			}
			if ctx.Err() != nil {
				t.Fatalf("child did not exit before deadline: %v", ctx.Err())
			}
			reopened, err := acquireProfileLock(dir)
			if err != nil {
				t.Fatalf("OS did not release ownership after %s: %v", name, err)
			}
			_ = reopened.Close()
		})
	}
}
