//go:build windows

package main

import (
	"errors"
	"fmt"
	"runtime"
	"syscall"
	"unsafe"

	"github.com/go-ole/go-ole"
	"golang.org/x/sys/windows"
)

// Windows SDK 10.0.26100 ShObjIdl_core.h IShellLinkW and objidl.h
// IPersistFile: wide-character methods avoid WScript.Shell's locale-sensitive
// TargetPath automation boundary. No link resolution or target execution occurs.
type sendToLinkVTable struct {
	ole.IUnknownVtbl
	GetPath, GetIDList, SetIDList                                       uintptr
	GetDescription, SetDescription                                      uintptr
	GetWorkingDirectory, SetWorkingDirectory                            uintptr
	GetArguments, SetArguments                                          uintptr
	GetHotkey, SetHotkey, GetShowCmd, SetShowCmd                        uintptr
	GetIconLocation, SetIconLocation, SetRelativePath, Resolve, SetPath uintptr
}
type sendToPersistVTable struct {
	ole.IUnknownVtbl
	GetClassID, IsDirty, Load, Save, SaveCompleted, GetCurFile uintptr
}

func sendToCOMCall(object *ole.IUnknown, method uintptr, args ...uintptr) error {
	arguments := append([]uintptr{uintptr(unsafe.Pointer(object))}, args...)
	hr, _, _ := syscall.SyscallN(method, arguments...)
	runtime.KeepAlive(object)
	if int32(hr) < 0 {
		return fmt.Errorf("Shell Link HRESULT 0x%08x", uint32(hr))
	}
	return nil
}
func sendToWideCall(object *ole.IUnknown, method uintptr, value string, tail ...uintptr) error {
	encoded, err := windows.UTF16FromString(value)
	if err != nil {
		return err
	}
	args := append([]uintptr{uintptr(unsafe.Pointer(&encoded[0]))}, tail...)
	err = sendToCOMCall(object, method, args...)
	runtime.KeepAlive(encoded)
	return err
}
func withSendToShellLink(fn func(*ole.IUnknown, *sendToLinkVTable, *ole.IUnknown, *sendToPersistVTable) error) error {
	done := make(chan error, 1)
	go func() {
		result := func() error {
			runtime.LockOSThread()
			defer runtime.UnlockOSThread()
			if err := ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED); err != nil {
				var comErr *ole.OleError
				if !errors.As(err, &comErr) || comErr.Code() != 1 {
					return err
				}
			}
			defer ole.CoUninitialize()
			object, err := ole.CreateInstance(ole.NewGUID("{00021401-0000-0000-C000-000000000046}"), ole.NewGUID("{000214F9-0000-0000-C000-000000000046}"))
			if err != nil {
				return err
			}
			defer object.Release()
			var persist *ole.IUnknown
			if err = object.PutQueryInterface(ole.NewGUID("{0000010B-0000-0000-C000-000000000046}"), &persist); err != nil {
				return err
			}
			defer persist.Release()
			return fn(object, (*sendToLinkVTable)(unsafe.Pointer(object.RawVTable)), persist, (*sendToPersistVTable)(unsafe.Pointer(persist.RawVTable)))
		}()
		done <- result // all interfaces released before publication/readback
	}()
	return <-done
}

func writeSendToShortcut(path string, value sendToShortcut) error {
	return withSendToShellLink(func(link *ole.IUnknown, table *sendToLinkVTable, persist *ole.IUnknown, file *sendToPersistVTable) error {
		for _, property := range []struct {
			name   string
			method uintptr
			value  string
		}{
			{"target", table.SetPath, value.target}, {"arguments", table.SetArguments, value.arguments},
			{"description", table.SetDescription, value.description}, {"working directory", table.SetWorkingDirectory, value.workingDir},
		} {
			if err := sendToWideCall(link, property.method, property.value); err != nil {
				return fmt.Errorf("set %s: %w", property.name, err)
			}
		}
		return sendToWideCall(persist, file.Save, path, 1)
	})
}
func readSendToShortcut(path string) (sendToShortcut, error) {
	var result sendToShortcut
	err := withSendToShellLink(func(link *ole.IUnknown, table *sendToLinkVTable, persist *ole.IUnknown, file *sendToPersistVTable) error {
		if err := sendToWideCall(persist, file.Load, path, 0); err != nil {
			return err
		} // STGM_READ
		for _, property := range []struct {
			method uintptr
			to     *string
			path   bool
		}{
			{table.GetPath, &result.target, true}, {table.GetArguments, &result.arguments, false},
			{table.GetDescription, &result.description, false}, {table.GetWorkingDirectory, &result.workingDir, false},
		} {
			buffer := make([]uint16, 32768)
			args := []uintptr{uintptr(unsafe.Pointer(&buffer[0])), uintptr(len(buffer))}
			if property.path {
				args = append(args, 0, 4)
			} // no WIN32_FIND_DATA, SLGP_RAWPATH
			if err := sendToCOMCall(link, property.method, args...); err != nil {
				return err
			}
			runtime.KeepAlive(buffer)
			*property.to = windows.UTF16ToString(buffer)
		}
		return nil
	})
	return result, err
}
