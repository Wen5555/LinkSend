package app

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/Wen5555/LinkSend/internal/transfer"
)

var errClipboardCachePrivate = errors.New("CLIPBOARD_CACHE_PATH_PRIVATE")

func clipboardPathWithin(parent, path string) bool {
	parent = filepath.Clean(parent)
	path = filepath.Clean(path)
	if runtime.GOOS == "windows" {
		parent = strings.ToLower(parent)
		path = strings.ToLower(path)
	}
	rel, err := filepath.Rel(parent, path)
	return err == nil && (rel == "." || (!filepath.IsAbs(rel) && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))))
}

// clipboardSourceAccess is a denial gate, not a lease granting access to cache
// data. Ordinary source preparation, hash and every re-open must use it. A
// selected ancestor stays allowed until traversal actually enters the private
// tree; excluded subtrees are therefore not rejected by lexical ancestry.
func (s *Service) clipboardSourceAccess(ctx context.Context, access transfer.SourceAccess) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s == nil || strings.TrimSpace(s.cfg.DataDir) == "" {
		return nil, errClipboardCacheOwnership
	}
	base, err := filepath.Abs(filepath.Join(s.cfg.DataDir, "clipboard-files"))
	if err != nil {
		return nil, err
	}
	if clipboardPathWithin(base, access.Path) {
		return nil, errors.Join(transfer.ErrPath, errClipboardCachePrivate)
	}
	if access.File == nil {
		return nil, nil
	}
	profile, err := os.OpenRoot(s.cfg.DataDir)
	if err != nil {
		return nil, err
	}
	defer profile.Close()
	info, err := profile.Lstat("clipboard-files")
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	// A linked/foreign cache namespace is never claimed as ownership of its
	// target. The lexical private path is already denied above; external user
	// files must not become private merely because somebody linked this name.
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, nil
	}
	root, err := profile.OpenRoot("clipboard-files")
	if err != nil {
		return nil, err
	}
	rootFile, err := root.Open(".")
	if err != nil {
		_ = root.Close()
		return nil, err
	}
	closeRoots := func() { _ = rootFile.Close(); _ = root.Close() }
	actual, err := rootFile.Stat()
	if err != nil || !os.SameFile(info, actual) {
		closeRoots()
		return nil, errors.Join(errClipboardCacheOwnership, err)
	}
	if access.Info != nil && access.Info.IsDir() && os.SameFile(actual, access.Info) {
		closeRoots()
		return nil, errors.Join(transfer.ErrPath, errClipboardCachePrivate)
	}
	rootPath, err := clipboardOpenedPath(rootFile)
	if err != nil {
		closeRoots()
		return nil, err
	}
	openedPath, err := clipboardOpenedPath(access.File)
	if err != nil {
		closeRoots()
		return nil, err
	}
	if clipboardPathWithin(rootPath, openedPath) {
		closeRoots()
		return nil, errors.Join(transfer.ErrPath, errClipboardCachePrivate)
	}
	// Retain the real root through the borrowed source FD's successful close.
	// transfer owns that ordering; cancellation alone is not a release signal.
	return closeRoots, nil
}

func (s *Service) prepareUserSources(ctx context.Context, paths []string, size int, selection *transfer.SourceSelection) (*transfer.Prepared, error) {
	return transfer.PrepareSelectedWithAccess(ctx, paths, size, selection, s.clipboardSourceAccess)
}

func (s *Service) prepareUserSourcesForResume(ctx context.Context, paths []string, size int, id string, selection *transfer.SourceSelection) (*transfer.Prepared, error) {
	return transfer.PrepareSelectedForResumeWithAccess(ctx, paths, size, id, selection, s.clipboardSourceAccess)
}

// ValidateExternalFilePath is an internal Go/desktop boundary, not a Wails
// service. Core resolves opaque record IDs first, then checks the real opened
// target before handing a path to an OS Open/Reveal/directory launcher.
func (s *Service) ValidateExternalFilePath(ctx context.Context, path string) error {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if _, err = s.clipboardSourceAccess(ctx, transfer.SourceAccess{Path: absolute}); err != nil {
		return err
	}
	file, err := os.Open(absolute)
	if err != nil {
		return err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return err
	}
	release, guardErr := s.clipboardSourceAccess(ctx, transfer.SourceAccess{Path: absolute, File: file, Info: info})
	closeErr := file.Close()
	if closeErr == nil && release != nil {
		release()
	}
	return errors.Join(guardErr, closeErr)
}
