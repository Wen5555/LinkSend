//go:build !windows && !darwin

package nativeclipboard

import "context"

func watchChanges(context.Context, uintptr, func(Change)) (func(), error) {
	return nil, ErrUnsupported
}
