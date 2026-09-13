//go:build darwin && !cgo

package nativeclipboard

import "context"

func watchChanges(context.Context, uintptr, func(Change)) (func(), error) {
	return nil, ErrUnsupported
}
