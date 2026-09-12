package transfer

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"strconv"
	"time"
)

const maxResumeStateBytes = 2 * MaxMetadata

const checkpointChunkBatch = 8
const checkpointMaxDelay = 500 * time.Millisecond

// ResumeState is atomically checkpointed only after file.Sync. Recovery rehashes
// every purportedly completed chunk and thus tolerates lost checkpoints and damage.
type ResumeState struct {
	Peer      string                  `json:"peer"`
	Manifest  Manifest                `json:"manifest"`
	Digest    string                  `json:"digest"`
	State     string                  `json:"state"`
	Verified  map[uint32][]bool       `json:"verified"`
	Committed map[uint32]CommitRecord `json:"committed,omitempty"`
}

type CommitRecord struct {
	FileID      uint32 `json:"file_id"`
	Path        string `json:"path"`
	Digest      string `json:"digest"`
	Size        int64  `json:"size"`
	CommittedAt string `json:"committed_at"`
}

type Receiver struct {
	root, stage           *os.Root
	State                 ResumeState
	files                 map[uint32]*os.File
	verifiedBytes         int64
	chunksSinceCheckpoint int
	lastCheckpoint        time.Time
}

func OpenReceiver(ctx context.Context, directory, peer string, m Manifest) (_ *Receiver, err error) {
	if err = m.Validate(); err != nil {
		return nil, err
	}
	if peer == "" {
		return nil, errors.New("AUTH_FAILED")
	}
	if err = os.MkdirAll(directory, 0700); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, err
	}
	r := &Receiver{root: root, files: make(map[uint32]*os.File), State: ResumeState{Peer: peer, Manifest: m, Digest: m.Digest(), State: "Recovering", Verified: make(map[uint32][]bool), Committed: make(map[uint32]CommitRecord)}}
	defer func() {
		if err != nil {
			_ = r.Close()
		}
	}()
	stageName := ".linksend-" + m.TransferID
	if st, e := root.Lstat(stageName); e == nil {
		if !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
			return nil, ErrPath
		}
	} else if !errors.Is(e, fs.ErrNotExist) {
		return nil, e
	} else if e = root.Mkdir(stageName, 0700); e != nil {
		return nil, e
	}
	r.stage, err = root.OpenRoot(stageName)
	if err != nil {
		return nil, err
	}
	if f, e := r.stage.Open("state.json"); e == nil {
		var saved ResumeState
		data, readErr := io.ReadAll(io.LimitReader(f, maxResumeStateBytes+1))
		_ = f.Close()
		if readErr != nil {
			return nil, readErr
		}
		if len(data) > maxResumeStateBytes {
			return nil, errors.New("resume state too large")
		}
		if err := json.Unmarshal(data, &saved); err != nil {
			return nil, err
		}
		if saved.Peer != peer || saved.Digest != m.Digest() || saved.Manifest.Digest() != m.Digest() || saved.Manifest.TransferID != m.TransferID || saved.Manifest.ChunkSize != m.ChunkSize {
			return nil, errors.New("RESUME_IDENTITY_MISMATCH")
		}
		if saved.State != "Recovering" && saved.State != "Transferring" && saved.State != "Verifying" && saved.State != "Completed" && saved.State != "Paused" && saved.State != "Cancelled" && saved.State != "Failed" {
			return nil, errors.New("INVALID_RESUME_STATE")
		}
		for id := range saved.Verified {
			if int(id) >= len(m.Files) || m.Files[id].Type != "file" || len(saved.Verified[id]) != len(m.Files[id].Chunks) {
				return nil, errors.New("INVALID_RESUME_STATE")
			}
		}
		for id, committed := range saved.Committed {
			if int(id) >= len(m.Files) || m.Files[id].Type != "file" || committed.FileID != id || committed.Path != m.Files[id].Path || committed.Digest != m.Files[id].Hash || committed.Size != m.Files[id].Size {
				return nil, errors.New("INVALID_COMMIT_RECORD")
			}
			r.State.Committed[id] = committed
		}
	} else if !errors.Is(e, fs.ErrNotExist) {
		return nil, e
	}
	buf := make([]byte, m.ChunkSize)
	for _, e := range m.Files {
		if e.Type != "file" {
			continue
		}
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		name := strconv.FormatUint(uint64(e.ID), 10) + ".part"
		// Never reopen a preexisting inode for writing: it may be a hard link.
		// Copy valid blocks into a fresh O_EXCL inode, then atomically replace only staging.
		token := make([]byte, 12)
		if _, err = rand.Read(token); err != nil {
			return nil, err
		}
		fresh := hex.EncodeToString(token) + ".new"
		committed := false
		defer func(name string, keep *bool) {
			if !*keep {
				_ = r.stage.Remove(name)
			}
		}(fresh, &committed)
		f, openErr := r.stage.OpenFile(fresh, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
		if openErr != nil {
			return nil, openErr
		}
		r.files[e.ID] = f
		bits := make([]bool, len(e.Chunks))
		r.State.Verified[e.ID] = bits
		if st, lerr := r.stage.Lstat(name); lerr == nil {
			if !st.Mode().IsRegular() {
				return nil, ErrPath
			}
			old, eopen := r.stage.Open(name)
			if eopen != nil {
				return nil, eopen
			}
			for i, h := range e.Chunks {
				if err = ctx.Err(); err != nil {
					_ = old.Close()
					return nil, err
				}
				off := int64(i) * int64(m.ChunkSize)
				n := min(int64(m.ChunkSize), e.Size-off)
				if count, readErr := old.ReadAt(buf[:n], off); readErr == nil && int64(count) == n && Sum(buf[:n]) == h {
					if _, err = f.WriteAt(buf[:n], off); err != nil {
						_ = old.Close()
						return nil, err
					}
					bits[i] = true
					r.verifiedBytes += n
				}
			}
			_ = old.Close()
		} else if !errors.Is(lerr, fs.ErrNotExist) {
			return nil, lerr
		}
		if err = f.Truncate(e.Size); err != nil {
			return nil, err
		}
		if err = f.Sync(); err != nil {
			return nil, err
		}
		if err = f.Close(); err != nil {
			return nil, err
		}
		delete(r.files, e.ID)
		if err = r.stage.Rename(fresh, name); err != nil {
			return nil, err
		}
		committed = true
		f, err = r.stage.OpenFile(name, os.O_RDWR, 0600)
		if err != nil {
			return nil, err
		}
		r.files[e.ID] = f
	}
	r.State.State = "Transferring"
	if err = r.checkpoint(); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *Receiver) checkpoint() error {
	b, err := json.Marshal(r.State)
	if err != nil {
		return err
	}
	f, err := r.stage.OpenFile("checkpoint.tmp", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if errors.Is(err, fs.ErrExist) {
		if err = r.stage.Remove("checkpoint.tmp"); err != nil {
			return err
		}
		f, err = r.stage.OpenFile("checkpoint.tmp", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	}
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = r.stage.Remove("checkpoint.tmp")
		}
	}()
	if _, err = f.Write(b); err != nil {
		_ = f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = r.stage.Rename("checkpoint.tmp", "state.json"); err != nil {
		return err
	}
	committed = true
	r.chunksSinceCheckpoint = 0
	r.lastCheckpoint = time.Now()
	return nil
}

func (r *Receiver) checkpointIfDue() error {
	if r.chunksSinceCheckpoint < checkpointChunkBatch && !r.lastCheckpoint.IsZero() && time.Since(r.lastCheckpoint) < checkpointMaxDelay {
		return nil
	}
	return r.checkpoint()
}

func (r *Receiver) WriteChunk(ctx context.Context, id uint32, index int, data []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if int(id) >= len(r.State.Manifest.Files) {
		return errors.New("INVALID_CHUNK")
	}
	e := r.State.Manifest.Files[id]
	if index < 0 || index >= len(e.Chunks) {
		return errors.New("INVALID_CHUNK")
	}
	off := int64(index) * int64(r.State.Manifest.ChunkSize)
	want := min(int64(r.State.Manifest.ChunkSize), e.Size-off)
	if int64(len(data)) != want || Sum(data) != e.Chunks[index] {
		return ErrIntegrity
	}
	f := r.files[id]
	if f == nil {
		return errors.New("CLOSED")
	}
	if _, err := f.WriteAt(data, off); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if !r.State.Verified[id][index] {
		r.State.Verified[id][index] = true
		r.verifiedBytes += want
		r.chunksSinceCheckpoint++
	}
	return r.checkpointIfDue()
}

func (r *Receiver) VerifiedBytes() int64 {
	return r.verifiedBytes
}

func (r *Receiver) verifyParents(p string) error {
	if p == "." {
		return nil
	}
	if err := r.verifyParents(path.Dir(p)); err != nil {
		return err
	}
	st, err := r.root.Lstat(p)
	if errors.Is(err, fs.ErrNotExist) {
		return r.root.Mkdir(p, 0700)
	}
	if err != nil {
		return err
	}
	if !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
		return ErrPath
	}
	return nil
}

