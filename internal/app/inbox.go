package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Wen5555/LinkSend/internal/protocol"
	"github.com/Wen5555/LinkSend/internal/transfer"
	"github.com/Wen5555/LinkSend/internal/transport"
)

// InboxStatus describes the persistent receiver without creating a visible
// transfer task merely because the application is idle.
type InboxStatus struct {
	Enabled   bool   `json:"enabled"`
	Listening bool   `json:"listening"`
	Directory string `json:"directory,omitempty"`
	LastError string `json:"last_error,omitempty"`
}

type inboxManager struct {
	mu        sync.Mutex
	enabled   bool
	listening bool
	directory string
	cfg       DirectConfig
	cancel    context.CancelFunc
	done      chan struct{}
	shutdown  bool
	lastError string
}

// StartInbox keeps this device reachable while the app is open. It replaces
// the old user-visible "start receiving" task with a quiet background listener.
func (s *Service) StartInbox(directory string, cfg DirectConfig) error {
	directory = filepath.Clean(strings.TrimSpace(directory))
	if directory == "." || directory == "" {
		return errors.New("INVALID_ARGUMENT: receive directory is required")
	}
	if err := os.MkdirAll(directory, 0700); err != nil {
		return classifyTaskError(err)
	}
	if err := s.stopInbox(false); err != nil {
		return err
	}
	s.inbox.mu.Lock()
	s.inbox.enabled = true
	s.inbox.directory = directory
	s.inbox.cfg = cfg
	s.inbox.lastError = ""
	s.inbox.mu.Unlock()
	s.ensureInbox()
	return nil
}

func (s *Service) StopInbox() error { return s.stopInbox(true) }

func (s *Service) InboxStatus() InboxStatus {
	s.inbox.mu.Lock()
	defer s.inbox.mu.Unlock()
	return InboxStatus{Enabled: s.inbox.enabled, Listening: s.inbox.listening, Directory: s.inbox.directory, LastError: s.inbox.lastError}
}

func (s *Service) stopInbox(disable bool) error {
	s.inbox.mu.Lock()
	if disable {
		s.inbox.enabled = false
	}
	cancel, done := s.inbox.cancel, s.inbox.done
	if cancel == nil || done == nil {
		s.inbox.listening = false
		s.inbox.mu.Unlock()
		return nil
	}
	cancel()
	s.inbox.mu.Unlock()

	select {
	case <-done:
		return nil
	case <-time.After(5 * time.Second):
		return errors.New("INBOX_STOP_TIMEOUT: background receiver did not stop")
	}
}

