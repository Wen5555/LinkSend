//go:build windows

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

func TestNativeWindowsDriveRelativeArguments(t *testing.T) {
	paths, err := normalizeNativePaths([]string{`C:child.txt`, `\rooted.txt`}, `C:\launch directory`)
	if err != nil || len(paths) != 2 || paths[0] != `C:\launch directory\child.txt` || paths[1] != `C:\rooted.txt` {
		t.Fatalf("drive-relative resolution: %v %v", paths, err)
	}
	if _, err := normalizeNativePaths([]string{`D:unknown-working-directory.txt`}, `C:\launch`); err == nil {
		t.Fatal("guessed another drive's working directory")
	}
	if canonicalNativePathKey(`\\?\UNC\server\share\Profile`) != canonicalNativePathKey(`\\server\share\profile`) {
		t.Fatal("extended UNC form split profile identity")
	}
}

func TestSendToNativeShortcutRoundTripInIsolatedDirectory(t *testing.T) {
	directory := t.TempDir()
	executable := filepath.Join(t.TempDir(), "LinkSend 中文 空格 🛰.exe")
	if err := os.WriteFile(executable, []byte("test target; never executed"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := installSendToAt(executable, directory); err != nil {
		t.Fatal(err)
	}
	observed, observedErr := readSendToShortcut(filepath.Join(directory, sendToName))
	t.Logf("isolated shortcut fixture: expected_target=%q observed_target=%q expected_arguments=%q observed_arguments=%q expected_description=%q observed_description=%q expected_working_directory=%q observed_working_directory=%q read_error=%v", executable, observed.target, sendToArgs, observed.arguments, sendToMarker, observed.description, filepath.Dir(executable), observed.workingDir, observedErr)
	if err := installSendToAt(executable, directory); err != nil {
		t.Fatalf("idempotent registration failed: %v", err)
	}
	value, err := readSendToShortcut(filepath.Join(directory, sendToName))
	if err != nil || !ownsSendTo(value, executable) || value.workingDir != filepath.Dir(executable) {
		t.Fatalf("native shortcut round trip: %+v %v", value, err)
	}
	if err := uninstallSendToAt(executable, directory); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(directory, sendToName)); !os.IsNotExist(err) {
		t.Fatal("owned entry was not removed")
	}
	if err := uninstallSendToAt(executable, directory); err != nil {
		t.Fatal(err)
	}
}

func TestSendToPreservesForeignEntriesAndOtherInstallations(t *testing.T) {
	executable := filepath.Join(t.TempDir(), "LinkSend.exe")
	if err := os.WriteFile(executable, []byte("not executed"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, foreign := range []bool{true, false} {
		directory := t.TempDir()
		link := filepath.Join(directory, sendToName)
		if foreign {
			if err := os.WriteFile(link, []byte("user-owned document"), 0600); err != nil {
				t.Fatal(err)
			}
		} else {
			if err := writeSendToShortcut(link, sendToShortcut{target: filepath.Join(t.TempDir(), "other.exe"), arguments: sendToArgs, description: sendToMarker}); err != nil {
				t.Fatal(err)
			}
		}
		before, err := os.ReadFile(link)
		if err != nil {
			t.Fatal(err)
		}
		if err := installSendToAt(executable, directory); err == nil {
			t.Fatal("foreign/other installation was overwritten")
		}
		if err := uninstallSendToAt(executable, directory); err == nil {
			t.Fatal("foreign/other installation was removed")
		}
		after, err := os.ReadFile(link)
		if err != nil || !bytes.Equal(before, after) {
			t.Fatalf("foreign entry changed: %v", err)
		}
	}
}

func TestSendToNativeShortcutAcceptsDOSPathAlias(t *testing.T) {
	executable := filepath.Join(t.TempDir(), "LinkSend 中文 空格 🛰.exe")
	if err := os.WriteFile(executable, []byte("test target; never executed"), 0600); err != nil {
		t.Fatal(err)
	}
	encoded, err := windows.UTF16PtrFromString(executable)
	if err != nil {
		t.Fatal(err)
	}
	buffer := make([]uint16, 32768)
	n, err := windows.GetShortPathName(encoded, &buffer[0], uint32(len(buffer)))
	if err != nil || n == 0 || n >= uint32(len(buffer)) {
		t.Fatalf("query DOS path alias: length=%d error=%v", n, err)
	}
	shortPath := windows.UTF16ToString(buffer[:n])
	if canonicalNativePathKey(shortPath) == canonicalNativePathKey(executable) {
		t.Skip("fixture volume did not provide a DOS 8.3 alias")
	}
	directory := t.TempDir()
	for _, path := range []string{shortPath, executable, shortPath} {
		if err := installSendToAt(path, directory); err != nil {
			t.Fatalf("register the same installation via %q: %v", path, err)
		}
	}
	value, err := readSendToShortcut(filepath.Join(directory, sendToName))
	if err != nil || !ownsSendTo(value, shortPath) || !ownsSendTo(value, executable) {
		t.Fatalf("DOS alias ownership: value=%+v error=%v", value, err)
	}
	if err := uninstallSendToAt(shortPath, directory); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(directory, sendToName)); !os.IsNotExist(err) {
		t.Fatal("owned DOS alias entry was not removed")
	}
}

func TestSendToPreservesAnotherInstallationWithSameFileIdentity(t *testing.T) {
	executable := filepath.Join(t.TempDir(), "LinkSend.exe")
	if err := os.WriteFile(executable, []byte("test target; never executed"), 0600); err != nil {
		t.Fatal(err)
	}
	otherExecutable := filepath.Join(t.TempDir(), "LinkSend.exe")
	if err := os.Link(executable, otherExecutable); err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	if err := installSendToAt(otherExecutable, directory); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(directory, sendToName)
	before, err := os.ReadFile(link)
	if err != nil {
		t.Fatal(err)
	}
	if err := installSendToAt(executable, directory); err == nil {
		t.Fatal("accepted another installation because its EXE shares file identity")
	}
	if err := uninstallSendToAt(executable, directory); err == nil {
		t.Fatal("removed another installation because its EXE shares file identity")
	}
	after, err := os.ReadFile(link)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("another installation's shortcut changed: %v", err)
	}
}
