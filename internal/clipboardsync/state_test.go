package clipboardsync

import (
	"errors"
	"testing"
	"time"
)

func TestLeaseCausalityExpiryAndNoRebinding(t *testing.T) {
	now := time.Unix(100, 0)
	receiver := New("b", func() time.Time { return now })
	sender := New("a", func() time.Time { return now })
	receiver.Reset(false, 20)
	sender.Reset(false, 10)
	lease, err := receiver.Issue("a", "session", 3, []Kind{Text}, 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if err = sender.Install(lease.ID, "b", "session", 3, []Kind{Text}, 10*time.Second, 10); err != nil {
		t.Fatal(err)
	}
	if _, err = sender.LocalChange(lease.ID, 10, Text, []byte("old")); !errors.Is(err, ErrStale) {
		t.Fatal("baseline event accepted", err)
	}
	event, err := sender.LocalChange(lease.ID, 11, Text, []byte("new"))
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(11 * time.Second)
	if _, err = receiver.Begin("a", "session", 3, event); !errors.Is(err, ErrExpired) {
		t.Fatal("expired event accepted", err)
	}
	newLease, err := receiver.Issue("a", "session", 3, []Kind{Text}, 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	event.LeaseID = newLease.ID
	if _, err = receiver.Begin("a", "session", 3, event); !errors.Is(err, ErrStale) {
		t.Fatal("old event rebound to new lease", err)
	}
}

func TestCommitRevisionLocalPriorityAndHighWater(t *testing.T) {
	now := time.Unix(200, 0)
	s := New("receiver", func() time.Time { return now })
	s.Reset(false, 7)
	lease, _ := s.Issue("peer", "session", 2, []Kind{Text}, 10*time.Second)
	event := Event{LeaseID: lease.ID, OriginID: "peer", Boot: "boot", OriginSeq: 1, Lamport: 4, Kind: Text, Payload: []byte("remote")}
	event.Digest = eventDigest(event)
	candidate, err := s.Begin("peer", "session", 2, event)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.ObserveLocal(8); err != nil {
		t.Fatal(err)
	}
	if err = s.Commit(candidate, func(uint64, Kind, []byte) (uint64, error) { return 9, nil }); !errors.Is(err, ErrConflict) {
		t.Fatal("local copy did not invalidate candidate", err)
	}
	event.OriginSeq = 2
	event.Digest = eventDigest(event)
	candidate, _ = s.Begin("peer", "session", 2, event)
	if err = s.Commit(candidate, func(expected uint64, _ Kind, _ []byte) (uint64, error) { return expected + 1, nil }); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Begin("peer", "session", 2, event); !errors.Is(err, ErrStale) {
		t.Fatal("duplicate event accepted", err)
	}
}

func TestResetAndLimits(t *testing.T) {
	s := New("a", nil)
	s.Reset(false, 1)
	if err := s.Install("one", "b", "s", 1, []Kind{Image}, time.Minute, 1); err != nil {
		t.Fatal(err)
	}
	first := Lease{ID: "one"}
	if _, err := s.LocalChange(first.ID, 2, Image, make([]byte, MaxImageBytes+1)); !errors.Is(err, ErrLimit) {
		t.Fatal(err)
	}
	if err := s.Install("two", "b", "s", 1, []Kind{Text}, time.Minute, 1); err != nil {
		t.Fatal(err)
	}
	if err := s.Install("three", "b", "s", 1, []Kind{Link}, time.Minute, 1); !errors.Is(err, ErrLimit) {
		t.Fatal("lease cap ignored", err)
	}
	s.Reset(true, 3)
	if _, err := s.LocalChange(first.ID, 4, Image, []byte{1}); !errors.Is(err, ErrStale) {
		t.Fatal("reset lease survived", err)
	}
}

func TestHeaderPrecheckPinsRevisionBeforeBody(t *testing.T) {
	now := time.Unix(300, 0)
	receiver := New("receiver", func() time.Time { return now })
	receiver.Reset(false, 4)
	lease, _ := receiver.Issue("peer", "session", 2, []Kind{Text}, 10*time.Second)
	event := Event{LeaseID: lease.ID, OriginID: "peer", Boot: "boot", OriginSeq: 1, Lamport: 1, Kind: Text, OSGeneration: 9, Payload: []byte("delayed")}
	event.Digest = eventDigest(event)
	header := event
	header.Payload = nil
	candidate, err := receiver.BeginHeader("peer", "session", 2, header)
	if err != nil {
		t.Fatal(err)
	}
	if err = receiver.ObserveLocal(5); err != nil {
		t.Fatal(err)
	}
	candidate, err = receiver.AttachPayload(candidate, event.Payload)
	if err != nil {
		t.Fatal(err)
	}
	if err = receiver.Commit(candidate, func(uint64, Kind, []byte) (uint64, error) { return 6, nil }); !errors.Is(err, ErrConflict) {
		t.Fatal("body crossed local-copy revision", err)
	}
}

func TestFanoutSharesOriginSequenceAndLoopWriteIsSuppressed(t *testing.T) {
	now := time.Unix(400, 0)
	sender := New("sender", func() time.Time { return now })
	sender.Reset(false, 10)
	if err := sender.Install("l1", "p1", "s1", 1, []Kind{Text}, 10*time.Second, 10); err != nil {
		t.Fatal(err)
	}
	if err := sender.Install("l2", "p2", "s2", 1, []Kind{Text}, 10*time.Second, 10); err != nil {
		t.Fatal(err)
	}
	base, err := sender.PrepareLocal(11, Text, []byte("same copy"))
	if err != nil {
		t.Fatal(err)
	}
	one, _ := sender.Bind(base, "l1")
	two, _ := sender.Bind(base, "l2")
	if one.OriginSeq != two.OriginSeq || one.Lamport != two.Lamport || one.Digest == two.Digest {
		t.Fatalf("fanout identities: %+v %+v", one, two)
	}
	receiver := New("receiver", func() time.Time { return now })
	receiver.Reset(false, 20)
	lease, _ := receiver.Issue("sender", "session", 1, []Kind{Text}, 10*time.Second)
	event := Event{LeaseID: lease.ID, OriginID: "sender", Boot: "boot", OriginSeq: 1, Lamport: 2, Kind: Text, OSGeneration: 11, Payload: []byte("remote")}
	event.Digest = eventDigest(event)
	candidate, _ := receiver.Begin("sender", "session", 1, event)
	if err = receiver.Commit(candidate, func(uint64, Kind, []byte) (uint64, error) { return 21, nil }); err != nil {
		t.Fatal(err)
	}
	if _, err = receiver.PrepareLocal(21, Text, []byte("remote")); !errors.Is(err, ErrStale) {
		t.Fatal("remote write looped", err)
	}
	if _, err = receiver.PrepareLocal(22, Text, []byte("remote")); err != nil {
		t.Fatal("real same-text recopy suppressed", err)
	}
}

func TestLeaseRenewalWindowIsBoundedPerDirection(t *testing.T) {
	now := time.Unix(500, 0)
	state := New("a", func() time.Time { return now })
	state.Reset(false, 1)
	if _, err := state.Issue("b", "s", 1, []Kind{Text}, 10*time.Second); err != nil {
		t.Fatal(err)
	}
	now = now.Add(7 * time.Second)
	if _, err := state.Issue("b", "s", 1, []Kind{Text}, 10*time.Second); err != nil {
		t.Fatal(err)
	}
	if _, err := state.Issue("b", "s", 1, []Kind{Text}, 10*time.Second); !errors.Is(err, ErrLimit) {
		t.Fatal("third inbound lease accepted", err)
	}
	now = now.Add(7 * time.Second)
	if _, err := state.Issue("b", "s", 1, []Kind{Text}, 10*time.Second); err != nil {
		t.Fatal("renewal gap after oldest expiry", err)
	}
}

func TestSameRevisionCandidatesChooseStableOrderAndKeepOriginalDeadline(t *testing.T) {
	now := time.Unix(600, 0)
	state := New("receiver", func() time.Time { return now })
	state.Reset(false, 1)
	lowLease, _ := state.Issue("a", "sa", 1, []Kind{Text}, 10*time.Second)
	highLease, _ := state.Issue("z", "sz", 1, []Kind{Text}, 10*time.Second)
	makeEvent := func(lease Lease, origin, body string) Event {
		event := Event{LeaseID: lease.ID, OriginID: origin, Boot: "boot", OriginSeq: 1, Lamport: 5, Kind: Text, Payload: []byte(body)}
		event.Digest = eventDigest(event)
		return event
	}
	low, err := state.Begin("a", "sa", 1, makeEvent(lowLease, "a", "low"))
	if err != nil {
		t.Fatal(err)
	}
	high, err := state.Begin("z", "sz", 1, makeEvent(highLease, "z", "high"))
	if err != nil {
		t.Fatal(err)
	}
	if err = state.Commit(low, func(uint64, Kind, []byte) (uint64, error) { return 2, nil }); !errors.Is(err, ErrConflict) {
		t.Fatal("lower stable candidate won after higher header", err)
	}
	if err = state.Commit(high, func(expected uint64, _ Kind, _ []byte) (uint64, error) { return expected + 1, nil }); err != nil {
		t.Fatal("higher stable candidate did not win", err)
	}
	deadlineLease, _ := state.Issue("b", "sb", 1, []Kind{Text}, 10*time.Second)
	deadlineEvent := Event{LeaseID: deadlineLease.ID, OriginID: "b", Boot: "boot", OriginSeq: 1, Lamport: 7, Kind: Text, Payload: []byte("late")}
	deadlineEvent.Digest = eventDigest(deadlineEvent)
	late, err := state.Begin("b", "sb", 1, deadlineEvent)
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(10 * time.Second)
	if err = state.Commit(late, func(uint64, Kind, []byte) (uint64, error) { return 3, nil }); !errors.Is(err, ErrExpired) {
		t.Fatal("candidate crossed original lease deadline", err)
	}
}
