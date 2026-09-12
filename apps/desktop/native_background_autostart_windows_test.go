//go:build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

func nativeAutostartTestPaths(t *testing.T) (string, string) {
	t.Helper()
	base := fmt.Sprintf(`Software\LinkSend Native Tests\%d-%d`, os.Getpid(), time.Now().UnixNano())
	runPath, ownerPath := base+`\Run`, base+`\Owner`
	t.Cleanup(func() {
		_ = registry.DeleteKey(registry.CURRENT_USER, runPath)
		_ = registry.DeleteKey(registry.CURRENT_USER, ownerPath)
		_ = registry.DeleteKey(registry.CURRENT_USER, base)
		_ = registry.DeleteKey(registry.CURRENT_USER, `Software\LinkSend Native Tests`)
	})
	return runPath, ownerPath
}

func TestNativeAutostartWindowsRegistryOwnershipAndQuoting(t *testing.T) {
	runPath, ownerPath := nativeAutostartTestPaths(t)
	executable := filepath.Join(t.TempDir(), "LinkSend 中文 & 空格.exe")
	if err := os.WriteFile(executable, []byte("test target, never executed"), 0600); err != nil {
		t.Fatal(err)
	}
	for j := 0; j < 2; j++ {
		if err := installNativeAutostartAt(executable, runPath, ownerPath); err != nil {
			t.Fatal(err)
		}
	}
	command, err := nativeAutostartReadValue(runPath, nativeAutostartRunName)
	if err != nil {
		t.Fatal(err)
	}
	arguments, err := windows.DecomposeCommandLine(command)
	if err != nil || len(arguments) != 2 || arguments[0] != executable || arguments[1] != "--background" {
		t.Fatalf("startup command did not preserve argv: %v %v", arguments, err)
	}
	for j := 0; j < 2; j++ {
		if err := uninstallNativeAutostartAt(executable, runPath, ownerPath); err != nil {
			t.Fatal(err)
		}
	}
	if current, err := nativeAutostartReadValue(runPath, nativeAutostartRunName); err != nil || current != "" {
		t.Fatalf("startup entry retained: %q %v", current, err)
	}
}

func TestNativeAutostartWindowsPreservesForeignRunValue(t *testing.T) {
	runPath, ownerPath := nativeAutostartTestPaths(t)
	executable := filepath.Join(t.TempDir(), "LinkSend.exe")
	if err := os.WriteFile(executable, []byte("not executed"), 0600); err != nil {
		t.Fatal(err)
	}
	key, _, err := registry.CreateKey(registry.CURRENT_USER, runPath, registry.SET_VALUE)
	if err != nil {
		t.Fatal(err)
	}
	const foreign = `"C:\Users\example\Another App.exe" --user-options`
	if err := key.SetStringValue(nativeAutostartRunName, foreign); err != nil {
		t.Fatal(err)
	}
	_ = key.Close()
	if err := installNativeAutostartAt(executable, runPath, ownerPath); err == nil {
		t.Fatal("foreign Run value overwritten")
	}
	if err := uninstallNativeAutostartAt(executable, runPath, ownerPath); err == nil {
		t.Fatal("foreign Run value removed")
	}
	if value, err := nativeAutostartReadValue(runPath, nativeAutostartRunName); err != nil || value != foreign {
		t.Fatalf("foreign Run value changed: %q %v", value, err)
	}
}

func TestNativeBackgroundActivationArgumentsAreNotDraftFiles(t *testing.T) {
	for _, args := range [][]string{{"-Embedding"}, {"--background"}} {
		paths, err := parseNativeFileArguments(args, t.TempDir())
		if err != nil || len(paths) != 0 {
			t.Fatalf("native activation became a file: %v %v", paths, err)
		}
	}
}
