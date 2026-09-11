package app

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Wen5555/LinkSend/internal/identity"
	"github.com/Wen5555/LinkSend/internal/protocol"
	"github.com/Wen5555/LinkSend/internal/transfer"
)

func waitTask(t *testing.T, svc *Service, id string, match func(TaskSnapshot) bool) TaskSnapshot {
	t.Helper()
	deadline := time.NewTimer(15 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		if snap, ok := svc.Task(id); ok && match(snap) {
			return snap
		}
		select {
		case <-deadline.C:
			snap, _ := svc.Task(id)
			t.Fatalf("task %s did not reach expected state: %+v", id, snap)
		case <-ticker.C:
		}
	}
}

func TestPauseResumeTransfersOnlyMissingOrDamagedChunksOverRealQUIC(t *testing.T) {
	for _, corrupt := range []bool{false, true} {
		name := "missing_chunks"
		if corrupt {
			name = "damaged_chunk"
		}
		t.Run(name, func(t *testing.T) {
			f := newDirectFixtureServices(t)
			payload := bytes.Repeat([]byte{0x5a}, 3*transfer.DefaultChunkSize)
			source := filepath.Join(t.TempDir(), "resume.bin")
			if err := os.WriteFile(source, payload, 0600); err != nil {
				t.Fatal(err)
			}
			destination := filepath.Join(t.TempDir(), "receive")
			receiverTask, err := f.b.StartReceive(f.aID.ID(), destination, DirectConfig{AllowLoopback: true, CheckTimeout: 5 * time.Second, WaitTimeout: 10 * time.Second})
			if err != nil {
				t.Fatal(err)
			}
			waitTask(t, f.b, receiverTask.ID, func(s TaskSnapshot) bool { return s.Phase == "waiting" })

			firstChunk := make(chan struct{})
			releaseSender := make(chan struct{})
			senderTask, err := f.a.StartSend(f.bID.ID(), []string{source}, DirectConfig{
				AllowLoopback: true,
				CheckTimeout:  5 * time.Second,
				onChunkSent: func(sent transfer.ChunkTransmission) {
					if sent.FileID == 0 && sent.Index == 0 {
						select {
						case <-firstChunk:
						default:
							close(firstChunk)
						}
						select {
						case <-releaseSender:
						case <-time.After(10 * time.Second):
						}
					}
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			waitTask(t, f.b, receiverTask.ID, func(s TaskSnapshot) bool { return s.State == "awaiting_acceptance" })
			if err = f.b.AcceptTask(receiverTask.ID); err != nil {
				t.Fatal(err)
			}
			select {
			case <-firstChunk:
			case <-time.After(10 * time.Second):
				t.Fatal("first chunk was not sent")
			}
			if err = f.a.PauseTask(senderTask.ID); err != nil {
				t.Fatal(err)
			}
			close(releaseSender)
			paused := waitTask(t, f.a, senderTask.ID, func(s TaskSnapshot) bool { return s.State == "paused" })
			recovering := waitTask(t, f.b, receiverTask.ID, func(s TaskSnapshot) bool { return s.State == "recovering" })
			if paused.TransferID == "" || paused.TransferID != recovering.TransferID || paused.SessionID == "" || recovering.SessionID == "" {
				t.Fatalf("pause recovery identity mismatch: sender=%+v receiver=%+v", paused, recovering)
			}
			firstAttempt, firstSession := paused.AttemptID, paused.SessionID
			if corrupt {
				part := filepath.Join(destination, ".linksend-"+paused.TransferID, "0.part")
				file, openErr := os.OpenFile(part, os.O_WRONLY, 0600)
				if openErr != nil {
					t.Fatal(openErr)
				}
				if _, openErr = file.WriteAt([]byte{0x00}, 0); openErr != nil {
					_ = file.Close()
					t.Fatal(openErr)
				}
				if openErr = file.Sync(); openErr != nil {
					_ = file.Close()
					t.Fatal(openErr)
				}
				if openErr = file.Close(); openErr != nil {
					t.Fatal(openErr)
				}
			}

			resumedReceiver, err := f.b.ResumeTask(receiverTask.ID, DirectConfig{AllowLoopback: true, CheckTimeout: 5 * time.Second, WaitTimeout: 10 * time.Second})
			if err != nil {
				t.Fatal(err)
			}
			waitTask(t, f.b, receiverTask.ID, func(s TaskSnapshot) bool { return s.AttemptID == resumedReceiver.AttemptID && s.Phase == "waiting" })
			resumedSender, err := f.a.ResumeTask(senderTask.ID, DirectConfig{AllowLoopback: true, CheckTimeout: 5 * time.Second})
			if err != nil {
				t.Fatal(err)
			}
			completedSender := waitTask(t, f.a, senderTask.ID, func(s TaskSnapshot) bool { return s.State == "completed" })
			completedReceiver := waitTask(t, f.b, receiverTask.ID, func(s TaskSnapshot) bool { return s.State == "completed" })
			if completedSender.AttemptID == firstAttempt || completedSender.SessionID == firstSession || completedSender.SessionID != completedReceiver.SessionID || completedSender.AttemptID != resumedSender.AttemptID {
				t.Fatalf("resume did not use a fresh converged attempt/session: sender=%+v receiver=%+v", completedSender, completedReceiver)
			}
			wantRetransmit := int64(0)
			wantActual := int64(len(payload))
			if corrupt {
				wantRetransmit = transfer.DefaultChunkSize
				wantActual += transfer.DefaultChunkSize
			}
			if completedSender.SentBytes != wantActual || completedSender.RetransmittedBytes != wantRetransmit || completedReceiver.ReceivedBytes != wantActual {
				t.Fatalf("unexpected byte accounting: sender=%+v receiver=%+v want actual=%d retransmit=%d", completedSender, completedReceiver, wantActual, wantRetransmit)
			}
			for _, snap := range []TaskSnapshot{completedSender, completedReceiver} {
				if snap.ProcessedBytes != int64(len(payload)) || snap.VerifiedBytes != int64(len(payload)) || snap.CommittedBytes != int64(len(payload)) || !snap.BilateralConfirmed {
					t.Fatalf("logical completion or terminal confirmation mismatch: %+v", snap)
				}
			}
			got, err := os.ReadFile(filepath.Join(destination, "resume.bin"))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, payload) {
				t.Fatal("resumed file content mismatch")
			}
		})
	}
}

func preparePausedTaskForResumeNegativeTest(t *testing.T) (directFixtureServices, string, TaskSnapshot) {
	t.Helper()
	f := newDirectFixtureServices(t)
	t.Cleanup(func() {
		f.a.Shutdown()
		f.b.Shutdown()
	})
	payload := bytes.Repeat([]byte{0x6d}, 2*transfer.DefaultChunkSize)
	source := filepath.Join(t.TempDir(), "resume-negative.bin")
	if err := os.WriteFile(source, payload, 0600); err != nil {
		t.Fatal(err)
	}
	receiverTask, err := f.b.StartReceive(f.aID.ID(), filepath.Join(t.TempDir(), "receive"), DirectConfig{AllowLoopback: true, CheckTimeout: 5 * time.Second, WaitTimeout: 10 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	waitTask(t, f.b, receiverTask.ID, func(s TaskSnapshot) bool { return s.Phase == "waiting" })
	firstChunk := make(chan struct{})
	releaseSender := make(chan struct{})
	senderTask, err := f.a.StartSend(f.bID.ID(), []string{source}, DirectConfig{
		AllowLoopback: true,
		CheckTimeout:  5 * time.Second,
		onChunkSent: func(sent transfer.ChunkTransmission) {
			if sent.Index != 0 {
				return
			}
			select {
			case <-firstChunk:
			default:
				close(firstChunk)
			}
			select {
			case <-releaseSender:
			case <-time.After(10 * time.Second):
			}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	waitTask(t, f.b, receiverTask.ID, func(s TaskSnapshot) bool { return s.State == "awaiting_acceptance" })
	if err = f.b.AcceptTask(receiverTask.ID); err != nil {
		t.Fatal(err)
	}
	select {
	case <-firstChunk:
	case <-time.After(10 * time.Second):
		t.Fatal("first chunk was not sent")
	}
	if err = f.a.PauseTask(senderTask.ID); err != nil {
		t.Fatal(err)
	}
	close(releaseSender)
	paused := waitTask(t, f.a, senderTask.ID, func(s TaskSnapshot) bool { return s.State == "paused" && s.CanResume })
	waitTask(t, f.b, receiverTask.ID, func(s TaskSnapshot) bool { return s.State == "recovering" })
	return f, source, paused
}

func TestResumeRejectsChangedSourceBeforeSendingMoreBytes(t *testing.T) {
	f, source, paused := preparePausedTaskForResumeNegativeTest(t)
	if err := os.WriteFile(source, bytes.Repeat([]byte{0x3c}, 2*transfer.DefaultChunkSize), 0600); err != nil {
		t.Fatal(err)
	}
	resumed, err := f.a.ResumeTask(paused.ID, DirectConfig{AllowLoopback: true, CheckTimeout: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	failed := waitTask(t, f.a, paused.ID, func(s TaskSnapshot) bool { return s.AttemptID == resumed.AttemptID && s.State == "failed" })
	if failed.ErrorCode != string(protocol.SourceChanged) {
		t.Fatalf("changed source error = %q, want %q", failed.ErrorCode, protocol.SourceChanged)
	}
	if failed.SentBytes != paused.SentBytes {
		t.Fatalf("changed source sent more body bytes: before=%d after=%d", paused.SentBytes, failed.SentBytes)
	}
}

func TestResumeRejectsChangedPinnedPeerBeforeStartingAttempt(t *testing.T) {
	f, _, paused := preparePausedTaskForResumeNegativeTest(t)
	if err := os.Remove(filepath.Join(f.a.cfg.DataDir, "trust.json")); err != nil {
		t.Fatal(err)
	}
	replacement, err := identity.Generate()
	if err != nil {
		t.Fatal(err)
	}
	if err = identity.TrustPeer(f.a.cfg.DataDir, identity.TrustedPeer{ID: replacement.ID(), Name: "replacement", PublicKey: replacement.PublicKey()}, replacement.ID()); err != nil {
		t.Fatal(err)
	}
	if _, err = f.a.ResumeTask(paused.ID, DirectConfig{AllowLoopback: true, CheckTimeout: 5 * time.Second}); protocol.ErrorCode(err) != protocol.ResumeMismatch {
		t.Fatalf("changed peer fingerprint error = %v, want %s", err, protocol.ResumeMismatch)
	}
	after, ok := f.a.Task(paused.ID)
	if !ok || after.AttemptID != paused.AttemptID || after.State != "paused" || !after.CanResume {
		t.Fatalf("rejected resume mutated the task: before=%+v after=%+v", paused, after)
	}
}

func TestRestartRecoveryUsesPersistedTaskAndMissingBlocksOverRealQUIC(t *testing.T) {
	f := newDirectFixtureServices(t)
	payload := bytes.Repeat([]byte{0x39}, 2*transfer.DefaultChunkSize)
	source := filepath.Join(t.TempDir(), "restart.bin")
	if err := os.WriteFile(source, payload, 0600); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(t.TempDir(), "receive")
	receiverTask, err := f.b.StartReceive(f.aID.ID(), destination, DirectConfig{AllowLoopback: true, CheckTimeout: 5 * time.Second, WaitTimeout: 10 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	waitTask(t, f.b, receiverTask.ID, func(s TaskSnapshot) bool { return s.Phase == "waiting" })
	firstChunk := make(chan struct{})
	releaseSender := make(chan struct{})
	senderTask, err := f.a.StartSend(f.bID.ID(), []string{source}, DirectConfig{
		AllowLoopback: true,
		CheckTimeout:  5 * time.Second,
		onChunkSent: func(sent transfer.ChunkTransmission) {
			if sent.Index == 0 {
				select {
				case <-firstChunk:
				default:
					close(firstChunk)
				}
				<-releaseSender
			}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	waitTask(t, f.b, receiverTask.ID, func(s TaskSnapshot) bool { return s.State == "awaiting_acceptance" })
	if err = f.b.AcceptTask(receiverTask.ID); err != nil {
		t.Fatal(err)
	}
	select {
	case <-firstChunk:
	case <-time.After(10 * time.Second):
		t.Fatal("first chunk was not sent before restart")
	}
	if err = f.a.PauseTask(senderTask.ID); err != nil {
		t.Fatal(err)
	}
	close(releaseSender)
	waitTask(t, f.a, senderTask.ID, func(s TaskSnapshot) bool { return s.State == "paused" })
	waitTask(t, f.b, receiverTask.ID, func(s TaskSnapshot) bool { return s.State == "recovering" })
	f.a.Shutdown()
	f.b.Shutdown()

	a2, err := New(Config{DataDir: f.a.cfg.DataDir, ServerURL: f.http.URL, AllowInsecureLoopback: true, Identity: f.aID})
	if err != nil {
		t.Fatal(err)
	}
	b2, err := New(Config{DataDir: f.b.cfg.DataDir, ServerURL: f.http.URL, AllowInsecureLoopback: true, Identity: f.bID})
	if err != nil {
		t.Fatal(err)
	}
	defer a2.Shutdown()
	defer b2.Shutdown()
	restoredSender := waitTask(t, a2, senderTask.ID, func(s TaskSnapshot) bool { return s.State == "recovering" && s.CanResume })
	restoredReceiver := waitTask(t, b2, receiverTask.ID, func(s TaskSnapshot) bool { return s.State == "recovering" && s.CanResume })
	if !restoredSender.HistoryPersisted || !restoredSender.RestartRecoverySupported || !restoredReceiver.ByteResumeSupported {
		t.Fatalf("restart capabilities were not restored: sender=%+v receiver=%+v", restoredSender, restoredReceiver)
	}
	resumedReceiver, err := b2.ResumeTask(receiverTask.ID, DirectConfig{AllowLoopback: true, CheckTimeout: 5 * time.Second, WaitTimeout: 10 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	waitTask(t, b2, receiverTask.ID, func(s TaskSnapshot) bool { return s.AttemptID == resumedReceiver.AttemptID && s.Phase == "waiting" })
	if _, err = a2.ResumeTask(senderTask.ID, DirectConfig{AllowLoopback: true, CheckTimeout: 5 * time.Second}); err != nil {
		t.Fatal(err)
	}
	completedSender := waitTask(t, a2, senderTask.ID, func(s TaskSnapshot) bool { return s.State == "completed" })
	completedReceiver := waitTask(t, b2, receiverTask.ID, func(s TaskSnapshot) bool { return s.State == "completed" })
	if completedSender.SentBytes != int64(len(payload)) || completedReceiver.ReceivedBytes != int64(len(payload)) || completedSender.RetransmittedBytes != 0 {
		t.Fatalf("restart resume byte accounting mismatch: sender=%+v receiver=%+v", completedSender, completedReceiver)
	}
	got, err := os.ReadFile(filepath.Join(destination, "restart.bin"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatal("restart-resumed file content mismatch")
	}
}
