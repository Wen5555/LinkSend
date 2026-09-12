package transfer

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"sort"
	"strings"
	"time"
)

type DirectoryIdentity struct {
	Device uint64 `json:"device"`
	File   uint64 `json:"file"`
}

type DirectoryRecord struct {
	FileID   uint32            `json:"file_id"`
	Path     string            `json:"path"`
	Identity DirectoryIdentity `json:"identity"`
	Owned    bool              `json:"owned"`
}

func (r *Receiver) directoryIdentity(target string) (DirectoryIdentity, error) {
	if err := r.checkExistingParents(target); err != nil {
		return DirectoryIdentity{}, err
	}
	st, err := r.root.Lstat(target)
	if err != nil {
		return DirectoryIdentity{}, err
	}
	if !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
		return DirectoryIdentity{}, ErrPath
	}
	f, err := r.root.Open(target)
	if err != nil {
		return DirectoryIdentity{}, err
	}
	defer f.Close()
	return directoryFileIdentity(f)
}

func (r *Receiver) verifyDirectoryRecord(record DirectoryRecord) error {
	current, err := r.directoryIdentity(record.Path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("%w: saved directory unavailable: %w", ErrConflict, err)
		}
		return err
	}
	if current != record.Identity {
		return ErrConflict
	}
	return nil
}

func (r *Receiver) rebuildSelection() {
	r.selected = make(map[uint32]PlannedEntry, len(r.plan.Entries))
	for _, e := range r.plan.Entries {
		r.selected[e.FileID] = e
	}
}

func (r *Receiver) Plan() ReceivePlan { return clonePlan(r.plan) }

// RemainingBytes is the selected content not yet recoverably verified. Commit
// uses hard links (zero additional file-body copies); directory/inode overhead
// and the earlier defensive staging-copy peak are not a free-space guarantee.
func (r *Receiver) Summary() PlanSummary {
	s := r.plan.Summary(r.State.Manifest)
	s.RemainingBytes = max(0, s.SelectedTotal-r.VerifiedBytes())
	return s
}

func (r *Receiver) validCommitRecord(id uint32, record CommitRecord) bool {
	p, selected := r.selected[id]
	if !selected || uint64(id) >= uint64(len(r.State.Manifest.Files)) {
		return false
	}
	e := r.State.Manifest.Files[id]
	return e.Type == "file" && record.FileID == id && record.Path == p.Path && record.Digest == e.Hash && record.Size == e.Size
}

func (r *Receiver) checkExistingParents(p string) error {
	for parent := path.Dir(p); parent != "."; parent = path.Dir(parent) {
		st, err := r.root.Lstat(parent)
		if err != nil {
			return err
		}
		if !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
			return ErrPath
		}
	}
	return nil
}

