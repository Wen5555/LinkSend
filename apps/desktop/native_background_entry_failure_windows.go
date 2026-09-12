//go:build windows

package main

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// Call on a failed second-instance intake before exiting. The caller supplies
// fixed, public Chinese UI text, never err.Error() or a selected private path.
func showNativeEntryFailure(message string) {
	text, err := windows.UTF16PtrFromString(message)
	if err != nil || message == "" {
		text, _ = windows.UTF16PtrFromString("无法添加所选内容，请打开 LinkSend 后重试。")
	}
	title, _ := windows.UTF16PtrFromString("LinkSend")
	messageBox := windows.NewLazySystemDLL("user32.dll").NewProc("MessageBoxW")
	_, _, _ = messageBox.Call(0, uintptr(unsafe.Pointer(text)), uintptr(unsafe.Pointer(title)), 0x10) // MB_OK | MB_ICONERROR
}
