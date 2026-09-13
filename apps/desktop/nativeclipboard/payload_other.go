//go:build !windows && !darwin

package nativeclipboard

import (
	"context"
	"github.com/Wen5555/LinkSend/internal/clipboardsync"
	"time"
)

func clipboardGeneration() uint64 { return 0 }
func readClipboardText(context.Context, clipboardsync.Kind, uint64) ([]byte, uint64, error) {
	return nil, 0, ErrUnsupported
}
func writeClipboardPayload(clipboardsync.Kind, []byte, uint64, time.Time) (uint64, error) {
	return 0, ErrUnsupported
}
