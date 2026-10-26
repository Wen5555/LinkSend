//go:build windows

package app

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
)

func TestClipboardCacheExportsRejectsAncestorJunction(t *testing.T) {
	s, _, private, _ := clipboardExportFixture(t)
	alias := filepath.Join(t.TempDir(), "source-alias")
	command := exec.Command("cmd.exe", "/c", "mklink", "/J", alias, filepath.Join(s.cfg.DataDir, "clipboard-files"))
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("cannot create self-owned junction: %v %s", err, output)
	}
	aliased := filepath.Join(alias, filepath.Base(filepath.Dir(private)), filepath.Base(private))
	if p, err := s.prepareUserSources(t.Context(), []string{aliased}, 0, nil); p != nil || !errors.Is(err, errClipboardCachePrivate) {
		if p != nil {
			_ = p.Close()
		}
		t.Fatal("ancestor junction bypassed actual-FD source check", err)
	}
	if err := s.ValidateExternalFilePath(t.Context(), aliased); !errors.Is(err, errClipboardCachePrivate) {
		t.Fatal("OS export alias bypassed actual-FD check", err)
	}
	if _, err := os.Stat(private); err != nil {
		t.Fatal("junction check altered source", err)
	}
}
