//go:build windows

package nativeclipboard

import (
	"encoding/binary"
	"testing"
	"unicode/utf16"
)

func TestFileDropMetadataIsUnicodeDoubleNullTerminated(t *testing.T) {
	paths := []string{`C:\复制\第一份.txt`, `D:\folder with space\second.bin`}
	raw := clipboardDropData(paths)
	if binary.LittleEndian.Uint32(raw) != 20 || binary.LittleEndian.Uint32(raw[16:]) != 1 || len(raw)%2 != 0 {
		t.Fatal("invalid DROPFILES header")
	}
	units := make([]uint16, (len(raw)-20)/2)
	for i := range units {
		units[i] = binary.LittleEndian.Uint16(raw[20+i*2:])
	}
	if units[len(units)-1] != 0 || units[len(units)-2] != 0 {
		t.Fatal("missing double NUL")
	}
	var got []string
	start := 0
	for i, u := range units {
		if u == 0 && i > start {
			got = append(got, string(utf16.Decode(units[start:i])))
			start = i + 1
		}
	}
	if len(got) != 2 || got[0] != paths[0] || got[1] != paths[1] {
		t.Fatal("file path metadata changed")
	}
}
