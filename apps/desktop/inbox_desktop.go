package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	coreapp "github.com/Wen5555/LinkSend/internal/app"
)

func (a *App) Inbox(query coreapp.InboxQuery) (coreapp.InboxPage, error) {
	if err := a.workspaceAvailable(); err != nil {
		return coreapp.InboxPage{}, err
	}
	ctx, cancel := context.WithTimeout(a.ctx, 10*time.Second)
	defer cancel()
	return a.core.Inbox(ctx, query)
}

func (a *App) InboxFiles(taskID, cursor string, limit int) (coreapp.InboxFilesPage, error) {
	if err := a.workspaceAvailable(); err != nil {
		return coreapp.InboxFilesPage{}, err
	}
	ctx, cancel := context.WithTimeout(a.ctx, 10*time.Second)
	defer cancel()
	return a.core.InboxFiles(ctx, taskID, cursor, limit)
}

func (a *App) ResendInbox(request coreapp.ResendInboxRequest) (coreapp.QueueItem, error) {
	if err := a.workspaceAvailable(); err != nil {
		return coreapp.QueueItem{}, err
	}
	return a.core.ResendInbox(a.ctx, request)
}

func (a *App) ForgetInboxRecords(records []coreapp.InboxRecordRef) error {
	if err := a.workspaceAvailable(); err != nil {
		return err
	}
	return a.core.ForgetInboxRecords(records)
}

func (a *App) CleanupInboxStaging(limit int) (coreapp.InboxCleanupResult, error) {
	if err := a.workspaceAvailable(); err != nil {
		return coreapp.InboxCleanupResult{}, err
	}
	ctx, cancel := context.WithTimeout(a.ctx, 30*time.Second)
	defer cancel()
	return a.core.CleanupInboxStaging(ctx, limit)
}

// RevealInboxFile accepts opaque IDs, never a path or shell command from JS.
// Core resolves the persisted receive plan and checks the actual filesystem.
func (a *App) RevealInboxFile(taskID string, fileID uint32) error {
	if err := a.workspaceAvailable(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(a.ctx, 10*time.Second)
	defer cancel()
	location, err := a.core.ResolveInboxFile(ctx, taskID, fileID)
	if err != nil {
		return err
	}
	program, arguments, err := inboxRevealCommand(runtime.GOOS, os.Getenv("SystemRoot"), location.Path)
	if err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	command := exec.Command(program, arguments...)
	if err = command.Start(); err != nil {
		return err
	}
	// Reap the short-lived launcher without tying the user's Finder/Explorer
	// window to a request timeout or killing it when the app later shuts down.
	go func() { _ = command.Wait() }()
	return nil
}

func inboxRevealCommand(platform, systemRoot, filename string) (string, []string, error) {
	if filename == "" || strings.ContainsRune(filename, 0) {
		return "", nil, errors.New("INVALID_REVEAL_TARGET")
	}
	switch platform {
	case "windows":
		if !filepath.IsAbs(systemRoot) || !filepath.IsAbs(filename) {
			return "", nil, errors.New("INVALID_REVEAL_TARGET")
		}
		return filepath.Join(systemRoot, "explorer.exe"), []string{"/select,", filename}, nil
	case "darwin":
		if !strings.HasPrefix(filename, "/") {
			return "", nil, errors.New("INVALID_REVEAL_TARGET")
		}
		return "/usr/bin/open", []string{"-R", filename}, nil
	default:
		return "", nil, errors.New("NATIVE_REVEAL_UNSUPPORTED")
	}
}
