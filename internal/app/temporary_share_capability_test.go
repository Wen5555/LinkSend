package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Wen5555/LinkSend/internal/protocol"
	"github.com/Wen5555/LinkSend/internal/signaling"
)

func TestTemporaryShareUnsupportedServiceDoesNotPrepareOrPersistIntent(t *testing.T) {
	for _, prototype := range []bool{false, true} {
		name := "ordinary service"
		if prototype {
			name = "old control prototype"
		}
		t.Run(name, func(t *testing.T) {
			var actions atomic.Int32
			h := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/healthz" {
					actions.Add(1)
				}
				body := map[string]any{"status": "ok", "capabilities": protocol.Supported()}
				if prototype {
					body["share_control"] = signaling.ShareControlInfo{Version: 1, Epoch: strings.Repeat("4", 32), FileTransferReady: false}
				}
				_ = json.NewEncoder(w).Encode(body)
			}))
			defer h.Close()
			s, err := New(Config{DataDir: t.TempDir(), ServerURL: h.URL, AllowInsecureLoopback: true})
			if err != nil {
				t.Fatal(err)
			}
			defer s.Shutdown()
			s.signal, err = signaling.New(signaling.Config{ServerURL: h.URL, Identity: s.identity, HTTPClient: h.Client()})
			if err != nil {
				t.Fatal(err)
			}
			var prepared atomic.Bool
			s.temporaryShares.beforePrepare = func(_ context.Context) { prepared.Store(true) }
			created, err := s.CreateTemporaryShare(t.Context(), []string{filepath.Join(t.TempDir(), "must-not-prepare-missing.txt")}, nil)
			if protocol.ErrorCode(err) != protocol.ShareUnsupported || created.Code != "" || created.Share.ID != "" {
				t.Fatalf("unsupported create minted authority or prepared files: result=%+v err=%v", created, err)
			}
			claimed, err := s.ClaimTemporaryShare(t.Context(), strings.Repeat("A", 32))
			if protocol.ErrorCode(err) != protocol.ShareUnsupported || claimed.ID != "" {
				t.Fatalf("unsupported claim persisted an intent: result=%+v err=%v", claimed, err)
			}
			shares, err := s.TemporaryShares()
			if err != nil || len(shares) != 0 || len(s.Tasks()) != 0 || prepared.Load() || actions.Load() != 0 {
				t.Fatalf("unsupported entry changed local state: shares=%d tasks=%d prepared=%t actions=%d err=%v", len(shares), len(s.Tasks()), prepared.Load(), actions.Load(), err)
			}
		})
	}
}
