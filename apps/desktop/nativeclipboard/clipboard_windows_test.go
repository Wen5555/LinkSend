//go:build windows

package nativeclipboard

import (
	"bytes"
	"errors"
	"os"
	"runtime"
	"testing"
	"unsafe"

	"github.com/Wen5555/LinkSend/internal/content"
)

func TestWindowsClipboardReadOnly(t *testing.T) {
	if os.Getenv("LINKSEND_NATIVE_CLIPBOARD_READONLY") != "1" {
		t.Skip("opt-in native read-only clipboard check")
	}
	sequence := user32.NewProc("GetClipboardSequenceNumber")
	before, _, _ := sequence.Call()
	img, err := captureImage(t.Context())
	if errors.Is(err, ErrNoImage) {
		t.Skip("real Windows clipboard contains no supported image; no clipboard mutation performed")
	}
	if err != nil {
		t.Fatal(err)
	}
	after, _, _ := sequence.Call()
	if before != after {
		t.Skip("clipboard changed externally while checking; retry with a stable image")
	}
	store, err := content.OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	snapshot, err := store.CreateImageFromImage(t.Context(), img, "native-readonly:image")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.OwnedPath(t.Context(), snapshot.ID); err != nil {
		t.Fatal(err)
	}
	if err = store.Release(snapshot.ID, "native-readonly:image"); err != nil {
		t.Fatal(err)
	}
	if removed, err := store.Cleanup(t.Context()); err != nil || len(removed) != 1 {
		t.Fatal(removed, err)
	}
	t.Logf("native clipboard read + PNG snapshot PASS: %dx%d, %d encoded bytes; clipboard sequence unchanged", snapshot.Width, snapshot.Height, snapshot.Size)
}

func TestWindowsLockedNativeMemoryCopy(t *testing.T) {
	if err := moveMemory.Find(); err != nil {
		t.Fatal(err)
	}
	alloc, free := kernel32.NewProc("GlobalAlloc"), kernel32.NewProc("GlobalFree")
	handle, _, err := alloc.Call(2|64, 4)
	if handle == 0 {
		t.Fatal(err)
	}
	defer free.Call(handle)
	address, _, err := globalLock.Call(handle)
	if address == 0 {
		t.Fatal(err)
	}
	defer globalUnlock.Call(handle)
	expected := []byte{180, 60, 30, 255}
	moveMemory.Call(address, uintptr(unsafe.Pointer(&expected[0])), uintptr(len(expected)))
	actual := copyNativeMemory(address, len(expected))
	runtime.KeepAlive(expected)
	if !bytes.Equal(actual, expected) {
		t.Fatal(actual)
	}
}