func (r *Receiver) verifyCommittedFile(ctx context.Context, e FileEntry, target string) error {
	if err := r.checkExistingParents(target); err != nil {
		return err
	}
	st, err := r.root.Lstat(target)
	if errors.Is(err, fs.ErrNotExist) {
		return ErrConflict
	}
	if err != nil {
		return err
	}
	if !st.Mode().IsRegular() {
		return ErrPath
	}
	f, err := r.root.Open(target)
	if err != nil {
		return err
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil {
		return err
	}
	if !opened.Mode().IsRegular() || opened.Size() != e.Size {
		return ErrIntegrity
	}
	digest, _, err := hashFile(ctx, f, r.State.Manifest.ChunkSize)
	if err != nil {
		return err
	}
	if digest != e.Hash {
		return ErrIntegrity
	}
	return nil
}

// A matching hash alone never proves ownership. For an interrupted Link before
// the commit checkpoint, both names must still identify the same inode/file ID.
func (r *Receiver) restoreCommitted(ctx context.Context, e FileEntry, part string) (bool, error) {
	record, committed := r.State.Committed[e.ID]
	_, hasIntent := r.State.Intents[e.ID]
	legacyInterrupted := r.State.Plan == nil && r.resumedCommit
	if !committed && !hasIntent && !legacyInterrupted {
		return false, nil
	}
	if !committed {
		staged, err := r.stage.Lstat(part)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return false, nil
			}
			return false, err
		}
		if !staged.Mode().IsRegular() {
			return false, ErrPath
		}
		target := r.selected[e.ID].Path
		if err = r.checkExistingParents(target); err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return false, nil
			}
			return false, err
		}
		dest, err := r.root.Open(target)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return false, nil
			}
			return false, err
		}
		targetInfo, statErr := dest.Stat()
		_ = dest.Close()
		if statErr != nil {
			return false, statErr
		}
		if !targetInfo.Mode().IsRegular() || !os.SameFile(staged, targetInfo) {
			return false, nil
		}
		record = CommitRecord{FileID: e.ID, Path: target, Digest: e.Hash, Size: e.Size, CommittedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	}
	if err := r.verifyCommittedFile(ctx, e, record.Path); err != nil {
		return false, err
	}
	r.State.Committed[e.ID] = record
	delete(r.State.Intents, e.ID)
	bits := make([]bool, len(e.Chunks))
	for i := range bits {
		bits[i] = true
	}
	r.State.Verified[e.ID] = bits
	r.verifiedBytes += e.Size
	return true, nil
}

func (r *Receiver) persistPlan() error {
	p := clonePlan(r.plan)
	r.State.Plan = &p
	r.rebuildSelection()
	if err := r.checkpoint(); err != nil {
		return fmt.Errorf("%w: %w", ErrPlanPersistence, err)
	}
	if r.planChanged != nil {
		if err := r.planChanged(clonePlan(r.plan)); err != nil {
			return fmt.Errorf("%w: %w", ErrPlanPersistence, err)
		}
	}
	return nil
}

func (r *Receiver) renamePlannedEntry(id uint32) error {
	entry := r.selected[id]
	if entry.Policy != ConflictKeepBoth {
		return ErrConflict
	}
	source := r.State.Manifest.Files[id]
	base := path.Join(path.Dir(entry.Path), path.Base(source.Path))
	used := make(map[string]bool, len(r.plan.Entries))
	for _, p := range r.plan.Entries {
		if p.FileID != id {
			used[portablePathKey(p.Path)] = true
		}
	}
	newPath, err := r.inventory.available(base, used)
	if err != nil {
		return err
	}
	oldPath := entry.Path
	next := clonePlan(r.plan)
	var changed []uint32
	for i, p := range next.Entries {
		if p.FileID == id || (source.Type == "directory" && strings.HasPrefix(p.Path, oldPath+"/")) {
			if _, committed := r.State.Committed[p.FileID]; committed {
				return ErrConflict
			}
			if _, recorded := r.State.Directories[p.FileID]; recorded {
				return ErrConflict
			}
			if p.FileID == id {
				next.Entries[i].Path = newPath
			} else {
				next.Entries[i].Path = newPath + strings.TrimPrefix(p.Path, oldPath)
			}
			changed = append(changed, p.FileID)
		}
	}
	if err = next.Validate(r.State.Manifest); err != nil {
		return err
	}
	r.plan = next
	for _, id := range changed {
		delete(r.State.Intents, id)
		delete(r.State.DirectoryIntents, id)
	}
	return r.persistPlan()
}

func (r *Receiver) commitDirectories(ctx context.Context) error {
	directories := make([]uint32, 0)
	for _, p := range r.plan.Entries {
		if r.State.Manifest.Files[p.FileID].Type == "directory" {
			directories = append(directories, p.FileID)
		}
	}
	sort.Slice(directories, func(i, j int) bool {
		pi, pj := r.selected[directories[i]].Path, r.selected[directories[j]].Path
		if di, dj := strings.Count(pi, "/"), strings.Count(pj, "/"); di != dj {
			return di < dj
		}
		return pi < pj
	})
	for _, id := range directories {
		if err := ctx.Err(); err != nil {
			return err
		}
		if record, ok := r.State.Directories[id]; ok {
			if err := r.verifyDirectoryRecord(record); err != nil {
				return err
			}
			continue
		}
		if err := r.commitDirectory(ctx, id); err != nil {
			return err
		}
	}
	return nil
}

