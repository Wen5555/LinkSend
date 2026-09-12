package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Wen5555/LinkSend/internal/protocol"
	"github.com/Wen5555/LinkSend/internal/transfer"
)

func TestIncomingSelectionOverRealQUIC(t *testing.T) {
	for _, mode := range []string{"subset", "no_content", "empty_directory"} {
		t.Run(mode, func(t *testing.T) {
			f := newDirectFixtureServices(t)
			source := t.TempDir()
			if err := os.WriteFile(filepath.Join(source, "chosen.txt"), []byte("chosen authenticated bytes"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(source, "skipped.txt"), []byte("must not be sent"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(filepath.Join(source, "empty"), 0700); err != nil {
				t.Fatal(err)
			}
			defaultDir, target := t.TempDir(), t.TempDir()
			r, err := f.b.StartReceive(f.aID.ID(), defaultDir, DirectConfig{AllowLoopback: true, CheckTimeout: 5 * time.Second, WaitTimeout: 10 * time.Second})
			if err != nil {
				t.Fatal(err)
			}
			waitTask(t, f.b, r.ID, func(v TaskSnapshot) bool { return v.Phase == "waiting" })
			s, err := f.a.StartSend(f.bID.ID(), []string{filepath.Join(source, "chosen.txt"), filepath.Join(source, "skipped.txt"), filepath.Join(source, "empty")}, DirectConfig{AllowLoopback: true, CheckTimeout: 5 * time.Second})
			if err != nil {
				t.Fatal(err)
			}
			pending := waitTask(t, f.b, r.ID, func(v TaskSnapshot) bool { return v.State == "awaiting_acceptance" })
			page, err := f.b.IncomingFiles(r.ID, IncomingFilesRequest{ExpectedRevision: pending.Revision, Limit: 200})
			if err != nil {
				t.Fatal(err)
			}
			if len(page.Files) != 3 || !page.SubsetSupported || page.TotalEntries != 3 {
				t.Fatalf("invalid page: %+v", page)
			}
			encoded, _ := json.Marshal(page)
			if strings.Contains(string(encoded), "chunks") || strings.Contains(string(encoded), source) {
				t.Fatalf("private manifest metadata in page: %s", encoded)
			}
			selected := make([]uint32, 0)
			for _, entry := range page.Files {
				if mode == "subset" && entry.Path == "chosen.txt" || mode == "empty_directory" && entry.Path == "empty" {
					selected = append(selected, entry.ID)
				}
			}
			preview, err := f.b.IncomingPlan(r.ID, IncomingPlanRequest{ExpectedRevision: page.Revision, Directory: target, SelectedIDs: selected})
			if err != nil {
				t.Fatal(err)
			}
			if preview.AvailableBytes == 0 || !preview.SpaceSufficient || preview.Directory != target {
				t.Fatalf("invalid actual disk preflight: %+v", preview)
			}
			if err = f.b.AcceptReceivePlan(r.ID, page.Revision, preview.PlanDigest); err == nil {
				t.Fatal("stale revision accepted")
			}
			if err = f.b.AcceptReceivePlan(r.ID, preview.Revision, preview.PlanDigest); err != nil {
				t.Fatal(err)
			}
			if err = f.b.AcceptReceivePlan(r.ID, preview.Revision, preview.PlanDigest); err == nil {
				t.Fatal("repeated acceptance succeeded")
			}
			state := "completed"
			if mode == "no_content" {
				state = "no_content"
			}
			received := waitTask(t, f.b, r.ID, func(v TaskSnapshot) bool { return isTerminal(v.State) })
			sent := waitTask(t, f.a, s.ID, func(v TaskSnapshot) bool { return isTerminal(v.State) })
			if received.State != state || sent.State != state {
				t.Fatalf("terminal receiver=%s/%s sender=%s/%s history receiver=%v sender=%v", received.State, received.ErrorCode, sent.State, sent.ErrorCode, f.b.tasks.historyError(), f.a.tasks.historyError())
			}
			if received.SelectionDigest == "" || received.SelectionDigest != sent.SelectionDigest || received.ReceivePlanDigest == "" || received.TargetDirectory != target {
				t.Fatalf("plan not durably bound receiver=%+v sender=%+v", received, sent)
			}
			if received.OriginalTotal != int64(len("chosen authenticated bytes")+len("must not be sent")) || received.SkippedFiles != preview.Summary.SkippedFiles || received.CommittedFiles != preview.Summary.SelectedFiles || received.SelectedEntries != len(selected) {
				t.Fatalf("misleading accounting: %+v", received)
			}
			if _, err = os.Stat(filepath.Join(target, "skipped.txt")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("skipped file written: %v", err)
			}
			if mode == "subset" {
				body, err := os.ReadFile(filepath.Join(target, "chosen.txt"))
				if err != nil || string(body) != "chosen authenticated bytes" {
					t.Fatalf("selected body mismatch %q %v", body, err)
				}
			}
			if mode == "empty_directory" {
				info, err := os.Stat(filepath.Join(target, "empty"))
				if err != nil || !info.IsDir() {
					t.Fatalf("selected empty directory missing: %v", err)
				}
			}
			if mode != "subset" && (sent.SentBytes != 0 || received.ReceivedBytes != 0 || received.CommittedBytes != 0) {
				t.Fatalf("zero-byte selection reports bytes sent=%+v received=%+v", sent, received)
			}
			for _, svc := range []*Service{f.a, f.b} {
				record, _ := svc.incomingTask(map[bool]string{true: s.ID, false: r.ID}[svc == f.a])
				record.mu.RLock()
				retained := record.incoming != nil
				record.mu.RUnlock()
				if retained {
					t.Fatal("terminal task retains full manifest")
				}
			}
		})
	}
}

func TestIncomingPlanDirectoryAndDurabilityBeforeAcceptanceOverRealQUIC(t *testing.T) {
	for _, failSide := range []string{"receiver", "sender"} {
		t.Run(failSide, func(t *testing.T) {
			f := newDirectFixtureServices(t)
			source := filepath.Join(t.TempDir(), "private.txt")
			if err := os.WriteFile(source, []byte("must remain unsent"), 0600); err != nil {
				t.Fatal(err)
			}
			target := t.TempDir()
			r, err := f.b.StartReceive(f.aID.ID(), target, DirectConfig{AllowLoopback: true, CheckTimeout: 5 * time.Second, WaitTimeout: 10 * time.Second})
			if err != nil {
				t.Fatal(err)
			}
			waitTask(t, f.b, r.ID, func(v TaskSnapshot) bool { return v.Phase == "waiting" })
			s, err := f.a.StartSend(f.bID.ID(), []string{source}, DirectConfig{AllowLoopback: true, CheckTimeout: 5 * time.Second})
			if err != nil {
				t.Fatal(err)
			}
			pending := waitTask(t, f.b, r.ID, func(v TaskSnapshot) bool { return v.State == "awaiting_acceptance" })
			missing := filepath.Join(t.TempDir(), "unavailable")
			_, err = f.b.IncomingPlan(r.ID, IncomingPlanRequest{ExpectedRevision: pending.Revision, Directory: missing})
			if protocol.ErrorCode(err) != protocol.ReceiveDirectoryUnavailable {
				t.Fatalf("wrong directory failure: %v", err)
			}
			if _, err = os.Stat(missing); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("explicit missing directory recreated")
			}
			preview, err := f.b.IncomingPlan(r.ID, IncomingPlanRequest{ExpectedRevision: pending.Revision})
			if err != nil {
				t.Fatal(err)
			}
			if failSide == "receiver" {
				f.b.tasks.setHistoryError(errors.New("injected durable storage failure"))
				if err = f.b.AcceptReceivePlan(r.ID, preview.Revision, preview.PlanDigest); !errors.Is(err, transfer.ErrPlanPersistence) {
					t.Fatalf("persistence failure ignored: %v", err)
				}
				if err = f.b.RejectTask(r.ID); err != nil {
					t.Fatal(err)
				}
			} else {
				waitTask(t, f.a, s.ID, func(v TaskSnapshot) bool { return v.State == "awaiting_acceptance" })
				f.a.tasks.setHistoryError(errors.New("injected sender storage failure"))
				if err = f.b.AcceptReceivePlan(r.ID, preview.Revision, preview.PlanDigest); err != nil {
					t.Fatal(err)
				}
			}
			sent := waitTask(t, f.a, s.ID, func(v TaskSnapshot) bool { return isTerminal(v.State) || v.State == "recovering" && v.CanResume })
			received := waitTask(t, f.b, r.ID, func(v TaskSnapshot) bool { return isTerminal(v.State) || v.State == "recovering" && v.CanResume })
			if sent.SentBytes != 0 || received.ReceivedBytes != 0 {
				t.Fatalf("bytes sent before durable decision: sent=%+v received=%+v", sent, received)
			}
			if failSide == "sender" && sent.ErrorCode != string(protocol.ReceivePlanPersistence) {
				t.Fatalf("sender error misclassified: %+v", sent)
			}
			if _, err = os.Stat(filepath.Join(target, "private.txt")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("file committed despite failed acceptance: %v", err)
			}
		})
	}
}

func TestIncomingSelectionRemainsBoundDuringResumeOverRealQUIC(t *testing.T) {
	f := newDirectFixtureServices(t)
	source := t.TempDir()
	payload := bytes.Repeat([]byte{0x72}, 2*transfer.DefaultChunkSize)
	if err := os.WriteFile(filepath.Join(source, "selected.bin"), payload, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "skipped.txt"), []byte("never send"), 0600); err != nil {
		t.Fatal(err)
	}
	target := t.TempDir()
	cfg := DirectConfig{AllowLoopback: true, CheckTimeout: 5 * time.Second, WaitTimeout: 10 * time.Second}
	r, err := f.b.StartReceive(f.aID.ID(), target, cfg)
	if err != nil {
		t.Fatal(err)
	}
	waitTask(t, f.b, r.ID, func(v TaskSnapshot) bool { return v.Phase == "waiting" })
	first, release := make(chan struct{}), make(chan struct{})
	cfg.onChunkSent = func(sent transfer.ChunkTransmission) {
		if sent.Index == 0 {
			select {
			case <-first:
			default:
				close(first)
			}
			select {
			case <-release:
			case <-time.After(10 * time.Second):
			}
		}
	}
	s, err := f.a.StartSend(f.bID.ID(), []string{filepath.Join(source, "selected.bin"), filepath.Join(source, "skipped.txt")}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	pending := waitTask(t, f.b, r.ID, func(v TaskSnapshot) bool { return v.State == "awaiting_acceptance" })
	page, err := f.b.IncomingFiles(r.ID, IncomingFilesRequest{ExpectedRevision: pending.Revision})
	if err != nil {
		t.Fatal(err)
	}
	var selected uint32
	for _, entry := range page.Files {
		if entry.Path == "selected.bin" {
			selected = entry.ID
		}
	}
	preview, err := f.b.IncomingPlan(r.ID, IncomingPlanRequest{ExpectedRevision: page.Revision, SelectedIDs: []uint32{selected}})
	if err != nil {
		t.Fatal(err)
	}
	if err = f.b.AcceptReceivePlan(r.ID, preview.Revision, preview.PlanDigest); err != nil {
		t.Fatal(err)
	}
	select {
	case <-first:
	case <-time.After(10 * time.Second):
		t.Fatal("no first body chunk")
	}
	waitTask(t, f.b, r.ID, func(v TaskSnapshot) bool { return v.VerifiedBytes >= transfer.DefaultChunkSize })
	if err = f.a.PauseTask(s.ID); err != nil {
		t.Fatal(err)
	}
	close(release)
	paused := waitTask(t, f.a, s.ID, func(v TaskSnapshot) bool { return v.State == "paused" })
	recovering := waitTask(t, f.b, r.ID, func(v TaskSnapshot) bool { return v.State == "recovering" })
	if paused.SelectionDigest == "" || paused.SelectionDigest != recovering.SelectionDigest {
		t.Fatalf("interrupted selection lost: %+v %+v", paused, recovering)
	}
	cfg.onChunkSent = nil
	moved := target + "-temporarily-moved"
	if err = os.Rename(target, moved); err != nil {
		t.Fatal(err)
	}
	if _, err = f.b.ResumeTask(r.ID, cfg); protocol.ErrorCode(err) != protocol.ReceiveDirectoryUnavailable {
		t.Fatalf("missing resume directory did not require attention: %v", err)
	}
	if _, err = os.Stat(target); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("resume recreated missing directory")
	}
	if err = os.Rename(moved, target); err != nil {
		t.Fatal(err)
	}
	if _, err = f.b.ResumeTask(r.ID, cfg); err != nil {
		t.Fatal(err)
	}
	waitTask(t, f.b, r.ID, func(v TaskSnapshot) bool { return v.Phase == "waiting" })
	if _, err = f.a.ResumeTask(s.ID, cfg); err != nil {
		t.Fatal(err)
	}
	received := waitTask(t, f.b, r.ID, func(v TaskSnapshot) bool { return isTerminal(v.State) })
	sent := waitTask(t, f.a, s.ID, func(v TaskSnapshot) bool { return isTerminal(v.State) })
	if received.State != "completed" || sent.State != "completed" || sent.SelectionDigest != paused.SelectionDigest || received.ReceivePlanDigest != preview.PlanDigest {
		t.Fatalf("resume changed saved selection: %+v %+v", received, sent)
	}
	body, err := os.ReadFile(filepath.Join(target, "selected.bin"))
	if err != nil || !bytes.Equal(body, payload) {
		t.Fatalf("resumed body mismatch: %v", err)
	}
	if _, err = os.Stat(filepath.Join(target, "skipped.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("resume expanded subset")
	}
}

func TestInboxReconfigurePreservesActiveReceivePlanOverRealQUIC(t *testing.T) {
	f := newDirectFixtureServices(t)
	oldDir, newDir := t.TempDir(), t.TempDir()
	cfg := DirectConfig{BindAddress: "127.0.0.1:0", AllowLoopback: true, CheckTimeout: 5 * time.Second}
	if err := f.b.StartInbox(oldDir, cfg); err != nil {
		t.Fatal(err)
	}
	waitFor(t, 5*time.Second, func() bool { return f.b.InboxStatus().Listening }, "inbox not listening")
	source := filepath.Join(t.TempDir(), "ongoing.bin")
	if err := os.WriteFile(source, bytes.Repeat([]byte{0x65}, 2*transfer.DefaultChunkSize), 0600); err != nil {
		t.Fatal(err)
	}
	first, release := make(chan struct{}), make(chan struct{})
	cfg.onChunkSent = func(sent transfer.ChunkTransmission) {
		if sent.Index == 0 {
			select {
			case <-first:
			default:
				close(first)
			}
			select {
			case <-release:
			case <-time.After(10 * time.Second):
			}
		}
	}
	sent, err := f.a.StartSend(f.bID.ID(), []string{source}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	var incoming TaskSnapshot
	waitFor(t, 10*time.Second, func() bool {
		for _, task := range f.b.Tasks() {
			if task.State == "awaiting_acceptance" {
				incoming = task
				return true
			}
		}
		return false
	}, "missing receive consent")
	if err = f.b.AcceptTask(incoming.ID); err != nil {
		t.Fatal(err)
	}
	select {
	case <-first:
	case <-time.After(10 * time.Second):
		t.Fatal("first chunk not sent")
	}
	waitTask(t, f.b, incoming.ID, func(v TaskSnapshot) bool { return v.VerifiedBytes >= transfer.DefaultChunkSize })
	cfg.onChunkSent = nil
	if err = f.b.StartInbox(newDir, cfg); err != nil {
		t.Fatal(err)
	}
	if snap, _ := f.b.Task(incoming.ID); snap.TargetDirectory != oldDir || snap.State == "cancel_requested" || snap.State == "cancelled" {
		t.Fatalf("settings changed active receive: %+v", snap)
	}
	close(release)
	got := waitTask(t, f.b, incoming.ID, func(v TaskSnapshot) bool { return isTerminal(v.State) })
	if got.State != "completed" {
		t.Fatalf("reconfigure interrupted transfer: %+v", got)
	}
	waitTask(t, f.a, sent.ID, func(v TaskSnapshot) bool { return v.State == "completed" })
	if _, err = os.Stat(filepath.Join(oldDir, "ongoing.bin")); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(newDir, "ongoing.bin")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("active file moved to new preferences directory")
	}
	waitFor(t, 5*time.Second, func() bool { return f.b.InboxStatus().Listening }, "inbox did not resume listening")
	second := filepath.Join(t.TempDir(), "next.txt")
	if err = os.WriteFile(second, []byte("new preference"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = f.b.SetAlwaysAccept(f.aID.ID(), true); err != nil {
		t.Fatal(err)
	}
	next, err := f.a.StartSend(f.bID.ID(), []string{second}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	final := waitTask(t, f.a, next.ID, func(v TaskSnapshot) bool { return isTerminal(v.State) })
	if final.State != "completed" {
		t.Fatalf("next send failed: %+v", final)
	}
	if _, err = os.Stat(filepath.Join(newDir, "next.txt")); err != nil {
		t.Fatalf("next transfer missed new directory: %v", err)
	}
}

func TestSenderRejectsChangedSelectionBeforeBody(t *testing.T) {
	task, err := newTaskManager().create(TaskSnapshot{Direction: "send"}, func() {})
	if err != nil {
		t.Fatal(err)
	}
	task.recovery.SelectionDigest = strings.Repeat("a", 64)
	if err = task.recordSelectionAccepted(task.snapshot().AttemptID, transfer.Selection{Digest: strings.Repeat("b", 64)}, transfer.PlanSummary{}); !errors.Is(err, transfer.ErrPlanMismatch) {
		t.Fatalf("changed selection accepted: %v", err)
	}
	task.mu.Lock()
	task.snap.State = "cancel_requested"
	task.mu.Unlock()
	if err = task.recordSelectionAccepted(task.snapshot().AttemptID, transfer.Selection{Digest: strings.Repeat("a", 64)}, transfer.PlanSummary{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("late acceptance overtook cancellation: %v", err)
	}
}

func TestReceiveCompletionCannotOvertakeControlIntent(t *testing.T) {
	for _, terminal := range []string{"Completed", "NoContent"} {
		for intent, want := range map[string]string{"cancel_requested": "cancelled", "pause_requested": "paused", "shutdown_requested": "recovering"} {
			t.Run(terminal+"/"+intent, func(t *testing.T) {
				record, err := newTaskManager().create(TaskSnapshot{Direction: "receive", PeerID: strings.Repeat("a", 64), TargetDirectory: t.TempDir()}, func() {})
				if err != nil {
					t.Fatal(err)
				}
				record.recovery.TransferID = strings.Repeat("b", 32)
				record.recovery.ManifestDigest = strings.Repeat("c", 64)
				record.recovery.ChunkSize = transfer.DefaultChunkSize
				record.mu.Lock()
				record.snap.State = intent
				record.mu.Unlock()
				record.completeTransfer(record.snapshot().AttemptID, transfer.Result{State: terminal}, "")
				if got := record.snapshot(); got.State != want {
					t.Fatalf("late %s replaced %s: %+v", terminal, intent, got)
				}
			})
		}
	}
}
