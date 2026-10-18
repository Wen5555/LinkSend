package app

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wen5555/LinkSend/internal/clipboardsync"
	"github.com/Wen5555/LinkSend/internal/transport"
)

type clipboardFilesFixture struct {
	directFixtureServices
	ctx        context.Context
	aGen, bGen *atomic.Uint64
	written    chan []string
	source     string
}

func newClipboardFilesFixture(t *testing.T, legacy ...bool) clipboardFilesFixture {
	t.Helper()
	f := newDirectFixtureServices(t)
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	t.Cleanup(cancel)
	source := filepath.Join(t.TempDir(), "copied.txt")
	if err := os.WriteFile(source, []byte("clipboard file bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	var aGen, bGen atomic.Uint64
	aGen.Store(1)
	bGen.Store(1)
	written := make(chan []string, 1)
	for _, service := range []*Service{f.a, f.b} {
		if _, err := service.Devices(ctx); err != nil {
			t.Fatal(err)
		}
	}
	f.a.ConfigureClipboard(ClipboardAdapter{Generation: aGen.Load, ReadFiles: func(context.Context, uint64) ([]string, uint64, error) { return []string{source}, aGen.Load(), nil }})
	f.b.ConfigureClipboard(ClipboardAdapter{Generation: bGen.Load, WriteFiles: func(paths []string, expected uint64, _ time.Time) (uint64, error) {
		if bGen.Load() != expected {
			return 0, clipboardsync.ErrConflict
		}
		bGen.Store(expected + 1)
		written <- append([]string(nil), paths...)
		return expected + 1, nil
	}})
	for _, item := range []struct {
		s    *Service
		peer string
	}{{f.a, f.bID.ID()}, {f.b, f.aID.ID()}} {
		for _, direction := range []string{"send", "receive"} {
			if _, err := item.s.SetClipboardGrant(ctx, ClipboardGrantPatch{PeerID: item.peer, Direction: direction, Kind: "files", Enabled: true}); err != nil {
				t.Fatal(err)
			}
		}
	}
	cfg := DirectConfig{AllowLoopback: true, CheckTimeout: 5 * time.Second, disableClipboardFiles: len(legacy) > 0 && legacy[0]}
	if err := f.b.StartInbox(t.TempDir(), cfg); err != nil {
		t.Fatal(err)
	}
	waitFor(t, 2*time.Second, func() bool { return f.b.InboxStatus().Listening }, "receiver not listening")
	if err := f.a.EnsureClipboardSessions(ctx, cfg); err != nil {
		t.Fatal(err)
	}
	waitFor(t, 2*time.Second, func() bool {
		for _, s := range f.a.ClipboardPeerStatuses() {
			if s.SendReady || len(legacy) > 0 && legacy[0] && s.State == "connecting" {
				return true
			}
		}
		return false
	}, "files lease not ready")
	return clipboardFilesFixture{f, ctx, &aGen, &bGen, written, source}
}

func (f clipboardFilesFixture) copy(t *testing.T, generation uint64) {
	t.Helper()
	f.aGen.Store(generation)
	if err := f.a.ClipboardChanged(f.ctx, ClipboardChange{Generation: generation, Kinds: []clipboardsync.Kind{clipboardsync.Files}}); err != nil {
		t.Fatal(err)
	}
}

func TestClipboardFilesUseAuthenticatedFileProtocolAndNativePaths(t *testing.T) {
	f := newClipboardFilesFixture(t)
	ctx, written, source := f.ctx, f.written, f.source
	f.copy(t, 2)
	select {
	case paths := <-written:
		if len(paths) != 1 || paths[0] == source {
			t.Fatalf("unsafe received paths: %v", paths)
		}
		data, err := os.ReadFile(paths[0])
		if err != nil || string(data) != "clipboard file bytes" {
			t.Fatal("received clipboard file mismatch", err)
		}
	case <-ctx.Done():
		t.Fatalf("file clipboard did not commit: a=%+v b=%+v", f.a.ClipboardFileProgresses(), f.b.ClipboardFileProgresses())
	}
	waitFor(t, 2*time.Second, func() bool {
		for _, p := range f.a.ClipboardFileProgresses() {
			if p.State == "completed" {
				return true
			}
		}
		return false
	}, "sender did not complete file protocol")
	waitFor(t, time.Second, func() bool {
		for _, status := range f.b.ClipboardPeerStatuses() {
			if status.Waiting != "" || status.Error != "" {
				return false
			}
		}
		return true
	}, "receiver retained a stale file-copy status")
}

func TestClipboardFilesCancellationBeforeBodyDoesNotPublish(t *testing.T) {
	for _, mode := range []string{"sender_cancel", "receiver_revoke", "receiver_new_copy", "receiver_pause"} {
		t.Run(mode, func(t *testing.T) {
			f := newClipboardFilesFixture(t)
			f.a.clipboardFilesMu.Lock()
			f.a.clipboardFileSendSlots = make(chan struct{}, 2)
			slots := f.a.clipboardFileSendSlots
			slots <- struct{}{}
			slots <- struct{}{}
			f.a.clipboardFilesMu.Unlock()
			f.copy(t, 2)
			waitFor(t, 2*time.Second, func() bool { return len(f.b.ClipboardFileProgresses()) == 1 }, "file offer not reserved")
			id := f.a.ClipboardFileProgresses()[0].ID
			switch mode {
			case "sender_cancel":
				f.a.CancelClipboardFile(id)
			case "receiver_revoke":
				grants, err := f.b.ClipboardGrants(f.ctx, f.aID.ID())
				if err != nil {
					t.Fatal(err)
				}
				for _, g := range grants {
					if g.Direction == "receive" && g.Kind == "files" {
						if _, err = f.b.SetClipboardGrant(f.ctx, ClipboardGrantPatch{PeerID: g.PeerID, Direction: g.Direction, Kind: g.Kind, Enabled: false, ExpectedRevision: g.Revision}); err != nil {
							t.Fatal(err)
						}
					}
				}
			case "receiver_new_copy":
				f.bGen.Store(2)
				if err := f.b.ClipboardChanged(f.ctx, ClipboardChange{Generation: 2}); err != nil {
					t.Fatal(err)
				}
			case "receiver_pause":
				f.b.ResetClipboard(true, f.bGen.Load())
			}
			waitFor(t, 2*time.Second, func() bool {
				a, b := f.a.ClipboardFileProgresses(), f.b.ClipboardFileProgresses()
				return len(a) == 1 && len(b) == 1 && a[0].State == "cancelled" && b[0].State == "cancelled"
			}, "cancel did not reach both pending file jobs")
			<-slots
			<-slots
			select {
			case paths := <-f.written:
				t.Fatalf("cancelled file was published: %v", paths)
			default:
			}
			if mode == "sender_cancel" || mode == "receiver_new_copy" {
				f.copy(t, 3)
				select {
				case <-f.written:
				case <-f.ctx.Done():
					t.Fatal("new copy after cancellation did not complete", f.a.ClipboardFileProgresses(), f.b.ClipboardFileProgresses())
				}
			}
		})
	}
}

func TestClipboardFilesCapabilityAndGrantGateNativeRead(t *testing.T) {
	for _, mode := range []string{"legacy_peer", "no_send_grant"} {
		t.Run(mode, func(t *testing.T) {
			f := newClipboardFilesFixture(t, mode == "legacy_peer")
			var reads atomic.Int32
			f.a.clipboardAdapterMu.Lock()
			f.a.clipboardAdapter.ReadFiles = func(context.Context, uint64) ([]string, uint64, error) {
				reads.Add(1)
				return []string{f.source}, 2, nil
			}
			f.a.clipboardAdapterMu.Unlock()
			if mode != "legacy_peer" {
				grants, _ := f.a.ClipboardGrants(f.ctx, f.bID.ID())
				for _, g := range grants {
					if g.Direction == "send" && g.Kind == "files" {
						if _, err := f.a.SetClipboardGrant(f.ctx, ClipboardGrantPatch{PeerID: g.PeerID, Direction: g.Direction, Kind: g.Kind, Enabled: false, ExpectedRevision: g.Revision}); err != nil {
							t.Fatal(err)
						}
					}
				}
			}
			f.copy(t, 2)
			if reads.Load() != 0 || len(f.a.ClipboardFileProgresses()) != 0 {
				t.Fatal("native file metadata read without negotiated capability and permission")
			}
		})
	}
}

func TestClipboardFileStreamRejectsMalformedBinding(t *testing.T) {
	f := newClipboardFilesFixture(t)
	f.a.directPoolMu.Lock()
	var peer *PeerSession
	for _, p := range f.a.directPool {
		peer = p.peer
	}
	f.a.directPoolMu.Unlock()
	for _, mode := range []string{"oversized", "zero", "bad_id", "unreserved"} {
		t.Run(mode, func(t *testing.T) {
			stream, err := peer.Data.Conn.OpenStreamSync(f.ctx)
			if err != nil {
				t.Fatal(err)
			}
			wrapped := transport.WrapStream(stream)
			defer wrapped.Abort()
			data, _ := json.Marshal(map[string]string{"id": "../not-a-transfer-id", "digest": "bad"})
			if mode == "unreserved" {
				data, _ = json.Marshal(map[string]string{"id": "11111111111111111111111111111111", "digest": "1111111111111111111111111111111111111111111111111111111111111111"})
			}
			var size [4]byte
			binary.BigEndian.PutUint32(size[:], uint32(len(data)))
			if mode == "oversized" {
				binary.BigEndian.PutUint32(size[:], 2049)
			}
			if mode == "zero" {
				binary.BigEndian.PutUint32(size[:], 0)
			}
			for _, part := range [][]byte{clipboardFileMagic, size[:], data} {
				if _, err = wrapped.Write(part); err != nil {
					break
				}
			}
			_ = stream.Close()
			_ = stream.SetReadDeadline(time.Now().Add(2 * time.Second))
			var one [1]byte
			if _, err = wrapped.Read(one[:]); err == nil {
				t.Fatal("malformed clipboard stream was accepted")
			}
		})
	}
	if len(f.b.ClipboardFileProgresses()) != 0 {
		t.Fatal("unreserved stream created a receive job")
	}
}

func TestClipboardFileCompletionCannotClearNewerCopyStatus(t *testing.T) {
	s := &Service{clipboardPeerRuntime: map[string]clipboardPeerRuntimeStatus{}}
	ctx, cancel := context.WithCancel(t.Context())
	job := &clipboardFileJob{peer: &PeerSession{PeerID: "peer"}, ctx: ctx, cancel: cancel, progress: ClipboardFileProgress{ID: "old", State: "transferring"}}
	s.setClipboardWaiting("peer", "copying_files", "new")
	s.finishClipboardFile(job, nil)
	if got := s.clipboardPeerRuntime["peer"]; got.FileID != "new" || got.Waiting != "copying_files" {
		t.Fatal("old completion cleared new copy", got)
	}
	for _, statusError := range []string{"file_copy_failed", "file_copy_disk_full", "write_failed"} {
		ctx, cancel = context.WithCancel(t.Context())
		job = &clipboardFileJob{peer: &PeerSession{PeerID: "peer"}, ctx: ctx, cancel: cancel, progress: ClipboardFileProgress{ID: "new", State: "transferring"}}
		s.clipboardPeerRuntime["peer"] = clipboardPeerRuntimeStatus{FileID: "new", Waiting: "copying_files", Error: statusError}
		s.finishClipboardFile(job, nil)
		got := s.clipboardPeerRuntime["peer"]
		if got.Waiting != "" || got.FileID != "" || (clipboardFileError(statusError) && got.Error != "") || (statusError == "write_failed" && got.Error != statusError) {
			t.Fatal("completion changed an unrelated error or retained old file status", got)
		}
	}
}
