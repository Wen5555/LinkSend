package nativeclipboard

import (
	"encoding/binary"
	"image"
	"image/color"
	"math/bits"

	"github.com/Wen5555/LinkSend/internal/content"
)

// dibImage retains a bounded private copy of native DIB data. At converts BGR
// on demand, avoiding an extra 160 MB RGBA allocation at the 40 MP limit.
type dibImage struct {
	data                                 []byte
	width, height, stride, offset, depth int
	topDown                              bool
	masks                                [4]uint32
}

func decodeDIB(data []byte) (image.Image, error) {
	if len(data) < 40 {
		return nil, ErrUnsupported
	}
	header := uint64(binary.LittleEndian.Uint32(data[:4]))
	if header != 40 && header != 52 && header != 56 && header != 108 && header != 124 {
		return nil, ErrUnsupported
	}
	if header > uint64(len(data)) {
		return nil, ErrUnsupported
	}
	width, signedHeight := int64(int32(binary.LittleEndian.Uint32(data[4:8]))), int64(int32(binary.LittleEndian.Uint32(data[8:12])))
	height := signedHeight
	if height < 0 {
		height = -height
	}
	if width <= 0 || height <= 0 || width > content.MaxImageDimension || height > content.MaxImageDimension || width > content.MaxImagePixels/height {
		return nil, content.ErrLimit
	}
	depth := int(binary.LittleEndian.Uint16(data[14:16]))
	compression := binary.LittleEndian.Uint32(data[16:20])
	if binary.LittleEndian.Uint16(data[12:14]) != 1 || (depth != 24 && depth != 32) || (compression != 0 && compression != 3 && compression != 6) {
		return nil, ErrUnsupported
	}
	if depth == 24 && compression != 0 {
		return nil, ErrUnsupported
	}
	if compression == 6 && header == 52 {
		return nil, ErrUnsupported
	}
	if binary.LittleEndian.Uint32(data[32:36]) != 0 {
		return nil, ErrUnsupported
	}
	offset := header
	masks := [4]uint32{0x00ff0000, 0x0000ff00, 0x000000ff, 0}
	if compression == 3 || compression == 6 {
		maskOffset, maskCount := uint64(40), 3
		if header == 40 {
			offset += 12
			if compression == 6 {
				offset += 4
				maskCount = 4
			}
		} else if header >= 56 {
			maskCount = 4
		}
		if maskOffset+uint64(maskCount*4) > uint64(len(data)) {
			return nil, ErrUnsupported
		}
		for i := 0; i < maskCount; i++ {
			masks[i] = binary.LittleEndian.Uint32(data[maskOffset+uint64(i*4):])
		}
		var occupied uint32
		for i, mask := range masks {
			if i < 3 && mask == 0 {
				return nil, ErrUnsupported
			}
			if mask == 0 {
				continue
			}
			shifted := mask >> bits.TrailingZeros32(mask)
			if shifted&(shifted+1) != 0 || occupied&mask != 0 {
				return nil, ErrUnsupported
			}
			occupied |= mask
		}
	}
	// Embedded V5 colour profiles are deliberately unsupported. Ignoring their
	// offset/size could reinterpret profile bytes as pixels.
	if header == 124 && (binary.LittleEndian.Uint32(data[112:116]) != 0 || binary.LittleEndian.Uint32(data[116:120]) != 0) {
		return nil, ErrUnsupported
	}
	stride := ((uint64(width)*uint64(depth) + 31) / 32) * 4
	if offset+stride*uint64(height) > uint64(len(data)) {
		return nil, ErrUnsupported
	}
	return &dibImage{data: data, width: int(width), height: int(height), stride: int(stride), offset: int(offset), depth: depth, topDown: signedHeight < 0, masks: masks}, nil
}

func (d *dibImage) ColorModel() color.Model { return color.NRGBAModel }
func (d *dibImage) Bounds() image.Rectangle { return image.Rect(0, 0, d.width, d.height) }
func (d *dibImage) At(x, y int) color.Color {
	if x < 0 || y < 0 || x >= d.width || y >= d.height {
		return color.NRGBA{}
	}
	if !d.topDown {
		y = d.height - 1 - y
	}
	offset := d.offset + y*d.stride + x*(d.depth/8)
	if d.depth == 24 {
		return color.NRGBA{R: d.data[offset+2], G: d.data[offset+1], B: d.data[offset], A: 255}
	}
	value := binary.LittleEndian.Uint32(d.data[offset:])
	component := func(mask uint32) uint8 {
		if mask == 0 {
			return 255
		}
		shift := bits.TrailingZeros32(mask)
		return uint8(uint64((value&mask)>>shift) * 255 / uint64(mask>>shift))
	}
	return color.NRGBA{R: component(d.masks[0]), G: component(d.masks[1]), B: component(d.masks[2]), A: component(d.masks[3])}
}
