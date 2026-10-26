//go:build windows

package clipboardnativecontract

import (
	"bytes"
	"encoding/hex"
	"unsafe"

	"golang.org/x/sys/windows"
)

// WindowsBootID reads the kernel boot environment's BootIdentifier GUID.
// SystemBootEnvironmentInformation is the pinned x/sys SystemInformationClass;
// its native result begins with GUID BootIdentifier, followed by firmware/flags.
// An unsupported class/layout/error fails closed. This is not a wall-clock,
// process epoch, uptime subtraction, or evidence permitting deletion.
func WindowsBootID() (string, error) {
	var data [64]byte
	var used uint32
	if err := windows.NtQuerySystemInformation(windows.SystemBootEnvironmentInformation, unsafe.Pointer(&data[0]), uint32(len(data)), &used); err != nil {
		return "", err
	}
	if used < 24 || used > uint32(len(data)) || bytes.Equal(data[:16], make([]byte, 16)) {
		return "", ErrContract
	}
	return "windows-boot-v1-" + hex.EncodeToString(data[:16]), nil
}
