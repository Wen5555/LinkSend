package transfer

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"math"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"
)

type Progress struct {
	TransferID     string  `json:"transfer_id"`
	State          string  `json:"state"`
	Total          int64   `json:"total"`
	Read           int64   `json:"read"`
	Sent           int64   `json:"sent"`
	Received       int64   `json:"received"`
	Retransmitted  int64   `json:"retransmitted"`
	Verified       int64   `json:"verified"`
	Committed      int64   `json:"committed"`
	CommittedFiles int     `json:"committed_files"`
	BytesPerSecond float64 `json:"bytes_per_second"`
}
type ChunkTransmission struct {
	FileID        uint32 `json:"file_id"`
	Index         int    `json:"index"`
	Bytes         int64  `json:"bytes"`
	Retransmitted bool   `json:"retransmitted"`
}
type SendHooks struct {
	Progress       func(Progress)
	PreviouslySent func(fileID uint32, index int) bool
	ChunkSent      func(ChunkTransmission)
}
type Result struct {
	TransferID string `json:"transfer_id"`
	Digest     string `json:"digest"`
	Bytes      int64  `json:"bytes"`
	State      string `json:"state"`
}
type control struct {
	Op       string    `json:"op"`
	Manifest *Manifest `json:"manifest,omitempty"`
	Digest   string    `json:"digest,omitempty"`
	File     uint32    `json:"file,omitempty"`
	Index    int       `json:"index,omitempty"`
	Verified int64     `json:"verified,omitempty"`
	Error    string    `json:"error,omitempty"`
}

const (
	opOffer        = "offer"
	opAccept       = "accept"
	opReject       = "reject"
	opChunk        = "chunk"
	opAck          = "ack"
	opReady        = "ready"
	opFinish       = "finish"
	opCompleted    = "completed"
	opConfirmed    = "confirmed"
	opConfirmedAck = "confirmed_ack"
	opError        = "error"
)

const maxPeerError = 4 << 10

var (
	ErrCancelled = errors.New("CANCELLED")
	ErrPeerError = errors.New("PEER_ERROR")
)

type PeerError struct{ Detail string }

func (e *PeerError) Error() string { return "PEER_ERROR: " + e.Detail }
func (e *PeerError) Unwrap() error { return ErrPeerError }

// Only exact, known codes acquire typed semantics. Never infer error classes
// from arbitrary peer text (which older clients may send).
func (e *PeerError) Is(target error) bool {
	switch e.Detail {
	case "FILE_CONFLICT":
		return target == ErrConflict
	case "PERMISSION_DENIED":
		return target == fs.ErrPermission
	case "DISK_FULL":
		return target == syscall.ENOSPC
	case "SOURCE_CHANGED":
		return target == ErrChanged
	case "CHECKSUM_FAILED":
		return target == ErrIntegrity
	case "DANGEROUS_PATH":
		return target == ErrPath
	case "CANCELLED":
		return target == ErrCancelled
	default:
		return false
	}
}

// Preserve the v1 error envelope while sending codes instead of local paths.
func peerErrorCode(err error) string {
	for _, candidate := range []struct {
		err  error
		code string
	}{
		{ErrConflict, "FILE_CONFLICT"}, {fs.ErrPermission, "PERMISSION_DENIED"},
		{syscall.ENOSPC, "DISK_FULL"}, {ErrChanged, "SOURCE_CHANGED"},
		{ErrIntegrity, "CHECKSUM_FAILED"}, {ErrPath, "DANGEROUS_PATH"},
		{context.Canceled, "CANCELLED"}, {ErrCancelled, "CANCELLED"},
	} {
		if errors.Is(err, candidate.err) {
			return candidate.code
		}
	}
	return "TRANSFER_FAILED"
}

type streamAborter interface{ Abort() }
type terminalFlusher interface {
	FlushTerminal(context.Context) error
}
type terminalDeliveryObserver interface {
	TerminalDeliveryObserved(error) bool
}

