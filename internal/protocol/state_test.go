package protocol

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSupportedCapabilitiesExposeProductVersionSeparately(t *testing.T) {
	caps := Supported()
	if ProductVersion != "0.3.0" || caps.ProductVersion != ProductVersion {
		t.Fatalf("product version drift: constant=%q capabilities=%q", ProductVersion, caps.ProductVersion)
	}
	if caps.ProtocolVersion != Version || Version != 1 {
		t.Fatalf("protocol version changed with product version: protocol=%d", caps.ProtocolVersion)
	}
}

func TestProductVersionMetadataSynchronized(t *testing.T) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test source")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(source), "..", ".."))
	checks := [][2]string{
		{"apps/desktop/build/config.yml", `version: "` + ProductVersion + `"`},
		{"apps/desktop/build/darwin/Info.plist", `<string>` + ProductVersion + `</string>`},
		{"apps/desktop/build/windows/info.json", `"file_version": "` + ProductVersion + `.0"`},
		{"apps/desktop/build/windows/info.json", `"product_version": "` + ProductVersion + `.0"`},
		{"apps/desktop/build/windows/info.json", `"0409"`},
		{"apps/desktop/build/windows/info.json", `"FileVersion": "` + ProductVersion + `"`},
		{"apps/desktop/build/windows/info.json", `"ProductVersion": "` + ProductVersion + `"`},
		{"apps/desktop/build/windows/wails.exe.manifest", `version="` + ProductVersion + `"`},
		{"apps/desktop/build/windows/nsis/project.nsi", `INFO_PRODUCTVERSION "` + ProductVersion + `"`},
		{"apps/desktop/frontend/package.json", `"version": "` + ProductVersion + `"`},
		{".github/workflows/wails3-packages.yml", `PRODUCT_VERSION: ` + ProductVersion},
	}
	for _, check := range checks {
		relative, expected := check[0], check[1]
		body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relative)))
		if err != nil {
			t.Fatalf("read %s: %v", relative, err)
		}
		if !strings.Contains(string(body), expected) {
			t.Errorf("%s does not contain %q", relative, expected)
		}
	}
}

func TestTransferStateTerminalAndRecoveryTransitions(t *testing.T) {
	completed := NewTransferState()
	for _, state := range []string{"AwaitingAcceptance", "Transferring", "Verifying", "Completed"} {
		if err := completed.Transition(state); err != nil {
			t.Fatalf("transition to %s: %v", state, err)
		}
	}
	if err := completed.Transition("Recovering"); err == nil {
		t.Fatal("completed task left its terminal state")
	}
	rejected := NewTransferState()
	if err := rejected.Transition("AwaitingAcceptance"); err != nil {
		t.Fatal(err)
	}
	if err := rejected.Transition("Rejected"); err != nil {
		t.Fatal(err)
	}
	if err := rejected.Transition("Transferring"); err == nil {
		t.Fatal("rejected task resumed without a new attempt")
	}
	paused := NewTransferState()
	for _, state := range []string{"AwaitingAcceptance", "Transferring", "Paused", "Recovering", "Transferring"} {
		if err := paused.Transition(state); err != nil {
			t.Fatalf("transition to %s: %v", state, err)
		}
	}
}
