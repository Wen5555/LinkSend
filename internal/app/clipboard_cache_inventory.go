package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/Wen5555/LinkSend/internal/transfer"
)

// Bound metadata and per-object I/O. Larger objects remain protected until a
// future incremental inventory can prove their full provenance within budget.
const maxClipboardCacheNodes = 1024

type clipboardCacheNode struct {
	Identity  string `json:"identity"`
	Directory bool   `json:"directory,omitempty"`
	Size      int64  `json:"size,omitempty"`
	Modified  int64  `json:"modified,omitempty"`
}

func cacheNode(file *os.File) (clipboardCacheNode, error) {
	info, err := file.Stat()
	if err != nil {
		return clipboardCacheNode{}, err
	}
	if !info.IsDir() && !info.Mode().IsRegular() {
		return clipboardCacheNode{}, errClipboardCacheOwnership
	}
	id, err := inboxFileIdentity(file)
	node := clipboardCacheNode{Identity: id, Directory: info.IsDir()}
	if !info.IsDir() {
		node.Size, node.Modified = info.Size(), info.ModTime().UnixNano()
	}
	return node, err
}

func validateClipboardCacheInventory(record clipboardCacheRecord) error {
	if len(record.Cleanup) > maxClipboardCacheNodes || (!record.Sealed && len(record.Cleanup) != 0) {
		return errClipboardCacheOwnership
	}
	if (record.State == "reclaimable" || record.State == "deleting") && !record.Sealed {
		return errClipboardCacheOwnership
	}
	for name, node := range record.Cleanup {
		stage := ".linksend-" + record.ID
		validPath := transfer.ValidatePath(name) == nil
		if name == stage {
			validPath = node.Directory && node.Identity == record.Stage
		}
		if strings.HasPrefix(name, stage+"/") {
			part := strings.TrimPrefix(name, stage+"/")
			validPath = !node.Directory && (part == "state.json" || record.Parts[part] == node.Identity)
		}
		if !validPath || name == "." || node.Identity == "" || node.Size < 0 || (!node.Directory && node.Modified == 0) {
			return errClipboardCacheOwnership
		}
		for parent := path.Dir(name); parent != "."; parent = path.Dir(parent) {
			if !record.Cleanup[parent].Directory {
				return errClipboardCacheOwnership
			}
		}
	}
	return nil
}

