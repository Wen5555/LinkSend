//go:build windows

package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"runtime"
	"unsafe"

	"github.com/wailsapp/wails/v3/pkg/application"
	"golang.org/x/sys/windows"
)

// Windows SDK CommCtrl.h wraps TASKDIALOGCONFIG/BUTTON in pshpack1.h. A normal
// Go struct inserts padding after cbSize; construct the documented packed ABI.
type taskDialogBuffer struct{ data []byte }

func (b *taskDialogBuffer) number(value uint32) {
	b.data = binary.LittleEndian.AppendUint32(b.data, value)
}
func (b *taskDialogBuffer) pointer(value uintptr) {
	if unsafe.Sizeof(uintptr(0)) == 8 {
		b.data = binary.LittleEndian.AppendUint64(b.data, uint64(value))
	} else {
		b.number(uint32(value))
	}
}

func showNativeChoice(window application.Window, title, message string, labels []string, defaultIndex, cancelIndex int, done func(int, error)) {
	go func() {
		var callErr error
		choice := application.InvokeSyncWithResult(func() int {
			if len(labels) < 1 || len(labels) > 8 || defaultIndex < 0 || defaultIndex >= len(labels) || cancelIndex < 0 || cancelIndex >= len(labels) {
				callErr = errors.New("NATIVE_CHOICE_INVALID")
				return -1
			}
			proc := windows.NewLazySystemDLL("comctl32.dll").NewProc("TaskDialogIndirect")
			if err := proc.Find(); err != nil {
				callErr = err
				return -1
			}
			strings := make([][]uint16, 0, len(labels)+2)
			text := func(value string) uintptr {
				encoded, err := windows.UTF16FromString(value)
				if err != nil {
					callErr = err
					return 0
				}
				strings = append(strings, encoded)
				return uintptr(unsafe.Pointer(&encoded[0]))
			}
			titlePointer, messagePointer := text(title), text(message)
			buttons := taskDialogBuffer{}
			for i, label := range labels {
				buttons.number(uint32(1000 + i))
				buttons.pointer(text(label))
			}
			if callErr != nil {
				return -1
			}
			owner := uintptr(0)
			if window != nil {
				owner = uintptr(window.NativeWindow())
			}
			config := taskDialogBuffer{}
			config.number(0)
			config.pointer(owner)
			config.pointer(0)
			config.number(0x0008 | 0x1000)
			config.number(0) // cancellable, relative to owner
			config.pointer(titlePointer)
			config.pointer(0)
			config.pointer(0)
			config.pointer(messagePointer)
			config.number(uint32(len(labels)))
			config.pointer(uintptr(unsafe.Pointer(&buttons.data[0])))
			config.number(uint32(1000 + defaultIndex))
			config.number(0)
			config.pointer(0)
			config.number(0)
			for i := 0; i < 8; i++ {
				config.pointer(0)
			} // verification/expanded/footer/callback/data
			config.number(0)
			binary.LittleEndian.PutUint32(config.data, uint32(len(config.data)))
			var selected int32
			hr, _, _ := proc.Call(uintptr(unsafe.Pointer(&config.data[0])), uintptr(unsafe.Pointer(&selected)), 0, 0)
			runtime.KeepAlive(strings)
			runtime.KeepAlive(buttons)
			runtime.KeepAlive(config)
			if int32(hr) < 0 {
				callErr = fmt.Errorf("NATIVE_CHOICE_FAILED: HRESULT 0x%08x", uint32(hr))
				return -1
			}
			if selected == 2 {
				return cancelIndex
			}
			if selected < 1000 || int(selected) >= 1000+len(labels) {
				callErr = errors.New("NATIVE_CHOICE_INVALID_RESULT")
				return -1
			}
			return int(selected) - 1000
		})
		done(choice, callErr)
	}()
}
