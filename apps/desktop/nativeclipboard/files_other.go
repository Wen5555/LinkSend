//go:build (!windows && !darwin) || (darwin && !cgo)

package nativeclipboard

import (
	"context"
	"time"
)

func readClipboardFiles(context.Context, uint64) ([]string, uint64, error) {
	return nil, 0, ErrUnsupported
}
func writeClipboardFiles([]string, uint64, time.Time) (uint64, error) { return 0, ErrUnsupported }
