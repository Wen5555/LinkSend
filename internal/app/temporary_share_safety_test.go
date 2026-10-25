package app

import (
	"bytes"
	"encoding/base64"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wen5555/LinkSend/internal/identity"
	"github.com/Wen5555/LinkSend/internal/protocol"
	"github.com/Wen5555/LinkSend/internal/sharegrant"
	"github.com/Wen5555/LinkSend/internal/signaling"
	"github.com/Wen5555/LinkSend/internal/transfer"
)

func TestTemporaryShareExactApprovalAndCopyCodeGate(t *testing.T) {
	f := newTemporaryShareFixture(t)
	created, err := f.a.CreateTemporaryShare(t.Context(), []string{f.source}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.a.CheckTemporaryShareCode(created.Share.ID, created.Code); err != nil {
		t.Fatal(err)
	}
	wrong, _, _ := sharegrant.GenerateCode()
	if err = f.a.CheckTemporaryShareCode(created.Share.ID, wrong); err == nil || strings.Contains(err.Error(), wrong) {
		t.Fatal("different share code was accepted or echoed")
	}
	claimed, err := f.b.ClaimTemporaryShare(t.Context(), created.Code)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.b.CheckTemporaryShareCode(claimed.ID, created.Code); err == nil {
		t.Fatal("receiver acquired sender copy authority")
	}
	pending, err := f.a.RefreshTemporaryShare(t.Context(), created.Share.ID)
	if err != nil {
		t.Fatal(err)
	}
	stranger, _ := identity.Generate()
	if _, err = f.a.ApproveTemporaryShare(t.Context(), pending.ID, pending.ClaimID, base64.RawStdEncoding.EncodeToString(stranger.PublicKey()), pending.Generation); err == nil {
		t.Fatal("changed receiver key was approved")
	}
	if _, err = f.a.ApproveTemporaryShare(t.Context(), pending.ID, pending.ClaimID, pending.ReceiverPublicKey, pending.Generation+1); err == nil {
		t.Fatal("changed claim generation was approved")
	}
	if after, err := f.a.RefreshTemporaryShare(t.Context(), pending.ID); err != nil || after.GrantState != sharegrant.Pending {
		t.Fatal("invalid approval mutated original pending grant", err)
	}
	if _, err = f.a.RevokeTemporaryShare(t.Context(), pending.ID); err != nil {
		t.Fatal(err)
	}
	if err = f.a.CheckTemporaryShareCode(pending.ID, created.Code); err == nil {
		t.Fatal("revoked code could still be copied")
	}
}

func TestTemporaryShareSourceChangeCannotReuseOriginalGrant(t *testing.T) {
	f := newTemporaryShareFixture(t)
	created, _ := createApprovedTemporaryShare(t, f)
	if err := os.WriteFile(f.source, []byte("changed after explicit source selection"), 0600); err != nil {
		t.Fatal(err)
	}
	task, err := f.a.StartTemporaryShare(t.Context(), created.Share.ID, DirectConfig{AllowLoopback: true})
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, time.Second, func() bool { v, _ := f.a.Task(task.ID); return v.State == "failed" }, "source change was not rejected before connecting")
	v, _ := f.a.Task(task.ID)
	if v.ErrorCode != string(protocol.SourceChanged) || v.SentBytes != 0 || v.SessionID != "" || v.CanRetry {
		t.Fatal("changed source reused grant or ordinary retry", v.ErrorCode, v.SentBytes, v.SessionID, v.CanRetry)
	}
	if _, err = f.a.RetryTask(task.ID); err == nil {
		t.Fatal("temporary task entered ordinary retry")
	}
	if owner := onlyTemporary(t, f.a); owner.State != "stopped" || owner.PendingAction != signaling.ShareRevoke {
		t.Fatal("source change failed to retire original grant")
	}
}

