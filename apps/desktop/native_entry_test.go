package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func TestNativeArgumentsPreserveUnicodeSpacesAndWorkingDirectory(t *testing.T) {
	root := t.TempDir()
	args := []string{"--send-files", "--", "中文 文件.txt", "子目录", "--literal-file", "$(not-a-command).txt"}
	paths, err := parseNativeFileArguments(args, root)
	if err != nil {
		t.Fatal(err)
	}
	for i, name := range args[2:] {
		if paths[i] != filepath.Join(root, name) {
			t.Fatalf("path %d changed: %q", i, paths[i])
		}
	}
	if _, err := parseNativeFileArguments([]string{"relative.txt"}, ""); err == nil {
		t.Fatal("missing working directory accepted")
	}
	if _, err := parseNativeFileArguments([]string{"--unknown"}, root); err == nil {
		t.Fatal("unknown option accepted as a path")
	}
}

func TestNativeArgumentsRejectEntireOversizedOrInvalidSelection(t *testing.T) {
	root := t.TempDir()
	for _, paths := range [][]string{
		make([]string, maxNativeEntryPaths+1),
		{"ok", ""},
		{"ok", "a\x00b"},
		{"ok", string([]byte{0xff})},
		{"ok", strings.Repeat("x", maxNativeEntryBytes)},
	} {
		if got, err := normalizeNativePaths(paths, root); err == nil || got != nil {
			t.Fatalf("invalid selection was partially accepted: %d paths, %v", len(got), err)
		}
	}
}

func TestNativeInstanceProfileIsolationAndSecondArguments(t *testing.T) {
	root := t.TempDir()
	profile := filepath.Join(root, "尚未创建", "配置")
	var received []string
	var receivedDir string
	a, err := profileSingleInstanceOptions(profile, func(args []string, dir string) { received, receivedDir = args, dir })
	if err != nil {
		t.Fatal(err)
	}
	b, err := profileSingleInstanceOptions(filepath.Join(root, ".", "尚未创建", "配置"), nil)
	if err != nil || a.UniqueID != b.UniqueID {
		t.Fatalf("equivalent profiles split ownership: %v", err)
	}
	c, err := profileSingleInstanceOptions(filepath.Join(root, "other"), nil)
	if err != nil || a.UniqueID == c.UniqueID {
		t.Fatalf("different profiles share instance: %v", err)
	}
	if strings.Contains(a.UniqueID, root) || strings.Contains(a.UniqueID, "配置") {
		t.Fatal("profile path leaked into instance ID")
	}
	if _, err := os.Stat(profile); !os.IsNotExist(err) {
		t.Fatal("instance options created the profile before ownership")
	}
	args := []string{"LinkSend executable", "中文 文件.txt", "other"}
	a.OnSecondInstanceLaunch(application.SecondInstanceData{Args: args, WorkingDir: root})
	if !reflect.DeepEqual(received, args[1:]) || receivedDir != root {
		t.Fatalf("argv[0]/working directory handling failed: %v %q", received, receivedDir)
	}
	args[1] = "mutated"
	if received[0] == "mutated" {
		t.Fatal("callback arguments alias upstream buffer")
	}
}

func TestNativeInstanceResolvesDirectoryAliases(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "real")
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(root, "alias")
	if err := os.Symlink(target, alias); err != nil {
		t.Skipf("directory symlinks unavailable: %v", err)
	}
	a, err := canonicalProfilePath(filepath.Join(target, "future"))
	if err != nil {
		t.Fatal(err)
	}
	b, err := canonicalProfilePath(filepath.Join(alias, "future"))
	if err != nil || a != b {
		t.Fatalf("directory alias splits profile ownership: %q %q %v", a, b, err)
	}
}

func TestNativeInstanceExistingDirectoryCaseAliases(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "ProfileCase")
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(root, "pROFILEcASE")
	info, err := os.Stat(other)
	if os.IsNotExist(err) {
		t.Skip("case-sensitive filesystem has no case alias")
	}
	if err != nil {
		t.Fatal(err)
	}
	original, err := os.Stat(target)
	if err != nil || !os.SameFile(info, original) {
		t.Fatal("case alias did not identify the same directory")
	}
	a, err := canonicalProfilePath(target)
	if err != nil {
		t.Fatal(err)
	}
	b, err := canonicalProfilePath(other)
	if err != nil || a != b {
		t.Fatalf("case aliases split an existing profile: %q %q %v", a, b, err)
	}
}
