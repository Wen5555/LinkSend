//go:build darwin && cgo

package nativeclipboard

import (
	"fmt"
	"testing"
	"time"
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
