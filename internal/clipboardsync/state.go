package clipboardsync

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"math"
	"sync"
	"time"
)

const (
	MaxTextBytes  = 64 << 10
	MaxImageBytes = 32 << 20
	MaxLeases     = 2
)

var (
	ErrExpired  = errors.New("CLIPBOARD_LEASE_EXPIRED")
	ErrStale    = errors.New("CLIPBOARD_EVENT_STALE")
	ErrConflict = errors.New("CLIPBOARD_COMMIT_CONFLICT")
	ErrLimit    = errors.New("CLIPBOARD_CONTENT_LIMIT")
)

type Kind string

const (
	Text  Kind = "text"
	Link  Kind = "link"
	Image Kind = "image"
)

type Lease struct {
	ID, PeerID, SessionID string
	Generation            uint64
	Kinds                 map[Kind]bool
	revisions             map[Kind]uint64
	issued, deadline      time.Time
	baseline              uint64
	outbound              bool
}
type Grant struct {
	Kind     Kind   `json:"kind"`
	Revision uint64 `json:"revision"`
}
type Event struct {
	LeaseID, OriginID, Boot string
	OriginSeq, Lamport      uint64
	Revision                uint64
	SenderGrantRevision     uint64
	ReceiverGrantRevision   uint64
	Kind                    Kind
	Digest                  string
	OSGeneration            uint64
	Payload                 []byte
}
type Candidate struct {
	Event
	PeerID, SessionID                string
	Generation, Revision, OSBaseline uint64
	Deadline                         time.Time
}
type orderKey struct {
	Lamport   uint64
	OriginID  string
	Boot      string
	OriginSeq uint64
}
type State struct {
	mu                            sync.Mutex
	now                           func() time.Time
	originID, boot                string
	seq, lamport, revision, osGen uint64
	paused                        bool
	leases                        map[string]Lease
	high                          map[string]uint64
	committed                     orderKey
	pending                       orderKey
	pendingRevision               uint64
	pendingDeadline               time.Time
	suppressedGeneration          uint64
	suppressedPayloadDigest       string
}

func New(originID string, now func() time.Time) *State {
	if now == nil {
		now = time.Now
	}
	var nonce [16]byte
	_, _ = rand.Read(nonce[:])
	return &State{originID: originID, boot: hex.EncodeToString(nonce[:]), now: now, leases: make(map[string]Lease), high: make(map[string]uint64)}
}

func (s *State) Issue(peerID, sessionID string, generation uint64, kinds []Kind, ttl time.Duration) (Lease, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	grants := make([]Grant, 0, len(kinds))
	for _, kind := range kinds {
		grants = append(grants, Grant{Kind: kind})
	}
	return s.issueLocked(peerID, sessionID, generation, grants, ttl)
}

