//go:build darwin && cgo

package nativeclipboard

import (
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

func TestDarwinNamedPasteboardFilesReadWriteAndCAS(t *testing.T) {
	name := fmt.Sprintf("com.linksend.native-test.files.%d", time.Now().UnixNano())
	paths := []string{filepath.Join(t.TempDir(), "复制 文件.txt"), filepath.Join(t.TempDir(), "folder")}
	generation := namedPasteboardGeneration(name)
	next, err := writeNamedPasteboardFiles(name, paths, generation)
	if err != nil {
		t.Fatal(err)
	}
	got, current, err := readNamedPasteboardFiles(name, next)
	if err != nil || current != next || len(got) != 2 || got[0] != paths[0] || got[1] != paths[1] {
		t.Fatal(got, current, err)
	}
	if _, err = writeNamedPasteboardFiles(name, paths, generation); !errors.Is(err, ErrChanged) {
		t.Fatal("stale write accepted", err)
	}
	if _, _, err = readNamedPasteboardFiles(name, generation); !errors.Is(err, ErrChanged) {
		t.Fatal("stale read accepted", err)
	}
	if _, err = writeNamedPasteboardFiles(name, []string{"../relative"}, next); err == nil {
		t.Fatal("relative path accepted")
	}
}
