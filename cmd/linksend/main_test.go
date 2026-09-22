package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/Wen5555/LinkSend/internal/app"
	"github.com/Wen5555/LinkSend/internal/protocol"
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

func TestDirectFailureJSONIncludesBoundedDiagnostic(t *testing.T) {
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	previous := os.Stdout
	os.Stdout = writer
	defer func() { os.Stdout = previous; _ = writer.Close() }()
	result := app.DirectTransferResult{FailurePhase: "quic_handshake", FailureDiagnostic: &app.TaskFailureDiagnostic{Stage: "quic_handshake", Category: "udp_socket", Code: "0x2751"}}
	err = printDirectFailure(result, protocol.Wrap(protocol.QUICHandshakeFailed, "QUIC handshake failed", errors.New("SECRET_RAW_CAUSE")))
	_ = writer.Close()
	os.Stdout = previous
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	var output struct {
		FailurePhase string                     `json:"failure_phase"`
		Diagnostic   *app.TaskFailureDiagnostic `json:"failure_diagnostic"`
		Error        string                     `json:"error"`
		Evidence     app.DirectEvidence         `json:"evidence"`
	}
	if err = json.Unmarshal(data, &output); err != nil {
		t.Fatal(err)
	}
	if output.FailurePhase != "quic_handshake" || output.Diagnostic == nil || output.Diagnostic.Code != "0x2751" || !strings.Contains(output.Error, "QUIC_HANDSHAKE_FAILED") {
		t.Fatalf("missing CLI evidence: %s", data)
	}
	if strings.Contains(string(data), "SECRET_RAW_CAUSE") || output.Evidence.TLSVersion != 0 || output.Evidence.ALPN != "" {
		t.Fatal("raw cause leaked or TLS fabricated")
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