func terminalDeliveryObserved(rw io.ReadWriteCloser, err error) bool {
	if errors.Is(err, io.EOF) {
		return true
	}
	observer, ok := rw.(terminalDeliveryObserver)
	return ok && observer.TerminalDeliveryObserved(err)
}

func flushTerminal(ctx context.Context, rw io.ReadWriteCloser, c control) error {
	if err := writeControl(rw, c); err != nil {
		return err
	}
	if flusher, ok := rw.(terminalFlusher); ok {
		return flusher.FlushTerminal(ctx)
	}
	return nil
}

// QUIC writes are buffered. A terminal frame must reach the peer before the
// deferred abort/session close can discard it. Synchronous test streams need
// no additional flush. This does not change the v1 control-frame protocol.
func writeTerminal(ctx context.Context, rw io.ReadWriteCloser, c control) {
	_ = flushTerminal(ctx, rw, c)
}

func abortStream(rw io.ReadWriteCloser) {
	if aborter, ok := rw.(streamAborter); ok {
		aborter.Abort()
		return
	}
	_ = rw.Close()
}

// streamLifecycle makes cancellation and normal completion mutually exclusive.
// The quic adapter aborts both directions; neutral streams retain Close fallback.
func streamLifecycle(ctx context.Context, rw io.ReadWriteCloser) (complete, abort func()) {
	var mu sync.Mutex
	finished := false
	stop := context.AfterFunc(ctx, func() {
		mu.Lock()
		if finished {
			mu.Unlock()
			return
		}
		finished = true
		mu.Unlock()
		abortStream(rw)
	})
	complete = func() {
		mu.Lock()
		if finished {
			mu.Unlock()
			return
		}
		finished = true
		mu.Unlock()
		stop()
		_ = rw.Close()
	}
	abort = func() {
		mu.Lock()
		if finished {
			mu.Unlock()
			return
		}
		finished = true
		mu.Unlock()
		stop()
		abortStream(rw)
	}
	return complete, abort
}

func writeFrame(w io.Writer, b []byte, limit int) error {
	if limit <= 0 {
		return errors.New("INVALID_FRAME_LIMIT")
	}
	if len(b) > limit {
		return errors.New("FRAME_TOO_LARGE")
	}
	var header [4]byte
	binary.BigEndian.PutUint32(header[:], uint32(len(b)))
	if err := writeAll(w, header[:]); err != nil {
		return err
	}
	return writeAll(w, b)
}
func writeAll(w io.Writer, b []byte) error {
	for len(b) > 0 {
		n, err := w.Write(b)
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrNoProgress
		}
		b = b[n:]
	}
	return nil
}
func readFrame(r io.Reader, limit int) ([]byte, error) {
	if limit <= 0 {
		return nil, errors.New("INVALID_FRAME_LIMIT")
	}
	var header [4]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return nil, err
	}
	n := binary.BigEndian.Uint32(header[:])
	if uint64(n) > uint64(limit) {
		return nil, errors.New("FRAME_TOO_LARGE")
	}
	b := make([]byte, int(n))
	_, err := io.ReadFull(r, b)
	return b, err
}
func writeControl(w io.Writer, c control) error {
	if c.Op == opError {
		c.Error = safePeerError(c.Error)
		if c.Error == "" {
			c.Error = "peer reported an error"
		}
	}
	if err := validateControl(c); err != nil {
		return err
	}
	b, err := json.Marshal(c)
	if err != nil {
		return err
	}
	return writeFrame(w, b, MaxMetadata)
}
func readControl(r io.Reader) (control, error) {
	var c control
	b, err := readFrame(r, MaxMetadata)
	if err != nil {
		return c, err
	}
	if len(b) == 0 {
		return c, errors.New("INVALID_CONTROL")
	}
	if err = json.Unmarshal(b, &c); err != nil {
		return c, err
	}
	if err = validateControl(c); err != nil {
		return c, err
	}
	if c.Op == opError {
		return c, &PeerError{Detail: safePeerError(c.Error)}
	}
	return c, nil
}