func TestTemporaryShareCompletionResponseLossReconcilesWithoutNewFileTask(t *testing.T) {
	f := newTemporaryShareFixture(t)
	created, claimed := createApprovedTemporaryShare(t, f)
	drop := &temporaryDropTransport{base: f.h.Client().Transport, path: "/v1/shares/complete"}
	client, _ := signaling.New(signaling.Config{ServerURL: f.origin, Identity: f.b.identity, HTTPClient: &http.Client{Transport: drop, Timeout: 5 * time.Second}})
	f.b.signal = client
	cfg := DirectConfig{AllowLoopback: true, CheckTimeout: 5 * time.Second, WaitTimeout: 8 * time.Second}
	destination := t.TempDir()
	rtask, err := f.b.StartTemporaryShare(t.Context(), claimed.ID, cfg)
	if err != nil {
		t.Fatal(err)
	}
	stask, err := f.a.StartTemporaryShare(t.Context(), created.Share.ID, cfg)
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, 8*time.Second, func() bool { return onlyTemporary(t, f.b).State == "awaiting_save" }, "preview missing")
	if _, err = f.b.ConfirmTemporaryShareSave(t.Context(), claimed.ID, destination, transfer.PlanRequest{}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, 4*time.Second, func() bool { v, _ := f.b.Task(rtask.ID); return v.Phase == "temporary_share_completion_check" }, "lost completion response did not retain explicit reconciliation")
	if _, err = f.b.ResumeTask(rtask.ID, cfg); err == nil {
		t.Fatal("unresolved completion reopened file engine")
	}
	if _, err = f.b.RetryTemporaryShareAction(t.Context(), claimed.ID); err != nil {
		t.Fatal(err)
	}
	waitFor(t, time.Second, func() bool {
		a, _ := f.a.Task(stask.ID)
		b, _ := f.b.Task(rtask.ID)
		return a.State == "completed" && b.State == "completed"
	}, "completion retry did not reconcile original terminal task")
	if len(f.b.Tasks()) != 1 {
		t.Fatal("completion retry created a second file task")
	}
	entries, err := os.ReadDir(destination)
	if err != nil {
		t.Fatal(err)
	}
	files := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			files++
		}
	}
	if files != 1 {
		t.Fatal("completion retry created a second file")
	}
}

