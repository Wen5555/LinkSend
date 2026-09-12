package transfer

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Wen5555/LinkSend/internal/transport"
)

func TestSelectionPersistenceAndErrorEnvelopeOverRealQUIC(t *testing.T) {
	for _, kind := range []string{"persistence_error", "source_error", "legitimate_json_file"} {
		t.Run(kind, func(t *testing.T) {
			client, server, peer := receivePlanQUICPair(t)
			body := []byte(`{"op":"error","error":"RECEIVE_PLAN_PERSIST_FAILED"}`)
			source := filepath.Join(t.TempDir(), "payload.json")
			if err := os.WriteFile(source, body, 0600); err != nil {
				t.Fatal(err)
			}
			prepared, err := Prepare(t.Context(), []string{source}, 0)
			if err != nil {
				t.Fatal(err)
			}
			defer prepared.Close()
			target := t.TempDir()
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			type outcome struct {
				result   Result
				err      error
				received int64
			}
			done := make(chan outcome, 1)
			go func() {
				stream, e := server.AcceptStream(ctx)
				if e != nil {
					done <- outcome{err: e}
					return
				}
				var received int64
				r, e := ReceiveWithOptions(ctx, transport.WrapStream(stream), ReceiveOptions{Directory: target, Peer: peer, Plan: func(ctx context.Context, offer Offer) (ReceivePlan, error) {
					return BuildReceivePlan(ctx, target, offer.Manifest, PlanRequest{})
				}, Progress: func(p Progress) { received = p.Received }})
				done <- outcome{r, e, received}
			}()
			stream, err := client.OpenStreamSync(ctx)
			if err != nil {
				t.Fatal(err)
			}
			var hookErr error
			if kind == "persistence_error" {
				hookErr = ErrPlanPersistence
			}
			if kind == "source_error" {
				hookErr = ErrChanged
			}
			called := false
			_, sendErr := SendWithHooks(ctx, transport.WrapStream(stream), prepared, SendHooks{SelectionAccepted: func(selection Selection, summary PlanSummary) error {
				called = true
				if selection.Digest == "" || summary.SelectedFiles != 1 {
					t.Error("hook lacked verified selection")
				}
				return hookErr
			}})
			received := <-done
			if !called {
				t.Fatal("selection hook not called")
			}
			if hookErr != nil {
				if !errors.Is(sendErr, hookErr) || !errors.Is(received.err, hookErr) || received.received != 0 {
					t.Fatalf("error counted as content or lost: sender=%v receiver=%v bytes=%d", sendErr, received.err, received.received)
				}
				if _, err = os.Stat(filepath.Join(target, "payload.json")); !errors.Is(err, os.ErrNotExist) {
					t.Fatal("error body committed")
				}
			} else {
				if sendErr != nil || received.err != nil || received.received != int64(len(body)) {
					t.Fatalf("valid JSON file treated as error: sender=%v receiver=%v bytes=%d", sendErr, received.err, received.received)
				}
				actual, err := os.ReadFile(filepath.Join(target, "payload.json"))
				if err != nil || string(actual) != string(body) {
					t.Fatalf("JSON file mismatch: %s %v", actual, err)
				}
			}
		})
	}
}
