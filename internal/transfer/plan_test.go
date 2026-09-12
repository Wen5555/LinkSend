package transfer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func planFixture(t *testing.T) (*Prepared, map[string][]byte) {
	t.Helper()
	dir := t.TempDir()
	contents := map[string][]byte{"a.txt": bytes.Repeat([]byte("selected 中文\n"), 6000), "b.txt": []byte("never selected"), "empty.txt": {}}
	paths := make([]string, 0, len(contents))
	for name, data := range contents {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, data, 0600); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, p)
	}
	p, err := Prepare(t.Context(), paths, 64<<10)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = p.Close() })
	return p, contents
}

func fillPlannedReceiver(t *testing.T, r *Receiver, p *Prepared) {
	t.Helper()
	buf := make([]byte, p.Manifest.ChunkSize)
	for _, e := range r.Plan().Entries {
		for i := range p.Manifest.Files[e.FileID].Chunks {
			if r.State.Verified[e.FileID][i] {
				continue
			}
			data, err := p.ReadChunk(t.Context(), e.FileID, i, buf)
			if err != nil {
				t.Fatal(err)
			}
			if err = r.WriteChunk(t.Context(), e.FileID, i, data); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestReceivePlanSelectionAndPathPolicy(t *testing.T) {
	p := fixture(t)
	m := p.Manifest
	before := m.Digest()
	dest := t.TempDir()
	var fileID, rootID uint32
	for _, e := range m.Files {
		if e.Path == "中文目录/data.bin" {
			fileID = e.ID
		}
		if e.Path == "中文目录" {
			rootID = e.ID
		}
	}
	partial, err := BuildReceivePlan(t.Context(), dest, m, PlanRequest{SelectedIDs: []uint32{fileID}})
	if err != nil {
		t.Fatal(err)
	}
	if len(partial.Entries) != 2 || partial.Entries[0].FileID != rootID || partial.Entries[1].FileID != fileID {
		t.Fatalf("ancestor closure: %+v", partial)
	}
	directory, err := BuildReceivePlan(t.Context(), dest, m, PlanRequest{SelectedIDs: []uint32{rootID}})
	if err != nil || len(directory.Entries) != len(m.Files) {
		t.Fatalf("recursive directory selection: %+v %v", directory, err)
	}
	all, err := BuildReceivePlan(t.Context(), dest, m, PlanRequest{})
	if err != nil || len(all.Entries) != len(m.Files) {
		t.Fatal("nil did not select all")
	}
	none, err := BuildReceivePlan(t.Context(), dest, m, PlanRequest{SelectedIDs: []uint32{}})
	if err != nil || none.Entries == nil || len(none.Entries) != 0 {
		t.Fatal("empty list did not skip all")
	}
	if m.Digest() != before || partial.OriginalDigest != before || all.OriginalDigest != before {
		t.Fatal("plan changed the original manifest")
	}
	encoded, err := json.Marshal(partial)
	if err != nil || bytes.Contains(encoded, []byte(dest)) {
		t.Fatal("absolute receive directory entered plan JSON")
	}
	bad := clonePlan(partial)
	bad.Entries[1].Path = "../escape"
	if bad.Validate(m) == nil {
		t.Fatal("traversing target plan accepted")
	}
	bad = clonePlan(partial)
	bad.Entries[1].Path = "中文目录/CON.txt"
	if bad.Validate(m) == nil {
		t.Fatal("Windows reserved target accepted")
	}
	bad = clonePlan(partial)
	bad.Entries[1].FileID = rootID
	if bad.Validate(m) == nil {
		t.Fatal("duplicate selected ID accepted")
	}
}

func TestReceivePlanConflictPreviewAndSkip(t *testing.T) {
	p, contents := planFixture(t)
	dest := t.TempDir()
	if err := os.WriteFile(filepath.Join(dest, "A.TXT"), contents["a.txt"], 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := BuildReceivePlan(t.Context(), dest, p.Manifest, PlanRequest{}); !errors.Is(err, ErrConflict) {
		t.Fatalf("same bytes/case collision was not a conflict: %v", err)
	}
	plan, err := BuildReceivePlan(t.Context(), dest, p.Manifest, PlanRequest{ConflictPolicy: ConflictSkip})
	if err != nil {
		t.Fatal(err)
	}
	if summary := plan.Summary(p.Manifest); summary.SkippedFiles != 1 || summary.SkippedBytes != int64(len(contents["a.txt"])) {
		t.Fatalf("skip accounting: %+v", summary)
	}
	kept, err := BuildReceivePlan(t.Context(), dest, p.Manifest, PlanRequest{ConflictPolicy: ConflictKeepBoth})
	if err != nil || kept.Entries[0].Path != "a (1).txt" {
		t.Fatalf("keep both preview: %+v %v", kept, err)
	}
}

func TestReceivePlanStreamPartialAndNoContent(t *testing.T) {
	for _, none := range []bool{false, true} {
		t.Run(map[bool]string{false: "partial", true: "all_skipped"}[none], func(t *testing.T) {
			p, contents := planFixture(t)
			dest := t.TempDir()
			ids := []uint32{0}
			if none {
				ids = []uint32{}
			}
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			a, b := net.Pipe()
			type outcome struct {
				result Result
				err    error
			}
			done := make(chan outcome, 1)
			var receiveProgress Progress
			go func() {
				result, err := ReceiveWithOptions(ctx, b, ReceiveOptions{Directory: dest, Peer: "authenticated-peer", Plan: func(ctx context.Context, offer Offer) (ReceivePlan, error) {
					return BuildReceivePlan(ctx, dest, offer.Manifest, PlanRequest{SelectedIDs: ids})
				}, Progress: func(p Progress) { receiveProgress = p }})
				done <- outcome{result, err}
			}()
			var sendProgress Progress
			result, err := Send(ctx, a, p, func(p Progress) { sendProgress = p })
			received := <-done
			if err != nil || received.err != nil {
				t.Fatalf("sender=%v receiver=%v", err, received.err)
			}
			want := int64(len(contents["a.txt"]))
			state := "Completed"
			if none {
				want = 0
				state = "NoContent"
			}
			if result.Bytes != want || received.result.Bytes != want || result.State != state || received.result.State != state || result.Digest != p.Manifest.Digest() || result.SelectionDigest == "" {
				t.Fatalf("bad bilateral result: %+v %+v", result, received.result)
			}
			if sendProgress.Total != want || sendProgress.Sent != want || sendProgress.Verified != want || receiveProgress.Received != want || receiveProgress.Committed != want || result.SkippedBytes != p.Manifest.TotalBytes()-want {
				t.Fatalf("selected accounting: send=%+v receive=%+v result=%+v", sendProgress, receiveProgress, result)
			}
			if _, err = os.Stat(filepath.Join(dest, "b.txt")); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("unselected file was committed")
			}
			if !none {
				data, err := os.ReadFile(filepath.Join(dest, "a.txt"))
				if err != nil || !bytes.Equal(data, contents["a.txt"]) {
					t.Fatal("selected bytes do not match")
				}
			}
		})
	}
}

func TestReceivePlanPersistsLateConflictBeforeNoReplaceCommit(t *testing.T) {
	p, _ := planFixture(t)
	dest := t.TempDir()
	plan, err := BuildReceivePlan(t.Context(), dest, p.Manifest, PlanRequest{SelectedIDs: []uint32{0}, ConflictPolicy: ConflictKeepBoth})
	if err != nil {
		t.Fatal(err)
	}
	var changed bool
	r, err := openReceiver(t.Context(), dest, "peer", p.Manifest, &plan, func(plan ReceivePlan) error {
		if plan.Entries[0].Path == "a.txt" {
			return nil
		}
		changed = true
		data, err := os.ReadFile(filepath.Join(dest, ".linksend-"+p.Manifest.TransferID, "state.json"))
		if err != nil {
			return err
		}
		var saved ResumeState
		if err = json.Unmarshal(data, &saved); err != nil {
			return err
		}
		if saved.Plan == nil || saved.Plan.Entries[0].Path != plan.Entries[0].Path {
			return errors.New("mapping callback preceded durable checkpoint")
		}
		if _, err = os.Stat(filepath.Join(dest, plan.Entries[0].Path)); !errors.Is(err, os.ErrNotExist) {
			return errors.New("target linked before mapping persistence")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	fillPlannedReceiver(t, r, p)
	protected := []byte("external file arrived after preview")
	if err = os.WriteFile(filepath.Join(dest, "a.txt"), protected, 0600); err != nil {
		t.Fatal(err)
	}
	if err = r.Finish(t.Context()); err != nil {
		t.Fatal(err)
	}
	if !changed || r.State.Committed[0].Path != "a (1).txt" {
		t.Fatal("late conflict was not persisted/reselected")
	}
	if data, err := os.ReadFile(filepath.Join(dest, "a.txt")); err != nil || !bytes.Equal(data, protected) {
		t.Fatal("no-replace overwritten external target")
	}
}

func TestReceivePlanPersistenceFailureStopsBeforeBody(t *testing.T) {
	p, _ := planFixture(t)
	dest := t.TempDir()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	a, b := net.Pipe()
	done := make(chan error, 1)
	go func() {
		_, err := ReceiveWithOptions(ctx, b, ReceiveOptions{Directory: dest, Peer: "peer", Plan: func(ctx context.Context, offer Offer) (ReceivePlan, error) {
			return BuildReceivePlan(ctx, dest, offer.Manifest, PlanRequest{})
		}, PlanChanged: func(ReceivePlan) error { return errors.New("application plan database readonly") }})
		done <- err
	}()
	var sent int64
	_, err := SendWithHooks(ctx, a, p, SendHooks{ChunkSent: func(c ChunkTransmission) { sent += c.Bytes }})
	if !errors.Is(err, ErrPlanPersistence) || !errors.Is(<-done, ErrPlanPersistence) || sent != 0 {
		t.Fatalf("body flowed after persistence failure: err=%v sent=%d", err, sent)
	}
}

func TestReceivePlanPartialCommitRestartKeepsNamesAndCommittedBytes(t *testing.T) {
	p, _ := planFixture(t)
	dest := t.TempDir()
	if err := os.WriteFile(filepath.Join(dest, "a.txt"), []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
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
		t.Fatalf("expected interruption after one commit: %v", err)
	}
	_ = r.Close()
	newPreview, err := BuildReceivePlan(t.Context(), dest, p.Manifest, PlanRequest{ConflictPolicy: ConflictKeepBoth})
	if err != nil || newPreview.Entries[0].Path != "a (1).txt" {
		t.Fatalf("resume preview did not load durable target: %+v %v", newPreview, err)
	}
	newPreview.Entries[0].Path = "a (2).txt" // a stale UI proposal still cannot replace the committed plan
	reopened, err := OpenReceiverWithPlan(t.Context(), dest, "peer", p.Manifest, newPreview)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if reopened.Plan().Entries[0].Path != "a (1).txt" || len(reopened.State.Committed) != 1 || reopened.Summary().RemainingBytes != 0 {
		t.Fatalf("resume discarded original plan/commit: %+v %+v", reopened.Plan(), reopened.Summary())
	}
	if err = reopened.Finish(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(dest, "a (2).txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("restart duplicated committed file")
	}
	if reopened.VerifiedBytes() != p.Manifest.TotalBytes() {
		t.Fatal("resumed verified bytes counted twice")
	}
}

func TestReceivePlanCommitIntentRequiresRealFileIdentity(t *testing.T) {
	for _, ours := range []bool{true, false} {
		t.Run(map[bool]string{true: "link_succeeded_before_checkpoint", false: "foreign_identical_file"}[ours], func(t *testing.T) {
			p, contents := planFixture(t)
			dest := t.TempDir()
			plan, err := BuildReceivePlan(t.Context(), dest, p.Manifest, PlanRequest{SelectedIDs: []uint32{0}, ConflictPolicy: ConflictKeepBoth})
			if err != nil {
				t.Fatal(err)
			}
			r, err := OpenReceiverWithPlan(t.Context(), dest, "peer", p.Manifest, plan)
			if err != nil {
				t.Fatal(err)
			}
			fillPlannedReceiver(t, r, p)
			e := p.Manifest.Files[0]
			r.State.Intents[0] = CommitRecord{FileID: 0, Path: e.Path, Digest: e.Hash, Size: e.Size}
			r.State.State = "Verifying"
			r.State.CommitStarted = true
			if err = r.checkpoint(); err != nil {
				t.Fatal(err)
			}
			if ours {
				err = r.root.Link(".linksend-"+p.Manifest.TransferID+"/0.part", e.Path)
			} else {
				err = os.WriteFile(filepath.Join(dest, e.Path), contents["a.txt"], 0600)
			}
			if err != nil {
				t.Fatal(err)
			}
			_ = r.Close()
			reopened, err := OpenReceiverWithPlan(t.Context(), dest, "peer", p.Manifest, plan)
			if err != nil {
				t.Fatal(err)
			}
			defer reopened.Close()
			if (len(reopened.State.Committed) == 1) != ours {
				t.Fatal("commit ownership inferred from hash instead of file identity")
			}
			if err = reopened.Finish(t.Context()); err != nil {
				t.Fatal(err)
			}
			want := "a.txt"
			if !ours {
				want = "a (1).txt"
			}
			if reopened.State.Committed[0].Path != want {
				t.Fatalf("wrong recovered path: %+v", reopened.State.Committed)
			}
		})
	}
}

func TestReceivePlanResumeSelectionMismatchAndLegacyState(t *testing.T) {
	p, _ := planFixture(t)
	dest := t.TempDir()
	plan, err := BuildReceivePlan(t.Context(), dest, p.Manifest, PlanRequest{SelectedIDs: []uint32{0}})
	if err != nil {
		t.Fatal(err)
	}
	r, err := OpenReceiverWithPlan(t.Context(), dest, "peer", p.Manifest, plan)
	if err != nil {
		t.Fatal(err)
	}
	_ = r.Close()
	data, err := os.ReadFile(filepath.Join(dest, ".linksend-"+p.Manifest.TransferID, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	var stored ResumeState
	if err = json.Unmarshal(data, &stored); err != nil || !strings.HasPrefix(stored.State, "ReceivePlanV1/") {
		t.Fatal("new plan checkpoint could be silently consumed by old receiver")
	}
	if _, err = OpenReceiverWithPlan(t.Context(), dest, "peer", p.Manifest, FullReceivePlan(p.Manifest)); !errors.Is(err, ErrPlanMismatch) {
		t.Fatalf("resume expanded accepted selection: %v", err)
	}
	legacyDir := t.TempDir()
	legacy, err := OpenReceiver(t.Context(), legacyDir, "peer", p.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	fillPlannedReceiver(t, legacy, p)
	_ = legacy.Close()
	recovered, err := OpenReceiverWithPlan(t.Context(), legacyDir, "peer", p.Manifest, FullReceivePlan(p.Manifest))
	if err != nil {
		t.Fatal(err)
	}
	defer recovered.Close()
	if recovered.VerifiedBytes() != p.Manifest.TotalBytes() {
		t.Fatal("new receiver failed to recover old checkpoint format")
	}
}

func TestReceivePlanRejectsSubsetForOldOfferAndBindsTerminal(t *testing.T) {
	p, _ := planFixture(t)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	a, b := net.Pipe()
	done := make(chan error, 1)
	dest := t.TempDir()
	go func() {
		_, err := ReceiveWithOptions(ctx, b, ReceiveOptions{Directory: dest, Peer: "peer", Plan: func(ctx context.Context, o Offer) (ReceivePlan, error) {
			return BuildReceivePlan(ctx, dest, o.Manifest, PlanRequest{SelectedIDs: []uint32{0}})
		}})
		done <- err
	}()
	if err := writeControl(a, control{Op: opOffer, Manifest: &p.Manifest, Digest: p.Manifest.Digest()}); err != nil {
		t.Fatal(err)
	}
	_, err := readControl(a)
	if !errors.Is(err, ErrPlanUnsupported) || !errors.Is(<-done, ErrPlanUnsupported) {
		t.Fatalf("old offer subset was not explicit: %v", err)
	}
	_ = a.Close()
	a, b = net.Pipe()
	done = make(chan error, 1)
	go func() {
		defer b.Close()
		offer, err := readControl(b)
		if err != nil {
			done <- err
			return
		}
		selection := FullReceivePlan(*offer.Manifest)
		selection.Entries = []PlannedEntry{}
		s := selection.Selection()
		if err = writeControl(b, control{Op: opAccept, Digest: offer.Digest, Selection: &s}); err == nil {
			err = writeControl(b, control{Op: opReady})
		}
		if err == nil {
			_, err = readControl(b)
		}
		if err == nil {
			err = writeControl(b, control{Op: opCompleted, Digest: offer.Digest, SelectionDigest: strings.Repeat("0", 64)})
		}
		done <- err
	}()
	if _, err = Send(ctx, a, p, nil); !errors.Is(err, ErrIntegrity) {
		t.Fatalf("wrong terminal selection digest accepted: %v", err)
	}
	if err = <-done; err != nil {
		t.Fatal(err)
	}
}

func TestReceivePlanUnselectedChunkRejected(t *testing.T) {
	p, _ := planFixture(t)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	a, b := net.Pipe()
	done := make(chan error, 1)
	go func() {
		defer b.Close()
		offer, err := readControl(b)
		if err != nil {
			done <- err
			return
		}
		plan := FullReceivePlan(*offer.Manifest)
		plan.Entries = plan.Entries[:1]
		s := plan.Selection()
		if err = writeControl(b, control{Op: opAccept, Digest: offer.Digest, Selection: &s}); err == nil {
			err = writeControl(b, control{Op: opChunk, File: 1, Index: 0})
		}
		if err == nil {
			_, err = readControl(b)
		}
		done <- err
	}()
	if _, err := Send(ctx, a, p, nil); !errors.Is(err, ErrPlanMismatch) {
		t.Fatalf("unselected request succeeded: %v", err)
	}
	if err := <-done; !errors.Is(err, ErrPlanMismatch) {
		t.Fatalf("peer did not receive stable error: %v", err)
	}
}

// legacyControl is intentionally frozen to the pre-plan JSON shape. These
// fixtures never call the new writeControl to construct old frames.
type legacyControl struct {
	Op       string    `json:"op"`
	Manifest *Manifest `json:"manifest,omitempty"`
	Digest   string    `json:"digest,omitempty"`
	File     uint32    `json:"file,omitempty"`
	Index    int       `json:"index,omitempty"`
	Verified int64     `json:"verified,omitempty"`
	Error    string    `json:"error,omitempty"`
}

func legacyWrite(w io.Writer, c legacyControl) error {
	b, err := json.Marshal(c)
	if err != nil {
		return err
	}
	return writeFrame(w, b, MaxMetadata)
}
func legacyRead(r io.Reader) (legacyControl, error) {
	b, err := readFrame(r, MaxMetadata)
	if err != nil {
		return legacyControl{}, err
	}
	var c legacyControl
	err = json.Unmarshal(b, &c)
	return c, err
}

func TestReceivePlanNewSenderWithFrozenLegacyReceiver(t *testing.T) {
	p, _ := planFixture(t)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	a, b := net.Pipe()
	done := make(chan error, 1)
	go func() {
		defer b.Close()
		offer, err := legacyRead(b)
		if err != nil {
			done <- err
			return
		}
		err = legacyWrite(b, legacyControl{Op: "accept", Digest: offer.Digest})
		var verified int64
		for _, e := range offer.Manifest.Files {
			for i := range e.Chunks {
				if err != nil {
					break
				}
				err = legacyWrite(b, legacyControl{Op: "chunk", File: e.ID, Index: i})
				var data []byte
				if err == nil {
					data, err = readFrame(b, offer.Manifest.ChunkSize)
				}
				verified += int64(len(data))
				if err == nil && Sum(data) != e.Chunks[i] {
					err = ErrIntegrity
				}
				if err == nil {
					err = legacyWrite(b, legacyControl{Op: "ack", File: e.ID, Index: i, Verified: verified})
				}
			}
		}
		if err == nil {
			err = legacyWrite(b, legacyControl{Op: "ready"})
		}
		if err == nil {
			_, err = legacyRead(b)
		}
		if err == nil {
			err = legacyWrite(b, legacyControl{Op: "completed", Digest: offer.Digest, Verified: verified})
		}
		if err == nil {
			var c legacyControl
			c, err = legacyRead(b)
			if c.Op != "confirmed" {
				err = ErrIntegrity
			}
		}
		done <- err
	}()
	result, err := Send(ctx, a, p, nil)
	if err != nil || result.Bytes != p.Manifest.TotalBytes() || result.SelectionDigest != "" {
		t.Fatalf("legacy acceptance changed full behavior: %+v %v", result, err)
	}
	if err = <-done; err != nil {
		t.Fatal(err)
	}
}

func TestReceivePlanFrozenLegacySenderWithNewReceiver(t *testing.T) {
	p, _ := planFixture(t)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	a, b := net.Pipe()
	done := make(chan error, 1)
	dest := t.TempDir()
	go func() {
		_, err := Receive(ctx, b, dest, "peer", func(m Manifest) bool { m.Files[0].Path = "mutated-callback"; return true }, nil)
		done <- err
	}()
	if err := legacyWrite(a, legacyControl{Op: "offer", Manifest: &p.Manifest, Digest: p.Manifest.Digest()}); err != nil {
		t.Fatal(err)
	}
	accept, err := legacyRead(a)
	if err != nil || accept.Op != "accept" {
		t.Fatalf("old offer failed: %+v %v", accept, err)
	}
	buf := make([]byte, p.Manifest.ChunkSize)
	for {
		c, err := legacyRead(a)
		if err != nil {
			t.Fatal(err)
		}
		if c.Op == "ready" {
			break
		}
		if c.Op != "chunk" {
			t.Fatalf("unexpected new frame before legacy chunk: %+v", c)
		}
		data, err := p.ReadChunk(ctx, c.File, c.Index, buf)
		if err != nil {
			t.Fatal(err)
		}
		if err = writeFrame(a, data, p.Manifest.ChunkSize); err != nil {
			t.Fatal(err)
		}
		if _, err = legacyRead(a); err != nil {
			t.Fatal(err)
		}
	}
	if err = legacyWrite(a, legacyControl{Op: "finish", Digest: p.Manifest.Digest()}); err != nil {
		t.Fatal(err)
	}
	completed, err := legacyRead(a)
	if err != nil || completed.Verified != p.Manifest.TotalBytes() {
		t.Fatalf("bad legacy completion: %+v %v", completed, err)
	}
	if err = legacyWrite(a, legacyControl{Op: "confirmed", Digest: p.Manifest.Digest()}); err != nil {
		t.Fatal(err)
	}
	ack, err := legacyRead(a)
	if err != nil || ack.Op != "confirmed_ack" {
		t.Fatalf("bad legacy confirmation: %+v %v", ack, err)
	}
	_ = a.Close()
	if err = <-done; err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(dest, "a.txt")); err != nil {
		t.Fatal("accept callback mutated immutable manifest")
	}
}

func TestReceivePlanCheckpointRoundTripExcludesAbsoluteDirectory(t *testing.T) {
	p, _ := planFixture(t)
	dest := t.TempDir()
	plan := FullReceivePlan(p.Manifest)
	plan.Directory = dest
	r, err := OpenReceiverWithPlan(t.Context(), dest, "peer", p.Manifest, plan)
	if err != nil {
		t.Fatal(err)
	}
	_ = r.Close()
	data, err := os.ReadFile(filepath.Join(dest, ".linksend-"+p.Manifest.TransferID, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte(dest)) {
		t.Fatal("absolute directory leaked into checkpoint plan")
	}
	var saved ResumeState
	if err = json.Unmarshal(data, &saved); err != nil {
		t.Fatal(err)
	}
	saved.Plan.Directory = dest
	if !reflect.DeepEqual(*saved.Plan, plan) {
		t.Fatalf("plan round trip changed mapping: %+v", saved.Plan)
	}
}

func TestReceivePlanKeepsEmptyDirectoriesAndBoundsNames(t *testing.T) {
	p := fixture(t)
	dest := t.TempDir()
	var emptyID uint32
	for _, e := range p.Manifest.Files {
		if e.Path == "中文目录/空目录" {
			emptyID = e.ID
		}
	}
	plan, err := BuildReceivePlan(t.Context(), dest, p.Manifest, PlanRequest{SelectedIDs: []uint32{emptyID}})
	if err != nil {
		t.Fatal(err)
	}
	r, err := OpenReceiverWithPlan(t.Context(), dest, "peer", p.Manifest, plan)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if err = r.Finish(t.Context()); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(filepath.Join(dest, "中文目录", "空目录")); err != nil || !info.IsDir() {
		t.Fatal("selected empty directory missing")
	}
	if s := r.Summary(); s.SelectedFiles != 0 || s.SelectedEntries != 2 || s.RemainingBytes != 0 {
		t.Fatalf("empty directory accounting: %+v", s)
	}
	flat, _ := planFixture(t)
	full := t.TempDir()
	for i := 0; i <= maxKeepBothNames; i++ {
		name := "a.txt"
		if i > 0 {
			name = fmt.Sprintf("a (%d).txt", i)
		}
		if err = os.WriteFile(filepath.Join(full, name), nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = BuildReceivePlan(t.Context(), full, flat.Manifest, PlanRequest{SelectedIDs: []uint32{0}, ConflictPolicy: ConflictKeepBoth}); !errors.Is(err, ErrConflict) {
		t.Fatalf("name search was not bounded: %v", err)
	}
}

func TestReceivePlanLatePersistenceFailureDoesNotCommitNewName(t *testing.T) {
	p, _ := planFixture(t)
	dest := t.TempDir()
	plan, err := BuildReceivePlan(t.Context(), dest, p.Manifest, PlanRequest{SelectedIDs: []uint32{0}, ConflictPolicy: ConflictKeepBoth})
	if err != nil {
		t.Fatal(err)
	}
	r, err := openReceiver(t.Context(), dest, "peer", p.Manifest, &plan, func(p ReceivePlan) error {
		if p.Entries[0].Path != "a.txt" {
			return errors.New("database write failed")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	fillPlannedReceiver(t, r, p)
	if err = os.WriteFile(filepath.Join(dest, "a.txt"), []byte("protected"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = r.Finish(t.Context()); !errors.Is(err, ErrPlanPersistence) {
		t.Fatalf("late persistence failure lost: %v", err)
	}
	if _, err = os.Stat(filepath.Join(dest, "a (1).txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("new target committed before application plan persistence")
	}
	if len(r.State.Committed) != 0 {
		t.Fatal("failed plan persistence claimed a committed file")
	}
}

func TestReceiverForeignIdenticalFileIsNotACommit(t *testing.T) {
	p, contents := planFixture(t)
	dest := t.TempDir()
	r, err := OpenReceiver(t.Context(), dest, "peer", p.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	fillPlannedReceiver(t, r, p)
	if err = os.WriteFile(filepath.Join(dest, "a.txt"), contents["a.txt"], 0600); err != nil {
		t.Fatal(err)
	}
	if err = r.Finish(t.Context()); !errors.Is(err, ErrConflict) {
		t.Fatalf("foreign matching hash was accepted as own commit: %v", err)
	}
	if len(r.State.Committed) != 0 {
		t.Fatal("foreign file counted as committed")
	}
}

func TestReceivePlanDetectsUnicodeDestinationCollision(t *testing.T) {
	source := filepath.Join(t.TempDir(), "é.txt")
	if err := os.WriteFile(source, []byte("new"), 0600); err != nil {
		t.Fatal(err)
	}
	p, err := Prepare(t.Context(), []string{source}, 64<<10)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	dest := t.TempDir()
	if err = os.WriteFile(filepath.Join(dest, "e\u0301.txt"), []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = BuildReceivePlan(t.Context(), dest, p.Manifest, PlanRequest{}); !errors.Is(err, ErrConflict) {
		t.Fatalf("normalization-equivalent target was missed: %v", err)
	}
	plan, err := BuildReceivePlan(t.Context(), dest, p.Manifest, PlanRequest{ConflictPolicy: ConflictKeepBoth})
	if err != nil || plan.Entries[0].Path != "é (1).txt" {
		t.Fatalf("Unicode keep-both mapping: %+v %v", plan, err)
	}
}

func TestReceivePlanRejectsCheckpointMappingCorruption(t *testing.T) {
	p, _ := planFixture(t)
	dest := t.TempDir()
	plan := FullReceivePlan(p.Manifest)
	r, err := OpenReceiverWithPlan(t.Context(), dest, "peer", p.Manifest, plan)
	if err != nil {
		t.Fatal(err)
	}
	_ = r.Close()
	statePath := filepath.Join(dest, ".linksend-"+p.Manifest.TransferID, "state.json")
	data, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	var saved ResumeState
	if err = json.Unmarshal(data, &saved); err != nil {
		t.Fatal(err)
	}
	saved.Plan.Entries[0].Path = "different.txt"
	data, err = json.Marshal(saved)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(statePath, data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = OpenReceiverWithPlan(t.Context(), dest, "peer", p.Manifest, plan); !errors.Is(err, ErrPlanMismatch) {
		t.Fatalf("corrupt persisted mapping accepted: %v", err)
	}
}

type planRecordingStream struct {
	net.Conn
	writes bytes.Buffer
}

func (s *planRecordingStream) Write(p []byte) (int, error) {
	_, _ = s.writes.Write(p)
	return s.Conn.Write(p)
}

func TestReceivePlanDirectoryChoiceStaysLocal(t *testing.T) {
	p, _ := planFixture(t)
	original, chosen := t.TempDir(), t.TempDir()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	a, b := net.Pipe()
	recorded := &planRecordingStream{Conn: b}
	done := make(chan error, 1)
	var persistedDirectory string
	go func() {
		_, err := ReceiveWithOptions(ctx, recorded, ReceiveOptions{Directory: original, Peer: "peer", Plan: func(ctx context.Context, o Offer) (ReceivePlan, error) {
			return BuildReceivePlan(ctx, chosen, o.Manifest, PlanRequest{SelectedIDs: []uint32{0}})
		}, PlanChanged: func(p ReceivePlan) error { persistedDirectory = p.Directory; return nil }})
		done <- err
	}()
	if _, err := Send(ctx, a, p, nil); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if persistedDirectory != chosen || bytes.Contains(recorded.writes.Bytes(), []byte(chosen)) {
		t.Fatal("destination choice did not remain local")
	}
	if _, err := os.Stat(filepath.Join(chosen, "a.txt")); err != nil {
		t.Fatal(err)
	}
	if entries, err := os.ReadDir(original); err != nil || len(entries) != 0 {
		t.Fatalf("old directory was used: %v %v", entries, err)
	}
}
