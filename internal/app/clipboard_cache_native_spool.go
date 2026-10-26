package app

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"

	"github.com/Wen5555/LinkSend/internal/clipboardnativecontract"
	"github.com/Wen5555/LinkSend/internal/transfer"
)

const cacheNativeSpoolDirectory = "clipboard-native-receipts-v1"

// The helper receives one already-created file handle, never this directory,
// a profile root, a DB handle, or an arbitrary destination path. Partial or
// invalid spool contents are retained and cannot authorize state changes.
func (s *Service) openCacheNativeSpool(id string, create bool) (*os.File, error) {
	if !validClipboardFileID(id, 16) {
		return nil, errClipboardCacheOwnership
	}
	root, err := os.OpenRoot(s.cfg.DataDir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	if create {
		if err = root.Mkdir(cacheNativeSpoolDirectory, 0700); err != nil && !errors.Is(err, fs.ErrExist) {
			return nil, err
		}
	}
	info, err := root.Lstat(cacheNativeSpoolDirectory)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.Join(errClipboardCacheOwnership, err)
	}
	spool, err := root.OpenRoot(cacheNativeSpoolDirectory)
	if err != nil {
		return nil, err
	}
	defer spool.Close()
	actual, err := spool.Stat(".")
	if err != nil || !os.SameFile(info, actual) {
		return nil, errors.Join(errClipboardCacheOwnership, err)
	}
	name := id + ".json"
	if create {
		return spool.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0600)
	}
	info, err = spool.Lstat(name)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() <= 0 || info.Size() > maxCacheNativeMessage {
		return nil, errors.Join(errClipboardCacheOwnership, err)
	}
	file, err := spool.Open(name)
	if err != nil {
		return nil, err
	}
	actual, err = file.Stat()
	if err != nil || !os.SameFile(info, actual) {
		_ = file.Close()
		return nil, errors.Join(errClipboardCacheOwnership, err)
	}
	return file, nil
}

func (s *Service) consumeCacheNativeSpool(ctx context.Context, id string, boot cacheNativeVerifiedBoot) (bool, error) {
	file, err := s.openCacheNativeSpool(id, false)
	if err != nil {
		return false, err
	}
	defer file.Close()
	body, err := io.ReadAll(io.LimitReader(file, maxCacheNativeMessage+1))
	if err != nil || len(body) > maxCacheNativeMessage {
		return false, errClipboardCacheOwnership
	}
	var receipt clipboardnativecontract.Receipt
	if clipboardnativecontract.Decode(body, &receipt) != nil || receipt.Binding.CacheID != id {
		return false, errClipboardCacheOwnership
	}
	var raw string
	if err = s.store.db.QueryRowContext(ctx, `SELECT CASE WHEN length(CAST(value AS BLOB))<=? THEN value ELSE '' END FROM metadata WHERE key=?`, 2*transfer.MaxMetadata, clipboardCacheKeyPrefix+id).Scan(&raw); err != nil {
		return false, err
	}
	return s.consumeCacheNativeReceipt(ctx, id, raw, body, boot)
}