func TestTemporaryShareApprovalAndRevokeResponseLossKeepExactIntents(t *testing.T) {
	f := newTemporaryShareFixture(t)
	created, err := f.a.CreateTemporaryShare(t.Context(), []string{f.source}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.b.ClaimTemporaryShare(t.Context(), created.Code); err != nil {
		t.Fatal(err)
	}
	pending, err := f.a.RefreshTemporaryShare(t.Context(), created.Share.ID)
	if err != nil {
		t.Fatal(err)
	}
	drop := &temporaryDropTransport{base: f.h.Client().Transport, path: "/v1/shares/approve"}
	client, _ := signaling.New(signaling.Config{ServerURL: f.origin, Identity: f.a.identity, HTTPClient: &http.Client{Transport: drop, Timeout: 5 * time.Second}})
	f.a.signal = client
	if _, err = f.a.ApproveTemporaryShare(t.Context(), pending.ID, pending.ClaimID, pending.ReceiverPublicKey, pending.Generation); err == nil {
		t.Fatal("controlled approval response was not lost")
	}
	e, _ := f.a.temporaryEntry(pending.ID)
	original := temporaryRecordCopy(e)
	if original.Intent == nil || original.Intent.Operation != signaling.ShareApprove {
		t.Fatal("lost approval discarded stable intent")
	}
	if _, err = f.a.RetryTemporaryShareAction(t.Context(), pending.ID); err != nil {
		t.Fatal(err)
	}
	approved := temporaryRecordCopy(e)
	if approved.ApprovalID != original.ApprovalID || approved.Status.TaskID != original.Manifest.TransferID || approved.Status.ClaimID != original.Status.ClaimID || approved.Status.ReceiverKey != original.Status.ReceiverKey {
		t.Fatal("approval retry changed the explicit confirmed actor or task")
	}
	drop2 := &temporaryDropTransport{base: f.h.Client().Transport, path: "/v1/shares/revoke"}
	client, _ = signaling.New(signaling.Config{ServerURL: f.origin, Identity: f.a.identity, HTTPClient: &http.Client{Transport: drop2, Timeout: 5 * time.Second}})
	f.a.signal = client
	if _, err = f.a.RevokeTemporaryShare(t.Context(), pending.ID); err == nil {
		t.Fatal("controlled revoke response was not lost")
	}
	stopped := temporaryRecordCopy(e)
	if !stopped.Stopped || stopped.Intent == nil || stopped.Intent.Operation != signaling.ShareRevoke {
		t.Fatal("revoke loss reopened local authority")
	}
	if _, err = f.a.StartTemporaryShare(t.Context(), pending.ID, DirectConfig{}); err == nil {
		t.Fatal("revoke intent allowed another preview")
	}
	if _, err = f.a.RetryTemporaryShareAction(t.Context(), pending.ID); err != nil {
		t.Fatal(err)
	}
	after := temporaryRecordCopy(e)
	if !after.Stopped || after.Status.State != sharegrant.Revoked || after.Intent != nil || after.TerminalObservedAt == 0 {
		t.Fatal("revoke retry changed its terminal result")
	}
}

func TestTemporaryShareBindResponseLossKeepsUserConfirmedPlanWithoutSendingBody(t *testing.T) {
	f := newTemporaryShareFixture(t)
	created, claimed := createApprovedTemporaryShare(t, f)
	drop := &temporaryDropTransport{base: f.h.Client().Transport, path: "/v1/shares/bind"}
	client, _ := signaling.New(signaling.Config{ServerURL: f.origin, Identity: f.b.identity, HTTPClient: &http.Client{Transport: drop, Timeout: 5 * time.Second}})
	f.b.signal = client
	cfg := DirectConfig{AllowLoopback: true, CheckTimeout: 5 * time.Second, WaitTimeout: 8 * time.Second}
	destination := t.TempDir()
	if _, err := f.b.StartTemporaryShare(t.Context(), claimed.ID, cfg); err != nil {
		t.Fatal(err)
	}
	stask, err := f.a.StartTemporaryShare(t.Context(), created.Share.ID, cfg)
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, 8*time.Second, func() bool { return onlyTemporary(t, f.b).State == "awaiting_save" }, "preview missing")
	if _, err = f.b.ConfirmTemporaryShareSave(t.Context(), claimed.ID, destination, transfer.PlanRequest{ConflictPolicy: transfer.ConflictKeepBoth}); err == nil {
		t.Fatal("controlled bind response was not lost")
	}
	e, _ := f.b.temporaryEntry(claimed.ID)
	before := temporaryRecordCopy(e)
	if before.Intent == nil || before.Intent.Operation != signaling.ShareBind || before.Plan == nil || before.Directory != destination {
		t.Fatal("bind loss discarded the explicitly confirmed fixed plan")
	}
	waitFor(t, time.Second, func() bool { e.mu.Lock(); defer e.mu.Unlock(); return !e.active }, "failed preview did not settle")
	if _, err = f.b.RetryTemporaryShareAction(t.Context(), claimed.ID); err != nil {
		t.Fatal(err)
	}
	after := temporaryRecordCopy(e)
	if after.Status.State != sharegrant.Bound || after.Status.Plan != before.Plan.Digest() || after.Status.TaskID != before.Status.TaskID || after.Status.ResumeUntil == 0 {
		t.Fatal("bind retry changed confirmed scope")
	}
	if entries, err := os.ReadDir(destination); err != nil || len(entries) != 0 {
		t.Fatal("bind reconciliation opened file body/storage implicitly", err)
	}
	if task, _ := f.a.Task(stask.ID); task.SentBytes != 0 {
		t.Fatal("response loss retried a file operation implicitly")
	}
}

func TestTemporaryShareRestartDoesNotApproveOrRunPreview(t *testing.T) {
	f := newTemporaryShareFixture(t)
	created, claimed := createApprovedTemporaryShare(t, f)
	cfg := DirectConfig{AllowLoopback: true, CheckTimeout: 5 * time.Second, WaitTimeout: 8 * time.Second}
	if _, err := f.b.StartTemporaryShare(t.Context(), claimed.ID, cfg); err != nil {
		t.Fatal(err)
	}
	if _, err := f.a.StartTemporaryShare(t.Context(), created.Share.ID, cfg); err != nil {
		t.Fatal(err)
	}
	waitFor(t, 8*time.Second, func() bool { return onlyTemporary(t, f.b).State == "awaiting_save" }, "preview was not presented")
	adir, bdir := f.a.cfg.DataDir, f.b.cfg.DataDir
	f.a.Shutdown()
	f.b.Shutdown()
	before := f.requests.Load()
	reopen := func(directory string) *Service {
		s, err := New(Config{DataDir: directory, ServerURL: f.origin, AllowInsecureLoopback: true})
		if err != nil {
			t.Fatal(err)
		}
		client, err := signaling.New(signaling.Config{ServerURL: f.origin, Identity: s.identity, HTTPClient: f.h.Client()})
		if err != nil {
			t.Fatal(err)
		}
		s.signal = client
		t.Cleanup(s.Shutdown)
		return s
	}
	a, b := reopen(adir), reopen(bdir)
	if onlyTemporary(t, a).State != "needs_attention" || onlyTemporary(t, b).State != "needs_attention" {
		t.Fatal("restart failed to expose explicit original recovery")
	}
	_ = a.Tasks()
	_ = b.Tasks()
	if f.requests.Load() != before {
		t.Fatal("restart or snapshot automatically contacted the share service")
	}
	if _, err := b.ResumeTemporaryShare(t.Context(), claimed.ID, cfg); err == nil {
		t.Fatal("unconfirmed preview resumed as a bound file task")
	}
}

