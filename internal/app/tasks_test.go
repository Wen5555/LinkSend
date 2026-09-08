package app

import (
	"context"
	"errors"
	"testing"
	"time"
)

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
