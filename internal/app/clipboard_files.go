package app

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/Wen5555/LinkSend/internal/clipboardsync"
	"github.com/Wen5555/LinkSend/internal/protocol"
	"github.com/Wen5555/LinkSend/internal/transfer"
	"github.com/Wen5555/LinkSend/internal/transport"
)

const ClipboardFilePopupThreshold = 32 << 20

var clipboardFileMagic = []byte{'L', 'S', 'C', 'F', '0', '1', 0, 0}

type ClipboardFileProgress struct {
	ID        string `json:"id"`
	PeerID    string `json:"peer_id"`
	Direction string `json:"direction"`
	State     string `json:"state"`
	Total     int64  `json:"total"`
	Verified  int64  `json:"verified"`
	FileCount int    `json:"file_count"`
	Large     bool   `json:"large"`
	Error     string `json:"error,omitempty"`
}

type clipboardFileOffer struct {
	ID    string `json:"id"`
	Count int    `json:"count"`
	Bytes int64  `json:"bytes"`
}

type clipboardFileJob struct {
	peer         *PeerSession
	event        clipboardsync.Event
	lease        clipboardsync.Lease
	reservation  *clipboardsync.FileReservation
	paths        []string
	progress     ClipboardFileProgress
	ctx          context.Context
	cancel       context.CancelFunc
	ready        chan struct{}
	started      bool
	localFailure bool // Rejection precedes peer cancellation/connection teardown.
}

func validClipboardFileID(id string, size int) bool {
	b, err := hex.DecodeString(id)
	return err == nil && len(b) == size && strings.ToLower(id) == id
}

func clipboardFileKey(peer *PeerSession, id string) string {
	return peer.PeerID + "/" + peer.SessionID + "/" + id
}

