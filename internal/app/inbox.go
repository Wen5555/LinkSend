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
	"github.com/Wen5555/LinkSend/internal/signaling"
	"github.com/Wen5555/LinkSend/internal/transfer"
	"github.com/Wen5555/LinkSend/internal/transport"
)

// InboxStatus describes the persistent receiver without creating a visible
// transfer task merely because the application is idle.
type InboxStatus struct {
	Enabled            bool   `json:"enabled"`
	SignalingConnected bool   `json:"signaling_connected"`
	Listening          bool   `json:"listening"`
	Directory          string `json:"directory,omitempty"`
	ConnectedAt        string `json:"connected_at,omitempty"`
	ConnectionCount    uint64 `json:"connection_count"`
	LastError          string `json:"last_error,omitempty"`
	LANAvailable       bool   `json:"lan_available"`
	LANPeerCount       int    `json:"lan_peer_count"`
	LANLastError       string `json:"lan_last_error,omitempty"`
}

type inboxManager struct {
	mu              sync.Mutex
	enabled         bool
	connected       bool
	listening       bool
	directory       string
	connectedAt     string
	connectionCount uint64
	cfg             DirectConfig
	cancel          context.CancelFunc
	done            chan struct{}
	preserve        bool
	spare           *signaling.Session
	shutdown        bool
	lastError       string
}

// StartInbox keeps this device reachable while the app is open. It replaces
// the old user-visible "start receiving" task with a quiet background listener.
func (s *Service) StartInbox(directory string, cfg DirectConfig) error {
	if s.isClosing() {
		return errors.New("APP_CLOSING")
	}
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
	s.operationMu.Lock()
	defer s.operationMu.Unlock()
	if s.isClosing() {
		return errors.New("APP_CLOSING")
	}
	s.inbox.mu.Lock()
	s.inbox.enabled = true
	s.inbox.directory = directory
	s.inbox.cfg = cfg
	s.inbox.lastError = ""
	s.inbox.mu.Unlock()
	s.prewarmNetwork(cfg)
	s.startLANDiscovery(directory, cfg)
	s.ensureInbox()
	return nil
}

func (s *Service) StopInbox() error { return errors.Join(s.stopInbox(true), s.stopLANDiscovery()) }

func (s *Service) InboxStatus() InboxStatus {
	s.inbox.mu.Lock()
	status := InboxStatus{Enabled: s.inbox.enabled, SignalingConnected: s.inbox.connected, Listening: s.inbox.listening, Directory: s.inbox.directory, ConnectedAt: s.inbox.connectedAt, ConnectionCount: s.inbox.connectionCount, LastError: s.inbox.lastError}
	s.inbox.mu.Unlock()
	status.LANAvailable, status.LANPeerCount, status.LANLastError = s.lanStatus()
	return status
}

func (s *Service) stopInbox(disable bool) error {
	s.inbox.mu.Lock()
	var spare *signaling.Session
	if disable {
		s.inbox.enabled = false
		s.inbox.preserve = false
		spare, s.inbox.spare = s.inbox.spare, nil
	} else {
		s.inbox.preserve = true
	}
	cancel, done := s.inbox.cancel, s.inbox.done
	if cancel == nil || done == nil {
		s.inbox.connected = false
		s.inbox.listening = false
		s.inbox.mu.Unlock()
		if spare != nil {
			_ = spare.Close()
		}
		return nil
	}
	cancel()
	s.inbox.mu.Unlock()

	select {
	case <-done:
		if spare != nil {
			_ = spare.Close()
		}
		return nil
	case <-time.After(5 * time.Second):
		return errors.New("INBOX_STOP_TIMEOUT: background receiver did not stop")
	}
}

func (s *Service) takeSpareSignal() *signaling.Session {
	s.inbox.mu.Lock()
	defer s.inbox.mu.Unlock()
	session := s.inbox.spare
	s.inbox.spare = nil
	return session
}

func (s *Service) keepSpareSignal(session *signaling.Session) bool {
	if session == nil {
		return false
	}
	s.inbox.mu.Lock()
	defer s.inbox.mu.Unlock()
	if !s.inbox.enabled || s.inbox.shutdown || s.inbox.spare != nil {
		return false
	}
	s.inbox.spare = session
	return true
}