func (r *Receiver) commitDirectory(ctx context.Context, id uint32) error {
	for attempt := 0; attempt <= maxKeepBothNames; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		entry := r.selected[id]
		r.inventory.refresh(path.Dir(entry.Path))
		st, err := r.inventory.lookup(entry.Path)
		if err != nil {
			return err
		}
		owned := false
		if st != nil {
			// A persisted mkdir intention is not proof that mkdir ran. Without
			// its directory identity record, stop rather than merge/duplicate.
			if _, uncertain := r.State.DirectoryIntents[id]; uncertain {
				return ErrConflict
			}
			if entry.Policy == ConflictKeepBoth {
				if err = r.renamePlannedEntry(id); err != nil {
					return err
				}
				continue
			}
			if st.Name() != path.Base(entry.Path) {
				return ErrConflict
			}
			if st.Mode()&os.ModeSymlink != 0 {
				return ErrPath
			}
			if !st.IsDir() {
				return ErrConflict
			}
		} else {
			if err = r.verifyParents(path.Dir(entry.Path)); err != nil {
				return err
			}
			r.State.DirectoryIntents[id] = entry.Path
			if err = r.checkpoint(); err != nil {
				return fmt.Errorf("%w: %w", ErrPlanPersistence, err)
			}
			if err = r.root.Mkdir(entry.Path, 0700); err != nil {
				if errors.Is(err, fs.ErrExist) && entry.Policy == ConflictKeepBoth {
					if err = r.renamePlannedEntry(id); err != nil {
						return err
					}
					continue
				}
				if errors.Is(err, fs.ErrExist) {
					return ErrConflict
				}
				return err
			}
			owned = true
		}
		identity, err := r.directoryIdentity(entry.Path)
		if err != nil {
			return err
		}
		r.State.Directories[id] = DirectoryRecord{FileID: id, Path: entry.Path, Identity: identity, Owned: owned}
		delete(r.State.DirectoryIntents, id)
		if err = r.checkpoint(); err != nil {
			return fmt.Errorf("%w: %w", ErrPlanPersistence, err)
		}
		return nil
	}
	return ErrConflict
}

func (r *Receiver) commitFile(ctx context.Context, e FileEntry, stageName string) error {
	for attempt := 0; attempt <= maxKeepBothNames; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		entry := r.selected[e.ID]
		r.inventory.refresh(path.Dir(entry.Path))
		st, err := r.inventory.lookup(entry.Path)
		if err != nil {
			return err
		}
		if st != nil {
			if err = r.renamePlannedEntry(e.ID); err != nil {
				return err
			}
			entry = r.selected[e.ID]
		}
		// Checkpoint intent precedes no-replace Link. A subsequent crash can
		// recover ownership through SameFile, without accepting a foreign copy.
		r.State.Intents[e.ID] = CommitRecord{FileID: e.ID, Path: entry.Path, Digest: e.Hash, Size: e.Size}
		if err = r.checkpoint(); err != nil {
			return fmt.Errorf("%w: %w", ErrPlanPersistence, err)
		}
		if err = r.root.Link(stageName, entry.Path); err == nil {
			return nil
		}
		if !errors.Is(err, fs.ErrExist) {
			return fmt.Errorf("COMMIT_FAILED: %w", err)
		}
		delete(r.inventory.directories, path.Dir(entry.Path))
		if err = r.renamePlannedEntry(e.ID); err != nil {
			return err
		}
	}
	return ErrConflict
}
