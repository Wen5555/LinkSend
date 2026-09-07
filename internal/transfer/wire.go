package transfer

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"
)

type Progress struct {
	TransferID     string  `json:"transfer_id"`
	State          string  `json:"state"`
	Total          int64   `json:"total"`
	Read           int64   `json:"read"`
	Sent           int64   `json:"sent"`
	Verified       int64   `json:"verified"`
	BytesPerSecond float64 `json:"bytes_per_second"`
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

func writeFrame(w io.Writer, b []byte, limit int) error {
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
	if err = json.Unmarshal(b, &c); err != nil {
		return c, err
	}
	if c.Op == "error" {
		return c, fmt.Errorf("PEER_ERROR: %s", c.Error)
	}
	return c, nil
}

// The caller must pass an already mutually authenticated stream. Cancellation
// closes it so blocked reads/writes terminate; file bytes never enter signaling.
func Send(ctx context.Context, rw io.ReadWriteCloser, p *Prepared, onProgress func(Progress)) (Result, error) {
	stop := context.AfterFunc(ctx, func() { _ = rw.Close() })
	defer stop()
	m := p.Manifest
	digest := m.Digest()
	start := time.Now()
	progress := Progress{TransferID: m.TransferID, State: "AwaitingAcceptance", Total: m.TotalBytes()}
	emit := func() {
		if onProgress != nil {
			onProgress(progress)
		}
	}
	emit()
	if err := writeControl(rw, control{Op: "offer", Manifest: &m, Digest: digest}); err != nil {
		return Result{}, err
	}
	c, err := readControl(rw)
	if err != nil {
		return Result{}, err
	}
	if c.Op == "reject" {
		return Result{}, ErrRejected
	}
	if c.Op != "accept" || c.Digest != digest {
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
		case "chunk":
			data, e := p.ReadChunk(ctx, c.File, c.Index, buf)
			if e != nil {
				_ = writeControl(rw, control{Op: "error", Error: e.Error()})
				return Result{}, e
			}
			progress.Read += int64(len(data))
			if e = writeFrame(rw, data, m.ChunkSize); e != nil {
				return Result{}, e
			}
			progress.Sent += int64(len(data))
			ack, e := readControl(rw)
			if e != nil {
				return Result{}, e
			}
			if ack.Op != "ack" || ack.File != c.File || ack.Index != c.Index || ack.Verified < progress.Verified || ack.Verified > progress.Total {
				return Result{}, errors.New("INVALID_ACK")
			}
			progress.Verified = ack.Verified
			progress.BytesPerSecond = float64(progress.Verified-initial) / time.Since(start).Seconds()
			emit()
		case "ready":
			progress.State = "Verifying"
			emit()
			if err = p.Revalidate(ctx); err != nil {
				_ = writeControl(rw, control{Op: "error", Error: err.Error()})
				return Result{}, err
			}
			if err = writeControl(rw, control{Op: "finish", Digest: digest}); err != nil {
				return Result{}, err
			}
			done, e := readControl(rw)
			if e != nil {
				return Result{}, e
			}
			if done.Op != "completed" || done.Digest != digest || done.Verified != m.TotalBytes() {
				return Result{}, ErrIntegrity
			}
			if err = writeControl(rw, control{Op: "confirmed", Digest: digest}); err != nil {
				return Result{}, err
			}
			progress.State = "Completed"
			progress.Verified = done.Verified
			emit()
			return Result{m.TransferID, digest, done.Verified, "Completed"}, nil
		default:
			return Result{}, errors.New("UNKNOWN_CRITICAL_MESSAGE")
		}
	}
}

func Receive(ctx context.Context, rw io.ReadWriteCloser, directory, peer string, accept func(Manifest) bool, onProgress func(Progress)) (Result, error) {
	stop := context.AfterFunc(ctx, func() { _ = rw.Close() })
	defer stop()
	offer, err := readControl(rw)
	if err != nil {
		return Result{}, err
	}
	if offer.Op != "offer" || offer.Manifest == nil {
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
		_ = writeControl(rw, control{Op: "reject"})
		return Result{}, ErrRejected
	}
	r, err := OpenReceiver(ctx, directory, peer, m)
	if err != nil {
		_ = writeControl(rw, control{Op: "error", Error: err.Error()})
		return Result{}, err
	}
	defer r.Close()
	fail := func(e error) (Result, error) { _ = r.Mark("Failed"); return Result{}, e }
	if err = writeControl(rw, control{Op: "accept", Digest: m.Digest(), Verified: r.VerifiedBytes()}); err != nil {
		return fail(err)
	}
	start := time.Now()
	initial := r.VerifiedBytes()
	emit := func(state string) {
		if onProgress != nil {
			onProgress(Progress{TransferID: m.TransferID, State: state, Total: m.TotalBytes(), Verified: r.VerifiedBytes(), BytesPerSecond: float64(r.VerifiedBytes()-initial) / time.Since(start).Seconds()})
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
			if err = writeControl(rw, control{Op: "chunk", File: e.ID, Index: i}); err != nil {
				return fail(err)
			}
			data, eRead := readFrame(rw, m.ChunkSize)
			if eRead != nil {
				return fail(eRead)
			}
			if err = r.WriteChunk(ctx, e.ID, i, data); err != nil {
				_ = writeControl(rw, control{Op: "error", Error: err.Error()})
				return fail(err)
			}
			if err = writeControl(rw, control{Op: "ack", File: e.ID, Index: i, Verified: r.VerifiedBytes()}); err != nil {
				return fail(err)
			}
			emit("Transferring")
		}
	}
	if err = writeControl(rw, control{Op: "ready"}); err != nil {
		return fail(err)
	}
	finish, err := readControl(rw)
	if err != nil {
		return fail(err)
	}
	if finish.Op != "finish" || finish.Digest != m.Digest() {
		return fail(ErrIntegrity)
	}
	emit("Verifying")
	if err = r.Finish(ctx); err != nil {
		_ = writeControl(rw, control{Op: "error", Error: err.Error()})
		return fail(err)
	}
	if err = writeControl(rw, control{Op: "completed", Digest: m.Digest(), Verified: r.VerifiedBytes()}); err != nil {
		return fail(err)
	}
	confirmed, err := readControl(rw)
	if err != nil {
		return Result{}, err
	}
	if confirmed.Op != "confirmed" || confirmed.Digest != m.Digest() {
		return Result{}, ErrIntegrity
	}
	emit("Completed")
	return Result{m.TransferID, m.Digest(), r.VerifiedBytes(), "Completed"}, nil
}
