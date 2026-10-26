//go:build windows

package main

import (
	"os"
	"runtime"

	"github.com/Wen5555/LinkSend/apps/desktop/internal/clipboardhelper"
)

// Set only by the same-candidate Windows builder. Plain go builds fail closed.
var clipboardHelperBuildAnchor string

// Initial parsing retains the linker-supplied anchor even before a publisher
// is wired. It performs no I/O and grants no native publication authority.
var clipboardHelperAnchor, clipboardHelperAnchorErr = clipboardhelper.Parse(clipboardHelperBuildAnchor, runtime.GOARCH)

// This private resolver has no Wails binding and does not activate a publisher.
// Its caller must pass the expected digest into the core's pinned-image launch.
func resolveClipboardHelperImage() (path, expectedHash string, err error) {
	a := clipboardHelperAnchor
	if clipboardHelperAnchorErr != nil {
		return "", "", clipboardHelperAnchorErr
	}
	executable, err := os.Executable()
	if err != nil {
		return "", "", clipboardhelper.ErrImage
	}
	return clipboardhelper.Resolve(executable, a)
}