func (r *Receiver) Finish(ctx context.Context) error {
	return r.FinishWithCommit(ctx, nil)
}

func (r *Receiver) FinishWithCommit(ctx context.Context, onCommit func(CommitRecord)) error {
	r.State.State = "Verifying"
	if err := r.checkpoint(); err != nil {
		return err
	}
	for _, e := range r.State.Manifest.Files {
		if err := ctx.Err(); err != nil {
			return err
		}
		if e.Type == "directory" {
			if err := r.verifyParents(e.Path); err != nil {
				return err
			}
			continue
		}
		for _, ok := range r.State.Verified[e.ID] {
			if !ok {
				return ErrIntegrity
			}
		}
		f := r.files[e.ID]
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			return err
		}
		hash, _, err := hashFile(ctx, f, r.State.Manifest.ChunkSize)
		if err != nil {
			return err
		}
		if hash != e.Hash {
			return ErrIntegrity
		}
		if err = r.verifyParents(path.Dir(e.Path)); err != nil {
			return err
		}
		stageName := ".linksend-" + r.State.Manifest.TransferID + "/" + strconv.FormatUint(uint64(e.ID), 10) + ".part"
		// Hard-link creation is atomic and cannot replace an existing target.
		// Unsupported filesystems fail explicitly, never fall back to overwrite.
		if err = r.root.Link(stageName, e.Path); err != nil {
			if errors.Is(err, fs.ErrExist) {
				st, statErr := r.root.Lstat(e.Path)
				if statErr != nil || !st.Mode().IsRegular() {
					return ErrConflict
				}
				// Resume after a crash may find an already committed, identical file.
				dest, openErr := r.root.Open(e.Path)
				if openErr != nil {
					return openErr
				}
				actual, _, hashErr := hashFile(ctx, dest, r.State.Manifest.ChunkSize)
				_ = dest.Close()
				if hashErr != nil || actual != e.Hash {
					return ErrConflict
				}
			} else {
				return fmt.Errorf("COMMIT_FAILED: %w", err)
			}
		}
		record := r.State.Committed[e.ID]
		if record.Digest == "" {
			record = CommitRecord{FileID: e.ID, Path: e.Path, Digest: e.Hash, Size: e.Size, CommittedAt: time.Now().UTC().Format(time.RFC3339Nano)}
		}
		r.State.Committed[e.ID] = record
		if err = r.checkpoint(); err != nil {
			return err
		}
		if onCommit != nil {
			onCommit(record)
		}
	}
	r.State.State = "Completed"
	return r.checkpoint()
}

func (r *Receiver) Mark(state string) error {
	if state != "Paused" && state != "Cancelled" && state != "Failed" {
		return errors.New("INVALID_STATE")
	}
	r.State.State = state
	return r.checkpoint()
}
func (r *Receiver) Close() error {
	var err error
	for id, f := range r.files {
		err = errors.Join(err, f.Close())
		delete(r.files, id)
	}
	if r.stage != nil {
		err = errors.Join(err, r.stage.Close())
		r.stage = nil
	}
	if r.root != nil {
		err = errors.Join(err, r.root.Close())
		r.root = nil
	}
	return err
}
