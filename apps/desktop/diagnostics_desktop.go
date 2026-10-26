package main

import (
	"errors"
	"os"
	"time"
)

// Only this explicit native-picker command exports a report. The frontend
// receives success/cancel state, never a private raw service snapshot.
func (a *App) ExportDiagnostics() (string, error) {
	if a.core == nil || a.runtimeApp == nil {
		return "", errors.New("DESKTOP_NOT_READY")
	}
	return exportDiagnosticsToSelection(a.runtimeApp.Dialog.SaveFile().SetFilename("LinkSend-diagnostics-"+time.Now().Format("20060102-150405")+".json").AddFilter("JSON 诊断报告", "*.json").PromptForSingleSelection,
		func(destination string) error { return a.core.ExportDiagnosticReport(a.contentContext(), destination) })
}

func exportDiagnosticsToSelection(prompt func() (string, error), save func(string) error) (string, error) {
	destination, err := selectedDirectory(prompt)
	if err != nil {
		return "", errors.New("DIAGNOSTIC_EXPORT_PICKER_FAILED")
	}
	if destination == "" {
		return "cancelled", nil
	}
	if err := save(destination); err != nil {
		if errors.Is(err, os.ErrExist) {
			return "", errors.New("DIAGNOSTIC_EXPORT_FILE_EXISTS")
		}
		return "", errors.New("DIAGNOSTIC_EXPORT_FAILED")
	}
	return "saved", nil
}
