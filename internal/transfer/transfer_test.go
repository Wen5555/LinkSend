package transfer

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type lifecycleTestStream struct {
	closed  atomic.Int32
	aborted atomic.Int32
}

func (s *lifecycleTestStream) Read([]byte) (int, error)    { return 0, io.EOF }
func (s *lifecycleTestStream) Write(b []byte) (int, error) { return len(b), nil }
func (s *lifecycleTestStream) Close() error                { s.closed.Add(1); return nil }
func (s *lifecycleTestStream) Abort()                      { s.aborted.Add(1) }

func TestStreamLifecycleCancelCompletionRace(t *testing.T) {
	for i := 0; i < 1000; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		stream := new(lifecycleTestStream)
		complete, abort := streamLifecycle(ctx, stream)
		done := make(chan struct{})
		go func() {
			complete()
			close(done)
		}()
		cancel()
		abort()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("lifecycle completion deadlocked")
		}
		if stream.closed.Load() > 1 || stream.aborted.Load() > 1 {
			t.Fatalf("lifecycle closed or aborted more than once: closed=%d aborted=%d", stream.closed.Load(), stream.aborted.Load())
		}
	}
}

func TestBLAKE3Vectors(t *testing.T) {
	for input, want := range map[string]string{"": "af1349b9f5f9a1a6a0404dea36dcc9499bcb25c9adc112b7cc9a93cae41f3262", "abc": "6437b3ac38465133ffb63b75273a8db548c558465d79db03fd359c6cd5bd9d85"} {
		if got := Sum([]byte(input)); got != want {
			t.Fatalf("%q: %s", input, got)
		}
	}
}
func TestDangerousPaths(t *testing.T) {
	for _, p := range []string{"../escape", "/etc/passwd", "C:/a", "\\\\server\\file", "a:stream", "a/../b", "CON.txt", "NUL", "COM1", "LPT²", "x.", "x ", "x//y", "x/", ".linksend-secret", "e\u0301", "a\nb"} {
		if ValidatePath(p) == nil {
			t.Errorf("accepted %q", p)
		}
	}
	for _, p := range []string{"中文目录/报告.pdf", "empty", "file.txt"} {
		if err := ValidatePath(p); err != nil {
			t.Errorf("%q: %v", p, err)
		}
	}
}

func fixture(t *testing.T) *Prepared {
	t.Helper()
	dir := t.TempDir()
	folder := filepath.Join(dir, "中文目录")
	if err := os.MkdirAll(filepath.Join(folder, "空目录"), 0700); err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string][]byte{"zero": {}, "data.bin": bytes.Repeat([]byte("hash content\x00"), 15000)} {
		if err := os.WriteFile(filepath.Join(folder, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	p, err := Prepare(context.Background(), []string{folder}, 64<<10)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = p.Close() })
	return p
}

func TestTransferOverStreamAndConsent(t *testing.T) {
	p := fixture(t)
	for _, accept := range []bool{false, true} {
		t.Run(strings.ToUpper(map[bool]string{true: "accept", false: "reject"}[accept]), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			a, b := net.Pipe()
			defer a.Close()
			defer b.Close()
			dest := t.TempDir()
			done := make(chan error, 1)
			go func() {
				_, err := Receive(ctx, b, dest, "verified-peer", func(Manifest) bool { return accept }, nil)
				_ = b.Close()
				done <- err
			}()
			result, err := Send(ctx, a, p, nil)
			if accept {
				if err != nil {
					t.Fatal(err)
				}
				if result.Bytes != p.Manifest.TotalBytes() {
					t.Fatal(result)
				}
			} else if !errors.Is(err, ErrRejected) {
				t.Fatal(err)
			}
			if recvErr := <-done; accept && recvErr != nil {
				t.Fatal(recvErr)
			}
			if accept {
				for _, e := range p.Manifest.Files {
					st, eStat := os.Stat(filepath.Join(dest, filepath.FromSlash(e.Path)))
					if eStat != nil {
						t.Fatal(eStat)
					}
					if e.Type == "file" {
						data, eRead := os.ReadFile(filepath.Join(dest, filepath.FromSlash(e.Path)))
						if eRead != nil || Sum(data) != e.Hash {
							t.Fatal("file integrity", eRead)
						}
					} else if !st.IsDir() {
						t.Fatal("empty directory absent")
					}
				}
			}
		})
	}
}

