//go:build darwin && cgo

package nativeclipboard

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"testing"
	"time"

	"github.com/Wen5555/LinkSend/internal/clipboardsync"
)

func TestDarwinNamedPasteboardChangeCountWatcher(t *testing.T) {
	name := fmt.Sprintf("com.linksend.native-test.watch.%d", time.Now().UnixNano())
	changes := make(chan Change, 1)
	stop, err := watchPasteboardChanges(t.Context(), name, func(change Change) {
		select {
		case changes <- change:
		default:
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	if !setTestPasteboardString(name, "watch fixture") {
		t.Fatal("dedicated pasteboard mutation failed")
	}
	select {
	case change := <-changes:
		if change.Sequence == 0 || !change.Text || change.Image {
			t.Fatalf("unexpected change metadata: %+v", change)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("NSPasteboard changeCount watcher did not report dedicated pasteboard mutation")
	}
}

func TestDarwinNamedPasteboardTextURLAndProhibitedMarker(t *testing.T) {
	name := fmt.Sprintf("com.linksend.native-test.types.%d", time.Now().UnixNano())
	if !setTestPasteboardString(name, "ordinary words") {
		t.Fatal("dedicated pasteboard text mutation failed")
	}
	generation := namedPasteboardGeneration(name)
	if _, _, err := readNamedPasteboardString(name, clipboardsync.Link, generation); !errors.Is(err, ErrUnsupported) {
		t.Fatal("ordinary text was classified as a link", err)
	}
	if body, _, err := readNamedPasteboardString(name, clipboardsync.Text, generation); err != nil || string(body) != "ordinary words" {
		t.Fatalf("text fallback=%q err=%v", body, err)
	}
	if !setTestPasteboardString(name, "https://linksend.example/path") {
		t.Fatal("dedicated pasteboard URL mutation failed")
	}
	generation = namedPasteboardGeneration(name)
	if body, _, err := readNamedPasteboardString(name, clipboardsync.Link, generation); err != nil || string(body) != "https://linksend.example/path" {
		t.Fatalf("URL read=%q err=%v", body, err)
	}
	if !setTestPasteboardMarker(name, "org.nspasteboard.ConcealedType") || !namedPasteboardProhibited(name) {
		t.Fatal("concealed clipboard marker was not prohibited")
	}
	pixels := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	pixels.SetNRGBA(0, 0, color.NRGBA{R: 18, G: 52, B: 86, A: 255})
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, pixels); err != nil {
		t.Fatal(err)
	}
	generation = namedPasteboardGeneration(name)
	if _, err := writeNamedPasteboardPayload(name, clipboardsync.Image, encoded.Bytes(), generation); err != nil {
		t.Fatal(err)
	}
	imageRead, err := capturePasteboard(t.Context(), name)
	if err != nil {
		t.Fatal(err)
	}
	r, g, b, a := imageRead.At(0, 0).RGBA()
	if r>>8 != 18 || g>>8 != 52 || b>>8 != 86 || a>>8 != 255 {
		t.Fatalf("PNG pixel mismatch: %d %d %d %d", r>>8, g>>8, b>>8, a>>8)
	}
}

func TestDarwinNamedPasteboardOwnedWriteReadback(t *testing.T) {
	name := fmt.Sprintf("com.linksend.native-test.write.%d", time.Now().UnixNano())
	expected := namedPasteboardGeneration(name)
	next, err := writeNamedPasteboardText(name, []byte("LinkSend named write"), expected)
	if err != nil {
		t.Fatal(err)
	}
	got, current, err := readNamedPasteboardText(name, next)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "LinkSend named write" || current != next {
		t.Fatalf("readback=%q generation=%d/%d", got, current, next)
	}
}
