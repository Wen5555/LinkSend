package transfer

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
)

// SourceAccess describes a local source admission, never wire metadata. Path
// is the original absolute name and may be an alias or stale after a rename;
// only the actual borrowed File/Root can establish filesystem identity.
// File is nil before an open, and non-nil after a successful open/stat.
// The hook may inspect metadata but must not read/seek/mutate, close or retain
// File or Root.
type SourceAccess struct {
	Root     *os.Root
	Relative string
	Path     string
	File     *os.File
	Info     fs.FileInfo
}

// SourceAccessHook may reject before open and must recheck actual identity
// after open. Returned leases remain held through the real FD close, including
// rejection, read failure and cancellation. A failed Close conservatively keeps
// leases; context cancellation alone never releases them.
type SourceAccessHook func(context.Context, SourceAccess) (release func(), err error)

func openSourceWithAccess(ctx context.Context, root *os.Root, relative string, expected fs.FileInfo, hooks []SourceAccessHook) (_ *os.File, closeSource func() error, err error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	access := SourceAccess{Root: root, Relative: relative, Path: filepath.Join(root.Name(), filepath.FromSlash(relative)), Info: expected}
	var file *os.File
	var releases []func()
	var once sync.Once
	var closeErr error
	closeSource = func() error {
		once.Do(func() {
			if file != nil {
				closeErr = file.Close()
			}
			if closeErr == nil {
				for i := len(releases) - 1; i >= 0; i-- {
					releases[i]()
				}
			}
		})
		return closeErr
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, closeSource())
		}
	}()
	check := func() error {
		for _, hook := range hooks {
			if hook == nil {
				continue
			}
			release, hookErr := hook(ctx, access)
			if release != nil {
				releases = append(releases, release)
			}
			if hookErr != nil {
				return hookErr
			}
		}
		return ctx.Err()
	}
	if err = check(); err != nil {
		return nil, closeSource, err
	}
	file, err = root.Open(relative)
	if err != nil {
		return nil, closeSource, err
	}
	opened, statErr := file.Stat()
	if statErr != nil {
		return nil, closeSource, statErr
	}
	if expected != nil && (!os.SameFile(expected, opened) || expected.IsDir() != opened.IsDir()) {
		return nil, closeSource, ErrChanged
	}
	access.File, access.Info = file, opened
	if err = check(); err != nil {
		return nil, closeSource, err
	}
	return file, closeSource, nil
}

func (p *Prepared) sourceAccessSnapshot() []SourceAccessHook {
	p.accessMu.RLock()
	defer p.accessMu.RUnlock()
	return append([]SourceAccessHook(nil), p.sourceAccess...)
}

// RestrictSourceAccess adds a constraint; it never replaces/removes existing
// rules, even when its immediate validation rejects. Call before admitting a
// prepared transfer for sending, not concurrently with its reads/Close.
// This checks actual selected directory/file FDs without reading bodies. It
// cannot retroactively cover hashes made by an earlier hook-free Prepare.
func (p *Prepared) RestrictSourceAccess(ctx context.Context, hook SourceAccessHook) error {
	if p == nil {
		return ErrPath
	}
	if hook == nil {
		return nil
	}
	p.accessMu.Lock()
	p.sourceAccess = append(p.sourceAccess, hook)
	hooks := append([]SourceAccessHook(nil), p.sourceAccess...)
	p.accessMu.Unlock()
	for _, node := range p.accessNodes {
		_, closeSource, err := openSourceWithAccess(ctx, node.root, node.relative, node.info, hooks)
		if err != nil {
			return err
		}
		if err = closeSource(); err != nil {
			return err
		}
	}
	return nil
}
