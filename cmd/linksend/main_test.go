package main

import (
	"context"
	"errors"
	"testing"

	"github.com/Wen5555/LinkSend/internal/app"
)

func TestUnimplementedTaskCommandsHaveNoSideEffects(t *testing.T) {
	for _, command := range []string{"accept", "reject", "status", "resume", "cancel"} {
		t.Run(command, func(t *testing.T) {
			if err := run(context.Background(), nil, command, nil); !errors.Is(err, app.ErrNotImplemented) {
				t.Fatalf("command %q returned %v, want NOT_IMPLEMENTED", command, err)
			}
		})
	}
}
