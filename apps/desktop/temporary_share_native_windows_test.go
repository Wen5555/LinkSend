//go:build windows

package main

import (
	"bytes"
	"context"
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf16"
	"unsafe"

	"github.com/Wen5555/LinkSend/apps/desktop/nativeclipboard"
	coreapp "github.com/Wen5555/LinkSend/internal/app"
	"github.com/Wen5555/LinkSend/internal/clipboardsync"
	"github.com/Wen5555/LinkSend/internal/identity"
	"github.com/Wen5555/LinkSend/internal/server"
	"golang.org/x/sys/windows"
)

// This opt-in test uses a real Windows clipboard and real Pion/QUIC. It skips
// before mutation if every original format cannot be safely backed up in
// memory. No original clipboard bytes or share code are written to a log/file.
func TestDesktopTemporaryCodeNativeClipboardDoesNotSyncOrCache(t *testing.T) {
	if os.Getenv("LINKSEND_TEMP_SHARE_NATIVE_CLIPBOARD") != "1" {
		t.Skip("opt-in real Windows clipboard backup/write/restore regression")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	board := newTemporaryNativeClipboard(t)
	defer board.close(t)
	root := t.TempDir()
	backend, err := server.New(server.Config{Listen: "127.0.0.1:0", Database: filepath.Join(root, "server.sqlite"), BootstrapToken: "native-share-code-fixture-token-0123456789", AllowInsecureLoopback: true, AllowLoopbackCandidates: true, ExperimentalShareControl: true})
	if err != nil {
		t.Fatal(err)
	}
	defer backend.Close()
	h := httptest.NewServer(backend.Handler())
	defer h.Close()
	aID, _ := identity.Generate()
	bID, _ := identity.Generate()
	a, err := coreapp.New(coreapp.Config{DataDir: filepath.Join(root, "a"), ServerURL: h.URL, Identity: aID, AllowInsecureLoopback: true})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Shutdown()
	b, err := coreapp.New(coreapp.Config{DataDir: filepath.Join(root, "b"), ServerURL: h.URL, Identity: bID, AllowInsecureLoopback: true})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Shutdown()
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	if _, err = a.Bootstrap(ctx, "native-share-code-fixture-token-0123456789", "native-sender"); err != nil {
		t.Fatal(err)
	}
	invitation, err := a.CreateInvitation(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = b.Join(ctx, invitation.Token, "isolated-receiver"); err != nil {
		t.Fatal(err)
	}
	if _, err = a.Devices(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = b.Devices(ctx); err != nil {
		t.Fatal(err)
	}
	var nativeReads atomic.Int32
	a.ConfigureClipboard(coreapp.ClipboardAdapter{Generation: nativeclipboard.Generation, Read: func(ctx context.Context, kind clipboardsync.Kind, generation uint64) ([]byte, uint64, error) {
		nativeReads.Add(1)
		return nativeclipboard.ReadPayload(ctx, kind, generation)
	}})
	var receiverGeneration atomic.Uint64
	receiverGeneration.Store(10)
	received := make(chan string, 4)
	b.ConfigureClipboard(coreapp.ClipboardAdapter{Generation: receiverGeneration.Load, Validate: nativeclipboard.ValidatePayload, Write: func(_ clipboardsync.Kind, body []byte, expected uint64, before time.Time) (uint64, error) {
		if receiverGeneration.Load() != expected || !before.IsZero() && !time.Now().Before(before) {
			return expected, clipboardsync.ErrStale
		}
		received <- string(body)
		return receiverGeneration.Add(1), nil
	}})
	for _, item := range []struct {
		s         *coreapp.Service
		peer      string
		direction string
	}{{a, bID.ID(), "send"}, {a, bID.ID(), "receive"}, {b, aID.ID(), "send"}, {b, aID.ID(), "receive"}} {
		if _, err := item.s.SetClipboardGrant(ctx, coreapp.ClipboardGrantPatch{PeerID: item.peer, Direction: item.direction, Kind: "text", Enabled: true}); err != nil {
			t.Fatal(err)
		}
	}
	a.ResetClipboard(false, nativeclipboard.Generation())
	b.ResetClipboard(false, receiverGeneration.Load())
	cfg := coreapp.DirectConfig{BindAddress: "127.0.0.1:0", AllowLoopback: true, CheckTimeout: 5 * time.Second}
	if err := b.StartInbox(filepath.Join(root, "received"), cfg); err != nil {
		t.Fatal(err)
	}
	waitTemporaryClipboardFixtureInbox(t, ctx, b)
	if err := a.EnsureClipboardSessions(ctx, cfg); err != nil {
		t.Fatal(err)
	}
	waitTemporaryClipboardFixtureReady(t, ctx, "initial_leases", a, b, aID.ID(), bID.ID())
	source := filepath.Join(root, "fixture.txt")
	if err := os.WriteFile(source, []byte("native secret-copy fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	created, err := a.CreateTemporaryShare(ctx, []string{source}, nil)
	if err != nil {
		t.Fatal(err)
	}
	app := NewApp()
	app.ctx, app.core = ctx, a
	app.clipboardWatchStart = func() (func(), error) { return func() {}, nil }
	for _, paused := range []bool{false, true} {
		if paused {
			app.SetClipboardPaused(true)
		}
		beforeSecret := nativeReads.Load()
		if err := app.copyTemporaryShareCode(created.Share.ID, created.Code, board.writeText, nativeclipboard.Generation); err != nil {
			t.Fatal(err)
		}
		secretGeneration := nativeclipboard.Generation()
		app.deliverClipboardChange(nativeclipboard.Change{Sequence: secretGeneration, Text: true})
		if paused {
			app.SetClipboardPaused(false) // Existing reset reads the actual current native generation.
			app.deliverClipboardChange(nativeclipboard.Change{Sequence: secretGeneration, Text: true})
			waitTemporaryClipboardFixtureReady(t, ctx, "resumed_leases", a, b, aID.ID(), bID.ID())
		}
		if nativeReads.Load() != beforeSecret {
			t.Fatal("secret generation reached the native payload reader")
		}
		select {
		case <-received:
			t.Fatal("share code was forwarded by automatic clipboard")
		default:
		}
		before := nativeReads.Load()
		text := "normal native copy after protected share code"
		if paused {
			text += " and resume"
		}
		if !board.writeText(text) {
			t.Fatal("native normal-text write failed")
		}
		app.deliverClipboardChange(nativeclipboard.Change{Sequence: nativeclipboard.Generation(), Text: true})
		select {
		case value := <-received:
			if value != text {
				t.Fatal("normal new copy did not arrive exactly")
			}
		case <-ctx.Done():
			t.Fatal("normal new native copy did not sync after protected code")
		}
		if nativeReads.Load() <= before {
			t.Fatal("normal native copy did not exercise the native reader")
		}
		if err := scanTemporaryNativeProfile(filepath.Join(root, "a"), []byte(created.Code)); err != nil {
			t.Fatal(err)
		}
	}
	app.closeClipboardOwner()
	t.Log("PASS: protected active/paused native generations did not read/cache/send code; subsequent native text crossed authenticated QUIC; original clipboard restoration follows")
}

var errTemporaryNativeProfileCode = errors.New("plaintext code found in private profile storage")

func scanTemporaryNativeProfile(profile string, code []byte) error {
	profile = filepath.Clean(profile)
	controlLock := filepath.Join(profile, ".profile.lock")
	return filepath.WalkDir(profile, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == controlLock {
			info, err := entry.Info()
			if err != nil {
				return err
			}
			// The live profile owner byte-locks this zero-byte control file on
			// Windows. No other path or read error is excluded from the scan.
			if !info.Mode().IsRegular() || info.Size() != 0 {
				return errors.New("profile control lock is not an empty regular file")
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if bytes.Contains(body, code) {
			return errTemporaryNativeProfileCode
		}
		return nil
	})
}

func TestDesktopTemporaryCodeProfileScanHandlesOwnedLock(t *testing.T) {
	profile := t.TempDir()
	service, err := coreapp.New(coreapp.Config{DataDir: profile})
	if err != nil {
		t.Fatal(err)
	}
	defer service.Shutdown()
	lock := filepath.Join(profile, ".profile.lock")
	info, err := os.Stat(lock)
	if err != nil || !info.Mode().IsRegular() || info.Size() != 0 {
		t.Fatal("fixture did not establish an empty profile control lock", err)
	}
	if _, err = os.ReadFile(lock); !errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
		t.Fatal("fixture did not hold the real Windows profile lock", err)
	}
	if err = scanTemporaryNativeProfile(profile, []byte("synthetic-profile-scan-code")); err != nil {
		t.Fatal(err)
	}
}

func TestDesktopTemporaryCodeProfileScanRejectsOtherStorage(t *testing.T) {
	const fakeCode = "synthetic-profile-scan-code"
	for _, relative := range []string{"state.bin", "nested/.profile.lock", "another.lock"} {
		t.Run(relative, func(t *testing.T) {
			profile := t.TempDir()
			name := filepath.Join(profile, filepath.FromSlash(relative))
			if err := os.MkdirAll(filepath.Dir(name), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(name, []byte(fakeCode), 0600); err != nil {
				t.Fatal(err)
			}
			if err := scanTemporaryNativeProfile(profile, []byte(fakeCode)); !errors.Is(err, errTemporaryNativeProfileCode) {
				t.Fatal("plaintext in profile storage was not rejected", err)
			}
		})
	}
	t.Run("nonempty_root_control_lock", func(t *testing.T) {
		profile := t.TempDir()
		if err := os.WriteFile(filepath.Join(profile, ".profile.lock"), []byte("nonempty-control-file"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := scanTemporaryNativeProfile(profile, []byte(fakeCode)); err == nil {
			t.Fatal("nonempty root profile lock was excluded")
		}
	})
	t.Run("other_locked_file", func(t *testing.T) {
		profile := t.TempDir()
		file, err := os.OpenFile(filepath.Join(profile, "another.lock"), os.O_CREATE|os.O_RDWR, 0600)
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		var overlapped windows.Overlapped
		if err = windows.LockFileEx(windows.Handle(file.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &overlapped); err != nil {
			t.Fatal(err)
		}
		defer windows.UnlockFileEx(windows.Handle(file.Fd()), 0, 1, 0, &overlapped)
		if err = scanTemporaryNativeProfile(profile, []byte(fakeCode)); !errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
			t.Fatal("read errors from other locked files were ignored", err)
		}
	})
	t.Run("root_control_lock_is_directory", func(t *testing.T) {
		profile := t.TempDir()
		if err := os.Mkdir(filepath.Join(profile, ".profile.lock"), 0700); err != nil {
			t.Fatal(err)
		}
		if err := scanTemporaryNativeProfile(profile, []byte(fakeCode)); err == nil {
			t.Fatal("non-regular root profile lock was excluded")
		}
	})
}

type temporaryNativeClipboard struct {
	owner            uintptr
	formats          map[uintptr][]byte
	last             uint64
	user32, kernel32 *windows.LazyDLL
}

func newTemporaryNativeClipboard(t *testing.T) *temporaryNativeClipboard {
	t.Helper()
	b := &temporaryNativeClipboard{formats: make(map[uintptr][]byte), user32: windows.NewLazySystemDLL("user32.dll"), kernel32: windows.NewLazySystemDLL("kernel32.dll")}
	class, _ := windows.UTF16PtrFromString("STATIC")
	name, _ := windows.UTF16PtrFromString("LinkSend protected-copy fixture")
	b.owner, _, _ = b.user32.NewProc("CreateWindowExW").Call(0, uintptr(unsafe.Pointer(class)), uintptr(unsafe.Pointer(name)), 0, 0, 0, 0, 0, 0, 0, 0, 0)
	if b.owner == 0 {
		t.Skip("no isolated native clipboard owner window")
	}
	if !b.open() {
		b.user32.NewProc("DestroyWindow").Call(b.owner)
		t.Skip("native clipboard busy; no mutation")
	}
	initialGeneration := nativeclipboard.Generation()
	var failure bool
	for format := uintptr(0); ; {
		format, _, _ = b.user32.NewProc("EnumClipboardFormats").Call(format)
		if format == 0 {
			break
		}
		if len(b.formats) >= 32 {
			failure = true
			break
		}
		allowed := format == 1 || format == 7 || format == 8 || format == 13 || format == 15 || format == 16 || format == 17
		if format >= 0xc000 {
			var text [256]uint16
			count, _, _ := b.user32.NewProc("GetClipboardFormatNameW").Call(format, uintptr(unsafe.Pointer(&text[0])), uintptr(len(text)))
			if count != 0 {
				label := windows.UTF16ToString(text[:count])
				allowed = label == "HTML Format" || label == "Rich Text Format" || label == "PNG"
			}
		}
		if !allowed {
			failure = true
			break
		}
		handle, _, _ := b.user32.NewProc("GetClipboardData").Call(format)
		size, _, _ := b.kernel32.NewProc("GlobalSize").Call(handle)
		address, _, _ := b.kernel32.NewProc("GlobalLock").Call(handle)
		if handle == 0 || size == 0 || size > 32<<20 || address == 0 {
			if address != 0 {
				b.kernel32.NewProc("GlobalUnlock").Call(handle)
			}
			failure = true
			break
		}
		data := make([]byte, int(size))
		b.kernel32.NewProc("RtlMoveMemory").Call(uintptr(unsafe.Pointer(&data[0])), address, size)
		runtime.KeepAlive(data)
		b.kernel32.NewProc("GlobalUnlock").Call(handle)
		b.formats[format] = data
	}
	b.user32.NewProc("CloseClipboard").Call()
	if nativeclipboard.Generation() != initialGeneration {
		b.user32.NewProc("DestroyWindow").Call(b.owner)
		t.Skip("clipboard changed during backup; newer user data preserved")
	}
	if failure {
		b.user32.NewProc("DestroyWindow").Call(b.owner)
		t.Skip("original clipboard contains an unproven backup format; no mutation")
	}
	b.last = initialGeneration
	return b
}

func (b *temporaryNativeClipboard) open() bool {
	until := time.Now().Add(time.Second)
	for time.Now().Before(until) {
		if ok, _, _ := b.user32.NewProc("OpenClipboard").Call(b.owner); ok != 0 {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

func (b *temporaryNativeClipboard) put(format uintptr, data []byte) bool {
	handle, _, _ := b.kernel32.NewProc("GlobalAlloc").Call(2|64, uintptr(len(data)))
	if handle == 0 {
		return false
	}
	address, _, _ := b.kernel32.NewProc("GlobalLock").Call(handle)
	if address == 0 {
		b.kernel32.NewProc("GlobalFree").Call(handle)
		return false
	}
	b.kernel32.NewProc("RtlMoveMemory").Call(address, uintptr(unsafe.Pointer(&data[0])), uintptr(len(data)))
	runtime.KeepAlive(data)
	b.kernel32.NewProc("GlobalUnlock").Call(handle)
	result, _, _ := b.user32.NewProc("SetClipboardData").Call(format, handle)
	if result == 0 {
		b.kernel32.NewProc("GlobalFree").Call(handle)
		return false
	}
	return true
}

func (b *temporaryNativeClipboard) writeText(value string) bool {
	if nativeclipboard.Generation() != b.last || !b.open() {
		return false
	}
	defer func() { b.user32.NewProc("CloseClipboard").Call(); b.last = nativeclipboard.Generation() }()
	if nativeclipboard.Generation() != b.last {
		return false
	}
	if ok, _, _ := b.user32.NewProc("EmptyClipboard").Call(); ok == 0 {
		return false
	}
	units := append(utf16.Encode([]rune(value)), 0)
	data := make([]byte, len(units)*2)
	for i, unit := range units {
		data[2*i], data[2*i+1] = byte(unit), byte(unit>>8)
	}
	return b.put(13, data)
}

func (b *temporaryNativeClipboard) close(t *testing.T) {
	t.Helper()
	defer b.user32.NewProc("DestroyWindow").Call(b.owner)
	if nativeclipboard.Generation() != b.last {
		t.Log("clipboard changed externally; preserved the newer user clipboard instead of restoring an older snapshot")
		return
	}
	if !b.open() {
		t.Error("could not reopen clipboard for original-format restoration")
		return
	}
	defer b.user32.NewProc("CloseClipboard").Call()
	if ok, _, _ := b.user32.NewProc("EmptyClipboard").Call(); ok == 0 {
		t.Error("original clipboard restoration failed")
		return
	}
	for format, data := range b.formats {
		if !b.put(format, data) {
			t.Error("an original clipboard format could not be restored")
		}
	}
}
