package main

import (
	"errors"
	"os"
	"runtime"
	"strings"
	"testing"
)

func TestDiagnosticsExportSelectionCancellationAndSafeErrors(t *testing.T) {
	for _, test := range []struct {
		name, path          string
		promptErr, saveErr  error
		wantState, wantCode string
		wantWrite           bool
	}{
		{name: "cancel", wantState: "cancelled"},
		{name: "save", path: "selected.json", wantState: "saved", wantWrite: true},
		{name: "native failure", path: "FAKE_PRIVATE_STALE_PATH", promptErr: errors.New("FAKE_PRIVATE_PICKER_EXCEPTION"), wantCode: "DIAGNOSTIC_EXPORT_PICKER_FAILED"},
		{name: "existing user file", path: "selected.json", saveErr: os.ErrExist, wantWrite: true, wantCode: "DIAGNOSTIC_EXPORT_FILE_EXISTS"},
		{name: "write failure", path: "selected.json", saveErr: errors.New("FAKE_PRIVATE_WRITE_EXCEPTION"), wantWrite: true, wantCode: "DIAGNOSTIC_EXPORT_FAILED"},
	} {
		t.Run(test.name, func(t *testing.T) {
			wrote := false
			state, err := exportDiagnosticsToSelection(func() (string, error) { return test.path, test.promptErr }, func(path string) error {
				wrote = true
				if path != test.path {
					t.Fatal("selection changed")
				}
				return test.saveErr
			})
			code := ""
			if err != nil {
				code = err.Error()
			}
			if state != test.wantState || code != test.wantCode || wrote != test.wantWrite || strings.Contains(code, "FAKE_PRIVATE") {
				t.Fatal("export result leaked or misrepresented selection", state, code, wrote)
			}
		})
	}
	if runtime.GOOS == "windows" {
		state, err := exportDiagnosticsToSelection(func() (string, error) { return "FAKE_PRIVATE_STALE_PATH", errors.New("cancelled by user") }, func(string) error { t.Fatal("cancelled native dialog wrote output"); return nil })
		if err != nil || state != "cancelled" {
			t.Fatal("verified Windows CFD cancellation was shown as failure", state, err)
		}
	}
}
