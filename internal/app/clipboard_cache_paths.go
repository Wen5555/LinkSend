package app

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/Wen5555/LinkSend/internal/transfer"
)

// The profile owner pins every destination component before staging starts.
type clipboardCacheDestination struct {
	profile, cache, object             *os.Root
	profileInfo, cacheInfo, objectInfo fs.FileInfo
	profilePath, directory, id         string
}

func openClipboardCacheDestination(profilePath, id string) (_ *clipboardCacheDestination, err error) {
	if !validClipboardFileID(id, 16) {
		return nil, transfer.ErrPath
	}
	d := &clipboardCacheDestination{profilePath: profilePath, id: id, directory: filepath.Join(profilePath, "clipboard-files", id)}
	defer func() {
		if err != nil {
			_ = d.Close()
		}
	}()
	d.profile, err = os.OpenRoot(profilePath)
	if err != nil {
		return nil, err
	}
	d.profileInfo, err = d.profile.Stat(".")
	if err != nil {
		return nil, err
	}
	if err = d.profile.Mkdir("clipboard-files", 0700); err != nil && !errors.Is(err, fs.ErrExist) {
		return nil, err
	}
	info, err := d.profile.Lstat("clipboard-files")
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, transfer.ErrPath
	}
	d.cache, err = d.profile.OpenRoot("clipboard-files")
	if err != nil {
		return nil, err
	}
	d.cacheInfo, err = d.cache.Stat(".")
	if err != nil || !os.SameFile(info, d.cacheInfo) {
		return nil, errors.Join(transfer.ErrPath, err)
	}
	if err = d.cache.Mkdir(id, 0700); err != nil {
		return nil, err
	}
	info, err = d.cache.Lstat(id)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, transfer.ErrPath
	}
	d.object, err = d.cache.OpenRoot(id)
	if err != nil {
		return nil, err
	}
	d.objectInfo, err = d.object.Stat(".")
	if err != nil || !os.SameFile(info, d.objectInfo) {
		return nil, errors.Join(transfer.ErrPath, err)
	}
	listing, err := d.object.Open(".")
	if err != nil {
		return nil, err
	}
	entries, readErr := listing.ReadDir(1)
	_ = listing.Close()
	if len(entries) != 0 || (readErr != nil && !errors.Is(readErr, io.EOF)) {
		return nil, errors.Join(transfer.ErrPath, readErr)
	}
	return d, nil
}

func (d *clipboardCacheDestination) Close() error {
	var err error
	for _, root := range []*os.Root{d.object, d.cache, d.profile} {
		if root != nil {
			err = errors.Join(err, root.Close())
		}
	}
	return err
}

// Only publish a path while it still resolves to our pinned directories.
// Receiver and cleanup retain their handles if the visible name is replaced.
func (d *clipboardCacheDestination) visible() error {
	profile, err := os.OpenRoot(d.profilePath)
	if err != nil {
		return err
	}
	defer profile.Close()
	info, err := profile.Stat(".")
	if err != nil || !os.SameFile(info, d.profileInfo) {
		return errors.Join(transfer.ErrPath, err)
	}
	info, err = profile.Lstat("clipboard-files")
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !os.SameFile(info, d.cacheInfo) {
		return errors.Join(transfer.ErrPath, err)
	}
	cache, err := profile.OpenRoot("clipboard-files")
	if err != nil {
		return err
	}
	defer cache.Close()
	info, err = cache.Lstat(d.id)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !os.SameFile(info, d.objectInfo) {
		return errors.Join(transfer.ErrPath, err)
	}
	return nil
}

func (d *clipboardCacheDestination) discardUnpublished() error {
	info, err := d.cache.Lstat(d.id)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !os.SameFile(info, d.objectInfo) {
		return transfer.ErrPath
	}
	// No ownership inventory exists at this boundary. Only remove an empty
	// directory; an unknown entry must never become recursive-delete authority.
	return d.cache.Remove(d.id)
}
