package clipboardhelper

import (
	"path/filepath"

	"golang.org/x/sys/windows"
)

// Resolve accepts only the executable's actual path and the compiled anchor.
// The desktop's private wrapper supplies both, never user configuration.
func Resolve(executable string, a Anchor) (path, expectedHash string, err error) {
	if !filepath.IsAbs(executable) {
		return "", "", ErrImage
	}
	path = filepath.Join(filepath.Dir(executable), a.RelativePath())
	// The core launcher still pins actual image identity at process creation.
	for candidate := path; ; candidate = filepath.Dir(candidate) {
		wide, err := windows.UTF16PtrFromString(candidate)
		if err != nil {
			return "", "", ErrImage
		}
		attributes, err := windows.GetFileAttributes(wide)
		if err != nil || attributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
			return "", "", ErrImage
		}
		if filepath.Dir(candidate) == candidate {
			break
		}
	}
	if err = Verify(path, a); err != nil {
		return "", "", err
	}
	return path, a.HelperSHA256, nil
}