func safePeerError(s string) string {
	if !utf8.ValidString(s) {
		return "peer reported an invalid error"
	}
	s = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return ' '
		}
		return r
	}, s)
	for len(s) > maxPeerError {
		_, width := utf8.DecodeLastRuneInString(s)
		s = s[:len(s)-width]
	}
	return strings.TrimSpace(s)
}

func validateControl(c control) error {
	switch c.Op {
	case opOffer:
		if c.Manifest == nil || c.Digest == "" || c.Digest != c.Manifest.Digest() {
			return errors.New("INVALID_OFFER")
		}
		if c.File != 0 || c.Index != 0 || c.Verified != 0 || c.Error != "" {
			return errors.New("INVALID_OFFER")
		}
	case opAccept:
		if c.Manifest != nil || c.Digest == "" || c.File != 0 || c.Index != 0 || c.Verified < 0 || c.Error != "" {
			return errors.New("INVALID_ACCEPTANCE")
		}
	case opChunk:
		if c.Manifest != nil || c.Digest != "" || c.Index < 0 || c.Verified != 0 || c.Error != "" {
			return errors.New("INVALID_CHUNK_REQUEST")
		}
	case opAck:
		if c.Manifest != nil || c.Digest != "" || c.Index < 0 || c.Verified < 0 || c.Error != "" {
			return errors.New("INVALID_ACK")
		}
	case opReady, opReject:
		if c.Manifest != nil || c.Digest != "" || c.File != 0 || c.Index != 0 || c.Verified != 0 || c.Error != "" {
			return errors.New("INVALID_CONTROL")
		}
	case opFinish, opConfirmed, opConfirmedAck:
		if c.Manifest != nil || c.Digest == "" || c.File != 0 || c.Index != 0 || c.Verified != 0 || c.Error != "" {
			return errors.New("INVALID_CONTROL")
		}
	case opCompleted:
		if c.Manifest != nil || c.Digest == "" || c.File != 0 || c.Index != 0 || c.Verified < 0 || c.Error != "" {
			return errors.New("INVALID_CONTROL")
		}
	case opError:
		if c.Manifest != nil || c.Digest != "" || c.File != 0 || c.Index != 0 || c.Verified != 0 || len(c.Error) == 0 || len(c.Error) > maxPeerError || !utf8.ValidString(c.Error) {
			return errors.New("INVALID_PEER_ERROR")
		}
	default:
		return errors.New("UNKNOWN_CRITICAL_MESSAGE")
	}
	return nil
}

// The caller must pass an already mutually authenticated stream. Cancellation
// closes it so blocked reads/writes terminate; file bytes never enter signaling.
func Send(ctx context.Context, rw io.ReadWriteCloser, p *Prepared, onProgress func(Progress)) (result Result, err error) {
	return SendWithHooks(ctx, rw, p, SendHooks{Progress: onProgress})
}