func (s *State) IssueScoped(peerID, sessionID string, generation uint64, grants []Grant, ttl time.Duration) (Lease, error) {
	for _, grant := range grants {
		if grant.Revision == 0 {
			return Lease{}, ErrStale
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.issueLocked(peerID, sessionID, generation, grants, ttl)
}

func (s *State) issueLocked(peerID, sessionID string, generation uint64, grants []Grant, ttl time.Duration) (Lease, error) {
	if s.paused || peerID == "" || sessionID == "" || generation == 0 || ttl <= 0 {
		return Lease{}, ErrStale
	}
	s.pruneLocked()
	count := 0
	for _, lease := range s.leases {
		if !lease.outbound && lease.PeerID == peerID && lease.SessionID == sessionID {
			count++
		}
	}
	if count >= MaxLeases {
		return Lease{}, ErrLimit
	}
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return Lease{}, err
	}
	allowed, revisions := make(map[Kind]bool), make(map[Kind]uint64)
	for _, grant := range grants {
		if validKind(grant.Kind) {
			allowed[grant.Kind] = true
			revisions[grant.Kind] = grant.Revision
		}
	}
	if len(allowed) == 0 {
		return Lease{}, ErrStale
	}
	now := s.now()
	lease := Lease{ID: hex.EncodeToString(nonce[:]), PeerID: peerID, SessionID: sessionID, Generation: generation, Kinds: allowed, revisions: revisions, issued: now, deadline: now.Add(ttl), baseline: s.osGen}
	s.leases[lease.ID] = lease
	return lease, nil
}
func (s *State) Install(id, peerID, sessionID string, generation uint64, kinds []Kind, ttl time.Duration, baseline uint64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	grants := make([]Grant, 0, len(kinds))
	for _, kind := range kinds {
		grants = append(grants, Grant{Kind: kind})
	}
	return s.installLocked(id, peerID, sessionID, generation, grants, ttl, baseline)
}

func (s *State) InstallScoped(id, peerID, sessionID string, generation uint64, grants []Grant, ttl time.Duration, baseline uint64) error {
	for _, grant := range grants {
		if grant.Revision == 0 {
			return ErrStale
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.installLocked(id, peerID, sessionID, generation, grants, ttl, baseline)
}

func (s *State) installLocked(id, peerID, sessionID string, generation uint64, grants []Grant, ttl time.Duration, baseline uint64) error {
	if s.paused || id == "" || peerID == "" || sessionID == "" || generation == 0 || ttl <= 0 {
		return ErrStale
	}
	s.pruneLocked()
	allowed, revisions := make(map[Kind]bool), make(map[Kind]uint64)
	for _, grant := range grants {
		if validKind(grant.Kind) {
			allowed[grant.Kind] = true
			revisions[grant.Kind] = grant.Revision
		}
	}
	if len(allowed) == 0 {
		return ErrStale
	}
	count := 0
	for _, lease := range s.leases {
		if lease.outbound && lease.PeerID == peerID && lease.SessionID == sessionID {
			count++
		}
	}
	if count >= MaxLeases {
		return ErrLimit
	}
	now := s.now()
	s.leases[id] = Lease{ID: id, PeerID: peerID, SessionID: sessionID, Generation: generation, Kinds: allowed, revisions: revisions, issued: now, deadline: now.Add(ttl), baseline: baseline, outbound: true}
	return nil
}
func (l Lease) ExpiresAt() time.Time      { return l.deadline }
func (l Lease) Revision(kind Kind) uint64 { return l.revisions[kind] }

func (s *State) LocalChange(leaseID string, osGeneration uint64, kind Kind, payload []byte) (Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	base, err := s.prepareLocalLocked(osGeneration, kind, payload)
	if err != nil {
		return Event{}, err
	}
	return s.bindLocked(base, leaseID)
}

func (s *State) PrepareLocal(osGeneration uint64, kind Kind, payload []byte) (Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	base, err := s.observeLocalLocked(osGeneration)
	if err != nil {
		return Event{}, err
	}
	return s.prepareObservedLocked(base, kind, payload)
}

func (s *State) prepareLocalLocked(osGeneration uint64, kind Kind, payload []byte) (Event, error) {
	if err := validatePayload(kind, payload); err != nil {
		return Event{}, err
	}
	base, err := s.observeLocalLocked(osGeneration)
	if err != nil {
		return Event{}, err
	}
	return s.prepareObservedLocked(base, kind, payload)
}

func (s *State) ObserveLocalEvent(osGeneration uint64) (Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.observeLocalLocked(osGeneration)
}

func (s *State) observeLocalLocked(osGeneration uint64) (Event, error) {
	if s.paused || osGeneration <= s.osGen {
		return Event{}, ErrStale
	}
	if s.seq == math.MaxUint64 || s.lamport == math.MaxUint64 {
		return Event{}, ErrLimit
	}
	s.seq++
	s.lamport++
	s.osGen = osGeneration
	s.revision++
	s.pending = orderKey{}
	s.pendingRevision = 0
	s.pendingDeadline = time.Time{}
	event := Event{OriginID: s.originID, Boot: s.boot, OriginSeq: s.seq, Lamport: s.lamport, OSGeneration: osGeneration, Revision: s.revision}
	s.committed = event.key()
	return event, nil
}

func (s *State) PrepareObserved(base Event, kind Kind, payload []byte) (Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.prepareObservedLocked(base, kind, payload)
}

func (s *State) prepareObservedLocked(base Event, kind Kind, payload []byte) (Event, error) {
	if err := validatePayload(kind, payload); err != nil {
		return Event{}, err
	}
	payloadSum := sha256.Sum256(payload)
	if base.OSGeneration == s.suppressedGeneration && hex.EncodeToString(payloadSum[:]) == s.suppressedPayloadDigest {
		s.suppressedGeneration, s.suppressedPayloadDigest = 0, ""
		return Event{}, ErrStale
	}
	if s.paused || base.Revision != s.revision || base.OSGeneration != s.osGen || base.OriginID != s.originID || base.Boot != s.boot || base.OriginSeq != s.seq {
		return Event{}, ErrStale
	}
	base.Kind = kind
	base.Payload = append([]byte(nil), payload...)
	return base, nil
}

func (s *State) LocalCurrent(event Event) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return !s.paused && event.Revision == s.revision && event.OSGeneration == s.osGen && event.OriginID == s.originID && event.Boot == s.boot && event.OriginSeq == s.seq
}

func (s *State) Bind(base Event, leaseID string) (Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.bindLocked(base, leaseID)
}

func (s *State) BindScoped(base Event, leaseID string, senderRevision uint64) (Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	lease, ok := s.leases[leaseID]
	if !ok {
		return Event{}, ErrExpired
	}
	base.SenderGrantRevision = senderRevision
	base.ReceiverGrantRevision = lease.revisions[base.Kind]
	return s.bindLocked(base, leaseID)
}

func (s *State) bindLocked(base Event, leaseID string) (Event, error) {
	lease, ok := s.leases[leaseID]
	if !ok || !lease.outbound || s.paused || !s.now().Before(lease.deadline) {
		return Event{}, ErrExpired
	}
	if !lease.Kinds[base.Kind] || base.Revision != s.revision || base.OSGeneration <= lease.baseline || base.OSGeneration != s.osGen || base.OriginID != s.originID || base.Boot != s.boot || base.OriginSeq != s.seq {
		return Event{}, ErrStale
	}
	event := base
	event.LeaseID = leaseID
	event.Digest = eventDigest(event)
	return event, nil
}

func (s *State) Begin(peerID, sessionID string, generation uint64, event Event) (Candidate, error) {
	candidate, err := s.BeginHeader(peerID, sessionID, generation, event)
	if err != nil {
		return Candidate{}, err
	}
	return s.AttachPayload(candidate, event.Payload)
}

func (s *State) BeginHeader(peerID, sessionID string, generation uint64, event Event) (Candidate, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	lease, ok := s.leases[event.LeaseID]
	if !ok || lease.outbound || s.paused || !s.now().Before(lease.deadline) {
		return Candidate{}, ErrExpired
	}
	if lease.PeerID != peerID || lease.SessionID != sessionID || lease.Generation != generation || !lease.Kinds[event.Kind] {
		return Candidate{}, ErrStale
	}
	if event.ReceiverGrantRevision != lease.revisions[event.Kind] {
		return Candidate{}, ErrStale
	}
	if event.OriginID != peerID || event.Boot == "" || event.OriginSeq == 0 || event.Lamport == 0 || len(event.Digest) != 64 || event.OriginSeq <= s.high[event.OriginID+"/"+event.Boot] || compareOrder(event.key(), s.committed) <= 0 {
		return Candidate{}, ErrStale
	}
	if s.pendingRevision == s.revision && !s.now().Before(s.pendingDeadline) {
		s.pending = orderKey{}
		s.pendingRevision = 0
		s.pendingDeadline = time.Time{}
	}
	if s.pendingRevision == s.revision && compareOrder(event.key(), s.pending) <= 0 {
		return Candidate{}, ErrStale
	}
	s.pending = event.key()
	s.pendingRevision = s.revision
	s.pendingDeadline = lease.deadline
	event.Payload = nil
	return Candidate{Event: event, PeerID: peerID, SessionID: sessionID, Generation: generation, Revision: s.revision, OSBaseline: s.osGen, Deadline: lease.deadline}, nil
}

func (s *State) AttachPayload(candidate Candidate, payload []byte) (Candidate, error) {
	if err := validatePayload(candidate.Kind, payload); err != nil {
		return Candidate{}, err
	}
	candidate.Payload = append([]byte(nil), payload...)
	if candidate.Digest != eventDigest(candidate.Event) {
		return Candidate{}, ErrStale
	}
	return candidate, nil
}

func (s *State) Commit(candidate Candidate, write func(uint64, Kind, []byte) (uint64, error)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	lease, ok := s.leases[candidate.LeaseID]
	if !ok || lease.outbound || s.paused || !s.now().Before(lease.deadline) {
		return ErrExpired
	}
	if candidate.Revision != s.revision || candidate.OSBaseline != s.osGen || lease.PeerID != candidate.PeerID || lease.SessionID != candidate.SessionID || lease.Generation != candidate.Generation || compareOrder(candidate.Event.key(), s.committed) <= 0 {
		return ErrConflict
	}
	if s.pendingRevision == candidate.Revision && compareOrder(candidate.Event.key(), s.pending) != 0 {
		return ErrConflict
	}
	key := candidate.OriginID + "/" + candidate.Boot
	if candidate.OriginSeq <= s.high[key] {
		return ErrStale
	}
	if s.lamport == math.MaxUint64 || candidate.Lamport == math.MaxUint64 {
		return ErrLimit
	}
	newGeneration, err := write(s.osGen, candidate.Kind, append([]byte(nil), candidate.Payload...))
	if err != nil {
		return err
	}
	if newGeneration <= s.osGen {
		return ErrConflict
	}
	s.osGen = newGeneration
	s.revision++
	s.pending = orderKey{}
	s.pendingRevision = 0
	s.pendingDeadline = time.Time{}
	s.high[key] = candidate.OriginSeq
	s.committed = candidate.Event.key()
	payloadSum := sha256.Sum256(candidate.Payload)
	s.suppressedGeneration = newGeneration
	s.suppressedPayloadDigest = hex.EncodeToString(payloadSum[:])
	if candidate.Lamport >= s.lamport {
		s.lamport = candidate.Lamport + 1
	} else {
		s.lamport++
	}
	return nil
}

func (s *State) ObserveLocal(osGeneration uint64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.observeLocalLocked(osGeneration)
	return err
}

func (s *State) Reset(paused bool, osGeneration uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.paused = paused
	s.osGen = osGeneration
	s.revision++
	s.leases = make(map[string]Lease)
	s.pending = orderKey{}
	s.pendingRevision = 0
	s.pendingDeadline = time.Time{}
}

func (s *State) Outbound(peerID, sessionID string, generation uint64, kind Kind) (Lease, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneLocked()
	var selected Lease
	for _, lease := range s.leases {
		if lease.outbound && lease.PeerID == peerID && lease.SessionID == sessionID && lease.Generation == generation && lease.Kinds[kind] && (selected.ID == "" || lease.issued.After(selected.issued)) {
			selected = lease
		}
	}
	return selected, selected.ID != ""
}

func (s *State) HasOutbound(peerID, sessionID string, generation uint64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneLocked()
	for _, lease := range s.leases {
		if lease.outbound && lease.PeerID == peerID && lease.SessionID == sessionID && lease.Generation == generation {
			return true
		}
	}
	return false
}

func (s *State) DropSession(peerID, sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, lease := range s.leases {
		if lease.PeerID == peerID && (sessionID == "" || lease.SessionID == sessionID) {
			delete(s.leases, id)
		}
	}
}

func (s *State) InvalidateGrant(peerID string, outbound bool, kind Kind) {
	s.mu.Lock()
	defer s.mu.Unlock()
	changed := false
	for id, lease := range s.leases {
		if lease.PeerID != peerID || lease.outbound != outbound || !lease.Kinds[kind] {
			continue
		}
		changed = true
		delete(lease.Kinds, kind)
		delete(lease.revisions, kind)
		if len(lease.Kinds) == 0 {
			delete(s.leases, id)
		} else {
			s.leases[id] = lease
		}
	}
	if !changed {
		return
	}
	s.revision++
	s.pending = orderKey{}
	s.pendingRevision = 0
	s.pendingDeadline = time.Time{}
}
func (s *State) pruneLocked() {
	now := s.now()
	for id, lease := range s.leases {
		if !now.Before(lease.deadline) {
			delete(s.leases, id)
		}
	}
}
func validKind(kind Kind) bool { return kind == Text || kind == Link || kind == Image }
func validatePayload(kind Kind, payload []byte) error {
	if !validKind(kind) || len(payload) == 0 {
		return ErrStale
	}
	limit := MaxTextBytes
	if kind == Image {
		limit = MaxImageBytes
	}
	if len(payload) > limit {
		return ErrLimit
	}
	return nil
}

func (event Event) key() orderKey {
	return orderKey{Lamport: event.Lamport, OriginID: event.OriginID, Boot: event.Boot, OriginSeq: event.OriginSeq}
}

func compareOrder(left, right orderKey) int {
	if left.Lamport < right.Lamport {
		return -1
	}
	if left.Lamport > right.Lamport {
		return 1
	}
	if left.OriginID < right.OriginID {
		return -1
	}
	if left.OriginID > right.OriginID {
		return 1
	}
	if left.Boot < right.Boot {
		return -1
	}
	if left.Boot > right.Boot {
		return 1
	}
	if left.OriginSeq < right.OriginSeq {
		return -1
	}
	if left.OriginSeq > right.OriginSeq {
		return 1
	}
	return 0
}

func eventDigest(event Event) string {
	h := sha256.New()
	var number [8]byte
	for _, value := range []string{event.LeaseID, event.OriginID, event.Boot, string(event.Kind)} {
		binary.BigEndian.PutUint64(number[:], uint64(len(value)))
		h.Write(number[:])
		h.Write([]byte(value))
	}
	for _, value := range []uint64{event.OriginSeq, event.Lamport, event.OSGeneration, event.SenderGrantRevision, event.ReceiverGrantRevision} {
		binary.BigEndian.PutUint64(number[:], value)
		h.Write(number[:])
	}
	binary.BigEndian.PutUint64(number[:], uint64(len(event.Payload)))
	h.Write(number[:])
	h.Write(event.Payload)
	return hex.EncodeToString(h.Sum(nil))
}
