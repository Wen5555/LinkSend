package clipboardsync

import (
	"testing"
	"time"
)

func TestObservedCopyUsesOnlyAnApplicableOverlappingLease(t *testing.T) {
	for _, mode := range []string{"renewal", "expired", "policy_changed", "new_copy", "paused", "no_older_lease", "different_peer", "different_session", "different_generation"} {
		t.Run(mode, func(t *testing.T) {
			now := time.Unix(300, 0)
			s := New("sender", func() time.Time { return now })
			s.Reset(false, 1)
			if mode != "no_older_lease" {
				if err := s.InstallScoped("original", "receiver", "session", 7, []Grant{{Files, 3}}, 10*time.Second, 1); err != nil {
					t.Fatal(err)
				}
			}
			observation, err := s.ObserveLocalEvent(2)
			if err != nil {
				t.Fatal(err)
			}
			observation, err = s.PrepareObserved(observation, Files, []byte(`{"id":"copied-file"}`))
			if err != nil {
				t.Fatal(err)
			}
			now = now.Add(time.Second)
			revision := uint64(3)
			if mode == "policy_changed" {
				revision++
			}
			if err := s.InstallScoped("renewal", "receiver", "session", 7, []Grant{{Files, revision}}, 10*time.Second, 2); err != nil {
				t.Fatal(err)
			}
			peer, session, generation := "receiver", "session", uint64(7)
			switch mode {
			case "expired":
				now = now.Add(9 * time.Second) // The renewal remains live.
			case "new_copy":
				if _, err := s.ObserveLocalEvent(3); err != nil {
					t.Fatal(err)
				}
			case "paused":
				s.Reset(true, 2)
			case "different_peer":
				peer = "other"
			case "different_session":
				session = "other"
			case "different_generation":
				generation++
			}
			lease, ok := s.OutboundForObservation(peer, session, generation, Files, observation)
			if mode != "renewal" {
				if ok {
					t.Fatal("old copy escaped its lease, local generation or permission boundary", lease.ID)
				}
				return
			}
			if !ok || lease.ID != "original" {
				t.Fatal("same-policy renewal hid a live lease for the fresh copy", lease.ID, ok)
			}
			if event, err := s.BindScoped(observation, lease.ID, 4); err != nil || event.LeaseID != "original" {
				t.Fatal("original lease could not bind the same observed copy", event.LeaseID, err)
			}
		})
	}
}
