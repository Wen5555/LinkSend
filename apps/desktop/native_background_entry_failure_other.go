//go:build !windows && !darwin

package main

import (
	"fmt"
	"os"
)

func showNativeEntryFailure(message string) { fmt.Fprintln(os.Stderr, "LinkSend:", message) }
