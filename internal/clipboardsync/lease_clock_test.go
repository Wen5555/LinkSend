package clipboardsync

import (
	"errors"
	"maps"
	"math"
	"reflect"
	"testing"
	"time"
)

func TestLeaseClockOrdersFreshCopyAfterIndependentReceiverHistory(t *testing.T) {
	receiver, sender := New("receiver", nil), New("sender", nil)
	receiver.Reset(false, 1)
	sender.Reset(false, 1)
	for generation := uint64(2); generation <= 20; generation++ {
		if err := receiver.ObserveLocal(generation); err != nil {
			t.Fatal(err)
		}
	}
	grants := []Grant{{Kind: Text, Revision: 4}}
	lease, err := receiver.IssueScoped("sender", "session", 3, grants, 10*time.Second)
	if err != nil || lease.Lamport != 19 {
		t.Fatalf("issue snapshot: clock=%d err=%v", lease.Lamport, err)
	}
	if err = sender.InstallScopedWithClock(lease.ID, "receiver", "session", 3, grants, 10*time.Second, 1, lease.Lamport); err != nil {
		t.Fatal(err)
	}
	base, err := sender.PrepareLocal(2, Text, []byte("fresh after receiver lease"))
	if err != nil {
		t.Fatal(err)
	}
	event, err := sender.BindScoped(base, lease.ID, 2)
	if err != nil || event.Lamport <= lease.Lamport {
		t.Fatalf("fresh copy did not follow receiver clock: clock=%d err=%v", event.Lamport, err)
	}
	candidate, err := receiver.Begin("sender", "session", 3, event)
	if err != nil {
		t.Fatal("fresh copy was ordered behind earlier independent history", err)
	}
	if err = receiver.Commit(candidate, func(expected uint64, _ Kind, _ []byte) (uint64, error) {
		if expected != 20 {
			t.Fatalf("native generation=%d", expected)
		}
		return expected + 1, nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err = receiver.Begin("sender", "session", 3, event); !errors.Is(err, ErrStale) {
		t.Fatal("clock exchange disabled replay protection", err)
	}
}

func TestLeaseClockDoesNotRetimestampOrBackfillExistingCopy(t *testing.T) {
	sender := New("sender", nil)
	sender.Reset(false, 1)
	grants := []Grant{{Kind: Text, Revision: 1}}
	if err := sender.InstallScoped("old", "receiver", "session", 3, grants, 10*time.Second, 1); err != nil {
		t.Fatal(err)
	}
	base, err := sender.PrepareLocal(2, Text, []byte("copy before new lease"))
	if err != nil {
		t.Fatal(err)
	}
	old, err := sender.BindScoped(base, "old", 1)
	if err != nil {
		t.Fatal(err)
	}
	if err = sender.InstallScopedWithClock("new", "receiver", "session", 3, grants, 10*time.Second, 2, 50); err != nil {
		t.Fatal(err)
	}
	again, err := sender.BindScoped(base, "old", 1)
	if err != nil || again.Lamport != old.Lamport || again.Digest != old.Digest {
		t.Fatal("lease observation rewrote an existing event", err)
	}
	if _, err = sender.BindScoped(base, "new", 1); !errors.Is(err, ErrStale) {
		t.Fatal("clock exchange backfilled a pre-lease copy", err)
	}
	if !sender.LocalCurrent(old) {
		t.Fatal("clock exchange invalidated an otherwise current event")
	}
}

func TestLeaseClockDoesNotBypassCopyDuringReceive(t *testing.T) {
	receiver, sender := New("receiver", nil), New("sender", nil)
	receiver.Reset(false, 1)
	sender.Reset(false, 1)
	if err := receiver.ObserveLocal(2); err != nil {
		t.Fatal(err)
	}
	grants := []Grant{{Kind: Text, Revision: 1}}
	lease, err := receiver.IssueScoped("sender", "session", 3, grants, 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if err = sender.InstallScopedWithClock(lease.ID, "receiver", "session", 3, grants, 10*time.Second, 1, lease.Lamport); err != nil {
		t.Fatal(err)
	}
	base, err := sender.PrepareLocal(2, Text, []byte("delayed body"))
	if err != nil {
		t.Fatal(err)
	}
	event, err := sender.BindScoped(base, lease.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	header := event
	header.Payload = nil
	candidate, err := receiver.BeginHeader("sender", "session", 3, header)
	if err != nil {
		t.Fatal(err)
	}
	if err = receiver.ObserveLocal(3); err != nil {
		t.Fatal(err)
	}
	candidate, err = receiver.AttachPayload(candidate, event.Payload)
	if err != nil {
		t.Fatal(err)
	}
	if err = receiver.Commit(candidate, func(uint64, Kind, []byte) (uint64, error) {
		t.Fatal("candidate crossed a newer local copy")
		return 0, nil
	}); !errors.Is(err, ErrConflict) {
		t.Fatal("local copy during receive did not invalidate candidate", err)
	}
}

func TestFailedLeaseClockInstallPreservesClockAndLeaseWindow(t *testing.T) {
	for _, reason := range []string{"generation", "revision", "kind", "ttl", "capacity", "remote_overflow", "remote_max_minus_one", "remote_max_minus_two", "remote_over_limit", "remote_near_limit", "local_near_limit", "local_overflow", "paused"} {
		t.Run(reason, func(t *testing.T) {
			now := time.Unix(100, 0)
			state := New("sender", func() time.Time { return now })
			state.Reset(false, 1)
			grants := []Grant{{Kind: Text, Revision: 1}}
			for _, id := range []string{"one", "two"} {
				if err := state.InstallScoped(id, "peer", "session", 3, grants, 10*time.Second, 1); err != nil {
					t.Fatal(err)
				}
			}
			state.lamport = 7
			generation, clock, ttl := uint64(3), uint64(100), 10*time.Second
			switch reason {
			case "generation":
				generation = 0
			case "revision":
				grants = []Grant{{Kind: Text}}
			case "kind":
				grants = []Grant{{Kind: Kind("files"), Revision: 1}}
			case "ttl":
				ttl = 0
			case "remote_overflow":
				clock = math.MaxUint64
				grants = []Grant{{Kind: Image, Revision: 2}}
			case "remote_max_minus_one":
				clock = math.MaxUint64 - 1
				grants = []Grant{{Kind: Image, Revision: 2}}
			case "remote_max_minus_two":
				clock = math.MaxUint64 - 2
				grants = []Grant{{Kind: Image, Revision: 2}}
			case "remote_over_limit":
				clock = state.lamport + MaxLeaseLamportAdvance + 1
				grants = []Grant{{Kind: Image, Revision: 2}}
			case "remote_near_limit":
				state.lamport = math.MaxUint64 - 5
				clock = math.MaxUint64 - 2
				grants = []Grant{{Kind: Image, Revision: 2}}
			case "local_near_limit":
				state.lamport = math.MaxUint64 - 2
				clock = math.MaxUint64 - 3
				grants = []Grant{{Kind: Image, Revision: 2}}
			case "local_overflow":
				state.lamport = math.MaxUint64
				grants = []Grant{{Kind: Image, Revision: 2}}
			case "paused":
				state.paused = true
			}
			before, beforeClock := maps.Clone(state.leases), state.lamport
			if err := state.InstallScopedWithClock("failed", "peer", "session", generation, grants, ttl, 1, clock); err == nil {
				t.Fatal("invalid lease accepted")
			}
			if state.lamport != beforeClock || !reflect.DeepEqual(before, state.leases) {
				t.Fatal("failed lease installation changed clock or previous window")
			}
		})
	}
}

func TestLeaseClockExcessiveJumpLeavesNextCopyUsable(t *testing.T) {
	receiver, sender := New("receiver", nil), New("sender", nil)
	receiver.Reset(false, 1)
	sender.Reset(false, 1)
	grants := []Grant{{Kind: Text, Revision: 1}}
	lease, err := receiver.IssueScoped("sender", "session", 3, grants, 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if err = sender.InstallScopedWithClock(lease.ID, "receiver", "session", 3, grants, 10*time.Second, 1, lease.Lamport); err != nil {
		t.Fatal(err)
	}
	before := maps.Clone(sender.leases)
	for _, clock := range []uint64{MaxLeaseLamportAdvance + 1, math.MaxInt64, math.MaxInt64 + 1, math.MaxUint64 - 2, math.MaxUint64 - 1, math.MaxUint64} {
		if err = sender.InstallScopedWithClock("excessive", "receiver", "session", 3, []Grant{{Kind: Image, Revision: 2}}, 10*time.Second, 1, clock); !errors.Is(err, ErrLimit) {
			t.Fatalf("excessive jump %d was accepted: %v", clock, err)
		}
		if sender.lamport != 0 || !reflect.DeepEqual(before, sender.leases) {
			t.Fatal("rejected jump changed the local clock or lease window")
		}
	}
	base, err := sender.PrepareLocal(2, Text, []byte("new copy after rejected jump"))
	if err != nil {
		t.Fatal(err)
	}
	event, err := sender.BindScoped(base, lease.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := receiver.Begin("sender", "session", 3, event)
	if err != nil {
		t.Fatal(err)
	}
	if err = receiver.Commit(candidate, func(expected uint64, _ Kind, _ []byte) (uint64, error) { return expected + 1, nil }); err != nil {
		t.Fatal("rejected jump prevented the next copy from committing", err)
	}
}

func TestLeaseClockHighClocksRemainRenewable(t *testing.T) {
	for _, clocks := range []struct {
		name             string
		sender, receiver uint64
	}{
		{"maximum_forward_jump", 0, MaxLeaseLamportAdvance},
		{"already_high_clocks", math.MaxInt64, math.MaxInt64},
	} {
		t.Run(clocks.name, func(t *testing.T) {
			testLeaseClockRenewals(t, clocks.sender, clocks.receiver)
		})
	}
}

func testLeaseClockRenewals(t *testing.T, senderClock, receiverClock uint64) {
	t.Helper()
	now := time.Unix(100, 0)
	receiver := New("receiver", func() time.Time { return now })
	sender := New("sender", func() time.Time { return now })
	receiver.Reset(false, 1)
	sender.Reset(false, 1)
	// These clocks are already close to each other. Repeated legitimate
	// renewals may cross the signed-integer boundary without making a jump.
	receiver.lamport, sender.lamport = receiverClock, senderClock
	grants := []Grant{{Kind: Text, Revision: 1}}
	for generation := uint64(2); generation <= 5; generation++ {
		lease, err := receiver.IssueScoped("sender", "session", 3, grants, 10*time.Second)
		if err != nil {
			t.Fatal(err)
		}
		if err = sender.InstallScopedWithClock(lease.ID, "receiver", "session", 3, grants, 10*time.Second, generation-1, lease.Lamport); err != nil {
			t.Fatalf("normal renewal at generation %d failed: %v", generation, err)
		}
		base, err := sender.PrepareLocal(generation, Text, []byte("fresh after normal renewal"))
		if err != nil {
			t.Fatal("accepted lease prevented the next real copy", err)
		}
		event, err := sender.BindScoped(base, lease.ID, 1)
		if err != nil || event.Lamport <= lease.Lamport {
			t.Fatalf("fresh event clock=%d lease=%d err=%v", event.Lamport, lease.Lamport, err)
		}
		candidate, err := receiver.Begin("sender", "session", 3, event)
		if err != nil {
			t.Fatal(err)
		}
		if err = receiver.Commit(candidate, func(expected uint64, _ Kind, _ []byte) (uint64, error) {
			if expected != generation-1 {
				t.Fatalf("native generation=%d want=%d", expected, generation-1)
			}
			return expected + 1, nil
		}); err != nil {
			t.Fatal("normal renewal left no room for receiver clock advance", err)
		}
		now = now.Add(7 * time.Second)
	}
}

func TestLeaseClockReplayPreservesOriginalDeadlineAndBaseline(t *testing.T) {
	for _, clock := range []uint64{0, 41} {
		now := time.Unix(100, 0)
		state := New("sender", func() time.Time { return now })
		state.Reset(false, 1)
		grants := []Grant{{Kind: Text, Revision: 1}}
		if err := state.InstallScopedWithClock("lease", "peer", "session", 3, grants, 10*time.Second, 1, clock); err != nil {
			t.Fatal(err)
		}
		before, beforeClock := maps.Clone(state.leases), state.lamport
		now = now.Add(2 * time.Second)
		if err := state.InstallScopedWithClock("lease", "peer", "session", 3, grants, 10*time.Second, 2, clock); !errors.Is(err, ErrStale) {
			t.Fatal("existing lease ID was installed twice", err)
		}
		if state.lamport != beforeClock || !reflect.DeepEqual(before, state.leases) {
			t.Fatal("duplicate lease renewed the deadline, baseline or clock")
		}
	}
}
