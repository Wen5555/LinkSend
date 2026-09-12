package app

import (
	"context"
	"errors"
	"io/fs"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/Wen5555/LinkSend/internal/protocol"
	"github.com/Wen5555/LinkSend/internal/transfer"
)

func TestTaskPhaseTimelineRecordsOnlyTransitions(t *testing.T) {
	manager := newTaskManager()
	record, err := manager.create(TaskSnapshot{Direction: "send"}, func() {})
	if err != nil {
		t.Fatal(err)
	}
	initial := record.snapshot()
	if initial.Phase != "preparing" || len(initial.PhaseTimeline) != 1 || initial.PhaseTimeline[0].Phase != "preparing" {
		t.Fatalf("missing initial phase event: %+v", initial)
	}
	record.update(func(v *TaskSnapshot) { v.Phase = "signaling_connect" })
	record.update(func(v *TaskSnapshot) { v.Phase = "signaling_connect" })
	snapshot := record.snapshot()
	if len(snapshot.PhaseTimeline) != 2 || snapshot.PhaseTimeline[1].Phase != "signaling_connect" || snapshot.PhaseTimeline[1].At == "" {
		t.Fatalf("unexpected phase timeline: %+v", snapshot.PhaseTimeline)
	}
}

func TestTaskManagerTerminalAndCancelRace(t *testing.T) {
	svc, err := New(Config{DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	_, cancel := context.WithCancel(context.Background())
	task, err := svc.tasks.create(TaskSnapshot{Direction: "send"}, cancel)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.CancelTask(task.snap.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.CancelTask(task.snap.ID); err == nil {
		t.Fatal("repeated cancel should report pending cancellation")
	}
	task.finish("completed", nil)
	got := task.snapshot()
	if got.State != "cancelled" {
		t.Fatalf("cancel request was overwritten: %+v", got)
	}
	task.update(func(v *TaskSnapshot) { v.State = "transferring" })
	if task.snapshot().State != "cancelled" {
		t.Fatal("late progress changed cancellation request")
	}
	cancel()
}

func TestManifestSummaryIsBoundedAndUseful(t *testing.T) {
	m := transfer.Manifest{Files: []transfer.FileEntry{{Path: "目录/报告.txt", Type: "file"}, {Path: "目录/空目录", Type: "directory"}, {Path: "第二.txt", Type: "file"}, {Path: "第三.txt", Type: "file"}}}
	got := manifestSummary(m)
	if got == "" || len(got) > 128 || got == "4 个目录" {
		t.Fatalf("unexpected manifest summary: %q", got)
	}
}

func TestTaskAcceptanceDecisionAndRepeat(t *testing.T) {
	svc, err := New(Config{DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	_, cancel := context.WithCancel(context.Background())
	task, err := svc.tasks.create(TaskSnapshot{Direction: "receive"}, cancel)
	if err != nil {
		t.Fatal(err)
	}
	task.decision = make(chan bool, 1)
	task.update(func(v *TaskSnapshot) { v.State = "awaiting_acceptance" })
	if err := svc.AcceptTask(task.snap.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.AcceptTask(task.snap.ID); err == nil {
		t.Fatal("repeated decision should fail")
	}
	select {
	case accepted := <-task.decision:
		if !accepted {
			t.Fatal("acceptance lost")
		}
	case <-time.After(time.Second):
		t.Fatal("decision not delivered")
	}
	cancel()
}

func TestTaskBusyAndRetryCreatesNewID(t *testing.T) {
	svc, err := New(Config{DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(svc.Shutdown)
	_, cancel := context.WithCancel(context.Background())
	failed, err := svc.tasks.create(TaskSnapshot{Direction: "send"}, cancel)
	if err != nil {
		t.Fatal(err)
	}
	failed.finish("failed", errors.New("source failed"))
	failed.peerID, failed.paths, failed.cfg = "peer", []string{"missing"}, DirectConfig{}
	// A retry is a fresh task with a distinct local ID; its invalid source then fails independently.
	retried, err := svc.RetryTask(failed.snap.ID)
	if err != nil {
		t.Fatal(err)
	}
	if retried.ID == failed.snap.ID {
		t.Fatal("retry reused task id")
	}
	if len(svc.Tasks()) != 2 {
		t.Fatalf("original failure was not retained: %+v", svc.Tasks())
	}
	cancel()
}

func TestClassifyTaskErrorKeepsStableCodesAndHidesRawDetails(t *testing.T) {
	cases := []struct {
		name string
		err  error
		code protocol.Code
		want string
	}{
		{"rejected", transfer.ErrRejected, protocol.ReceiveRejected, "接收方拒绝"},
		{"changed", transfer.ErrChanged, protocol.SourceChanged, "源文件"},
		{"integrity", transfer.ErrIntegrity, protocol.IntegrityFailed, "完整性"},
		{"path", transfer.ErrPath, protocol.UnsafePath, "目标路径"},
		{"conflict", transfer.ErrConflict, "FILE_CONFLICT", "不会覆盖"},
		{"permission", fs.ErrPermission, "PERMISSION_DENIED", "权限"},
		{"disk", syscall.ENOSPC, protocol.DiskFull, "空间不足"},
		{"peer conflict", &transfer.PeerError{Detail: "FILE_CONFLICT"}, "FILE_CONFLICT", "不会覆盖"},
		{"peer permission", &transfer.PeerError{Detail: "PERMISSION_DENIED"}, "PERMISSION_DENIED", "权限"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := classifyTaskError(tc.err)
			if protocol.ErrorCode(got) != tc.code {
				t.Fatalf("code=%s, want %s", protocol.ErrorCode(got), tc.code)
			}
			if msg := userError(got); msg == "" || !strings.Contains(msg, tc.want) || strings.Contains(msg, tc.err.Error()) {
				t.Fatalf("unsafe/unhelpful user message: %q", msg)
			}
			if !errors.Is(got, tc.err) {
				t.Fatalf("wrapped cause was not retained: %v", got)
			}
		})
	}
	raw := errors.New("signaling HTTP 500: stack at secret/path")
	msg := userError(classifyTaskError(raw))
	if strings.Contains(msg, "secret/path") || strings.Contains(msg, "HTTP 500") {
		t.Fatalf("raw detail leaked: %q", msg)
	}
	msg = userError(protocol.Fail(protocol.DirectFailed, "private stack / token"))
	if strings.Contains(msg, "private stack") || strings.Contains(msg, "token") {
		t.Fatalf("protocol detail leaked: %q", msg)
	}
}

func TestOldAttemptCallbacksCannotOverwriteNewAttempt(t *testing.T) {
	manager := newTaskManager()
	record, err := manager.create(TaskSnapshot{Direction: "send", PeerID: "peer"}, func() {})
	if err != nil {
		t.Fatal(err)
	}
	oldAttempt := record.snapshot().AttemptID
	record.mu.Lock()
	record.snap.AttemptID = protocol.RandomID()
	record.snap.State = "recovering"
	record.snap.Phase = "recovering"
	record.snap.Revision++
	newAttempt := record.snap.AttemptID
	newRevision := record.snap.Revision
	record.mu.Unlock()
	if record.updateAttempt(oldAttempt, func(v *TaskSnapshot) { v.State = "completed" }) {
		t.Fatal("stale attempt update was accepted")
	}
	record.finishAttempt(oldAttempt, "failed", errors.New("late failure"))
	got := record.snapshot()
	if got.AttemptID != newAttempt || got.Revision != newRevision || got.State != "recovering" {
		t.Fatalf("late callback overwrote new attempt: %+v", got)
	}
}
