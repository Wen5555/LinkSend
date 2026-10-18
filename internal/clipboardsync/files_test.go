package clipboardsync

import (
	"errors"
	"testing"
	"time"
)

func TestFileReservationAdmitsBeforeLeaseExpiryAndPreservesNewCopyPriority(t *testing.T) {
	for _, mode := range []string{"complete_after_lease", "new_copy", "pause", "duplicate"} {
		t.Run(mode, func(t *testing.T) {
			now := time.Unix(100, 0)
			a := New("a", func() time.Time { return now })
			b := New("b", func() time.Time { return now })
			a.Reset(false, 1)
			b.Reset(false, 10)
			lease, err := b.IssueScoped("a", "session", 7, []Grant{{Files, 3}}, 10*time.Second)
			if err != nil {
				t.Fatal(err)
			}
			if err = a.InstallScoped(lease.ID, "b", "session", 7, []Grant{{Files, 3}}, 10*time.Second, 1); err != nil {
				t.Fatal(err)
			}
			base, err := a.ObserveLocalEvent(2)
			if err != nil {
				t.Fatal(err)
			}
			base, err = a.PrepareObserved(base, Files, []byte(`{"id":"file"}`))
			if err != nil {
				t.Fatal(err)
			}
			event, err := a.BindScoped(base, lease.ID, 2)
			if err != nil {
				t.Fatal(err)
			}
			candidate, err := b.Begin("a", "session", 7, event)
			if err != nil {
				t.Fatal(err)
			}
			reservation, err := b.ReserveFiles(candidate)
			if err != nil {
				t.Fatal(err)
			}
			if mode == "duplicate" {
				if _, err = b.ReserveFiles(candidate); err == nil {
					t.Fatal("duplicate offer reserved twice")
				}
				return
			}
			now = now.Add(time.Minute)
			if mode == "new_copy" {
				_, _ = b.ObserveLocalEvent(11)
			}
			if mode == "pause" {
				b.Reset(true, 10)
			}
			writes := 0
			err = b.CommitFiles(reservation, func(expected uint64) (uint64, error) { writes++; return expected + 1, nil })
			if mode == "complete_after_lease" {
				if err != nil || writes != 1 {
					t.Fatal("admitted file did not finish", err)
				}
			} else if !errors.Is(err, ErrConflict) || writes != 0 {
				t.Fatal("stale reservation wrote native clipboard", err)
			}
		})
	}
}
