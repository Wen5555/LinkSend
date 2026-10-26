// Package clipboardhelper validates the build-time private helper anchor. It
// does not launch a process, read configuration, or publish clipboard data.
package clipboardhelper

import (
	"bytes"
	"crypto/sha256"
	"debug/pe"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
)

const FileName = "LinkSendClipboardFileHelper.exe"

var ErrImage = errors.New("private clipboard helper unavailable")

// Anchor is supplied by the same build transaction as the main executable.
// A writable sidecar, helper response, environment or preference is not a root
// of trust. SourceDigest binds local working-tree builds without claiming that
// their source is the committed tree.
type Anchor struct {
	Version      int    `json:"version"`
	TargetOS     string `json:"target_os"`
	TargetArch   string `json:"target_arch"`
	HelperSHA256 string `json:"helper_sha256"`
	SourceCommit string `json:"source_commit"`
	SourceDigest string `json:"source_digest"`
	SourceClean  bool   `json:"source_clean"`
}

func lowerHex(value string, size int) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == size && hex.EncodeToString(decoded) == value
}

func Parse(encoded, arch string) (Anchor, error) {
	var a Anchor
	if len(encoded) == 0 || len(encoded) > 2048 {
		return a, ErrImage
	}
	raw, err := base64.RawURLEncoding.Strict().DecodeString(encoded)
	if err != nil {
		return a, ErrImage
	}
	// Check duplicate keys as well as unknown keys. encoding/json alone accepts
	// duplicates, which would make provenance interpretation ambiguous.
	decoder := json.NewDecoder(bytes.NewReader(raw))
	first, err := decoder.Token()
	if err != nil || first != json.Delim('{') {
		return a, ErrImage
	}
	seen := make(map[string]bool)
	allowed := map[string]bool{"version": true, "target_os": true, "target_arch": true, "helper_sha256": true, "source_commit": true, "source_digest": true, "source_clean": true}
	for decoder.More() {
		key, err := decoder.Token()
		name, ok := key.(string)
		if err != nil || !ok || !allowed[name] || seen[name] {
			return a, ErrImage
		}
		seen[name] = true
		var value json.RawMessage
		if decoder.Decode(&value) != nil {
			return a, ErrImage
		}
		if name == "source_clean" && !bytes.Equal(bytes.TrimSpace(value), []byte("true")) && !bytes.Equal(bytes.TrimSpace(value), []byte("false")) {
			return a, ErrImage
		}
	}
	if _, err = decoder.Token(); err != nil || len(seen) != 7 {
		return a, ErrImage
	}
	decoder = json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&a) != nil || decoder.Decode(new(any)) != io.EOF || a.Version != 1 ||
		a.TargetOS != "windows" || a.TargetArch != arch || (arch != "amd64" && arch != "arm64") ||
		!lowerHex(a.HelperSHA256, 32) || !lowerHex(a.SourceCommit, 20) || !lowerHex(a.SourceDigest, 32) {
		return Anchor{}, ErrImage
	}
	return a, nil
}

func (a Anchor) RelativePath() string {
	return filepath.Join("native-clipboard", a.HelperSHA256, FileName)
}

// Verify reads the actual image rather than trusting its name or self-report.
// The core launcher must still pin and recheck this image at process creation.
func Verify(path string, a Anchor) error {
	f, err := os.Open(path)
	if err != nil {
		return ErrImage
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > 64<<20 {
		return ErrImage
	}
	hash := sha256.New()
	if _, err = io.Copy(hash, io.LimitReader(f, (64<<20)+1)); err != nil || hex.EncodeToString(hash.Sum(nil)) != a.HelperSHA256 {
		return ErrImage
	}
	image, err := pe.NewFile(f)
	if err != nil {
		return ErrImage
	}
	defer image.Close()
	machine := uint16(pe.IMAGE_FILE_MACHINE_AMD64)
	if a.TargetArch == "arm64" {
		machine = pe.IMAGE_FILE_MACHINE_ARM64
	}
	if image.Machine != machine || image.Characteristics&pe.IMAGE_FILE_DLL != 0 {
		return ErrImage
	}
	if _, ok := image.OptionalHeader.(*pe.OptionalHeader64); !ok {
		return ErrImage
	}
	return nil
}
