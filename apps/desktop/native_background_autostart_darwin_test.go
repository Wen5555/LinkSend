//go:build darwin

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestNativeAutostartDarwinAgentOwnershipAndEscaping(t *testing.T) {
	directory := t.TempDir()
	executable := filepath.Join(t.TempDir(), "LinkSend 中文 & 空格")
	if err := os.WriteFile(executable, []byte("test target, never executed"), 0700); err != nil {
		t.Fatal(err)
	}
	for j := 0; j < 2; j++ {
		if err := installNativeAutostartAt(executable, directory); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(directory, nativeLaunchAgentLabel+".plist")
	if _, err := readOwnedNativeLaunchAgent(path, executable); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || !bytes.Contains(data, []byte("&amp;")) {
		t.Fatalf("XML did not preserve a metacharacter path: %v", err)
	}
	for j := 0; j < 2; j++ {
		if err := uninstallNativeAutostartAt(executable, directory); err != nil {
			t.Fatal(err)
		}
	}
}

func TestNativeAutostartDarwinPreservesForeignAndChangedAgents(t *testing.T) {
	executable := filepath.Join(t.TempDir(), "LinkSend")
	if err := os.WriteFile(executable, []byte("not executed"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, data := range [][]byte{
		[]byte("user-owned document"),
		nativeLaunchAgentXML(filepath.Join(t.TempDir(), "different-installation")),
		bytes.Replace(nativeLaunchAgentXML(executable), []byte("<true/>"), []byte("<false/>"), 1),
		bytes.Replace(nativeLaunchAgentXML(executable), []byte("<dict>"), []byte("<dict><dict>"), 1),
	} {
		directory := t.TempDir()
		path := filepath.Join(directory, nativeLaunchAgentLabel+".plist")
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		if err := installNativeAutostartAt(executable, directory); err == nil {
			t.Fatal("foreign launch agent overwritten")
		}
		if err := uninstallNativeAutostartAt(executable, directory); err == nil {
			t.Fatal("foreign launch agent removed")
		}
		after, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(after, data) {
			t.Fatalf("foreign launch agent changed: %v", err)
		}
	}
}
