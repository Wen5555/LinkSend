package main

import (
	"bytes"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"

	coreapp "github.com/Wen5555/LinkSend/internal/app"
)

func TestFileSendPreferencePersistsAppliesAndRejectsInvalidWithoutMutation(t *testing.T) {
	t.Cleanup(func() { _ = coreapp.ConfigureFileSendLimit(0) })
	a := &App{dataDir: t.TempDir(), prefs: DesktopPreferences{FormatVersion: 1, Revision: 1, DeviceName: "keep", ClipboardEnabled: true}}
	saved, err := a.SavePreferencesSection("sending", 1, DesktopPreferences{FileSendBytesPerSecond: 2 << 20, DeviceName: "must not replace", ClipboardEnabled: false})
	if err != nil || saved.FileSendBytesPerSecond != 2<<20 || saved.DeviceName != "keep" || !saved.ClipboardEnabled || coreapp.FileSendLimit() != 2<<20 {
		t.Fatalf("save %+v %v", saved, err)
	}
	path := filepath.Join(a.dataDir, "desktop-preferences.json")
	before, _ := os.ReadFile(path)
	for _, rate := range []int64{-1, 1023, 1 << 31, math.MaxInt64} {
		if _, err = a.SavePreferencesSection("sending", saved.Revision, DesktopPreferences{FileSendBytesPerSecond: rate}); err == nil {
			t.Fatal("invalid rate accepted", rate)
		}
		after, _ := os.ReadFile(path)
		if !bytes.Equal(before, after) || coreapp.FileSendLimit() != 2<<20 {
			t.Fatal("invalid save mutated disk or runtime")
		}
	}
	loaded, status := loadPreferencesDetailed(a.dataDir)
	if status.State != "valid" || loaded.FileSendBytesPerSecond != 2<<20 {
		t.Fatal("restart lost limit", loaded, status)
	}
	if _, err = a.SavePreferencesSection("sending", 1, DesktopPreferences{}); err == nil {
		t.Fatal("stale save accepted")
	}
	if _, err = a.SavePreferencesSection("sending", saved.Revision, DesktopPreferences{}); err != nil || coreapp.FileSendLimit() != 0 {
		t.Fatal("unlimited save failed", err)
	}
	for _, rate := range []int64{-1, 0} {
		b, _ := json.Marshal(DesktopPreferences{FormatVersion: 1, FileSendBytesPerSecond: rate})
		_ = os.WriteFile(path, b, 0600)
		_, status = loadPreferencesDetailed(a.dataDir)
		if (rate == 0) != (status.State == "valid") {
			t.Fatal("invalid/legacy preference load", rate, status)
		}
	}
}
