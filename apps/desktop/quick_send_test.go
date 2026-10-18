package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	coreapp "github.com/Wen5555/LinkSend/internal/app"
)

func TestQuickSendOwnDraftRepeatedOpenAndRestartPreserveMain(t *testing.T) {
	dir := t.TempDir()
	service, err := coreapp.New(coreapp.Config{DataDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(service.Shutdown)
	a := NewApp()
	a.core = service
	a.dataDir = dir
	mainPath := filepath.Join(t.TempDir(), "main.txt")
	quickPath := filepath.Join(t.TempDir(), "quick.txt")
	for _, path := range []string{mainPath, quickPath} {
		if err = os.WriteFile(path, []byte(path), 0600); err != nil {
			t.Fatal(err)
		}
	}
	main, err := service.SaveDraft(coreapp.SendDraft{ID: "main", Paths: []string{mainPath}}, "")
	if err != nil {
		t.Fatal(err)
	}
	quick, err := a.QuickSendDraft()
	if err != nil || quick.ID != "quick" || len(quick.Paths) != 0 {
		t.Fatal(quick, err)
	}
	quick.Paths = []string{quickPath}
	quick, err = a.SaveQuickSendDraft(quick)
	if err != nil {
		t.Fatal(err)
	}
	for range 3 {
		if err = a.OpenQuickSend(); err != nil {
			t.Fatal(err)
		}
	}
	if a.Background().QuickSendRevision != 3 {
		t.Fatal("open requests were lost")
	}
	if _, err = a.SaveQuickSendDraft(main); err == nil {
		t.Fatal("quick boundary accepted main draft")
	}
	if _, err = a.SaveQuickSendDraft(coreapp.SendDraft{ID: "quick", Paths: []string{quickPath}}); err == nil {
		t.Fatal("stale revision overwrote quick draft")
	}
	current, _ := service.SendDraft("main")
	if !reflect.DeepEqual(main, current) {
		t.Fatal("quick panel altered main")
	}
	service.Shutdown()
	reopened, err := coreapp.New(coreapp.Config{DataDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Shutdown()
	a.core = reopened
	actual, err := a.QuickSendDraft()
	if err != nil || !reflect.DeepEqual(actual, quick) {
		t.Fatal("quick draft lost after restart", actual, err)
	}
	current, _ = reopened.SendDraft("main")
	if !reflect.DeepEqual(main, current) {
		t.Fatal("restart altered main")
	}
	if files, err := reopened.DeviceProfiles(); err != nil || len(files) != 0 {
		t.Fatal("opening quick send granted trust", files, err)
	}
}
