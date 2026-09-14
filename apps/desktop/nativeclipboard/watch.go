package nativeclipboard

import (
	"context"
)

type Change struct {
	Sequence uint64 `json:"sequence"`
	Text     bool   `json:"text"`
	Link     bool   `json:"link"`
	Image    bool   `json:"image"`
}

func Watch(ctx context.Context, nativeWindow uintptr, notify func(Change)) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if notify == nil {
		return nil, ErrUnavailable
	}
	return watchChanges(ctx, nativeWindow, notify)
}