func (s *Service) ensureInbox() {
	if s.tasks.hasActive() {
		return
	}
	s.ensureLANDiscovery()
	s.inbox.mu.Lock()
	defer s.inbox.mu.Unlock()
	if !s.inbox.enabled || s.inbox.shutdown || s.isClosing() || s.inbox.cancel != nil || strings.TrimSpace(s.inbox.directory) == "" {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	s.inbox.cancel = cancel
	s.inbox.done = done
	s.inbox.listening = false
	s.listenerWorkers.Add(1)
	go func(directory string, cfg DirectConfig) {
		defer s.listenerWorkers.Done()
		s.runInbox(ctx, done, directory, cfg)
	}(s.inbox.directory, s.inbox.cfg)
}

func (s *Service) runInbox(ctx context.Context, done chan struct{}, directory string, cfg DirectConfig) {
	defer func() {
		s.inbox.mu.Lock()
		if s.inbox.done == done {
			s.inbox.cancel = nil
			s.inbox.done = nil
			s.inbox.connected = false
			s.inbox.listening = false
		}
		s.inbox.mu.Unlock()
		close(done)
	}()

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

	backoff := 500 * time.Millisecond
	for ctx.Err() == nil {
		phaseBudget := listenerCfg.CheckTimeout
		if phaseBudget <= 0 {
			phaseBudget = 20 * time.Second
		}
		client, err := s.client()
		if err == nil {
			signalSession := s.takeSpareSignal()
			if signalSession == nil {
				connectCtx, cancelConnect := context.WithTimeout(ctx, phaseBudget)
				signalSession, err = client.Connect(connectCtx)
				cancelConnect()
			}
			if err == nil {
				stopHeartbeat := startHeartbeat(ctx, signalSession)
				backoff = 500 * time.Millisecond
				s.inbox.mu.Lock()
				if s.inbox.done == done {
					s.inbox.connected = true
					s.inbox.connectedAt = time.Now().UTC().Format(time.RFC3339Nano)
					s.inbox.connectionCount++
					s.inbox.lastError = ""
				}
				s.inbox.mu.Unlock()
				for ctx.Err() == nil {
					var peer *PeerSession
					peer, err = s.acceptDirectOnSession(ctx, "", listenerCfg, signalSession)
					if err != nil {
						break
					}
					s.inbox.mu.Lock()
					s.inbox.listening = false
					s.inbox.mu.Unlock()
					if s.receiveIncoming(ctx, peer, directory) {
						_ = s.closePeerAfterTransfer(peer)
					} else {
						_ = peer.Close()
					}
				}
				stopHeartbeat()
				parked := false
				s.inbox.mu.Lock()
				if ctx.Err() != nil && s.inbox.done == done && s.inbox.preserve && s.inbox.spare == nil {
					s.inbox.spare = signalSession
					s.inbox.preserve = false
					parked = true
				}
				s.inbox.mu.Unlock()
				if !parked {
					_ = signalSession.Close()
				}
				s.inbox.mu.Lock()
				if s.inbox.done == done {
					s.inbox.connected = false
					s.inbox.listening = false
				}
				s.inbox.mu.Unlock()
			}
		}
		if ctx.Err() != nil {
			return
		}
		if err == nil {
			err = protocol.Fail(protocol.SignalingUnreachable, "persistent inbox signaling ended")
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

func (s *Service) receiveIncoming(inboxCtx context.Context, peer *PeerSession, directory string) bool {
	if err := s.checkPeerAllowed(peer.PeerID); err != nil {
		return false
	}
	var policyErr error
	directory, policyErr = s.receiveDirectory(peer.PeerID, directory)
	if policyErr != nil {
		s.inbox.mu.Lock()
		s.inbox.lastError = "RECEIVE_DIRECTORY_UNAVAILABLE"
		s.inbox.mu.Unlock()
		return false
	}
	ctx, cancel := context.WithCancel(inboxCtx)
	s.operationMu.Lock()
	if s.isClosing() {
		s.operationMu.Unlock()
		cancel()
		return false
	}
	t, err := s.tasks.create(TaskSnapshot{Direction: "receive", PeerID: peer.PeerID, TargetDirectory: directory}, cancel)
	if err != nil {
		s.operationMu.Unlock()
		cancel()
		return false
	}
	s.inbox.mu.Lock()
	t.cfg = s.inbox.cfg
	s.inbox.mu.Unlock()
	t.decision = make(chan bool, 1)
	s.tasks.workers.Add(1)
	s.operationMu.Unlock()
	defer s.tasks.workers.Done()
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
		return false
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
		if s.checkPeerAllowed(peer.PeerID) != nil {
			return false
		}
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
		return false
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
	return true
}
