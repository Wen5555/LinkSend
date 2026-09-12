package transport

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wen5555/LinkSend/internal/transfer"
	quic "github.com/quic-go/quic-go"
)

type completedReadGate struct {
	*QUICStream
	completed chan struct{}
	release   chan struct{}
	flushed   chan struct{}
	blocked   atomic.Bool
	once      sync.Once
	flushOnce sync.Once
}

func (s *completedReadGate) Write(p []byte) (int, error) {
	n, err := s.QUICStream.Write(p)
	if err == nil && bytes.Contains(p[:n], []byte(`"op":"completed"`)) {
		s.blocked.Store(true)
		s.once.Do(func() { close(s.completed) })
	}
	return n, err
}

func (s *completedReadGate) Read(p []byte) (int, error) {
	if s.blocked.CompareAndSwap(true, false) {
		select {
		case <-s.release:
		case <-s.Context().Done():
			return 0, s.Context().Err()
		}
	}
	return s.QUICStream.Read(p)
}

func (s *completedReadGate) FlushTerminal(ctx context.Context) error {
	s.flushOnce.Do(func() { close(s.flushed) })
	return s.QUICStream.FlushTerminal(ctx)
}

func TestTransferFailureSurvivesQUICStreamClose(t *testing.T) {
	for _, accept := range []bool{false, true} {
		t.Run(map[bool]string{false: "reject", true: "conflict"}[accept], func(t *testing.T) {
			client, server := fixturePair(t, false, false)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			source := filepath.Join(t.TempDir(), "empty.bin")
			if err := os.WriteFile(source, nil, 0600); err != nil {
				t.Fatal(err)
			}
			prepared, err := transfer.Prepare(ctx, []string{source}, 0)
			if err != nil {
				t.Fatal(err)
			}
			defer prepared.Close()
			dest := t.TempDir()
			if accept {
				if err := os.WriteFile(filepath.Join(dest, "empty.bin"), []byte("protected"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			done := make(chan error, 1)
			go func() {
				stream, err := server.AcceptStream(ctx)
				if err == nil {
					_, err = transfer.Receive(ctx, WrapStream(stream), dest, "pinned-peer", func(transfer.Manifest) bool { return accept }, nil)
				}
				done <- err
			}()
			stream, err := client.OpenStreamSync(ctx)
			if err != nil {
				t.Fatal(err)
			}
			_, err = transfer.Send(ctx, WrapStream(stream), prepared, nil)
			want := transfer.ErrRejected
			if accept {
				want = transfer.ErrConflict
			}
			if !errors.Is(err, want) {
				t.Errorf("sender lost failure class: %v; want %v", err, want)
			}
			if err = <-done; !errors.Is(err, want) {
				t.Errorf("receiver lost failure class: %v", err)
			}
		})
	}
}

func TestTransferCompletionWaitsForReceiverConfirmationOverQUIC(t *testing.T) {
	client, server := fixturePair(t, false, false)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	source := filepath.Join(t.TempDir(), "empty.bin")
	if err := os.WriteFile(source, nil, 0600); err != nil {
		t.Fatal(err)
	}
	prepared, err := transfer.Prepare(ctx, []string{source}, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer prepared.Close()

	completed := make(chan struct{})
	release := make(chan struct{})
	flushed := make(chan struct{})
	receiverDone := make(chan error, 1)
	go func() {
		stream, acceptErr := server.AcceptStream(ctx)
		if acceptErr == nil {
			gated := &completedReadGate{QUICStream: WrapStream(stream), completed: completed, release: release, flushed: flushed}
			_, acceptErr = transfer.Receive(ctx, gated, t.TempDir(), "pinned-peer", func(transfer.Manifest) bool { return true }, nil)
		}
		receiverDone <- acceptErr
	}()

	stream, err := client.OpenStreamSync(ctx)
	if err != nil {
		t.Fatal(err)
	}
	senderDone := make(chan error, 1)
	go func() {
		_, sendErr := transfer.Send(ctx, WrapStream(stream), prepared, nil)
		senderDone <- sendErr
	}()

	select {
	case <-completed:
	case <-ctx.Done():
		t.Fatal("receiver never wrote completed")
	}
	select {
	case err = <-senderDone:
		t.Fatalf("sender returned before receiver read confirmed: %v", err)
	default:
	}
	close(release)
	select {
	case <-flushed:
	case <-ctx.Done():
		t.Fatal("receiver did not flush confirmed_ack over QUIC")
	}
	if err = <-senderDone; err != nil {
		t.Fatalf("sender completion failed: %v", err)
	}
	if err = <-receiverDone; err != nil {
		t.Fatalf("receiver completion failed: %v", err)
	}
}

func TestTransferCompletionSurvivesReceiverNormalConnectionCloseOverQUIC(t *testing.T) {
	client, server := fixturePair(t, false, false)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	source := filepath.Join(t.TempDir(), "empty.bin")
	if err := os.WriteFile(source, nil, 0600); err != nil {
		t.Fatal(err)
	}
	prepared, err := transfer.Prepare(ctx, []string{source}, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer prepared.Close()

	receiverDone := make(chan error, 1)
	go func() {
		stream, receiveErr := server.AcceptStream(ctx)
		if receiveErr == nil {
			_, receiveErr = transfer.Receive(ctx, WrapStream(stream), t.TempDir(), "pinned-peer", func(transfer.Manifest) bool { return true }, nil)
		}
		if receiveErr == nil {
			receiveErr = server.CloseWithError(0, "closed")
		}
		receiverDone <- receiveErr
	}()

	stream, err := client.OpenStreamSync(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = transfer.Send(ctx, WrapStream(stream), prepared, nil); err != nil {
		t.Fatalf("sender lost a successful terminal exchange to normal receiver close: %v", err)
	}
	if err = <-receiverDone; err != nil {
		t.Fatalf("receiver completion failed: %v", err)
	}
}

func TestTerminalFlushRejectsNonzeroConnectionCloseOverQUIC(t *testing.T) {
	client, server := fixturePair(t, false, false)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	receiverDone := make(chan error, 1)
	go func() {
		_, acceptErr := server.AcceptStream(ctx)
		if acceptErr == nil {
			acceptErr = server.CloseWithError(42, "terminal failure")
		}
		receiverDone <- acceptErr
	}()

	stream, err := client.OpenStreamSync(ctx)
	if err != nil {
		t.Fatal(err)
	}
	err = WrapStream(stream).FlushTerminal(ctx)
	var applicationErr *quic.ApplicationError
	if !errors.As(err, &applicationErr) || !applicationErr.Remote || applicationErr.ErrorCode != 42 {
		t.Fatalf("nonzero connection close was not preserved: %v", err)
	}
	if err = <-receiverDone; err != nil {
		t.Fatalf("receiver close failed: %v", err)
	}
}
