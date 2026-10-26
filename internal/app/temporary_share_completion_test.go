package app

import (
	"os"
	"testing"

	"github.com/Wen5555/LinkSend/internal/protocol"
	"github.com/Wen5555/LinkSend/internal/sharegrant"
	"github.com/Wen5555/LinkSend/internal/signaling"
	"github.com/Wen5555/LinkSend/internal/transfer"
)

func TestTemporaryShareTerminalControlRetryDoesNotInventLocalEngineCompletion(t *testing.T) {
	for _, operation := range []signaling.ShareOperation{signaling.ShareCreate, signaling.ShareApprove, signaling.ShareBind} {
		t.Run(string(operation), func(t *testing.T) {
			f := newTemporaryShareFixture(t)
			created, claimed := createApprovedTemporaryShare(t, f)
			senderEntry, _ := f.a.temporaryEntry(created.Share.ID)
			receiverEntry, _ := f.b.temporaryEntry(claimed.ID)
			sender := temporaryRecordCopy(senderEntry)
			destination := t.TempDir()
			plan, err := transfer.BuildReceivePlan(t.Context(), destination, *sender.Manifest, transfer.PlanRequest{})
			if err != nil {
				t.Fatal(err)
			}
			binding := signaling.ShareRequest{ID: sender.Status.ID, TaskID: sender.Status.TaskID, Generation: sender.Status.Generation, Manifest: sender.Status.Manifest, Plan: plan.Digest()}
			// A real, signed remote actor may end its authorized grant without any
			// proof that this local process has run or verified the file engine.
			remote, _ := f.b.client()
			if _, err = remote.ShareControl(t.Context(), signaling.ShareBind, binding); err != nil {
				t.Fatal(err)
			}
			if ended, err := remote.ShareControl(t.Context(), signaling.ShareComplete, binding); err != nil || ended.State != sharegrant.Completed {
				t.Fatal("controlled remote terminal result missing", err)
			}
			local, entry := f.a, senderEntry
			request := signaling.ShareRequest{ID: sender.Status.ID, CodeDigest: sender.CodeDigest, Manifest: sender.Status.Manifest}
			switch operation {
			case signaling.ShareApprove:
				request = signaling.ShareRequest{ID: sender.Status.ID, ClaimID: sender.Status.ClaimID, ReceiverKey: sender.Status.ReceiverKey, Generation: sender.Status.Generation, ApprovalID: sender.ApprovalID, TaskID: sender.Manifest.TransferID, Manifest: sender.Status.Manifest}
			case signaling.ShareBind:
				local, entry = f.b, receiverEntry
				request = binding
			}
			entry.mu.Lock()
			next := cloneTemporaryRecord(entry.record)
			if operation == signaling.ShareBind {
				next.Manifest = sender.Manifest
				next.Plan = &plan
				next.Directory = destination
			}
			// Reconstruct the exact previously sent stable request as an unresolved
			// intent, modeling reconciliation after its response was lost.
			next.Intent = temporaryIntent(operation, request)
			err = local.saveTemporaryRecord(entry, next)
			entry.mu.Unlock()
			if err != nil {
				t.Fatal(err)
			}
			got, err := local.RetryTemporaryShareAction(t.Context(), next.ID)
			if err != nil {
				t.Fatal("idempotent terminal status was not readable", err)
			}
			after := temporaryRecordCopy(entry)
			if after.Completion != nil || got.GrantState != sharegrant.Completed || after.TerminalObservedAt == 0 || after.Intent != nil {
				t.Fatal("terminal reconciliation did not retain its actual evidence boundary")
			}
			if got.State != "needs_attention" {
				t.Fatalf("remote terminal status invented local file completion: state=%s, engine_receipt=%t", got.State, after.Completion != nil)
			}
			if _, err = local.StartTemporaryShare(t.Context(), next.ID, DirectConfig{}); err == nil {
				t.Fatal("terminal grant opened another file operation")
			}
			if _, err = local.ResumeTemporaryShare(t.Context(), next.ID, DirectConfig{}); err == nil {
				t.Fatal("terminal grant was revived by resume")
			}
			if _, err = remote.ShareControl(t.Context(), signaling.ShareAuthorize, binding); protocol.ErrorCode(err) != protocol.ShareTerminal {
				t.Fatal("terminal authorization reopened", err)
			}
			if len(f.a.Tasks()) != 0 || len(f.b.Tasks()) != 0 {
				t.Fatal("control-only reconciliation created a local file task")
			}
			if entries, err := os.ReadDir(destination); err != nil || len(entries) != 0 {
				t.Fatal("control-only reconciliation created destination storage", err)
			}
		})
	}
}
