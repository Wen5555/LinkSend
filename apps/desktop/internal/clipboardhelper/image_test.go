package clipboardhelper

import (
	"bytes"
	"crypto/sha256"
	"debug/pe"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testAnchor() Anchor {
	return Anchor{1, "windows", "amd64", strings.Repeat("a", 64), strings.Repeat("b", 40), strings.Repeat("c", 64), false}
}

func encode(raw string) string { return base64.RawURLEncoding.EncodeToString([]byte(raw)) }

func TestParseAnchorRejectsAmbiguousOrUntrustedMetadata(t *testing.T) {
	a := testAnchor()
	raw, _ := json.Marshal(a)
	if parsed, err := Parse(encode(string(raw)), "amd64"); err != nil || parsed != a {
		t.Fatalf("valid working-tree anchor rejected: %v", err)
	}
	cases := map[string]string{
		"empty": "", "base64": "!", "unknown": encode(strings.TrimSuffix(string(raw), "}") + `,"extra":1}`),
		"duplicate":                  encode(strings.TrimSuffix(string(raw), "}") + `,"version":1}`),
		"missing":                    encode(strings.Replace(string(raw), `,"source_clean":false`, "", 1)),
		"null boolean":               encode(strings.Replace(string(raw), `"source_clean":false`, `"source_clean":null`, 1)),
		"case aliased key":           encode(strings.Replace(string(raw), `"source_clean":false`, `"SOURCE_CLEAN":null`, 1)),
		"case duplicate missing key": encode(strings.Replace(strings.TrimSuffix(string(raw), "}"), `,"source_clean":false`, "", 1) + `,"VERSION":1}`),
		"wrong version":              encode(strings.Replace(string(raw), `"version":1`, `"version":2`, 1)),
		"wrong os":                   encode(strings.Replace(string(raw), `"windows"`, `"linux"`, 1)),
		"hash path":                  encode(strings.Replace(string(raw), a.HelperSHA256, "../"+strings.Repeat("a", 61), 1)),
		"uppercase":                  encode(strings.Replace(string(raw), a.HelperSHA256, strings.ToUpper(a.HelperSHA256), 1)),
		"commit":                     encode(strings.Replace(string(raw), a.SourceCommit, "wrong", 1)),
		"trailing json":              encode(string(raw) + "{}"),
	}
	for name, encoded := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse(encoded, "amd64"); err == nil {
				t.Fatal("accepted invalid anchor")
			}
		})
	}
	if _, err := Parse(encode(string(raw)), "arm64"); err == nil {
		t.Fatal("accepted wrong architecture")
	}
}

// Minimal PE fixture is parsed only, never executed or installed.
func testPE(t *testing.T, directory string) (string, Anchor) {
	t.Helper()
	var image bytes.Buffer
	dos := make([]byte, 128)
	copy(dos, "MZ")
	binary.LittleEndian.PutUint32(dos[0x3c:], uint32(len(dos)))
	image.Write(dos)
	image.WriteString("PE\x00\x00")
	for _, value := range []any{pe.FileHeader{Machine: pe.IMAGE_FILE_MACHINE_AMD64, SizeOfOptionalHeader: uint16(binary.Size(pe.OptionalHeader64{})), Characteristics: pe.IMAGE_FILE_EXECUTABLE_IMAGE}, pe.OptionalHeader64{Magic: 0x20b, NumberOfRvaAndSizes: 16}} {
		if err := binary.Write(&image, binary.LittleEndian, value); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(directory, FileName)
	if err := os.WriteFile(path, image.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(image.Bytes())
	a := testAnchor()
	a.HelperSHA256 = hex.EncodeToString(digest[:])
	return path, a
}

func TestVerifyActualPEAndDigest(t *testing.T) {
	path, a := testPE(t, t.TempDir())
	if err := Verify(path, a); err != nil {
		t.Fatalf("valid PE rejected: %v", err)
	}
	wrongArch := a
	wrongArch.TargetArch = "arm64"
	if Verify(path, wrongArch) == nil {
		t.Fatal("wrong PE machine accepted")
	}
	if Verify(path+".missing", a) == nil {
		t.Fatal("missing helper accepted")
	}
	if err := os.WriteFile(path, []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	if Verify(path, a) == nil {
		t.Fatal("changed helper accepted")
	}
	if Verify(filepath.Dir(path), a) == nil {
		t.Fatal("directory accepted")
	}
}