// This is called while the engine still holds its real storage handles, then
// used only after ReceiveWithOptions has closed every one of those handles.
func (r *clipboardCacheReceipt) captureClosingStorage(storage transfer.ReceiveStorage) error {
	if r.record.State != "receiving" || storage.Manifest.Digest() != r.record.Digest || storage.Peer != r.record.Peer {
		return errClipboardCacheOwnership
	}
	rootID, err := clipboardRootIdentity(storage.Root)
	if err != nil || rootID != r.record.Directory {
		return errors.Join(errClipboardCacheOwnership, err)
	}
	stageID, err := clipboardRootIdentity(storage.Stage)
	if err != nil || stageID != r.record.Stage {
		return errors.Join(errClipboardCacheOwnership, err)
	}
	if storage.DirectoryLimit {
		return nil
	}
	if storage.Checkpoint == nil {
		return errClipboardCacheOwnership
	}
	// Cleanup is optional to reception. Do not impose a smaller product file
	// count than the protocol supports merely to keep a collector pass bounded.
	// Without a complete seal, these larger objects can never be reclaimed.
	if 2+len(storage.Directories)+2*len(storage.Files) > maxClipboardCacheNodes {
		return nil
	}
	nodes := make(map[string]clipboardCacheNode)
	stageName := ".linksend-" + r.record.ID
	nodes[stageName] = clipboardCacheNode{Identity: stageID, Directory: true}
	add := func(name string, file *os.File) error {
		if file == nil || len(nodes) >= maxClipboardCacheNodes {
			return errClipboardCacheOwnership
		}
		node, err := cacheNode(file)
		if err != nil {
			return err
		}
		nodes[name] = node
		return nil
	}
	if err := add(stageName+"/state.json", storage.Checkpoint); err != nil {
		return err
	}
	for id, file := range storage.Files {
		name := fmt.Sprintf("%d.part", id)
		if err := add(stageName+"/"+name, file); err != nil {
			return err
		}
		if nodes[stageName+"/"+name].Identity != r.record.Parts[name] {
			return errClipboardCacheOwnership
		}
	}
	for name, file := range storage.Directories {
		if err := add(name, file); err != nil {
			return err
		}
		if !nodes[name].Directory {
			return errClipboardCacheOwnership
		}
	}
	// Committed files are hard links of the engine's actual parts, never an
	// arbitrary pre-existing pathname that happens to match the manifest name.
	for _, entry := range storage.Plan.Entries {
		if storage.Manifest.Files[entry.FileID].Type != "file" {
			continue
		}
		info, err := storage.Root.Lstat(entry.Path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil || !info.Mode().IsRegular() {
			return errors.Join(errClipboardCacheOwnership, err)
		}
		file, err := storage.Root.Open(entry.Path)
		if err != nil {
			return err
		}
		err = add(entry.Path, file)
		_ = file.Close()
		if err != nil {
			return err
		}
		if nodes[entry.Path].Identity != r.record.Parts[fmt.Sprintf("%d.part", entry.FileID)] {
			return errClipboardCacheOwnership
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := checkClipboardCacheTree(ctx, storage.Root, nodes, false); err != nil {
		return err
	}
	r.finalInventory = nodes
	return nil
}

// Each parent is opened without following symlinks and kept pinned while its
// children are checked. Unknown names, links, replaced or changed files protect
// the entire object before any removal starts. No recursive RemoveAll is used.
func checkClipboardCacheTree(ctx context.Context, root *os.Root, nodes map[string]clipboardCacheNode, missingOK bool) error {
	seen := make(map[string]bool)
	remaining := maxClipboardCacheNodes + 1
	var walk func(*os.Root, string) error
	walk = func(dir *os.Root, prefix string) error {
		listing, err := dir.Open(".")
		if err != nil {
			return err
		}
		defer listing.Close()
		for {
			if err := ctx.Err(); err != nil {
				return err
			}
			entries, readErr := listing.ReadDir(64)
			for _, entry := range entries {
				remaining--
				if remaining <= 0 {
					return errClipboardCacheOwnership
				}
				name := path.Join(prefix, entry.Name())
				expected, ok := nodes[name]
				if !ok {
					return errClipboardCacheOwnership
				}
				if err := checkCacheNode(dir, entry.Name(), expected); err != nil {
					return err
				}
				seen[name] = true
				if expected.Directory {
					child, err := dir.OpenRoot(entry.Name())
					if err != nil {
						return err
					}
					id, err := clipboardRootIdentity(child)
					if err == nil && id != expected.Identity {
						err = errClipboardCacheOwnership
					}
					if err == nil {
						err = walk(child, name)
					}
					_ = child.Close()
					if err != nil {
						return err
					}
				}
			}
			if errors.Is(readErr, io.EOF) {
				break
			}
			if readErr != nil {
				return readErr
			}
		}
		return nil
	}
	if err := walk(root, ""); err != nil {
		return err
	}
	if !missingOK && len(seen) != len(nodes) {
		return errClipboardCacheOwnership
	}
	return nil
}

func checkCacheNode(root *os.Root, name string, expected clipboardCacheNode) error {
	info, err := root.Lstat(name)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || expected.Directory != info.IsDir() || (!expected.Directory && !info.Mode().IsRegular()) {
		return errClipboardCacheOwnership
	}
	file, err := root.Open(name)
	if err != nil {
		return err
	}
	defer file.Close()
	actual, err := cacheNode(file)
	opened, statErr := file.Stat()
	if err != nil || statErr != nil || !os.SameFile(info, opened) || actual != expected {
		return errors.Join(errClipboardCacheOwnership, err, statErr)
	}
	return nil
}

// A bounded, signed deletion inventory survives a crash partway through
// unlinking. Missing registered names are fine; newly appearing names are not.
func removeClipboardCacheTree(ctx context.Context, root *os.Root, nodes map[string]clipboardCacheNode) error {
	if err := checkClipboardCacheTree(ctx, root, nodes, true); err != nil {
		return err
	}
	names := make([]string, 0, len(nodes))
	for name := range nodes {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		if a, b := strings.Count(names[i], "/"), strings.Count(names[j], "/"); a != b {
			return a > b
		}
		return names[i] < names[j]
	})
	for _, name := range names {
		if err := ctx.Err(); err != nil {
			return err
		}
		parent, err := openCacheParent(root, path.Dir(name), nodes)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		err = checkCacheNode(parent, path.Base(name), nodes[name])
		if err == nil {
			err = parent.Remove(path.Base(name))
		}
		_ = parent.Close()
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

func openCacheParent(root *os.Root, name string, nodes map[string]clipboardCacheNode) (*os.Root, error) {
	current, err := root.OpenRoot(".")
	if err != nil {
		return nil, err
	}
	if name == "." {
		return current, nil
	}
	prefix := ""
	for _, component := range strings.Split(name, "/") {
		prefix = path.Join(prefix, component)
		if err = checkCacheNode(current, component, nodes[prefix]); err != nil {
			_ = current.Close()
			return nil, err
		}
		next, e := current.OpenRoot(component)
		_ = current.Close()
		if e != nil {
			return nil, e
		}
		current = next
		id, e := clipboardRootIdentity(current)
		if e != nil || id != nodes[prefix].Identity {
			_ = current.Close()
			return nil, errors.Join(errClipboardCacheOwnership, e)
		}
	}
	return current, nil
}

func (r *clipboardCacheReceipt) discardUnpublished(ctx context.Context) error {
	if r.nativeStarted || r.record.Native != nil || r.record.Run != r.service.epoch {
		return errClipboardCacheOwnership
	}
	switch r.record.State {
	case "receiving", "verified", "publishing", "abandoned":
	default:
		return errClipboardCacheOwnership
	}
	if r.finalInventory == nil {
		// Failure before engine provenance: only a genuinely empty object is safe.
		if err := checkClipboardCacheTree(ctx, r.destination.object, nil, false); err != nil {
			return err
		}
	}
	next := r.record
	next.State, next.Sealed, next.Cleanup, next.Revision = "reclaimable", true, r.finalInventory, next.Revision+1
	if err := r.save(next); err != nil {
		return err
	}
	next = r.record
	next.State, next.Revision = "deleting", next.Revision+1
	if err := r.save(next); err != nil {
		return err
	}
	if err := removeClipboardCacheTree(ctx, r.destination.object, r.record.Cleanup); err != nil {
		return err
	}
	return r.destination.discardUnpublished()
}
