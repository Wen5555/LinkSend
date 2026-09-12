package transfer

import (
	"bytes"
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestReceivePlanSameSkipRequestResumesWithoutExpanding(t *testing.T) {
	p, _ := planFixture(t)
	dest := t.TempDir()
	if err := os.WriteFile(filepath.Join(dest, "a.txt"), []byte("protected"), 0600); err != nil {
		t.Fatal(err)
	}
	request := PlanRequest{ConflictPolicy: ConflictSkip}
	plan, err := BuildReceivePlan(t.Context(), dest, p.Manifest, request)
	if err != nil {
		t.Fatal(err)
	}
	r, err := OpenReceiverWithPlan(t.Context(), dest, "peer", p.Manifest, plan)
	if err != nil {
		t.Fatal(err)
	}
	_ = r.Close()
	if err = os.Remove(filepath.Join(dest, "a.txt")); err != nil {
		t.Fatal(err)
	}
	restored, err := BuildReceivePlan(t.Context(), dest, p.Manifest, request)
	if err != nil || restored.Selection().Digest != plan.Selection().Digest || restored.Summary(p.Manifest).SkippedFiles != 1 {
		t.Fatalf("same skip request expanded/fails after restart: %+v %v", restored, err)
	}
}

func TestReceivePlanDirectoryRestartRequiresActualOwnership(t *testing.T) {
	for _, intent := range []bool{false, true} {
		t.Run(map[bool]string{false: "before_mkdir_intent", true: "uncertain_mkdir_intent"}[intent], func(t *testing.T) {
			p := fixture(t)
			dest := t.TempDir()
			plan, err := BuildReceivePlan(t.Context(), dest, p.Manifest, PlanRequest{ConflictPolicy: ConflictKeepBoth})
			if err != nil {
				t.Fatal(err)
			}
			r, err := OpenReceiverWithPlan(t.Context(), dest, "peer", p.Manifest, plan)
			if err != nil {
				t.Fatal(err)
			}
			fillPlannedReceiver(t, r, p)
			r.State.State = "Verifying"
			r.State.CommitStarted = true
			rootID := plan.Entries[0].FileID
			target := plan.Entries[0].Path
			if intent {
				r.State.DirectoryIntents[rootID] = target
			}
			if err = r.checkpoint(); err != nil {
				t.Fatal(err)
			}
			_ = r.Close()
			if err = os.Mkdir(filepath.Join(dest, target), 0700); err != nil {
				t.Fatal(err)
			}
			protected := filepath.Join(dest, target, "external.txt")
			if err = os.WriteFile(protected, []byte("keep"), 0600); err != nil {
				t.Fatal(err)
			}
			reopened, err := OpenReceiverWithPlan(t.Context(), dest, "peer", p.Manifest, plan)
			if err != nil {
				t.Fatal(err)
			}
			defer reopened.Close()
			err = reopened.Finish(t.Context())
			if intent {
				if !errors.Is(err, ErrConflict) || len(reopened.State.Committed) != 0 {
					t.Fatalf("unknown directory ownership was accepted: %v", err)
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				if reopened.Plan().Entries[0].Path == target {
					t.Fatal("CommitStarted was treated as directory ownership")
				}
			}
			if _, err = os.Stat(filepath.Join(dest, target, "data.bin")); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("received contents merged into external directory")
			}
			if data, err := os.ReadFile(protected); err != nil || !bytes.Equal(data, []byte("keep")) {
				t.Fatal("external directory changed")
			}
		})
	}
}

func TestReceivePlanDirectoryIdentityRejectsReplacement(t *testing.T) {
	p := fixture(t)
	dest := t.TempDir()
	plan, err := BuildReceivePlan(t.Context(), dest, p.Manifest, PlanRequest{ConflictPolicy: ConflictKeepBoth})
	if err != nil {
		t.Fatal(err)
	}
	r, err := OpenReceiverWithPlan(t.Context(), dest, "peer", p.Manifest, plan)
	if err != nil {
		t.Fatal(err)
	}
	fillPlannedReceiver(t, r, p)
	ctx, cancel := context.WithCancel(t.Context())
	err = r.FinishWithCommit(ctx, func(CommitRecord) { cancel() })
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("did not reach partial commit: %v", err)
	}
	_ = r.Close()
	target := filepath.Join(dest, plan.Entries[0].Path)
	if err = os.Rename(target, target+"-saved"); err != nil {
		t.Fatal(err)
	}
	if err = os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	if restored, err := OpenReceiverWithPlan(t.Context(), dest, "peer", p.Manifest, plan); !errors.Is(err, ErrConflict) {
		if restored != nil {
			_ = restored.Close()
		}
		t.Fatalf("replacement directory matched persisted identity: %v", err)
	}
}

func TestReceivePlanRevalidatesOnlyAcceptedFiles(t *testing.T) {
	for _, none := range []bool{false, true} {
		t.Run(map[bool]string{false: "unselected_removed", true: "no_content_sources_removed"}[none], func(t *testing.T) {
			p, _ := planFixture(t)
			dest := t.TempDir()
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			a, b := net.Pipe()
			done := make(chan error, 1)
			go func() {
				_, err := ReceiveWithOptions(ctx, b, ReceiveOptions{Directory: dest, Peer: "peer", Plan: func(ctx context.Context, o Offer) (ReceivePlan, error) {
					ids := []uint32{0}
					if none {
						ids = []uint32{}
					}
					plan, err := BuildReceivePlan(ctx, dest, o.Manifest, PlanRequest{SelectedIDs: ids})
					if err != nil {
						return ReceivePlan{}, err
					}
					for id, source := range p.sources {
						if none || id != 0 {
							if err = source.root.Remove(source.relative); err != nil {
								return ReceivePlan{}, err
							}
						}
					}
					return plan, nil
				}})
				done <- err
			}()
			result, err := Send(ctx, a, p, nil)
			received := <-done
			if err != nil || received != nil {
				t.Fatalf("unselected source affected completion: %v / %v", err, received)
			}
			if none && result.State != "NoContent" {
				t.Fatal("empty selection revalidation changed result")
			}
		})
	}
}

func TestReceivePlanRefreshesUnicodeCollisionBeforeEachCommit(t *testing.T) {
	source := t.TempDir()
	var paths []string
	for _, name := range []string{"a.txt", "é.txt"} {
		p := filepath.Join(source, name)
		if err := os.WriteFile(p, []byte(name), 0600); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, p)
	}
	p, err := Prepare(t.Context(), paths, 64<<10)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	dest := t.TempDir()
	plan, err := BuildReceivePlan(t.Context(), dest, p.Manifest, PlanRequest{ConflictPolicy: ConflictKeepBoth})
	if err != nil {
		t.Fatal(err)
	}
	r, err := OpenReceiverWithPlan(t.Context(), dest, "peer", p.Manifest, plan)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	fillPlannedReceiver(t, r, p)
	err = r.FinishWithCommit(t.Context(), func(record CommitRecord) {
		if record.FileID == 0 {
			if err := os.WriteFile(filepath.Join(dest, "e\u0301.txt"), []byte("protected"), 0600); err != nil {
				t.Error(err)
			}
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if r.State.Committed[1].Path != "é (1).txt" {
		t.Fatalf("cached inventory hid new Unicode collision: %+v", r.State.Committed)
	}
}
