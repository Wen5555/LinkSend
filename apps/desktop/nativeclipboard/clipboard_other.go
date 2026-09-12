//go:build !windows && !darwin

package nativeclipboard

import (
	"context"
	"image"
)

func captureImage(ctx context.Context) (image.Image, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return nil, ErrUnsupported
}
