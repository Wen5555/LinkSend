package transfer

import (
	"bytes"
	"errors"
	"io"
	"os"
)

// Do not infer creation from a directory name or persisted mkdir intention.
// Only the live receiver's owned directory records supply this provenance.
func (r *Receiver) observeClosingStorage(observe func(ReceiveStorage) error) error {
	files := make(map[uint32]*os.File, len(r.files))
	for id, file := range r.files {
		files[id] = file
	}
	if len(r.State.Directories) > 64 {
		return observe(ReceiveStorage{Root: r.root, Stage: r.stage, Files: files,
			Manifest: cloneManifest(r.State.Manifest), Plan: r.Plan(), Peer: r.State.Peer, DirectoryLimit: true})
	}
	directories := make(map[string]*os.File)
	defer func() {
		for _, f := range directories {
			_ = f.Close()
		}
	}()
	for _, record := range r.State.Directories {
		if !record.Owned {
			continue
		}
		if err := r.verifyDirectoryRecord(record); err != nil {
			return err
		}
		f, err := r.root.Open(record.Path)
		if err != nil {
			return err
		}
		identity, err := directoryFileIdentity(f)
		if err != nil || identity != record.Identity {
			_ = f.Close()
			return errors.Join(ErrPath, err)
		}
		directories[record.Path] = f
	}
	checkpoint, err := r.stage.Open("state.json")
	if err != nil {
		return err
	}
	defer checkpoint.Close()
	info, err := checkpoint.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxResumeStateBytes {
		return errors.Join(ErrPath, err)
	}
	data, err := io.ReadAll(io.LimitReader(checkpoint, maxResumeStateBytes+1))
	if err != nil || len(r.checkpointData) == 0 || !bytes.Equal(data, r.checkpointData) {
		return errors.Join(ErrPlanPersistence, err)
	}
	return observe(ReceiveStorage{Root: r.root, Stage: r.stage, Files: files,
		Manifest: cloneManifest(r.State.Manifest), Plan: r.Plan(), Peer: r.State.Peer,
		Directories: directories, Checkpoint: checkpoint})
}
