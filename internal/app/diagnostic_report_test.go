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
)

func TestDiagnosticReportAllowlistDropsSecretBearingFields(t *testing.T) {
	secret := "SHOULD_NEVER_EXPORT_FAKE_TOKEN_7d843a"
	privatePath := `C:\Private\` + secret + `\source.txt`
	d := Diagnostics{Version: secret, Platform: secret, ServerURL: "https://user:" + secret + "@example.test/" + secret + "?token=" + secret,
		ServerHealth: secret, HealthFailure: secret, HistoryError: secret, Identity: DiagnosticIdentity{ID: secret, PublicKey: secret},
		LANProviderErrors: map[string]string{secret: privatePath}, Capabilities: protocol.Capabilities{ProductVersion: secret, Transport: secret, Relay: true}}
	task := TaskSnapshot{ID: secret, TaskID: secret, AttemptID: secret, PeerID: secret, SourceSummary: privatePath, ManifestSummary: secret, TargetDirectory: privatePath,
		State: secret, Phase: secret, Direction: secret, ErrorCode: secret, ErrorMessage: secret, SessionID: secret, TransferID: secret, ManifestDigest: secret,
		BaseSocket: secret, NetworkInterface: privatePath, LocalCandidate: secret, RemoteCandidate: secret, ConnectionMethod: secret, TransportProtocol: secret, UpdatedAt: privatePath, AttemptStartedAt: privatePath}
	task.Diagnostic = &DiagnosticTask{ProductVersion: secret, Label: secret, Guidance: DiagnosticGuidance{Code: secret, Stage: secret, Reason: secret, NextStep: secret, Action: secret}}
	tasks := make([]TaskSnapshot, 40)
	for i := range tasks {
		tasks[i] = task
	}
	report := makeDiagnosticReport(d, tasks, time.Date(2026, 10, 19, 1, 2, 3, 0, time.UTC))
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	for _, needle := range []string{secret, "C:\\Private", "example.test", "user:", "source.txt"} {
		if bytes.Contains(data, []byte(needle)) {
			t.Fatalf("private source field leaked: %q", needle)
		}
	}
	if report.ProductVersion != protocol.ProductVersion || report.RelayAvailable || len(report.Tasks) != 32 || !report.TasksTruncated {
		t.Fatal("report did not bound/normalize private input")
	}
	if report.Tasks[0].Guidance.Code != "UNKNOWN" || report.Tasks[0].LastAttempt != "" || report.Tasks[0].State != "unknown" {
		t.Fatal("unknown strings acquired export authority")
	}
	if len(data) > 128<<10 {
		t.Fatal("bounded report exceeded export budget")
	}
}

func TestDiagnosticAttemptTimeAndProjectionDoNotMutatePrivateRecord(t *testing.T) {
	s, err := New(Config{DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Shutdown)
	record, err := s.tasks.create(TaskSnapshot{Direction: "send"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, ok := s.Task(record.snap.ID)
	if !ok || snapshot.AttemptStartedAt == "" || snapshot.Diagnostic == nil || snapshot.Diagnostic.LastAttempt != safeDiagnosticTime(snapshot.AttemptStartedAt) {
		t.Fatal("new task has no truthful attempt time")
	}
	if record.snapshot().Diagnostic != nil {
		t.Fatal("presentation projection mutated private owner state")
	}
	legacy := makeDiagnosticTask(TaskSnapshot{UpdatedAt: time.Now().UTC().Format(time.RFC3339)}, "task")
	if legacy.LastAttempt != "" {
		t.Fatal("legacy status update was fabricated as an attempt start")
	}
	known := makeDiagnosticTask(TaskSnapshot{AttemptStartedAt: "2026-10-19T10:00:00+08:00"}, "task")
	if known.LastAttempt != "2026-10-19T02:00:00Z" {
		t.Fatal("attempt timezone was not normalized", known.LastAttempt)
	}
}

func TestDiagnosticGuidanceSeparatesActionableFailuresAndClearsCompletedError(t *testing.T) {
	cases := []struct {
		code          protocol.Code
		stage, action string
	}{
		{protocol.LANDiscoveryUnavailable, "discovery", "open_devices"},
		{protocol.Unpaired, "pairing", "open_devices"},
		{protocol.SignalingUnreachable, "signaling", "open_network"},
		{protocol.CheckTimeout, "ice", "open_network"},
		{protocol.AuthenticationFailed, "authentication", "open_devices"},
		{protocol.DiskFull, "storage", "open_receive"},
		{protocol.SourceChanged, "source", "review_source"},
	}
	for _, test := range cases {
		g := taskDiagnosticGuidance(TaskSnapshot{State: "failed", ErrorCode: string(test.code)})
		if g.Code != string(test.code) || g.Stage != test.stage || g.Action != test.action || g.Reason == "" || g.NextStep == "" {
			t.Fatal("missing actionable reason", test.code, g)
		}
		if test.code == protocol.DiskFull && strings.Contains(g.NextStep, "清理缓存") {
			t.Fatal("disk guidance delegates cache cleanup to user")
		}
	}
	if g := taskDiagnosticGuidance(TaskSnapshot{State: "awaiting_acceptance"}); g.Stage != "confirmation" || g.Action != "wait" {
		t.Fatal("receiver confirmation mistaken for network failure", g)
	}
	if g := taskDiagnosticGuidance(TaskSnapshot{State: "completed", ErrorCode: string(protocol.CheckTimeout)}); g.Code != "NONE" || g.Action != "none" {
		t.Fatal("completed task retained stale connection advice", g)
	}
	if g := taskDiagnosticGuidance(TaskSnapshot{State: "failed", ErrorCode: "AUTHENTICATION_FAILED: fake-private-detail"}); g.Code != "UNKNOWN" {
		t.Fatal("arbitrary log prefix became business state", g)
	}
}

func TestDiagnosticExportWritesOnlySafeJSONAndNeverOverwritesUserFile(t *testing.T) {
	s, err := New(Config{DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Shutdown)
	directory := t.TempDir()
	destination := filepath.Join(directory, "LinkSend-diagnostics.json")
	if err := s.ExportDiagnosticReport(t.Context(), destination); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	var report DiagnosticReport
	if err := json.Unmarshal(data, &report); err != nil || report.Version != 1 || report.ServerHealth != "not_configured" {
		t.Fatal("export is not a safe versioned report", err)
	}
	if bytes.Contains(data, []byte(s.cfg.DataDir)) || bytes.Contains(data, []byte(s.identity.ID())) {
		t.Fatal("export leaked profile location or stable identity")
	}
	if err := s.ExportDiagnosticReport(t.Context(), destination); !errors.Is(err, os.ErrExist) {
		t.Fatal("existing user file was replaced", err)
	}
	if got, err := os.ReadFile(destination); err != nil || !bytes.Equal(got, data) {
		t.Fatal("existing export changed on no-replace collision", err)
	}
	entries, err := os.ReadDir(directory)
	if err != nil || len(entries) != 1 {
		t.Fatal("failed export left partial temporary output", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	cancelled := filepath.Join(directory, "cancelled.json")
	if err := s.ExportDiagnosticReport(ctx, cancelled); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled export continued", err)
	}
	if _, err := os.Stat(cancelled); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("cancelled export wrote a file", err)
	}
	if err := s.ExportDiagnosticReport(t.Context(), filepath.Join(directory, "private.key")); err == nil {
		t.Fatal("non-JSON destination accepted")
	}
}