func TestRestartRehashesCorruptAndUncheckpointedBlocks(t *testing.T) {
	p := fixture(t)
	ctx := context.Background()
	dest := t.TempDir()
	r, err := OpenReceiver(ctx, dest, "peer-A", p.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, p.Manifest.ChunkSize)
	var file FileEntry
	for _, e := range p.Manifest.Files {
		if len(e.Chunks) > 1 {
			file = e
			break
		}
	}
	data, err := p.ReadChunk(ctx, file.ID, 0, buf)
	if err != nil {
		t.Fatal(err)
	}
	if err = r.WriteChunk(ctx, file.ID, 0, data); err != nil {
		t.Fatal(err)
	}
	if err = r.Close(); err != nil {
		t.Fatal(err)
	}
	part := filepath.Join(dest, ".linksend-"+p.Manifest.TransferID, stringID(file.ID)+".part")
	f, err := os.OpenFile(part, os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.WriteAt([]byte("corrupted"), 0); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	r, err = OpenReceiver(ctx, dest, "peer-A", p.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if r.State.Verified[file.ID][0] {
		t.Fatal("corrupt block trusted")
	}
	for _, e := range p.Manifest.Files {
		for i := range e.Chunks {
			data, err := p.ReadChunk(ctx, e.ID, i, buf)
			if err != nil {
				t.Fatal(err)
			}
			if err = r.WriteChunk(ctx, e.ID, i, data); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err = r.Finish(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = OpenReceiver(ctx, dest, "different-peer", p.Manifest); err == nil {
		t.Fatal("accepted peer substitution")
	}
}

func stringID(id uint32) string {
	const digits = "0123456789"
	if id < 10 {
		return string(digits[id])
	}
	panic("test fixture too large")
}

func TestSourceChangedAndConflict(t *testing.T) {
	p := fixture(t)
	var file FileEntry
	for _, e := range p.Manifest.Files {
		if e.Size > 0 {
			file = e
			break
		}
	}
	s := p.sources[file.ID]
	if err := s.root.WriteFile(s.relative, []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := p.ReadChunk(context.Background(), file.ID, 0, make([]byte, p.Manifest.ChunkSize)); !errors.Is(err, ErrChanged) {
		t.Fatal(err)
	}
	p2 := fixture(t)
	dest := t.TempDir()
	r, err := OpenReceiver(context.Background(), dest, "peer", p2.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	buf := make([]byte, p2.Manifest.ChunkSize)
	for _, e := range p2.Manifest.Files {
		if e.Type != "file" {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dest, e.Path)), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dest, e.Path), []byte("existing user data"), 0600); err != nil {
			t.Fatal(err)
		}
		for i := range e.Chunks {
			data, err := p2.ReadChunk(context.Background(), e.ID, i, buf)
			if err != nil {
				t.Fatal(err)
			}
			if err = r.WriteChunk(context.Background(), e.ID, i, data); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err = r.Finish(context.Background()); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
}

func TestCancellationUnblocksRead(t *testing.T) {
	p := fixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	a, b := net.Pipe()
	defer b.Close()
	done := make(chan error, 1)
	go func() { _, err := Send(ctx, a, p, nil); done <- err }()
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("cancel succeeded")
		}
	case <-time.After(time.Second):
		t.Fatal("cancel leaked blocked IO")
	}
}

func TestFrameLimitsAndCaseCollision(t *testing.T) {
	var b bytes.Buffer
	_ = binary.Write(&b, binary.BigEndian, uint32(MaxMetadata+1))
	if _, err := readFrame(&b, MaxMetadata); err == nil {
		t.Fatal("unbounded frame")
	}
	p := fixture(t)
	m := p.Manifest
	m.Files = append([]FileEntry(nil), m.Files...)
	m.Files = append(m.Files, FileEntry{ID: uint32(len(m.Files)), Path: strings.ToUpper(m.Files[0].Path), Type: "directory"})
	if err := m.Validate(); err == nil {
		t.Fatal("case collision accepted")
	}
}

func TestRootRejectsEscapingLink(t *testing.T) {
	p := fixture(t)
	dest := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(dest, "中文目录")); err != nil {
		t.Skipf("symlink creation unavailable: %v", err)
	}
	r, err := OpenReceiver(context.Background(), dest, "peer", p.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if err = r.Finish(context.Background()); !errors.Is(err, ErrPath) {
		t.Fatalf("want dangerous path, got %v", err)
	}
}

func FuzzPath(f *testing.F) {
	for _, p := range []string{"x", "../x", "C:/x", "目录/文件"} {
		f.Add(p)
	}
	f.Fuzz(func(t *testing.T, p string) {
		if len(p) > 2048 {
			t.Skip()
		}
		if ValidatePath(p) == nil && (strings.Contains(p, "\\") || strings.Contains(p, ":")) {
			t.Fatal("unsafe accepted")
		}
	})
}
func FuzzFrame(f *testing.F) {
	f.Add([]byte{0, 0, 0, 1, 'x'})
	f.Fuzz(func(t *testing.T, b []byte) {
		if len(b) > 4096 {
			t.Skip()
		}
		_, _ = readFrame(bytes.NewReader(b), 4096)
	})
}
