//go:build !windows

package identity

import (
	"bytes"
	"errors"
	"os"
)

func protect(seed []byte) ([]byte, error) { return append([]byte("LINKSEND-SEED-1\n"), seed...), nil }
func unprotect(data []byte) ([]byte, error) {
	prefix := []byte("LINKSEND-SEED-1\n")
	if !bytes.HasPrefix(data, prefix) {
		return nil, errors.New("unknown key storage format")
	}
	return bytes.Clone(data[len(prefix):]), nil
}
func checkPrivatePermissions(info os.FileInfo) error {
	if info.Mode().Perm()&0077 != 0 {
		return errors.New("identity key permissions must be 0600")
	}
	return nil
}
