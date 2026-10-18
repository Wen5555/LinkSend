package app

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestPooledPeerRemoteIdleCloseBeforeOfferReconnectsOnce(t *testing.T) {
	f := newDirectFixtureServices(t)
	if _, err := f.a.Devices(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := f.b.Devices(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := f.b.SetAlwaysAccept(f.aID.ID(), true); err != nil {
		t.Fatal(err)
	}
	cfg := DirectConfig{AllowLoopback: true, CheckTimeout: 5 * time.Second, WaitTimeout: 10 * time.Second}
	if err := f.b.StartInbox(t.TempDir(), cfg); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "idle.txt")
	if err := os.WriteFile(file, []byte("remote idle race"), 0600); err != nil {
		t.Fatal(err)
	}
	first, err := f.a.StartSend(f.bID.ID(), []string{file}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	completed := waitTask(t, f.a, first.ID, func(s TaskSnapshot) bool { return isTerminal(s.State) })
	if completed.State != "completed" {
		t.Fatal(completed.State, completed.ErrorCode)
	}
	var closed atomic.Bool
	f.a.beforeFileOpen = func(peer *PeerSession) {
		if closed.CompareAndSwap(false, true) {
			if peer.SessionID != completed.SessionID {
				t.Error("did not reach cached session race")
			}
			f.b.closePooledSessions(f.aID.ID())
			select {
			case <-peer.Data.Conn.Context().Done():
			case <-time.After(2 * time.Second):
				t.Error("remote close not observed")
			}
		}
	}
	next, err := f.a.StartSend(f.bID.ID(), []string{file}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	finished := waitTask(t, f.a, next.ID, func(s TaskSnapshot) bool { return isTerminal(s.State) })
	if finished.State != "completed" || finished.SessionID == completed.SessionID || !closed.Load() {
		t.Fatalf("remote idle race not recovered: state=%s phase=%s error=%s oldSessionReused=%t", finished.State, finished.Phase, finished.ErrorCode, finished.SessionID == completed.SessionID)
	}
}