func TestTemporaryShareRevokeRetiresBoundScopeBeforeBody(t *testing.T) {
	f := newTemporaryShareFixture(t)
	created, claimed := createApprovedTemporaryShare(t, f)
	destination := t.TempDir()
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	f.a.beforeFileOpen = func(*PeerSession) { close(entered); <-release }
	cfg := DirectConfig{AllowLoopback: true, CheckTimeout: 5 * time.Second, WaitTimeout: 8 * time.Second}
	if _, err := f.b.StartTemporaryShare(t.Context(), claimed.ID, cfg); err != nil {
		t.Fatal(err)
	}
	stask, err := f.a.StartTemporaryShare(t.Context(), created.Share.ID, cfg)
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, 8*time.Second, func() bool { return onlyTemporary(t, f.b).State == "awaiting_save" }, "preview was not presented")
	if _, err = f.b.ConfirmTemporaryShareSave(t.Context(), claimed.ID, destination, transfer.PlanRequest{}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-entered:
	case <-time.After(8 * time.Second):
		t.Fatal("bound pre-body gate was not reached")
	}
	if _, err = f.a.RevokeTemporaryShare(t.Context(), created.Share.ID); err != nil {
		t.Fatal(err)
	}
	waitFor(t, 2*time.Second, func() bool { return onlyTemporary(t, f.b).State == "stopped" }, "explicit terminal notification did not retire receiver owner")
	unblock()
	waitFor(t, time.Second, func() bool { v, _ := f.a.Task(stask.ID); return isTerminal(v.State) }, "revoked sender did not stop")
	if v, _ := f.a.Task(stask.ID); v.SentBytes != 0 {
		t.Fatal("revoked scope sent file body")
	}
	if entries, err := os.ReadDir(destination); err != nil || len(entries) != 0 {
		t.Fatal("revocation before file offer created destination data", err)
	}
	if _, err = f.b.ResumeTemporaryShare(t.Context(), claimed.ID, cfg); err == nil {
		t.Fatal("retired receiver resumed revoked scope")
	}
}

func TestTemporaryShareBoundPlanDoesNotRenameAroundNewUserFile(t *testing.T) {
	f := newTemporaryShareFixture(t)
	created, claimed := createApprovedTemporaryShare(t, f)
	destination := t.TempDir()
	target := filepath.Join(destination, filepath.Base(f.source))
	f.a.beforeFileOpen = func(*PeerSession) {
		if err := os.WriteFile(target, []byte("unrelated new user file"), 0600); err != nil {
			t.Error(err)
		}
	}
	cfg := DirectConfig{AllowLoopback: true, CheckTimeout: 5 * time.Second, WaitTimeout: 8 * time.Second}
	rtask, err := f.b.StartTemporaryShare(t.Context(), claimed.ID, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.a.StartTemporaryShare(t.Context(), created.Share.ID, cfg); err != nil {
		t.Fatal(err)
	}
	waitFor(t, 8*time.Second, func() bool { return onlyTemporary(t, f.b).State == "awaiting_save" }, "preview was not presented")
	if _, err = f.b.ConfirmTemporaryShareSave(t.Context(), claimed.ID, destination, transfer.PlanRequest{ConflictPolicy: transfer.ConflictKeepBoth}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, 3*time.Second, func() bool { v, _ := f.b.Task(rtask.ID); return v.ErrorCode != "" }, "changed destination plan was not rejected")
	if got, err := os.ReadFile(target); err != nil || string(got) != "unrelated new user file" {
		t.Fatal("bound plan changed or overwrote user data", err)
	}
	entries, err := os.ReadDir(destination)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Name() != filepath.Base(target) && !strings.HasPrefix(entry.Name(), ".linksend-") {
			t.Fatal("engine silently renamed the bound destination", entry.Name())
		}
	}
}

