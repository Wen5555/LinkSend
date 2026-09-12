//go:build darwin && cgo

package nativeclipboard

import (
	"os"
	"strings"
	"testing"

	"github.com/Wen5555/LinkSend/internal/content"
)

func TestDarwinNamedPasteboardSnapshot(t *testing.T) {
	name := os.Getenv("LINKSEND_TEST_PASTEBOARD")
	if !strings.HasPrefix(name, "com.linksend.native-test.") {
		t.Skip("requires a dedicated named pasteboard fixture; never mutates the general clipboard")
	}
	img, err := capturePasteboard(t.Context(), name)
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds().Dx() != 2 || img.Bounds().Dy() != 2 {
		t.Fatal(img.Bounds())
	}
	r, g, b, a := img.At(0, 0).RGBA()
	if r>>8 != 180 || g>>8 != 60 || b>>8 != 30 || a>>8 != 255 {
		t.Fatalf("pixel mismatch: %d %d %d %d", r>>8, g>>8, b>>8, a>>8)
	}
	store, err := content.OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	snapshot, err := store.CreateImageFromImage(t.Context(), img, "native-test:image")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.OwnedPath(t.Context(), snapshot.ID); err != nil {
		t.Fatal(err)
	}
	if err = store.Release(snapshot.ID, "native-test:image"); err != nil {
		t.Fatal(err)
	}
	if removed, err := store.Cleanup(t.Context()); err != nil || len(removed) != 1 {
		t.Fatal(removed, err)
	}
}
