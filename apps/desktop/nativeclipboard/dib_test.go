package nativeclipboard

import (
	"encoding/binary"
	"errors"
	"image/color"
	"testing"

	"github.com/Wen5555/LinkSend/internal/content"
)

func dibFixture(width, height int, depth uint16) []byte {
	stride := (width*int(depth) + 31) / 32 * 4
	data := make([]byte, 40+stride*height)
	binary.LittleEndian.PutUint32(data[0:], 40)
	binary.LittleEndian.PutUint32(data[4:], uint32(width))
	binary.LittleEndian.PutUint32(data[8:], uint32(height))
	binary.LittleEndian.PutUint16(data[12:], 1)
	binary.LittleEndian.PutUint16(data[14:], depth)
	return data
}

func TestDIBBottomUpTopDownAndOpaqueRGB(t *testing.T) {
	for _, depth := range []uint16{24, 32} {
		data := dibFixture(1, 2, depth)
		copy(data[40:], []byte{9, 20, 201, 0, 70, 80, 90, 0})
		img, err := decodeDIB(data)
		if err != nil {
			t.Fatal(err)
		}
		if got := color.NRGBAModel.Convert(img.At(0, 1)).(color.NRGBA); got != (color.NRGBA{R: 201, G: 20, B: 9, A: 255}) {
			t.Fatal(got)
		}
		binary.LittleEndian.PutUint32(data[8:], 0xfffffffe)
		img, err = decodeDIB(data)
		if err != nil {
			t.Fatal(err)
		}
		if got := color.NRGBAModel.Convert(img.At(0, 0)).(color.NRGBA); got.R != 201 || got.A != 255 {
			t.Fatal(got)
		}
	}
}

func TestDIBRejectsMalformedOrHuge(t *testing.T) {
	data := dibFixture(1, 1, 32)
	binary.LittleEndian.PutUint32(data[4:], 40000001)
	if _, err := decodeDIB(data); !errors.Is(err, content.ErrLimit) {
		t.Fatal(err)
	}
	data = dibFixture(2, 2, 32)
	if _, err := decodeDIB(data[:len(data)-1]); !errors.Is(err, ErrUnsupported) {
		t.Fatal(err)
	}
	binary.LittleEndian.PutUint32(data[16:], 1)
	if _, err := decodeDIB(data); !errors.Is(err, ErrUnsupported) {
		t.Fatal(err)
	}
	data = dibFixture(1, 1, 32)
	binary.LittleEndian.PutUint32(data[8:], 0x80000000)
	if _, err := decodeDIB(data); !errors.Is(err, content.ErrLimit) {
		t.Fatal(err)
	}
}

func TestDIBBitfieldsAndAlpha(t *testing.T) {
	data := make([]byte, 56+4)
	copy(data, dibFixture(1, 1, 32)[:40])
	binary.LittleEndian.PutUint32(data[0:], 56)
	binary.LittleEndian.PutUint32(data[16:], 3)
	for i, mask := range []uint32{0xff0000, 0xff00, 0xff, 0xff000000} {
		binary.LittleEndian.PutUint32(data[40+i*4:], mask)
	}
	copy(data[56:], []byte{11, 22, 33, 128})
	img, err := decodeDIB(data)
	if err != nil {
		t.Fatal(err)
	}
	if got := color.NRGBAModel.Convert(img.At(0, 0)).(color.NRGBA); got != (color.NRGBA{R: 33, G: 22, B: 11, A: 128}) {
		t.Fatal(got)
	}
	binary.LittleEndian.PutUint32(data[44:], 0xff0000)
	if _, err := decodeDIB(data); !errors.Is(err, ErrUnsupported) {
		t.Fatal("overlapping masks accepted", err)
	}
}
