// Package nativeclipboard reads images only after an explicit user action.
// Encoded bytes and pixels never cross the Wails JavaScript bridge.
package nativeclipboard

import (
	"context"
	"errors"

	"github.com/Wen5555/LinkSend/internal/content"
)

var (
	ErrUnavailable = errors.New("CLIPBOARD_UNAVAILABLE")
	ErrNoImage     = errors.New("CLIPBOARD_IMAGE_NOT_AVAILABLE")
	ErrUnsupported = errors.New("CLIPBOARD_IMAGE_FORMAT_UNSUPPORTED")
	ErrChanged     = errors.New("CLIPBOARD_CHANGED_DURING_CAPTURE")
)

// CaptureImageSnapshot reads the current clipboard once and persists the image
// before returning metadata. Run on a Go worker, never in a UI callback. macOS
// uses NSPasteboard/ImageIO and Windows uses a locked native clipboard read.
func CaptureImageSnapshot(ctx context.Context, store *content.Store, ownerRef string) (content.Snapshot, error) {
	if err := ctx.Err(); err != nil {
		return content.Snapshot{}, err
	}
	if store == nil {
		return content.Snapshot{}, errors.New("CONTENT_STORE_REQUIRED")
	}
	img, err := captureImage(ctx)
	if err != nil {
		return content.Snapshot{}, err
	}
	return store.CreateImageFromImage(ctx, img, ownerRef)
}
