//go:build windows

package app

import "github.com/Wen5555/LinkSend/internal/clipboardnativecontract"

type cacheNativeWindowsBoot struct{}

func (cacheNativeWindowsBoot) cacheNativeBootID() (string, error) {
	return clipboardnativecontract.WindowsBootID()
}
