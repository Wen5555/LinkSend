package clipboardnativecontract

import (
	"context"
	"encoding/hex"
	"sync"
	"time"
)

// PublicationLifetime owns process-stop policy, never native reference counts
// or a quiescent/deletion capability. Only the native source-close gate may
// establish those facts. ShellPublisher is a separate, not-yet-wired contract.
type PublicationLifetime struct {
	mu                    sync.Mutex
	shell                 bool
	admissionDeadline     time.Time
	authorization         string
	exportPossible        bool
	publicationGeneration uint64
}

func NewControlledLifetime(deadline time.Time) *PublicationLifetime {
	return &PublicationLifetime{admissionDeadline: deadline}
}

// The deadline limits initial admission only. Once export is possible a Shell
// producer has no expiry and may outlive its launcher, IPC and application.
func NewShellLifetime(deadline time.Time) *PublicationLifetime {
	return &PublicationLifetime{shell: true, admissionDeadline: deadline}
}

// Authorize records the digest only after the caller re-read its real durable
// signed delegation. This method itself is not a signature/identity verifier.
func (l *PublicationLifetime) Authorize(digest string) error {
	if l == nil {
		return ErrContract
	}
	b, err := hex.DecodeString(digest)
	if err != nil || len(b) != 32 || hex.EncodeToString(b) != digest {
		return ErrContract
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.authorization != "" || l.exportPossible {
		return ErrContract
	}
	l.authorization = digest
	return nil
}

// CommitMayStart is irreversible and must precede the first control write.
// Even a failed/short write may have reached the helper; an unacknowledged
// commit must therefore preserve it. Neither retry nor cancellation resets it.
func (l *PublicationLifetime) CommitMayStart(digest string, now time.Time) error {
	if l == nil {
		return ErrContract
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.authorization == "" || digest != l.authorization || l.exportPossible {
		return ErrContract
	}
	if l.admissionDeadline.IsZero() || !now.Before(l.admissionDeadline) {
		return context.DeadlineExceeded
	}
	l.exportPossible = true
	return nil
}

// AcknowledgeGeneration stores one actual publication result after the caller
// validated its native signed ack. Watcher events cannot manufacture this ack
// or replace it with a later clipboard generation.
func (l *PublicationLifetime) AcknowledgeGeneration(generation uint64) error {
	if l == nil || generation == 0 {
		return ErrContract
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.exportPossible || l.publicationGeneration != 0 {
		return ErrContract
	}
	l.publicationGeneration = generation
	return nil
}

func (l *PublicationLifetime) MayTerminate() bool {
	if l == nil {
		return false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return !l.exportPossible
}

// Controlled probes keep their fixed test deadline. This policy must never
// be copied to the future system Shell producer after export may have begun.
func (l *PublicationLifetime) NativeDeadline() (time.Time, bool) {
	if l == nil {
		return time.Time{}, false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.shell && l.exportPossible {
		return time.Time{}, false
	}
	return l.admissionDeadline, !l.admissionDeadline.IsZero()
}
