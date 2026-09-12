package app

import (
	"context"
	"errors"
	"time"

	"github.com/Wen5555/LinkSend/internal/protocol"
)

func (s *Service) isClosing() bool {
	return s.closing.Load()
}

// ShutdownContext establishes the dispatch barrier synchronously, then joins
// all owners. Timeout keeps the profile locked while cleanup continues.
func (s *Service) ShutdownContext(ctx context.Context) error {
	s.shutdownOnce.Do(func() {
		s.operationMu.Lock()
		s.closing.Store(true)
		s.shutdownDone = make(chan struct{})
		s.operationMu.Unlock()
		go s.shutdownOwners()
	})
	select {
	case <-s.shutdownDone:
		return s.tasks.historyError()
	case <-ctx.Done():
		return errors.Join(errors.New("SHUTDOWN_TIMEOUT"), ctx.Err())
	}
}

func (s *Service) shutdownOwners() {
	defer close(s.shutdownDone)
	s.inbox.mu.Lock()
	s.inbox.shutdown = true
	s.inbox.mu.Unlock()
	if s.workCancel != nil {
		s.workCancel()
	}
	s.stopQueue()
	// Set intent before cancelling parent listeners, including inbound tasks.
	s.tasks.mu.RLock()
	for _, task := range s.tasks.tasks {
		task.mu.Lock()
		cancel := task.cancel
		if cancel == nil || isTerminal(task.snap.State) {
			task.mu.Unlock()
			continue
		}
		if task.snap.State != "cancel_requested" && task.snap.State != "pause_requested" {
			task.snap.State = "shutdown_requested"
			task.snap.Phase = "saving_before_exit"
			task.snap.CanPause, task.snap.CanCancel = false, false
			task.snap.Revision++
			task.snap.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		}
		snap, recovery := task.snap, task.recovery
		// Publish the control intent and cancellation as one observable step.
		// Persisting first leaves a window where observers see shutdown_requested
		// but a slow SQLite commit has not yet cancelled the transfer context.
		cancel()
		task.mu.Unlock()
		task.save(snap, recovery)
	}
	s.tasks.mu.RUnlock()
	_ = s.stopInbox(true)
	_ = s.stopLANDiscovery()
	s.listenerWorkers.Wait()
	s.tasks.workers.Wait()
	s.content.mu.Lock()
	if s.content.store != nil {
		_ = s.content.store.Close()
	}
	s.content.mu.Unlock()
	if s.store != nil {
		_ = s.store.Close()
	}
	if s.profileLock != nil {
		_ = s.profileLock.Close()
	}
}

func shutdownTask(t *taskRecord, attemptID string, err error) {
	if recoveryUsable(t.recoverySnapshot()) {
		t.recoverAttempt(attemptID, err)
	} else {
		t.finishAttempt(attemptID, "failed", protocol.Wrap(protocol.ConnectionInterrupted, "application exited before recovery data was ready", err))
	}
}
