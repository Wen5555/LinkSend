package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/Wen5555/LinkSend/internal/content"
	"github.com/Wen5555/LinkSend/internal/transfer"
)

type ContentTaskInfo struct {
	TaskID     string       `json:"task_id"`
	Kind       content.Kind `json:"kind"`
	Size       int64        `json:"size"`
	Width      int          `json:"width,omitempty"`
	Height     int          `json:"height,omitempty"`
	Mode       string       `json:"mode"`
	Available  bool         `json:"available"`
	CanPreview bool         `json:"can_preview"`
	CanCopy    bool         `json:"can_copy"`
	CanOpen    bool         `json:"can_open"`
	CanSave    bool         `json:"can_save"`
}
type ContentActionResult struct {
	TaskID string `json:"task_id"`
	Action string `json:"action"`
	State  string `json:"state"`
}

type contentTaskRecord struct {
	taskID, direction, snapshotID, binding, mode string
	snapshot                                     *content.Snapshot
	descriptor                                   *transfer.ContentDescriptor
	allowFallback                                bool
}

func (s *Service) loadContentTask(ctx context.Context, id string) (contentTaskRecord, error) {
	if !validInboxID(id) {
		return contentTaskRecord{}, ErrMetadataInvalid
	}
	record := contentTaskRecord{taskID: id}
	var snapshotData, descriptorData []byte
	err := s.store.db.QueryRowContext(ctx, `SELECT direction,snapshot_id,snapshot,descriptor,binding,mode,allow_file_fallback FROM content_tasks WHERE task_id=?`, id).Scan(&record.direction, &record.snapshotID, &snapshotData, &descriptorData, &record.binding, &record.mode, &record.allowFallback)
	if err != nil {
		return record, err
	}
	if len(snapshotData) > 4096 || len(descriptorData) > 4096 {
		return record, ErrMetadataInvalid
	}
	if len(snapshotData) > 0 {
		if err = json.Unmarshal(snapshotData, &record.snapshot); err != nil {
			return record, err
		}
	}
	if len(descriptorData) > 0 {
		if err = json.Unmarshal(descriptorData, &record.descriptor); err != nil {
			return record, err
		}
	}
	return record, nil
}

func (s *Service) ContentTask(ctx context.Context, id string) (ContentTaskInfo, error) {
	done, err := s.beginWorkspaceWork()
	if err != nil {
		return ContentTaskInfo{}, err
	}
	defer done()
	info := ContentTaskInfo{TaskID: id}
	record, err := s.loadContentTask(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return info, nil
	}
	if err != nil {
		return info, err
	}
	info.Mode = record.mode
	if record.snapshot != nil {
		info.Kind = record.snapshot.Kind
		info.Size = record.snapshot.Size
		info.Width = record.snapshot.Width
		info.Height = record.snapshot.Height
	}
	if record.descriptor != nil {
		info.Kind = record.descriptor.Kind
		info.Size = record.descriptor.Size
		info.Width = record.descriptor.Width
		info.Height = record.descriptor.Height
	}
	if record.direction != "receive" || record.mode != "native" {
		return info, nil
	}
	body, descriptor, err := s.receivedContentBody(ctx, id)
	if errors.Is(err, ErrInboxFileUnavailable) || errors.Is(err, ErrInboxFileMissing) || errors.Is(err, ErrInboxFileChanged) || errors.Is(err, os.ErrNotExist) {
		return info, nil
	}
	if err != nil {
		return info, err
	}
	info.Available = true
	info.CanPreview = descriptor.Kind == content.Text || descriptor.Kind == content.URL
	info.CanCopy = info.CanPreview
	info.CanOpen = descriptor.Kind == content.URL && content.ValidateURL(string(body)) == nil
	info.CanSave = descriptor.Kind == content.Image
	return info, nil
}

