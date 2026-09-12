package protocol

import "testing"

func TestNoContentIsTerminalTransferState(t *testing.T) {
	state := NewTransferState()
	for _, next := range []string{"AwaitingAcceptance", "Transferring", "Verifying", "NoContent"} {
		if err := state.Transition(next); err != nil {
			t.Fatal(err)
		}
	}
	if err := state.Transition("Completed"); err == nil {
		t.Fatal("empty accepted subset changed into full completion")
	}
}