func (s *Service) ensureInbox() {
	if s.tasks.hasActive() {
		return
	}
	s.inbox.mu.Lock()
	defer s.inbox.mu.Unlock()
	if !s.inbox.enabled || s.inbox.shutdown || s.inbox.cancel != nil || strings.TrimSpace(s.inbox.directory) == "" {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	s.inbox.cancel = cancel
	s.inbox.done = done
	s.inbox.listening = false
	go s.runInbox(ctx, done, s.inbox.directory, s.inbox.cfg)
}

func (s *Service) runInbox(ctx context.Context, done chan struct{}, directory string, cfg DirectConfig) {
	defer func() {
		s.inbox.mu.Lock()
		if s.inbox.done == done {
			s.inbox.cancel = nil
			s.inbox.done = nil
			s.inbox.listening = false
		}
		s.inbox.mu.Unlock()
		close(done)
	}()

	backoff := 500 * time.Millisecond
	for ctx.Err() == nil {
		listenerCfg := cfg
		listenerCfg.WaitTimeout = 30 * time.Minute
		listenerCfg.onSession = nil
		listenerCfg.onEvidence = nil
		listenerCfg.onChunkSent = nil
		listenerCfg.onPhase = func(phase string) {
			s.inbox.mu.Lock()
			if s.inbox.done == done {
				s.inbox.listening = phase == "waiting"
				if s.inbox.listening {
					s.inbox.lastError = ""
				}
			}
			s.inbox.mu.Unlock()
		}
		peer, err := s.AcceptDirect(ctx, "", listenerCfg)
		if err == nil {
			backoff = 500 * time.Millisecond
			s.inbox.mu.Lock()
			s.inbox.listening = false
			s.inbox.mu.Unlock()
			s.receiveIncoming(ctx, peer, directory)
			_ = peer.Close()
			continue
		}
		if ctx.Err() != nil {
			return
		}
		s.inbox.mu.Lock()
		s.inbox.listening = false
		s.inbox.lastError = string(protocol.ErrorCode(err))
		s.inbox.mu.Unlock()
		timer := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		if backoff < 10*time.Second {
			backoff *= 2
		}
	}
}

func (s *Service) receiveIncoming(inboxCtx context.Context, peer *PeerSession, directory string) {
	ctx, cancel := context.WithCancel(inboxCtx)
	t, err := s.tasks.create(TaskSnapshot{Direction: "receive", PeerID: peer.PeerID, TargetDirectory: directory}, cancel)
	if err != nil {
		cancel()
		return
	}
	t.cfg = s.inbox.cfg
	t.decision = make(chan bool, 1)
	attemptID := t.snapshot().AttemptID
	t.updateAttempt(attemptID, func(v *TaskSnapshot) {
		v.SessionID = peer.SessionID
		v.ICEGeneration = firstGeneration
		v.Phase = "connected"
		applyDirectEvidence(v, peer.Evidence())
	})

	stream, err := peer.Data.Conn.AcceptStream(ctx)
	if err != nil {
		handleTaskRunError(t, attemptID, ctx, err)
		cancel()
		return
	}
	var lastReceived int64
	result, runErr := transfer.Receive(ctx, transport.WrapStream(stream), directory, peer.PeerID, func(m transfer.Manifest) bool {
		n := m.TotalBytes()
		t.updateRecordAttempt(attemptID, func(v *TaskSnapshot, recovery *taskRecovery) {
			v.State = "awaiting_acceptance"
			v.Phase = "awaiting_acceptance"
			v.TotalBytes = &n
			v.FileCount = len(m.Files)
			v.ManifestSummary = manifestSummary(m)
			v.TransferID = m.TransferID
			v.ManifestDigest = m.Digest()
			v.ChunkSize = m.ChunkSize
			v.CanCancel = true
			v.CanPause = false
			v.RestartRecoverySupported = v.HistoryPersisted
			v.ByteResumeSupported = true
			recovery.Direction = "receive"
			recovery.PeerID = peer.PeerID
			recovery.PeerFingerprint = peer.PeerID
			recovery.TargetDirectory = directory
			recovery.TransferID = m.TransferID
			recovery.ManifestDigest = m.Digest()
			recovery.ChunkSize = m.ChunkSize
			recovery.TotalBytes = n
			recovery.FileCount = len(m.Files)
		})
		if s.alwaysAccept(peer.PeerID) {
			return true
		}
		select {
		case accepted := <-t.decision:
			return accepted
		case <-ctx.Done():
			return false
		}
	}, func(p transfer.Progress) {
		if p.Received >= lastReceived {
			t.recordReceived(attemptID, p.Received-lastReceived)
			lastReceived = p.Received
		}
		t.progress(attemptID, p)
	})
	if runErr != nil {
		handleTaskRunError(t, attemptID, ctx, runErr)
		cancel()
		return
	}
	t.updateRecordAttempt(attemptID, func(v *TaskSnapshot, saved *taskRecovery) {
		v.TransferID = result.TransferID

		v.Phase = "completed"
		v.ProcessedBytes = result.Bytes
		v.VerifiedBytes = result.Bytes
		v.ReceivedBytes = result.Bytes
		v.CommittedBytes = result.Bytes
		v.CommittedFiles = v.FileCount
		v.BilateralConfirmed = true
		saved.LogicalCompleted = result.Bytes
		n := result.Bytes
		v.TotalBytes = &n
	})
	t.finishAttempt(attemptID, "completed", nil)
	cancel()
}