func SendWithHooks(ctx context.Context, rw io.ReadWriteCloser, p *Prepared, hooks SendHooks) (result Result, err error) {
	complete, abort := streamLifecycle(ctx, rw)
	defer func() {
		if ctx.Err() != nil && err != nil {
			err = errors.Join(ErrCancelled, ctx.Err(), err)
		}
		abort()
	}()
	m := p.Manifest
	digest := m.Digest()
	start := time.Now()
	progress := Progress{TransferID: m.TransferID, State: "AwaitingAcceptance", Total: m.TotalBytes()}
	emit := func() {
		if hooks.Progress != nil {
			hooks.Progress(progress)
		}
	}
	emit()
	if err := writeControl(rw, control{Op: opOffer, Manifest: &m, Digest: digest}); err != nil {
		return Result{}, err
	}
	c, err := readControl(rw)
	if err != nil {
		return Result{}, err
	}
	if c.Op == opReject {
		return Result{}, ErrRejected
	}
	if c.Op != opAccept || c.Digest != digest || c.Verified < 0 || c.Verified > progress.Total {
		return Result{}, errors.New("INVALID_ACCEPTANCE")
	}
	progress.State = "Transferring"
	progress.Verified = c.Verified
	initial := c.Verified
	emit()
	buf := make([]byte, m.ChunkSize)
	for {
		if err = ctx.Err(); err != nil {
			return Result{}, err
		}
		c, err = readControl(rw)
		if err != nil {
			return Result{}, err
		}
		switch c.Op {
		case opChunk:
			retransmitted := hooks.PreviouslySent != nil && hooks.PreviouslySent(c.File, c.Index)
			data, e := p.ReadChunk(ctx, c.File, c.Index, buf)
			if e != nil {
				writeTerminal(ctx, rw, control{Op: opError, Error: peerErrorCode(e)})
				return Result{}, e
			}
			progress.Read += int64(len(data))
			if e = writeFrame(rw, data, m.ChunkSize); e != nil {
				return Result{}, e
			}
			progress.Sent += int64(len(data))
			if retransmitted {
				progress.Retransmitted += int64(len(data))
			}
			if hooks.ChunkSent != nil {
				hooks.ChunkSent(ChunkTransmission{FileID: c.File, Index: c.Index, Bytes: int64(len(data)), Retransmitted: retransmitted})
			}
			ack, e := readControl(rw)
			if e != nil {
				return Result{}, e
			}
			maxVerified := initial + progress.Sent
			if ack.Op != opAck || ack.File != c.File || ack.Index != c.Index || ack.Verified < progress.Verified || ack.Verified > progress.Total || ack.Verified > maxVerified {
				return Result{}, errors.New("INVALID_ACK")
			}
			progress.Verified = ack.Verified
			progress.BytesPerSecond = safeRate(progress.Verified-initial, time.Since(start))
			emit()
		case opReady:
			progress.State = "Verifying"
			emit()
			if err = p.Revalidate(ctx); err != nil {
				writeTerminal(ctx, rw, control{Op: opError, Error: peerErrorCode(err)})
				return Result{}, err
			}
			if err = writeControl(rw, control{Op: opFinish, Digest: digest}); err != nil {
				return Result{}, err
			}
			done, e := readControl(rw)
			if e != nil {
				return Result{}, e
			}
			if done.Op != opCompleted || done.Digest != digest || done.Verified != m.TotalBytes() {
				return Result{}, ErrIntegrity
			}
			// New peers explicitly acknowledge that they consumed confirmed. Older
			// receivers close their send direction after consuming it, which remains
			// equivalent delivery evidence and keeps this change wire-compatible.
			if err = writeControl(rw, control{Op: opConfirmed, Digest: digest}); err != nil {
				return Result{}, err
			}
			confirmation, confirmationErr := readControl(rw)
			if confirmationErr != nil {
				if !terminalDeliveryObserved(rw, confirmationErr) {
					return Result{}, confirmationErr
				}
			} else if confirmation.Op != opConfirmedAck || confirmation.Digest != digest {
				return Result{}, ErrIntegrity
			}
			progress.State = "Completed"
			progress.Verified = done.Verified
			emit()
			complete()
			return Result{m.TransferID, digest, done.Verified, "Completed"}, nil
		default:
			return Result{}, errors.New("UNKNOWN_CRITICAL_MESSAGE")
		}
	}
}

func safeRate(bytes int64, elapsed time.Duration) float64 {
	if bytes <= 0 || elapsed <= 0 {
		return 0
	}
	r := float64(bytes) / elapsed.Seconds()
	if math.IsNaN(r) || math.IsInf(r, 0) || r < 0 {
		return 0
	}
	return r
}

