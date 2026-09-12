package transfer

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Wen5555/LinkSend/internal/content"
	"github.com/Wen5555/LinkSend/internal/transport"
)

// Preserve the real stream's abort/terminal-delivery methods while observing
// or corrupting the application control frame before QUIC encrypts it.
type acceptedControlStream struct {
	*transport.QUICStream
	onAccepted func(control)
	tamper     bool
}

func (s *acceptedControlStream) Write(data []byte) (int, error) {
	var message control
	if json.Unmarshal(data, &message) == nil && message.Op == opAccepted {
		if s.onAccepted != nil {
			s.onAccepted(message)
		}
		if s.tamper {
			original := message.ContentDigest
			data = []byte(strings.Replace(string(data), original, strings.Repeat("0", 64), 1))
		}
	}
	return s.QUICStream.Write(data)
}

func persistAcceptanceFixture(path string, value any) error {
	body, err := json.Marshal(value)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	if _, err = file.Write(body); err == nil {
		err = file.Sync()
	}
	return errors.Join(err, file.Close())
}

func TestContentSelectionAcceptanceBarrierOverRealQUIC(t *testing.T) {
	for _, mode := range []string{"native_json", "all_skipped", "explicit_fallback", "ordinary_file", "selection_persist_failed", "content_persist_failed", "accepted_digest_tampered"} {
		t.Run(mode, func(t *testing.T) {
			// This exact JSON also appears in a persistence error control. Only
			// the negotiated barrier can distinguish it before requesting bytes.
			body := []byte(`{"op":"error","error":"RECEIVE_PLAN_PERSIST_FAILED"}`)
			source := filepath.Join(t.TempDir(), "content.txt")
			if err := os.WriteFile(source, body, 0600); err != nil {
				t.Fatal(err)
			}
			prepared, err := Prepare(t.Context(), []string{source}, 0)
			if err != nil {
				t.Fatal(err)
			}
			defer prepared.Close()
			descriptor := &ContentDescriptor{Version: 1, EntryID: 0, Kind: content.Text, MediaType: "text/plain; charset=utf-8", Size: int64(len(body)), Digest: Sum(body)}
			if mode == "ordinary_file" {
				descriptor = nil
			}
			client, server, peer := receivePlanQUICPair(t)
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			destination, records := t.TempDir(), t.TempDir()
			selectionFile, contentFile := filepath.Join(records, "selection.json"), filepath.Join(records, "content.json")
			type receivedOutcome struct {
				result Result
				err    error
				bytes  int64
			}
			done := make(chan receivedOutcome, 1)
			go func() {
				stream, e := server.AcceptStream(ctx)
				if e != nil {
					done <- receivedOutcome{err: e}
					return
				}
				var received int64
				result, e := ReceiveWithOptions(ctx, transport.WrapStream(stream), ReceiveOptions{Directory: destination, Peer: peer, AcceptNativeContent: mode != "explicit_fallback", Plan: func(ctx context.Context, offer Offer) (ReceivePlan, error) {
					if !hasCapability(offer.Capabilities, CapabilityReceivePlan) || !hasCapability(offer.Capabilities, CapabilityAcceptanceCommit) || (descriptor != nil && !hasCapability(offer.Capabilities, CapabilityContent)) {
						return ReceivePlan{}, errors.New("joint capabilities absent")
					}
					if offer.FileFallback != (mode == "explicit_fallback") {
						return ReceivePlan{}, errors.New("fallback fact not passed to receive decision")
					}
					var selected []uint32
					if mode == "all_skipped" {
						selected = []uint32{}
					}
					return BuildReceivePlan(ctx, destination, offer.Manifest, PlanRequest{SelectedIDs: selected})
				}, Progress: func(p Progress) { received = p.Received }})
				done <- receivedOutcome{result, e, received}
			}()
			stream, err := client.OpenStreamSync(ctx)
			if err != nil {
				t.Fatal(err)
			}
			selectionSaved, contentSaved, acceptedSeen := false, false, false
			var acceptedContent ContentAcceptance
			wire := &acceptedControlStream{QUICStream: transport.WrapStream(stream), tamper: mode == "accepted_digest_tampered", onAccepted: func(c control) {
				acceptedSeen = true
				if !selectionSaved || !contentSaved {
					t.Error("accepted preceded durable hooks")
				}
				if _, e := os.ReadFile(selectionFile); e != nil {
					t.Error(e)
				}
				if _, e := os.ReadFile(contentFile); e != nil {
					t.Error(e)
				}
				if c.Digest != prepared.Manifest.Digest() || c.ContentDigest != acceptedContent.ContentDigest || c.SelectionDigest == "" {
					t.Error("accepted lost manifest/content/selection binding")
				}
			}}
			var actualSent int64
			result, sendErr := SendWithOptions(ctx, wire, prepared, SendOptions{Content: descriptor, AllowFileFallback: mode == "explicit_fallback", Hooks: SendHooks{
				SelectionAccepted: func(selection Selection, summary PlanSummary) error {
					if mode == "selection_persist_failed" {
						return ErrPlanPersistence
					}
					if mode == "all_skipped" && (len(selection.IDs) != 0 || summary.SelectedTotal != 0) {
						return ErrPlanMismatch
					}
					if e := persistAcceptanceFixture(selectionFile, selection); e != nil {
						return e
					}
					selectionSaved = true
					return nil
				}, ContentAccepted: func(acceptance ContentAcceptance) error {
					if !selectionSaved {
						t.Error("content hook ran before valid selection persisted")
					}
					if mode == "content_persist_failed" {
						return ErrPlanPersistence
					}
					if e := persistAcceptanceFixture(contentFile, acceptance); e != nil {
						return e
					}
					contentSaved = true
					acceptedContent = acceptance
					// A hook cannot rewrite the descriptor that the wire will bind.
					if acceptance.Content != nil {
						acceptance.Content.Kind = "modified hook copy"
					}
					return nil
				}, ChunkSent: func(sent ChunkTransmission) { actualSent += sent.Bytes },
			}})
			received := <-done
			if strings.HasSuffix(mode, "persist_failed") {
				if !errors.Is(sendErr, ErrPlanPersistence) || !errors.Is(received.err, ErrPlanPersistence) || acceptedSeen || actualSent != 0 || received.bytes != 0 {
					t.Fatalf("failed persistence crossed barrier: sender=%v receiver=%v accepted=%v sent=%d received=%d", sendErr, received.err, acceptedSeen, actualSent, received.bytes)
				}
			} else if mode == "accepted_digest_tampered" {
				if !errors.Is(received.err, ErrContentMismatch) || actualSent != 0 || received.bytes != 0 {
					t.Fatalf("bad content digest crossed accepted: sender=%v receiver=%v sent=%d received=%d", sendErr, received.err, actualSent, received.bytes)
				}
			} else {
				if sendErr != nil || received.err != nil || !acceptedSeen {
					t.Fatalf("sender=%v receiver=%v accepted=%v", sendErr, received.err, acceptedSeen)
				}
				if result.SelectionDigest != received.result.SelectionDigest || result.ContentDigest != received.result.ContentDigest {
					t.Fatal("independent bindings differed")
				}
				if result.Content != nil && result.Content.Kind != content.Text {
					t.Fatal("content hook mutated wire descriptor")
				}
				if mode == "explicit_fallback" && (!result.FileFallback || !acceptedContent.FileFallback || acceptedContent.Content != nil || acceptedContent.ContentDigest != "") {
					t.Fatal("actual fallback was not persisted")
				}
				if mode == "ordinary_file" && (acceptedContent.Content != nil || acceptedContent.ContentDigest != "" || acceptedContent.FileFallback) {
					t.Fatal("ordinary file acquired content interpretation")
				}
				if mode == "all_skipped" {
					if result.State != "NoContent" || received.result.State != "NoContent" || actualSent != 0 || received.bytes != 0 {
						t.Fatal("empty selection acquired content bytes")
					}
				} else {
					actual, e := os.ReadFile(filepath.Join(destination, "content.txt"))
					if e != nil || string(actual) != string(body) || actualSent != int64(len(body)) || received.bytes != int64(len(body)) {
						t.Fatalf("JSON file body mismatch: %q %v", actual, e)
					}
				}
			}
			if strings.HasSuffix(mode, "persist_failed") || mode == "accepted_digest_tampered" || mode == "all_skipped" {
				if _, e := os.Stat(filepath.Join(destination, "content.txt")); !errors.Is(e, os.ErrNotExist) {
					t.Fatal("unaccepted/skipped content committed")
				}
			}
		})
	}
}
