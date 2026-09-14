package nativeclipboard

import (
	"bytes"
	"context"
	"image/png"
	"net/url"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/Wen5555/LinkSend/internal/clipboardsync"
	"github.com/Wen5555/LinkSend/internal/content"
)

var clipboardOwner atomic.Uintptr

func SetOwnerWindow(window uintptr) { clipboardOwner.Store(window) }

type cappedBuffer struct {
	bytes.Buffer
	limit int
}

func (b *cappedBuffer) Write(payload []byte) (int, error) {
	if len(payload) > b.limit-b.Len() {
		return 0, clipboardsync.ErrLimit
	}
	return b.Buffer.Write(payload)
}

func Generation() uint64 { return clipboardGeneration() }

func ReadPayload(ctx context.Context, kind clipboardsync.Kind, expected uint64) ([]byte, uint64, error) {
	if err := ctx.Err(); err != nil {
		return nil, 0, err
	}
	if clipboardReadProhibited() {
		return nil, Generation(), ErrUnsupported
	}
	if kind != clipboardsync.Image {
		return readClipboardText(ctx, kind, expected)
	}
	if clipboardGeneration() != expected {
		return nil, clipboardGeneration(), ErrChanged
	}
	img, err := captureImage(ctx)
	if err != nil {
		return nil, clipboardGeneration(), err
	}
	data := cappedBuffer{limit: clipboardsync.MaxImageBytes}
	if err = png.Encode(&data, img); err != nil {
		return nil, clipboardGeneration(), err
	}
	current := clipboardGeneration()
	if current != expected {
		return nil, current, ErrChanged
	}
	return data.Bytes(), current, nil
}

func WritePayload(kind clipboardsync.Kind, payload []byte, expected uint64) (uint64, error) {
	return WritePayloadBefore(kind, payload, expected, time.Time{})
}

func WritePayloadBefore(kind clipboardsync.Kind, payload []byte, expected uint64, deadline time.Time) (uint64, error) {
	if err := ValidatePayload(kind, payload); err != nil {
		return Generation(), err
	}
	return WritePreparedPayloadBefore(kind, payload, expected, deadline)
}

func ValidatePayload(kind clipboardsync.Kind, payload []byte) error {
	if kind != clipboardsync.Text && kind != clipboardsync.Link && kind != clipboardsync.Image {
		return ErrUnsupported
	}
	if len(payload) == 0 || len(payload) > clipboardsync.MaxImageBytes {
		return clipboardsync.ErrLimit
	}
	if kind == clipboardsync.Image {
		if _, err := content.DecodeImage(context.Background(), bytes.NewReader(payload)); err != nil {
			return err
		}
	} else {
		if len(payload) > clipboardsync.MaxTextBytes || !utf8.Valid(payload) {
			return clipboardsync.ErrLimit
		}
		if kind == clipboardsync.Link {
			parsed, err := url.Parse(string(payload))
			if err != nil || parsed.Scheme == "" {
				return ErrUnsupported
			}
		}
	}
	return nil
}

func WritePreparedPayloadBefore(kind clipboardsync.Kind, payload []byte, expected uint64, deadline time.Time) (uint64, error) {
	if !deadline.IsZero() && !time.Now().Before(deadline) {
		return Generation(), clipboardsync.ErrExpired
	}
	return writeClipboardPayload(kind, payload, expected, deadline)
}
