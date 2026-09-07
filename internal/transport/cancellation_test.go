package transport

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"example.com/linksend/internal/transfer"
)

func writeTestFrame(t *testing.T, w io.Writer, value any) {
	t.Helper()
	b, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var header [4]byte
	binary.BigEndian.PutUint32(header[:], uint32(len(b)))
	if _, err = w.Write(header[:]); err != nil {
		t.Fatal(err)
	}
	if _, err = w.Write(b); err != nil {
		t.Fatal(err)
	}
}

func readTestFrame(t *testing.T, r io.Reader, limit int) []byte {
	t.Helper()
	var header [4]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		t.Fatal(err)
	}
	n := binary.BigEndian.Uint32(header[:])
	if n > uint32(limit) {
		t.Fatalf("frame too large: %d", n)
	}
	b := make([]byte, n)
	if _, err := io.ReadFull(r, b); err != nil {
		t.Fatal(err)
	}
	return b
}

func TestRealQUICTransferCancellationUnblocksAcceptanceRead(t *testing.T) {
	client, server := fixturePair(t, false, false)
	root := t.TempDir()
	source := filepath.Join(root, "source.bin")
	if err := os.WriteFile(source, []byte("cancellation fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	prepared, err := transfer.Prepare(context.Background(), []string{source}, 64<<10)
	if err != nil {
		t.Fatal(err)
	}
	defer prepared.Close()

	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		stream, openErr := client.OpenStreamSync(ctx)
		if openErr != nil {
			result <- openErr
			return
		}
		_, sendErr := transfer.Send(ctx, WrapStream(stream), prepared, nil)
		result <- sendErr
	}()

	stream, err := server.AcceptStream(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer stream.CancelRead(0)
	var header [4]byte
	if _, err = io.ReadFull(stream, header[:]); err != nil {
		t.Fatal(err)
	}
	n := binary.BigEndian.Uint32(header[:])
	if n == 0 || n > transfer.MaxMetadata {
		t.Fatalf("unexpected offer frame length %d", n)
	}
	if _, err = io.CopyN(io.Discard, stream, int64(n)); err != nil {
		t.Fatal(err)
	}

	cancel()
	select {
	case err = <-result:
		if err == nil {
			t.Fatal("cancellation unexpectedly completed transfer")
		}
	case <-time.After(500 * time.Millisecond):
		_ = client.CloseWithError(0, "test cleanup")
		t.Fatal("real QUIC transfer cancellation did not unblock acceptance read")
	}
}

func TestRealQUICTransferCancellationUnblocksBlockedReceive(t *testing.T) {
	client, server := fixturePair(t, false, false)
	stream, err := client.OpenStreamSync(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = stream.Write([]byte{0}); err != nil {
		t.Fatal(err)
	}
	accepted, err := server.AcceptStream(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, receiveErr := transfer.Receive(ctx, WrapStream(accepted), t.TempDir(), "peer", func(transfer.Manifest) bool { return true }, nil)
		result <- receiveErr
	}()
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, transfer.ErrCancelled) || !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v, want cancellation with cause", err)
		}
	case <-time.After(500 * time.Millisecond):
		stream.CancelWrite(0)
		t.Fatal("real QUIC receive cancellation did not unblock blocked application read")
	}
}

func TestRealQUICTransferCancellationUnblocksAckWait(t *testing.T) {
	client, server := fixturePair(t, false, false)
	root := t.TempDir()
	source := filepath.Join(root, "source.bin")
	if err := os.WriteFile(source, []byte("ack cancellation fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	prepared, err := transfer.Prepare(context.Background(), []string{source}, 64<<10)
	if err != nil {
		t.Fatal(err)
	}
	defer prepared.Close()
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		stream, openErr := client.OpenStreamSync(ctx)
		if openErr != nil {
			result <- openErr
			return
		}
		_, sendErr := transfer.Send(ctx, WrapStream(stream), prepared, nil)
		result <- sendErr
	}()
	stream, err := server.AcceptStream(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	_ = readTestFrame(t, stream, transfer.MaxMetadata)
	writeTestFrame(t, stream, map[string]any{"op": "accept", "digest": prepared.Manifest.Digest(), "verified": 0})
	writeTestFrame(t, stream, map[string]any{"op": "chunk", "file": 0, "index": 0})
	_ = readTestFrame(t, stream, prepared.Manifest.ChunkSize)
	cancel()
	select {
	case err = <-result:
		if !errors.Is(err, transfer.ErrCancelled) || !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v, want cancellation with cause", err)
		}
	case <-time.After(500 * time.Millisecond):
		_ = client.CloseWithError(0, "test cleanup")
		t.Fatal("real QUIC transfer cancellation did not unblock ACK wait")
	}
}

func TestRealQUICStreamAbortIsIdempotent(t *testing.T) {
	client, _ := fixturePair(t, false, false)
	stream, err := client.OpenStreamSync(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	wrapped := WrapStream(stream)
	for i := 0; i < 3; i++ {
		wrapped.Abort()
	}
	// Close after abort is permitted to be a no-op or return a stream error;
	// the important property is that repeated abort did not panic.
	_ = stream.Close()
}
