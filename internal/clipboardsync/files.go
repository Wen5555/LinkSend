package clipboardsync

import (
	"math"
	"time"
)

// FileReservation admits a fresh, leased offer before expensive file hashing
// or transport. It never extends an ordinary text/image event's lifetime.
// A later local/remote copy, reset or grant revocation invalidates its revision.
type FileReservation struct {
	Candidate
	OwnerRevision uint64
}

func (s *State) ReserveFiles(candidate Candidate) (FileReservation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	lease, ok := s.leases[candidate.LeaseID]
	if !ok || lease.outbound || s.paused || !s.now().Before(lease.deadline) {
		return FileReservation{}, ErrExpired
	}
	if candidate.Kind != Files || !lease.Kinds[Files] || candidate.ReceiverGrantRevision != lease.revisions[Files] || candidate.Digest != eventDigest(candidate.Event) ||
		candidate.Revision != s.revision || candidate.OSBaseline != s.osGen ||
		lease.PeerID != candidate.PeerID || lease.SessionID != candidate.SessionID || lease.Generation != candidate.Generation ||
		compareOrder(candidate.Event.key(), s.committed) <= 0 ||
		s.pendingRevision != candidate.Revision || compareOrder(candidate.Event.key(), s.pending) != 0 {
		return FileReservation{}, ErrConflict
	}
	key := candidate.OriginID + "/" + candidate.Boot
	if candidate.OriginSeq <= s.high[key] || s.lamport == math.MaxUint64 || candidate.Lamport == math.MaxUint64 {
		return FileReservation{}, ErrStale
	}
	s.revision++
	s.high[key] = candidate.OriginSeq
	s.committed = candidate.Event.key()
	s.pending, s.pendingRevision, s.pendingDeadline = orderKey{}, 0, time.Time{}
	s.lamport = max(s.lamport, candidate.Lamport) + 1
	// Only an admitted files offer gets this bounded transfer reservation.
	// Native mutation still requires the same OS baseline and owner revision.
	candidate.Deadline = s.now().Add(24 * time.Hour)
	return FileReservation{Candidate: candidate, OwnerRevision: s.revision}, nil
}

func (s *State) FilesCurrent(reservation FileReservation) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.filesCurrentLocked(reservation)
}

func (s *State) filesCurrentLocked(r FileReservation) bool {
	return r.Kind == Files && !s.paused && s.now().Before(r.Deadline) &&
		r.OwnerRevision == s.revision && r.OSBaseline == s.osGen && compareOrder(r.Event.key(), s.committed) == 0
}

func (s *State) CommitFiles(reservation FileReservation, write func(uint64) (uint64, error)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.filesCurrentLocked(reservation) {
		return ErrConflict
	}
	next, err := write(s.osGen)
	if err != nil {
		return err
	}
	if next <= s.osGen {
		return ErrConflict
	}
	s.osGen = next
	s.revision++
	return nil
}
