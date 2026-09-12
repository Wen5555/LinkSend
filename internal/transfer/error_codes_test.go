package transfer

import (
	"bytes"
	"errors"
	"io/fs"
	"syscall"
	"testing"
)

func TestPeerErrorCodeRoundTrip(t *testing.T) {
	for _, cause := range []error{ErrConflict, fs.ErrPermission, syscall.ENOSPC, ErrChanged, ErrIntegrity, ErrPath, ErrCancelled, ErrPlanUnsupported, ErrPlanMismatch, ErrPlanInvalid, ErrPlanPersistence, ErrResumeMismatch} {
		var wire bytes.Buffer
		wrapped := &fs.PathError{Op: "write", Path: "/private/receiver/file", Err: cause}
		if err := writeControl(&wire, control{Op: opError, Error: peerErrorCode(wrapped)}); err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(wire.Bytes(), []byte("/private")) {
			t.Fatal("local path leaked into peer error")
		}
		_, err := readControl(&wire)
		if !errors.Is(err, cause) || !errors.Is(err, ErrPeerError) {
			t.Fatalf("lost error classification for %v", cause)
		}
	}
	for _, text := range []string{"old peer error", "FILE_CONFLICT: attacker text", "prefix PERMISSION_DENIED"} {
		err := &PeerError{Detail: text}
		if errors.Is(err, ErrConflict) || errors.Is(err, fs.ErrPermission) {
			t.Fatal("unrecognized peer detail gained typed semantics")
		}
	}
	if peerErrorCode(errors.New("private path or secret")) != "TRANSFER_FAILED" {
		t.Fatal("unknown cause was exposed")
	}
}
