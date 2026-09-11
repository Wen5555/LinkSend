package main

import (
	"context"
	"errors"
	"testing"

	"github.com/Wen5555/LinkSend/internal/app"
)

func TestUnimplementedCrossProcessTaskCommandsHaveNoSideEffects(t *testing.T) {
	for _, command := range []string{"accept", "reject", "cancel"} {
		t.Run(command, func(t *testing.T) {
			if err := run(context.Background(), nil, command, nil); !errors.Is(err, app.ErrNotImplemented) {
				t.Fatalf("command %q returned %v, want NOT_IMPLEMENTED", command, err)
			}
		})
	}
}

func TestStatusReadsPersistedTaskList(t *testing.T) {
	svc, err := app.New(app.Config{DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(svc.Shutdown)
	if err = run(context.Background(), svc, "status", nil); err != nil {
		t.Fatal(err)
	}
}

func TestResumeRequiresExplicitTask(t *testing.T) {
	svc, err := app.New(app.Config{DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(svc.Shutdown)
	if err = run(context.Background(), svc, "resume", nil); err == nil || err.Error() != "resume requires --task" {
		t.Fatalf("unexpected resume validation: %v", err)
	}
}
