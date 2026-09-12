//go:build windows

package identity

import (
	"errors"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

const fileIsEncrypted = 1

var (
	advapi32                 = windows.NewLazySystemDLL("advapi32.dll")
	kernel32                 = windows.NewLazySystemDLL("kernel32.dll")
	fileEncryptionStatusProc = advapi32.NewProc("FileEncryptionStatusW")
	encryptFileProc          = advapi32.NewProc("EncryptFileW")
	decryptFileProc          = advapi32.NewProc("DecryptFileW")
	replaceFileProc          = kernel32.NewProc("ReplaceFileW")
)

func replaceAtomicFile(source, target string) (bool, error) {
	replacement, err := windows.UTF16PtrFromString(source)
	if err != nil {
		return true, err
	}
	replaced, err := windows.UTF16PtrFromString(target)
	if err != nil {
		return true, err
	}
	result, _, callErr := replaceFileProc.Call(
		uintptr(unsafe.Pointer(replaced)),
		uintptr(unsafe.Pointer(replacement)),
		0,
		1, // REPLACEFILE_WRITE_THROUGH
		0,
		0,
	)
	if result == 0 {
		return true, fmt.Errorf("atomic ReplaceFileW: %w", callErr)
	}
	return true, nil
}

func alignAtomicReplaceSource(source, target string) (bool, error) {
	sourceStatus, err := fileEncryptionStatus(source)
	if err != nil {
		return false, err
	}
	targetStatus, err := fileEncryptionStatus(target)
	if err != nil {
		return false, err
	}
	sourceEncrypted := sourceStatus == fileIsEncrypted
	targetEncrypted := targetStatus == fileIsEncrypted
	if sourceEncrypted == targetEncrypted {
		return false, nil
	}
	if err = setFileEncryption(source, targetEncrypted); err == nil {
		return true, nil
	}
	// Some EFS directory policies refuse decrypting a newly inherited file.
	// Aligning the existing target in the safer encrypted direction preserves
	// its bytes and still permits an atomic replacement retry.
	targetErr := setFileEncryption(target, sourceEncrypted)
	if targetErr == nil {
		return true, nil
	}
	return true, errors.Join(err, targetErr)
}

func setFileEncryption(path string, encrypted bool) error {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	var result uintptr
	var callErr error
	if encrypted {
		result, _, callErr = encryptFileProc.Call(uintptr(unsafe.Pointer(name)))
	} else {
		result, _, callErr = decryptFileProc.Call(uintptr(unsafe.Pointer(name)), 0)
	}
	if result == 0 {
		return fmt.Errorf("align replacement file encryption: %w", callErr)
	}
	return nil
}

func fileEncryptionStatus(path string) (uint32, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	var status uint32
	result, _, callErr := fileEncryptionStatusProc.Call(uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(&status)))
	if result == 0 {
		return 0, fmt.Errorf("read file encryption status: %w", callErr)
	}
	return status, nil
}