func TestTemporaryShareBoundRestartResumesOnlyOriginalTaskAndPlan(t *testing.T) {
	f := newTemporaryShareFixture(t)
	data := bytes.Repeat([]byte("0123456789abcdef"), 512<<10)
	if err := os.WriteFile(f.source, data, 0600); err != nil {
		t.Fatal(err)
	}
	previousRate := FileSendLimit()
	if err := ConfigureFileSendLimit(1 << 20); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ConfigureFileSendLimit(previousRate) })
	created, claimed := createApprovedTemporaryShare(t, f)
	destination := t.TempDir()
	cfg := DirectConfig{AllowLoopback: true, CheckTimeout: 5 * time.Second, WaitTimeout: 8 * time.Second}
	rtask, err := f.b.StartTemporaryShare(t.Context(), claimed.ID, cfg)
	if err != nil {
		t.Fatal(err)
	}
	stask, err := f.a.StartTemporaryShare(t.Context(), created.Share.ID, cfg)
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, 8*time.Second, func() bool { return onlyTemporary(t, f.b).State == "awaiting_save" }, "preview did not require save confirmation")
	if _, err = f.b.ConfirmTemporaryShareSave(t.Context(), claimed.ID, destination, transfer.PlanRequest{ConflictPolicy: transfer.ConflictKeepBoth}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, 10*time.Second, func() bool { v, _ := f.a.Task(stask.ID); return v.SentBytes >= 4<<20 && v.State == "transferring" }, "original task did not checkpoint a partial file")
	if err = f.a.PauseTask(stask.ID); err != nil {
		t.Fatal(err)
	}
	waitFor(t, 3*time.Second, func() bool {
		a, _ := f.a.Task(stask.ID)
		e, _ := f.b.temporaryEntry(claimed.ID)
		e.mu.Lock()
		active := e.active
		e.mu.Unlock()
		return a.State == "paused" && !active
	}, "paused task or remote stream did not settle")
	original := temporaryRecordCopy(f.b.temporaryShares.entries[claimed.ID])
	if original.Plan == nil {
		t.Fatal("confirmed receive plan missing")
	}
	adir, bdir := f.a.cfg.DataDir, f.b.cfg.DataDir
	f.a.Shutdown()
	f.b.Shutdown()
	before := f.requests.Load()
	reopen := func(dir string) *Service {
		s, err := New(Config{DataDir: dir, ServerURL: f.origin, AllowInsecureLoopback: true})
		if err != nil {
			t.Fatal(err)
		}
		client, _ := signaling.New(signaling.Config{ServerURL: f.origin, Identity: s.identity, HTTPClient: f.h.Client()})
		s.signal = client
		t.Cleanup(s.Shutdown)
		return s
	}
	a, b := reopen(adir), reopen(bdir)
	if f.requests.Load() != before {
		t.Fatal("restart implicitly reconnected")
	}
	if err = ConfigureFileSendLimit(0); err != nil {
		t.Fatal(err)
	}
	if _, err = b.ResumeTemporaryShare(t.Context(), claimed.ID, cfg); err != nil {
		t.Fatal(err)
	}
	if _, err = a.ResumeTask(stask.ID, cfg); err != nil {
		t.Fatal(err)
	}
	waitFor(t, 10*time.Second, func() bool {
		av, _ := a.Task(stask.ID)
		bv, _ := b.Task(rtask.ID)
		return av.State == "completed" && bv.State == "completed"
	}, "bound restart did not complete original file task")
	after := temporaryRecordCopy(b.temporaryShares.entries[claimed.ID])
	if after.Status.Plan != original.Status.Plan || after.Status.TaskID != original.Status.TaskID || after.Status.ClaimID != original.Status.ClaimID || after.Status.Generation != original.Status.Generation || after.Directory != original.Directory || after.Plan.Digest() != original.Plan.Digest() {
		t.Fatal("restart changed the original authorization or save plan")
	}
	got, err := os.ReadFile(filepath.Join(destination, filepath.Base(f.source)))
	if err != nil || !bytes.Equal(got, data) {
		t.Fatal("resumed file bytes changed", err)
	}
	if len(a.Tasks()) != 1 || len(b.Tasks()) != 1 {
		t.Fatal("restart created a second share file task")
	}
}
