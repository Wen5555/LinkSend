//go:build darwin && cgo

package nativeclipboard

/*
#cgo CFLAGS: -mmacosx-version-min=13.0 -x objective-c
#cgo LDFLAGS: -mmacosx-version-min=13.0 -framework Cocoa -framework ImageIO -framework CoreGraphics
#include <stdlib.h>
int linksendClipboardPNG(const char *name, void **output, size_t *size);
*/
import "C"

import (
	"bytes"
	"context"
	"image"
	"unsafe"

	"github.com/Wen5555/LinkSend/internal/content"
)

func captureImage(ctx context.Context) (image.Image, error) { return capturePasteboard(ctx, "") }

// Named pasteboards let native tests exercise the real platform decoder without
// modifying any representation on the user's general clipboard.
func capturePasteboard(ctx context.Context, name string) (image.Image, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var nativeName *C.char
	if name != "" {
		nativeName = C.CString(name)
		defer C.free(unsafe.Pointer(nativeName))
	}
	var output unsafe.Pointer
	var size C.size_t
	code := int(C.linksendClipboardPNG(nativeName, &output, &size))
	if output != nil {
		defer C.free(output)
	}
	switch code {
	case 0:
	case 1:
		return nil, ErrNoImage
	case 2:
		return nil, content.ErrLimit
	case 3:
		return nil, ErrChanged
	default:
		return nil, ErrUnavailable
	}
	if size == 0 || uint64(size) > content.MaxImageBytes || output == nil {
		return nil, content.ErrLimit
	}
	data := C.GoBytes(output, C.int(size))
	return content.DecodeImage(ctx, bytes.NewReader(data))
}