func Receive(ctx context.Context, rw io.ReadWriteCloser, directory, peer string, accept func(Manifest) bool, onProgress func(Progress)) (result Result, err error) {
	complete, abort := streamLifecycle(ctx, rw)
	defer func() {
		if ctx.Err() != nil && err != nil {
			err = errors.Join(ErrCancelled, ctx.Err(), err)
		}
		abort()
	}()
	offer, err := readControl(rw)
	if err != nil {
		return Result{}, err
	}
	if offer.Op != opOffer || offer.Manifest == nil {
		return Result{}, errors.New("INVALID_OFFER")
	}
	m := *offer.Manifest
	if err = m.Validate(); err != nil {
		return Result{}, err
	}
	if offer.Digest != m.Digest() {
		return Result{}, ErrIntegrity
	}
	if accept == nil || !accept(m) {
		writeTerminal(ctx, rw, control{Op: opReject})
		return Result{}, ErrRejected
	}
	r, err := OpenReceiver(ctx, directory, peer, m)
	if err != nil {
		writeTerminal(ctx, rw, control{Op: opError, Error: peerErrorCode(err)})
		return Result{}, err
	}
	defer r.Close()
	fail := func(e error) (Result, error) {
		state := "Failed"
		if errors.Is(e, context.Canceled) {
			state = "Cancelled"
		}
		_ = r.Mark(state)
		return Result{}, e
	}
	if err = writeControl(rw, control{Op: opAccept, Digest: m.Digest(), Verified: r.VerifiedBytes()}); err != nil {
		return fail(err)
	}
	start := time.Now()
	initial := r.VerifiedBytes()
	var received int64
	var committed int64
	var committedFiles int
	emit := func(state string) {
		if onProgress != nil {
			onProgress(Progress{TransferID: m.TransferID, State: state, Total: m.TotalBytes(), Received: received, Verified: r.VerifiedBytes(), Committed: committed, CommittedFiles: committedFiles, BytesPerSecond: safeRate(r.VerifiedBytes()-initial, time.Since(start))})
		}
	}
	for _, e := range m.Files {
		for i, ok := range r.State.Verified[e.ID] {
			if ok {
				continue
			}
			if err = ctx.Err(); err != nil {
				return fail(err)
			}
			if err = writeControl(rw, control{Op: opChunk, File: e.ID, Index: i}); err != nil {
				return fail(err)
			}
			data, eRead := readFrame(rw, m.ChunkSize)
			if eRead != nil {
				return fail(eRead)
			}
			received += int64(len(data))
			emit("Transferring")
			if err = r.WriteChunk(ctx, e.ID, i, data); err != nil {
				writeTerminal(ctx, rw, control{Op: opError, Error: peerErrorCode(err)})
				return fail(err)
			}
			if err = writeControl(rw, control{Op: opAck, File: e.ID, Index: i, Verified: r.VerifiedBytes()}); err != nil {
				return fail(err)
			}
			emit("Transferring")
		}
	}
	if err = writeControl(rw, control{Op: opReady}); err != nil {
		return fail(err)
	}
	finish, err := readControl(rw)
	if err != nil {
		return fail(err)
	}
	if finish.Op != opFinish || finish.Digest != m.Digest() {
		return fail(ErrIntegrity)
	}
	emit("Verifying")
	if err = r.FinishWithCommit(ctx, func(record CommitRecord) {
		committed += record.Size
		committedFiles++
		emit("Verifying")
	}); err != nil {
		writeTerminal(ctx, rw, control{Op: opError, Error: peerErrorCode(err)})
		return fail(err)
	}
	if err = writeControl(rw, control{Op: opCompleted, Digest: m.Digest(), Verified: r.VerifiedBytes()}); err != nil {
		return fail(err)
	}
	confirmed, err := readControl(rw)
	if err != nil {
		return Result{}, err
	}
	if confirmed.Op != opConfirmed || confirmed.Digest != m.Digest() {
		return Result{}, ErrIntegrity
	}
	// The explicit receipt removes the former fixed teardown wait. FlushTerminal
	// keeps the acknowledgement safe if the sender is an older implementation:
	// old senders ignore these bytes and have already half-closed their stream.
	if err = flushTerminal(ctx, rw, control{Op: opConfirmedAck, Digest: m.Digest()}); err != nil {
		return Result{}, err
	}
	emit("Completed")
	complete()
	return Result{m.TransferID, m.Digest(), r.VerifiedBytes(), "Completed"}, nil
}