// The body is returned only within Go to native action callbacks. Every action
// checks the actual completed task, manifest binding and current file digest;
// neither frontend paths nor a task's historical label authorise filesystem IO.
func (s *Service) receivedContentBody(ctx context.Context, id string) ([]byte, transfer.ContentDescriptor, error) {
	var empty transfer.ContentDescriptor
	record, err := s.loadContentTask(ctx, id)
	if err != nil {
		return nil, empty, err
	}
	if record.direction != "receive" || record.mode != "native" || record.descriptor == nil {
		return nil, empty, ErrInboxFileUnavailable
	}
	snap, recovery, err := s.loadInboxTask(ctx, id)
	if err != nil {
		return nil, empty, err
	}
	if snap.State != "completed" || !snap.BilateralConfirmed || snap.Direction != "receive" {
		return nil, empty, ErrInboxFileUnavailable
	}
	var manifestData []byte
	if err = s.store.db.QueryRowContext(ctx, `SELECT manifest FROM inbox_manifests WHERE task_id=?`, id).Scan(&manifestData); err != nil {
		return nil, empty, err
	}
	if len(manifestData) > transfer.MaxMetadata {
		return nil, empty, ErrMetadataInvalid
	}
	var manifest transfer.Manifest
	if err = json.Unmarshal(manifestData, &manifest); err != nil {
		return nil, empty, err
	}
	descriptor := *record.descriptor
	if err = descriptor.Validate(manifest); err != nil {
		return nil, empty, err
	}
	if record.binding != descriptor.BindingDigest(manifest) || snap.ManifestDigest != manifest.Digest() {
		return nil, empty, transfer.ErrContentMismatch
	}
	location, err := s.ResolveInboxFile(ctx, id, descriptor.EntryID)
	if err != nil {
		return nil, empty, err
	}
	root, err := os.OpenRoot(recovery.TargetDirectory)
	if err != nil {
		return nil, empty, err
	}
	defer root.Close()
	info, err := root.Lstat(location.Name)
	if err != nil {
		return nil, empty, err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return nil, empty, ErrInboxOwnership
	}
	f, err := root.Open(location.Name)
	if err != nil {
		return nil, empty, err
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil {
		return nil, empty, err
	}
	if !os.SameFile(info, opened) || opened.Size() != descriptor.Size {
		return nil, empty, ErrInboxFileChanged
	}
	body, err := io.ReadAll(io.LimitReader(f, descriptor.Size+1))
	if err != nil {
		return nil, empty, err
	}
	if int64(len(body)) != descriptor.Size || transfer.Sum(body) != descriptor.Digest {
		return nil, empty, ErrInboxFileChanged
	}
	if descriptor.Kind != content.Image && (!utf8.Valid(body) || strings.ContainsRune(string(body), 0)) {
		return nil, empty, transfer.ErrContentInvalid
	}
	if err = ctx.Err(); err != nil {
		return nil, empty, err
	}
	return body, descriptor, nil
}

func (s *Service) PreviewReceivedText(ctx context.Context, id string, show func(string) error) (ContentActionResult, error) {
	return s.receivedTextAction(ctx, id, "preview", show)
}
func (s *Service) CopyReceivedText(ctx context.Context, id string, copyText func(string) error) (ContentActionResult, error) {
	return s.receivedTextAction(ctx, id, "copy", copyText)
}
func (s *Service) OpenReceivedURL(ctx context.Context, id string, openURL func(string) error) (ContentActionResult, error) {
	return s.receivedTextAction(ctx, id, "open_url", openURL)
}
func (s *Service) receivedTextAction(ctx context.Context, id, action string, perform func(string) error) (ContentActionResult, error) {
	result := ContentActionResult{TaskID: id, Action: action}
	done, err := s.beginWorkspaceWork()
	if err != nil {
		return result, err
	}
	defer done()
	if !s.content.enabled.Load() || perform == nil {
		return result, errors.New("CONTENT_NATIVE_ACTIONS_UNAVAILABLE")
	}
	body, descriptor, err := s.receivedContentBody(ctx, id)
	if err != nil {
		return result, err
	}
	if descriptor.Kind != content.Text && descriptor.Kind != content.URL {
		return result, ErrInboxFileUnavailable
	}
	value := string(body)
	if action == "open_url" {
		if descriptor.Kind != content.URL {
			return result, content.ErrInvalidURL
		}
		if err = content.ValidateURL(value); err != nil {
			return result, err
		}
	}
	if action == "preview" {
		runes := []rune(value)
		if len(runes) > 4096 {
			value = string(runes[:4096]) + "\n…（仅预览前4096字符）"
		}
	}
	if err = perform(value); err != nil {
		return result, err
	}
	result.State = "completed"
	if action == "preview" || action == "open_url" {
		result.State = "submitted"
	}
	return result, nil
}

// Destination comes only from a native save dialog in the desktop boundary.
// O_EXCL and same-file cleanup never overwrite or delete an existing user file.
func (s *Service) SaveReceivedImage(ctx context.Context, id, destination string) (ContentActionResult, error) {
	result := ContentActionResult{TaskID: id, Action: "save_image"}
	done, err := s.beginWorkspaceWork()
	if err != nil {
		return result, err
	}
	defer done()
	if !s.content.enabled.Load() {
		return result, errors.New("CONTENT_NATIVE_ACTIONS_UNAVAILABLE")
	}
	if !filepath.IsAbs(destination) || strings.ToLower(filepath.Ext(destination)) != ".png" {
		return result, ErrMetadataInvalid
	}
	body, descriptor, err := s.receivedContentBody(ctx, id)
	if err != nil {
		return result, err
	}
	if descriptor.Kind != content.Image {
		return result, ErrInboxFileUnavailable
	}
	root, err := os.OpenRoot(filepath.Dir(destination))
	if err != nil {
		return result, err
	}
	defer root.Close()
	name := filepath.Base(destination)
	if transfer.ValidatePath(name) != nil {
		return result, ErrMetadataInvalid
	}
	f, err := root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return result, err
	}
	owned, statErr := f.Stat()
	if statErr != nil {
		_ = f.Close()
		return result, statErr
	}
	if _, err = f.Write(body); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	err = errors.Join(err, closeErr, ctx.Err())
	if err != nil {
		if current, e := root.Lstat(name); e == nil && os.SameFile(owned, current) {
			_ = root.Remove(name)
		}
		return result, err
	}
	result.State = "completed"
	return result, nil
}
