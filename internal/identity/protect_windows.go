//go:build windows

package identity

import (
	"bytes"
	"errors"
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

func protect(seed []byte) ([]byte, error) {
	in := windows.DataBlob{Size: uint32(len(seed)), Data: &seed[0]}
	var out windows.DataBlob
	if err := windows.CryptProtectData(&in, nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out); err != nil {
		return nil, err
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	return append([]byte("LINKSEND-DPAPI-1\n"), unsafe.Slice(out.Data, int(out.Size))...), nil
}
func unprotect(data []byte) ([]byte, error) {
	prefix := []byte("LINKSEND-DPAPI-1\n")
	if !bytes.HasPrefix(data, prefix) || len(data) <= len(prefix) {
		return nil, errors.New("unknown key storage format")
	}
	raw := data[len(prefix):]
	in := windows.DataBlob{Size: uint32(len(raw)), Data: &raw[0]}
	var out windows.DataBlob
	if err := windows.CryptUnprotectData(&in, nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out); err != nil {
		return nil, err
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	return bytes.Clone(unsafe.Slice(out.Data, int(out.Size))), nil
}

// Seed confidentiality uses user-scoped DPAPI, not Unix permission emulation.
func checkPrivatePermissions(os.FileInfo) error { return nil }