func (s *Service) ClipboardFileProgresses() []ClipboardFileProgress {
	s.clipboardFilesMu.Lock()
	defer s.clipboardFilesMu.Unlock()
	out := make([]ClipboardFileProgress, 0, len(s.clipboardFileJobs))
	for _, job := range s.clipboardFileJobs {
		out = append(out, job.progress)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (s *Service) CancelClipboardFile(id string) {
	s.clipboardFilesMu.Lock()
	var cancelled []*clipboardFileJob
	for _, job := range s.clipboardFileJobs {
		if job.progress.ID == id && job.progress.State != "completed" && job.progress.State != "cancelled" {
			job.progress.State = "cancelled"
			job.cancel()
			cancelled = append(cancelled, job)
		}
	}
	s.clipboardFilesMu.Unlock()
	for _, job := range cancelled {
		s.notifyClipboardFileCancel(job)
	}
}

func clipboardFileFinished(state string) bool {
	return state == "completed" || state == "failed" || state == "cancelled"
}

func (s *Service) cancelClipboardFileJobs(peerID string, dropFinished bool) {
	s.clipboardFilesMu.Lock()
	var cancelled []*clipboardFileJob
	for key, job := range s.clipboardFileJobs {
		if peerID != "" && job.peer.PeerID != peerID {
			continue
		}
		if clipboardFileFinished(job.progress.State) {
			if dropFinished {
				delete(s.clipboardFileJobs, key)
			}
		} else {
			job.progress.State = "cancelled"
			job.cancel()
			cancelled = append(cancelled, job)
		}
	}
	s.clipboardFilesMu.Unlock()
	for _, job := range cancelled {
		s.notifyClipboardFileCancel(job)
	}
}

func (s *Service) clipboardFileCurrent(job *clipboardFileJob) bool {
	if job.ctx.Err() != nil || s.clipboardPaused.Load() || s.checkAuthorizationGeneration(job.peer.PeerID, job.peer.AuthorizationGeneration) != nil {
		return false
	}
	if job.reservation != nil {
		grant, ok := s.clipboardGrant(job.peer.PeerID, "receive", clipboardsync.Files, job.peer.AuthorizationGeneration)
		return ok && grant.Revision == job.event.ReceiverGrantRevision && s.clipboardSync.FilesCurrent(*job.reservation)
	}
	grant, ok := s.clipboardGrant(job.peer.PeerID, "send", clipboardsync.Files, job.peer.AuthorizationGeneration)
	return ok && grant.Revision == job.event.SenderGrantRevision && s.clipboardSync.LocalCurrent(job.event)
}

func (s *Service) cancelStaleClipboardFiles() {
	s.clipboardFilesMu.Lock()
	var jobs []*clipboardFileJob
	for _, job := range s.clipboardFileJobs {
		if !clipboardFileFinished(job.progress.State) {
			jobs = append(jobs, job)
		}
	}
	s.clipboardFilesMu.Unlock()
	for _, job := range jobs {
		if !s.clipboardFileCurrent(job) {
			s.CancelClipboardFile(job.progress.ID)
		}
	}
}

// Caller holds operationMu when starting a worker, closing the Add/Wait gate.
func (s *Service) addClipboardFileJob(job *clipboardFileJob) error {
	s.clipboardFilesMu.Lock()
	defer s.clipboardFilesMu.Unlock()
	if s.clipboardFileJobs == nil {
		s.clipboardFileJobs = make(map[string]*clipboardFileJob)
	}
	if s.clipboardFileSendSlots == nil {
		s.clipboardFileSendSlots = make(chan struct{}, 2)
	}
	if s.clipboardFileReadSlots == nil {
		s.clipboardFileReadSlots = make(chan struct{}, 2)
	}
	for key, previous := range s.clipboardFileJobs {
		if clipboardFileFinished(previous.progress.State) || previous.ctx.Err() != nil {
			delete(s.clipboardFileJobs, key)
		}
	}
	if len(s.clipboardFileJobs) >= 16 {
		return clipboardsync.ErrLimit
	}
	key := clipboardFileKey(job.peer, job.progress.ID)
	if s.clipboardFileJobs[key] != nil {
		return clipboardsync.ErrStale
	}
	s.clipboardFileJobs[key] = job
	return nil
}

func (s *Service) updateClipboardFile(job *clipboardFileJob, state string, p transfer.Progress) {
	s.clipboardFilesMu.Lock()
	if clipboardFileFinished(job.progress.State) {
		s.clipboardFilesMu.Unlock()
		return
	}
	job.progress.State = state
	if p.Total > 0 {
		job.progress.Total = p.Total
		job.progress.Large = p.Total >= ClipboardFilePopupThreshold
	}
	job.progress.Verified = p.Verified
	s.clipboardFilesMu.Unlock()
	if !s.clipboardFileCurrent(job) {
		job.cancel()
	}
}

func (s *Service) finishClipboardFile(job *clipboardFileJob, err error) {
	s.clipboardFilesMu.Lock()
	wasCancelled := job.progress.State == "cancelled"
	localFailure := job.localFailure
	job.progress.State = "completed"
	if err != nil {
		job.progress.State = "failed"
		job.progress.Error = clipboardFileFailureCode(err)
		if !localFailure && (job.ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, clipboardsync.ErrConflict)) {
			job.progress.State = "cancelled"
			job.progress.Error = ""
		}
	}
	if wasCancelled {
		job.progress.State, job.progress.Error = "cancelled", ""
	}
	job.cancel()
	failed := job.progress.State == "failed"
	failure := job.progress.Error
	s.clipboardFilesMu.Unlock()
	s.clipboardStatusMu.Lock()
	status := s.clipboardPeerRuntime[job.peer.PeerID]
	if status.FileID == job.progress.ID {
		status.Waiting, status.FileID = "", ""
		if failed {
			status.Error = failure
		} else if clipboardFileError(status.Error) {
			status.Error = ""
		}
	}
	if status.Waiting == "" && status.Error == "" {
		delete(s.clipboardPeerRuntime, job.peer.PeerID)
	} else {
		s.clipboardPeerRuntime[job.peer.PeerID] = status
	}
	s.clipboardStatusMu.Unlock()
	if err != nil && !wasCancelled {
		s.notifyClipboardFileCancel(job)
	}
}

func clipboardFileError(code string) bool {
	return code == "file_copy_failed" || code == "file_copy_disk_full"
}

func clipboardFileFailureCode(err error) string {
	if protocol.ErrorCode(err) == protocol.DiskFull || errors.Is(err, syscall.ENOSPC) {
		return "file_copy_disk_full"
	}
	return "file_copy_failed"
}

func (s *Service) clipboardFileCancelled(peer *PeerSession, message clipboardsync.Message) {
	s.clipboardFilesMu.Lock()
	defer s.clipboardFilesMu.Unlock()
	job := s.clipboardFileJobs[clipboardFileKey(peer, message.FileID)]
	if job != nil && job.event.Digest == message.Digest && job.event.LeaseID == message.LeaseID && !job.localFailure && !clipboardFileFinished(job.progress.State) {
		job.progress.State = "cancelled"
		job.cancel()
	}
}

func (s *Service) notifyClipboardFileCancel(job *clipboardFileJob) {
	s.operationMu.Lock()
	if s.isClosing() || job.peer.Data.Conn.Context().Err() != nil {
		s.operationMu.Unlock()
		return
	}
	s.clipboardWorkers.Add(1)
	go func() {
		defer s.clipboardWorkers.Done()
		ctx, cancel := context.WithTimeout(s.workCtx, 500*time.Millisecond)
		defer cancel()
		// Publish the current causal clock before cancellation. A receiver's
		// newer local copy may have advanced past the sender's previous lease;
		// the next copy must bind a fresh lease rather than reuse that clock.
		s.sendClipboardLease(ctx, job.peer)
		stream, err := job.peer.Data.Conn.OpenUniStreamSync(ctx)
		if err != nil {
			return
		}
		_ = stream.SetWriteDeadline(time.Now().Add(500 * time.Millisecond))
		message := clipboardsync.Message{Type: "file_cancel", LeaseID: job.event.LeaseID, SessionID: job.peer.SessionID, Generation: job.peer.RemoteAuthorizationGeneration, FileID: job.progress.ID, Digest: job.event.Digest}
		if clipboardsync.Write(stream, message, nil) != nil {
			stream.CancelWrite(0)
		} else {
			_ = stream.Close()
		}
	}()
	s.operationMu.Unlock()
}

func (s *Service) clipboardFilesChanged(ctx context.Context, observation clipboardsync.Event) error {
	s.clipboardAdapterMu.RLock()
	adapter := s.clipboardAdapter
	s.clipboardAdapterMu.RUnlock()
	if adapter.ReadFiles == nil {
		return errors.New("CLIPBOARD_FILES_ADAPTER_UNAVAILABLE")
	}
	s.directPoolMu.Lock()
	var peers []*PeerSession
	for _, pooled := range s.directPool {
		if pooled.peer != nil && pooled.peer.ClipboardSync && pooled.peer.Data.Conn.Context().Err() == nil {
			peers = append(peers, pooled.peer)
		}
	}
	s.directPoolMu.Unlock()
	// Permission and capability gates precede reading the native file list.
	var eligible []*PeerSession
	for _, peer := range peers {
		_, permitted := s.clipboardGrant(peer.PeerID, "send", clipboardsync.Files, peer.AuthorizationGeneration)
		if permitted && !peer.ClipboardFiles {
			s.setClipboardPeerError(peer.PeerID, "files_unsupported")
			continue
		}
		_, leased := s.clipboardSync.Outbound(peer.PeerID, peer.SessionID, peer.RemoteAuthorizationGeneration, clipboardsync.Files)
		if permitted && leased {
			eligible = append(eligible, peer)
		}
	}
	if len(eligible) == 0 {
		return nil
	}
	paths, generation, err := adapter.ReadFiles(ctx, observation.OSGeneration)
	if err != nil || generation != observation.OSGeneration || len(paths) == 0 || len(paths) > 128 {
		return errors.Join(clipboardsync.ErrStale, err)
	}
	var size int64
	for _, path := range paths {
		info, err := os.Lstat(path)
		if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() && !info.IsDir() {
			return errors.Join(transfer.ErrPath, err)
		}
		if !info.IsDir() {
			size += info.Size()
		}
	}
	if adapter.Generation == nil || adapter.Generation() != generation || !s.clipboardSync.LocalCurrent(observation) {
		return clipboardsync.ErrStale
	}
	offer := clipboardFileOffer{ID: protocol.RandomID(), Count: len(paths), Bytes: size}
	payload, _ := json.Marshal(offer)
	base, err := s.clipboardSync.PrepareObserved(observation, clipboardsync.Files, payload)
	if err != nil {
		return err
	}
	for _, peer := range eligible {
		lease, leased := s.clipboardSync.Outbound(peer.PeerID, peer.SessionID, peer.RemoteAuthorizationGeneration, clipboardsync.Files)
		grant, permitted := s.clipboardGrant(peer.PeerID, "send", clipboardsync.Files, peer.AuthorizationGeneration)
		if !leased || !permitted {
			continue
		}
		event, err := s.clipboardSync.BindScoped(base, lease.ID, grant.Revision)
		if err != nil {
			continue
		}
		jobCtx, cancel := context.WithTimeout(peer.Data.Conn.Context(), 24*time.Hour)
		job := &clipboardFileJob{peer: peer, event: event, lease: lease, paths: append([]string(nil), paths...), ctx: jobCtx, cancel: cancel, ready: make(chan struct{}), progress: ClipboardFileProgress{ID: offer.ID, PeerID: peer.PeerID, Direction: "send", State: "queued", Total: size, Large: size >= ClipboardFilePopupThreshold, FileCount: len(paths)}}
		s.operationMu.Lock()
		if s.isClosing() || s.addClipboardFileJob(job) != nil {
			s.operationMu.Unlock()
			cancel()
			continue
		}
		s.clipboardWorkers.Add(1)
		go func() {
			defer s.clipboardWorkers.Done()
			err := s.sendClipboardFiles(job)
			s.finishClipboardFile(job, err)
		}()
		s.operationMu.Unlock()
	}
	return nil
}

func (s *Service) clipboardFileReady(peer *PeerSession, message clipboardsync.Message) {
	s.clipboardFilesMu.Lock()
	defer s.clipboardFilesMu.Unlock()
	job := s.clipboardFileJobs[clipboardFileKey(peer, message.FileID)]
	if job == nil || job.reservation != nil || job.event.Digest != message.Digest || job.event.LeaseID != message.LeaseID {
		return
	}
	select {
	case <-job.ready:
	default:
		close(job.ready)
	}
}

func (s *Service) admitClipboardFiles(peer *PeerSession, candidate clipboardsync.Candidate) error {
	if !peer.ClipboardFiles {
		return clipboardsync.ErrStale
	}
	var offer clipboardFileOffer
	if json.Unmarshal(candidate.Payload, &offer) != nil || !validClipboardFileID(offer.ID, 16) || offer.Count < 1 || offer.Count > 128 || offer.Bytes < 0 {
		return clipboardsync.ErrLimit
	}
	s.clipboardAdapterMu.RLock()
	adapter := s.clipboardAdapter
	s.clipboardAdapterMu.RUnlock()
	if adapter.Generation == nil || adapter.WriteFiles == nil || adapter.Generation() != candidate.OSBaseline {
		return clipboardsync.ErrConflict
	}
	s.clipboardGrantMu.Lock()
	grant, allowed := s.clipboardGrantLocked(peer.PeerID, "receive", clipboardsync.Files, peer.AuthorizationGeneration)
	if !allowed || grant.Revision != candidate.ReceiverGrantRevision || s.checkAuthorizationGeneration(peer.PeerID, peer.AuthorizationGeneration) != nil {
		s.clipboardGrantMu.Unlock()
		return clipboardsync.ErrStale
	}
	reservation, err := s.clipboardSync.ReserveFiles(candidate)
	s.clipboardGrantMu.Unlock()
	if err != nil {
		return err
	}
	jobCtx, cancel := context.WithDeadline(peer.Data.Conn.Context(), reservation.Deadline)
	job := &clipboardFileJob{peer: peer, event: candidate.Event, reservation: &reservation, ctx: jobCtx, cancel: cancel, progress: ClipboardFileProgress{ID: offer.ID, PeerID: peer.PeerID, Direction: "receive", State: "preparing", Total: offer.Bytes, Large: offer.Bytes >= ClipboardFilePopupThreshold, FileCount: offer.Count}}
	s.operationMu.Lock()
	if s.isClosing() {
		s.operationMu.Unlock()
		cancel()
		return context.Canceled
	}
	err = s.addClipboardFileJob(job)
	if err == nil {
		s.clipboardWorkers.Add(1)
		go func() {
			defer s.clipboardWorkers.Done()
			<-jobCtx.Done()
			s.clipboardFilesMu.Lock()
			if !clipboardFileFinished(job.progress.State) && !job.localFailure {
				job.progress.State = "cancelled"
			}
			s.clipboardFilesMu.Unlock()
		}()
	}
	s.operationMu.Unlock()
	if err != nil {
		cancel()
		return err
	}
	s.setClipboardWaiting(peer.PeerID, "copying_files", job.progress.ID)
	s.cancelStaleClipboardFiles()
	ctx, stop := context.WithDeadline(jobCtx, candidate.Deadline)
	defer stop()
	stream, err := peer.Data.Conn.OpenUniStreamSync(ctx)
	if err != nil {
		s.finishClipboardFile(job, err)
		return err
	}
	_ = stream.SetWriteDeadline(candidate.Deadline)
	message := clipboardsync.Message{Type: "file_ready", LeaseID: candidate.LeaseID, SessionID: peer.SessionID, Generation: peer.AuthorizationGeneration, FileID: offer.ID, Digest: candidate.Digest}
	if err = clipboardsync.Write(stream, message, nil); err != nil {
		stream.CancelWrite(0)
		s.finishClipboardFile(job, err)
		return err
	}
	return stream.Close()
}

func (s *Service) sendClipboardFiles(job *clipboardFileJob) error {
	if err := s.sendClipboardJob(job.ctx, &clipboardSendJob{peer: job.peer, lease: job.lease, event: job.event, fileID: job.progress.ID}); err != nil {
		return err
	}
	s.setClipboardWaiting(job.peer.PeerID, "copying_files", job.progress.ID)
	ackCtx, cancelAck := context.WithDeadline(job.ctx, job.lease.ExpiresAt())
	defer cancelAck()
	select {
	case <-job.ready:
	case <-ackCtx.Done():
		return ackCtx.Err()
	}
	select {
	case s.clipboardFileSendSlots <- struct{}{}:
	case <-job.ctx.Done():
		return job.ctx.Err()
	}
	defer func() { <-s.clipboardFileSendSlots }()
	if !s.clipboardFileCurrent(job) {
		return clipboardsync.ErrStale
	}
	s.updateClipboardFile(job, "preparing", transfer.Progress{})
	prepared, err := transfer.PrepareForResume(job.ctx, job.paths, transfer.DefaultChunkSize, job.progress.ID)
	if err != nil {
		return err
	}
	defer prepared.Close()
	for {
		if !s.clipboardFileCurrent(job) {
			return clipboardsync.ErrStale
		}
		s.directPoolMu.Lock()
		pooled := s.directPool[directPoolKey(job.peer.PeerID, job.peer.AuthorizationGeneration)]
		if pooled == nil || pooled.peer != job.peer || job.peer.Data.Conn.Context().Err() != nil {
			s.directPoolMu.Unlock()
			return errors.New("CLIPBOARD_CONNECTION_LOST")
		}
		if !pooled.inUse && !pooled.receiving {
			pooled.inUse = true
			if pooled.timer != nil {
				pooled.timer.Stop()
				pooled.timer = nil
			}
			s.directPoolMu.Unlock()
			break
		}
		s.directPoolMu.Unlock()
		select {
		case <-job.ctx.Done():
			return job.ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
	var operationErr error
	defer func() { _ = s.releaseClipboardFileSession(job, operationErr) }()
	stream, err := job.peer.Data.Conn.OpenStreamSync(job.ctx)
	if err != nil {
		operationErr = err
		return err
	}
	wrapped := transport.WrapStream(stream)
	stopAbort := context.AfterFunc(job.ctx, wrapped.Abort)
	defer stopAbort()
	link, _ := json.Marshal(struct {
		ID     string `json:"id"`
		Digest string `json:"digest"`
	}{job.progress.ID, job.event.Digest})
	var size [4]byte
	binary.BigEndian.PutUint32(size[:], uint32(len(link)))
	_ = stream.SetWriteDeadline(time.Now().Add(10 * time.Second))
	for _, part := range [][]byte{clipboardFileMagic, size[:], link} {
		if _, err = wrapped.Write(part); err != nil {
			operationErr = err
			return err
		}
	}
	_ = stream.SetWriteDeadline(time.Time{})
	_, err = transfer.SendWithOptions(job.ctx, wrapped, prepared, transfer.SendOptions{BodyBudget: processFileBudget, Hooks: transfer.SendHooks{Progress: func(p transfer.Progress) { s.updateClipboardFile(job, "transferring", p) }}})
	operationErr = classifyTaskError(err)
	return operationErr
}

func (s *Service) releaseClipboardFileSession(job *clipboardFileJob, err error) error {
	// Stream interruption can surface EOF/another I/O error after cancellation.
	// Keep that diagnostic, but also preserve the canceled operation's control
	// intent so a healthy connection is not mistaken for a failed QUIC path.
	// releasePeerSession still rejects reuse when the connection itself closed.
	if cause := job.ctx.Err(); cause != nil {
		err = errors.Join(cause, err)
	}
	return s.releasePeerSession(job.peer, err)
}

func (s *Service) receiveClipboardFileStream(peer *PeerSession, stream *transport.QUICStream) (handled bool) {
	_ = stream.SetReadDeadline(time.Now().Add(10 * time.Second))
	magic, err := stream.PeekPrefix(len(clipboardFileMagic))
	if err != nil || !bytes.Equal(magic, clipboardFileMagic) {
		_ = stream.SetReadDeadline(time.Time{})
		return false
	}
	if !peer.ClipboardFiles {
		stream.Abort()
		return true
	}
	_, _ = io.CopyN(io.Discard, stream, int64(len(clipboardFileMagic)))
	var size [4]byte
	if _, err = io.ReadFull(stream, size[:]); err != nil {
		stream.Abort()
		return true
	}
	n := binary.BigEndian.Uint32(size[:])
	if n == 0 || n > 2048 {
		stream.Abort()
		return true
	}
	data := make([]byte, n)
	if _, err = io.ReadFull(stream, data); err != nil {
		stream.Abort()
		return true
	}
	_ = stream.SetReadDeadline(time.Time{})
	var link struct {
		ID     string `json:"id"`
		Digest string `json:"digest"`
	}
	if json.Unmarshal(data, &link) != nil || !validClipboardFileID(link.ID, 16) || !validClipboardFileID(link.Digest, 32) {
		stream.Abort()
		return true
	}
	s.clipboardFilesMu.Lock()
	job := s.clipboardFileJobs[clipboardFileKey(peer, link.ID)]
	valid := job != nil && job.reservation != nil && !job.started && job.event.Digest == link.Digest
	if valid {
		job.started = true
	}
	s.clipboardFilesMu.Unlock()
	if !valid || !s.clipboardFileCurrent(job) {
		stream.Abort()
		return true
	}
	stopAbort := context.AfterFunc(job.ctx, stream.Abort)
	defer stopAbort()
	err = s.receiveClipboardFiles(job, stream)
	stopAbort()
	s.finishClipboardFile(job, err)
	if err != nil {
		stream.Abort()
	}
	return true
}

func (s *Service) receiveClipboardFiles(job *clipboardFileJob, stream *transport.QUICStream) error {
	select {
	case s.clipboardFileReadSlots <- struct{}{}:
	case <-job.ctx.Done():
		return job.ctx.Err()
	}
	defer func() { <-s.clipboardFileReadSlots }()
	root := filepath.Join(s.cfg.DataDir, "clipboard-files")
	if err := os.MkdirAll(root, 0700); err != nil {
		return err
	}
	directory := filepath.Join(root, job.progress.ID)
	// Transfer IDs are fixed-width lowercase hex and the per-transfer directory
	// must be new. No remote path can choose the local cache root.
	if err := os.Mkdir(directory, 0700); err != nil {
		return err
	}
	cache, err := os.OpenRoot(root)
	if err != nil {
		return err
	}
	defer cache.Close()
	published := false
	defer func() {
		if !published {
			_ = cache.RemoveAll(job.progress.ID)
		}
	}()
	var roots []string
	var releaseSpace func()
	defer func() {
		if releaseSpace != nil {
			releaseSpace()
		}
	}()
	_, err = transfer.ReceiveWithOptions(job.ctx, stream, transfer.ReceiveOptions{Directory: directory, Peer: job.peer.PeerID, Plan: func(_ context.Context, offer transfer.Offer) (transfer.ReceivePlan, error) {
		if offer.Manifest.TransferID != job.progress.ID || offer.Content != nil || !s.clipboardFileCurrent(job) {
			return transfer.ReceivePlan{}, clipboardsync.ErrStale
		}
		seen := map[string]bool{}
		for _, entry := range offer.Manifest.Files {
			name := strings.Split(entry.Path, "/")[0]
			if !seen[name] {
				seen[name] = true
				roots = append(roots, filepath.Join(directory, name))
			}
		}
		if len(roots) == 0 || len(roots) > 128 {
			return transfer.ReceivePlan{}, clipboardsync.ErrLimit
		}
		if releaseSpace != nil {
			return transfer.ReceivePlan{}, clipboardsync.ErrStale
		}
		releaseSpace, err = s.clipboardCacheSpace.reserve(root, offer.Manifest)
		if err != nil {
			// Record local rejection before the engine sends its terminal error.
			// Otherwise the peer's resulting file_cancel can arrive first and
			// erase the actionable local reason. Earlier user cancellation wins.
			s.clipboardFilesMu.Lock()
			if !clipboardFileFinished(job.progress.State) && job.ctx.Err() == nil {
				job.localFailure = true
			}
			s.clipboardFilesMu.Unlock()
			return transfer.ReceivePlan{}, err
		}
		plan := transfer.FullReceivePlan(offer.Manifest)
		plan.Directory = directory
		return plan, nil
	}, Progress: func(p transfer.Progress) { s.updateClipboardFile(job, "receiving", p) }})
	if err != nil {
		return err
	}
	s.clipboardAdapterMu.RLock()
	adapter := s.clipboardAdapter
	s.clipboardAdapterMu.RUnlock()
	if adapter.Generation == nil || adapter.WriteFiles == nil {
		return errors.New("CLIPBOARD_FILES_ADAPTER_UNAVAILABLE")
	}
	if err := job.ctx.Err(); err != nil {
		return err
	}
	s.clipboardGrantMu.Lock()
	defer s.clipboardGrantMu.Unlock()
	grant, allowed := s.clipboardGrantLocked(job.peer.PeerID, "receive", clipboardsync.Files, job.peer.AuthorizationGeneration)
	if !allowed || grant.Revision != job.event.ReceiverGrantRevision || s.checkAuthorizationGeneration(job.peer.PeerID, job.peer.AuthorizationGeneration) != nil {
		return clipboardsync.ErrStale
	}
	err = s.clipboardSync.CommitFiles(*job.reservation, func(expected uint64) (uint64, error) {
		if err := job.ctx.Err(); err != nil {
			return 0, err
		}
		if adapter.Generation() != expected {
			return 0, clipboardsync.ErrConflict
		}
		return adapter.WriteFiles(roots, expected, job.reservation.Deadline)
	})
	published = err == nil
	return err
}
