package main

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"testing"
)

func TestNativePickerCancellationReturnsNoSelectionOrFailure(t *testing.T) {
	for _, err := range []error{nil, errors.New("cancelled by user"), errors.New("permission denied"), errors.New("cancelled by user: access denied"), fmt.Errorf("dialog failed: %w", errors.New("cancelled by user")), context.Canceled} {
		path, got := selectedDirectory(func() (string, error) { return "stale directory", err })
		files, filesErr := selectedFiles(func() ([]string, error) { return []string{"stale file"}, err })
		if runtime.GOOS == "windows" && err != nil && err.Error() == "cancelled by user" {
			if got != nil || filesErr != nil || path != "" || len(files) != 0 {
				t.Fatal("cancel exposed stale selection or failure", path, files, got, filesErr)
			}
		} else if got != err || filesErr != err || path != "stale directory" || len(files) != 1 {
			t.Fatal("normal result or actual failure was changed", path, files, got, filesErr)
		}
	}
}
