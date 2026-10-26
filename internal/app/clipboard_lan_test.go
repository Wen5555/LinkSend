package app

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wen5555/LinkSend/internal/clipboardsync"
	"github.com/Wen5555/LinkSend/internal/discovery"
)

// Exercise the LAN TLS control path, rather than the WSS path used by the
// original clipboard tests. Loopback isolates this test from physical peers.
func TestClipboardLANBootstrapsWithoutFile(t *testing.T) {
	testClipboardLAN(t, false, false)
}

func TestClipboardLANConcurrentBootstrap(t *testing.T) {
	testClipboardLAN(t, true, false)
}

func TestClipboardLANNetworkRefreshPreservesHealthySession(t *testing.T) {
	testClipboardLAN(t, false, true)
}

func testClipboardLAN(t *testing.T, concurrent, refresh bool) {
	t.Helper()
	f := newDirectFixtureServices(t)
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	var aGeneration, bGeneration atomic.Uint64
	aGeneration.Store(1)
	bGeneration.Store(1)
	written := make(chan string, 4)
	for _, item := range []struct {
		service *Service
		peerID  string
		gen     *atomic.Uint64
	}{
		{f.a, f.bID.ID(), &aGeneration},
		{f.b, f.aID.ID(), &bGeneration},
	} {
		if _, err := item.service.Devices(ctx); err != nil {
			t.Fatal(err)
		}
		item.service.ConfigureClipboard(ClipboardAdapter{
			Generation: item.gen.Load,
			Read: func(_ context.Context, _ clipboardsync.Kind, expected uint64) ([]byte, uint64, error) {
				return []byte("LAN clipboard"), expected, nil
			},
			Write: func(_ clipboardsync.Kind, payload []byte, expected uint64, _ time.Time) (uint64, error) {
				item.gen.Store(expected + 1)
				written <- string(payload)
				return expected + 1, nil
			},
		})
		for _, direction := range []string{"send", "receive"} {
			if _, err := item.service.SetClipboardGrant(ctx, ClipboardGrantPatch{PeerID: item.peerID, Direction: direction, Kind: "text", Enabled: true}); err != nil {
				t.Fatal(err)
			}
		}
	}
	cfg := DirectConfig{AllowLoopback: true, BindAddress: "127.0.0.1:0", CheckTimeout: 5 * time.Second}
	aReceived, bReceived := filepath.Join(t.TempDir(), "a-received"), filepath.Join(t.TempDir(), "b-received")
	if err := f.a.StartInbox(aReceived, cfg); err != nil {
		t.Fatal(err)
	}
	if err := f.b.StartInbox(bReceived, cfg); err != nil {
		t.Fatal(err)
	}
	am, bm := f.a.lanManager(), f.b.lanManager()
	if am == nil || bm == nil {
		t.Fatal("LAN managers unavailable")
	}
	interfaces, err := net.Interfaces()
	if err != nil {
		t.Fatal(err)
	}
	var loopback string
	for _, iface := range interfaces {
		if iface.Flags&net.FlagLoopback != 0 {
			loopback = iface.Name
			break
		}
	}
	for _, item := range []struct {
		manager *discovery.Manager
		peer    discovery.Device
		port    int
	}{
		{am, discovery.Device{ID: f.bID.ID(), Name: "b", PublicKey: f.bID.PublicKey()}, bm.ControlPort()},
		{bm, discovery.Device{ID: f.aID.ID(), Name: "a", PublicKey: f.aID.PublicKey()}, am.ControlPort()},
	} {
		if err := item.manager.RememberVerifiedPeer(item.peer, discovery.Route{RemoteAddress: "127.0.0.1", LocalAddress: "127.0.0.1", ControlPort: item.port, Interface: loopback, Family: "ipv4", Generation: 1, LastSeen: time.Now()}); err != nil {
			t.Fatal(err)
		}
	}
	if concurrent {
		// Also exercise recovery of failures left by an earlier attempt.
		f.a.setClipboardPeerError(f.bID.ID(), "connection_failed")
		f.b.setClipboardPeerError(f.aID.ID(), "connection_failed")
		var dialing atomic.Int32
		bothDialing := make(chan struct{})
		// Separate LAN TLS channels can both finish QUIC before either inbound
		// session is published. Force that ordering instead of depending on the
		// scheduler to expose the two-pool collision.
		var adopting atomic.Int32
		bothAdopting := make(chan struct{})
		for _, service := range []*Service{f.a, f.b} {
			service.directPoolBeforePublish = func(kind string) {
				if kind != "adopt" {
					return
				}
				if adopting.Add(1) == 2 {
					close(bothAdopting)
				}
				select {
				case <-bothAdopting:
				case <-ctx.Done():
				}
			}
		}
		cfg.onPhase = func(phase string) {
			if phase == "requesting_peer" {
				if dialing.Add(1) == 2 {
					close(bothDialing)
				}
				select {
				case <-bothDialing:
				case <-ctx.Done():
				}
			}
		}
		ensured := make(chan error, 2)
		go func() { ensured <- f.a.EnsureClipboardSessions(ctx, cfg) }()
		go func() { ensured <- f.b.EnsureClipboardSessions(ctx, cfg) }()
		for range 2 {
			if err := <-ensured; err != nil {
				t.Fatal(err)
			}
		}
	} else {
		if err := f.a.EnsureClipboardSessions(ctx, cfg); err != nil {
			t.Fatal(err)
		}
	}
	ready := func(service *Service, peerID string) bool {
		for _, status := range service.ClipboardPeerStatuses() {
			if status.PeerID == peerID && status.SendReady && status.ReceiveReady && status.Error == "" && status.Waiting == "" {
				return true
			}
		}
		return false
	}
	waitFor(t, 3*time.Second, func() bool { return ready(f.a, f.bID.ID()) && ready(f.b, f.aID.ID()) }, "LAN clipboard did not become ready without a file transfer")
	sessions := make([]string, 0, 2)
	for _, service := range []*Service{f.a, f.b} {
		service.directPoolMu.Lock()
		for _, pooled := range service.directPool {
			if !pooled.peer.localPeer {
				t.Error("clipboard used WSS fallback instead of LAN TLS control")
			}
			sessions = append(sessions, pooled.peer.SessionID)
		}
		service.directPoolMu.Unlock()
	}
	if len(sessions) != 2 || sessions[0] != sessions[1] {
		t.Fatalf("LAN peers retained different sessions: %v", sessions)
	}
	if refresh {
		// An OS wake/address notification is only a hint. No file stream is
		// reserved here, but the authenticated QUIC connection owns live
		// clipboard leases. Refresh must not tear it down or require a new
		// connection before the next copy/file operation.
		for _, service := range []*Service{f.a, f.b} {
			if err := service.NetworkChanged("wake"); err != nil {
				t.Fatal(err)
			}
			service.directPoolMu.Lock()
			retained := false
			for _, pooled := range service.directPool {
				retained = pooled.peer.SessionID == sessions[0] && pooled.peer.Data.Conn.Context().Err() == nil
			}
			service.directPoolMu.Unlock()
			if !retained {
				t.Fatal("network notification destroyed healthy authenticated clipboard session")
			}
		}
	}
	for _, item := range []struct {
		service *Service
		gen     *atomic.Uint64
	}{{f.a, &aGeneration}, {f.b, &bGeneration}} {
		item.gen.Add(1)
		if err := item.service.ClipboardChanged(ctx, ClipboardChange{Generation: item.gen.Load(), Kinds: []clipboardsync.Kind{clipboardsync.Text}}); err != nil {
			t.Fatal(err)
		}
		select {
		case payload := <-written:
			if payload != "LAN clipboard" {
				t.Fatal("incorrect clipboard payload")
			}
		case <-ctx.Done():
			t.Fatalf("LAN clipboard was not delivered: a=%+v b=%+v", f.a.ClipboardPeerStatuses(), f.b.ClipboardPeerStatuses())
		}
	}
	if len(f.a.Tasks()) != 0 || len(f.b.Tasks()) != 0 {
		t.Fatal("clipboard created a file transfer task")
	}
	// The same LAN session must survive the file-idle timeout and remain usable
	// for files as well as clipboard data after its inbound owner changed.
	select {
	case <-time.After(directSessionIdleTTL + 150*time.Millisecond):
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if !ready(f.a, f.bID.ID()) || !ready(f.b, f.aID.ID()) {
		t.Fatal("LAN clipboard session did not survive file idle expiry")
	}
	cfg.onPhase = nil
	if err := f.b.SetAlwaysAccept(f.aID.ID(), true); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(t.TempDir(), "clipboard-session-file.txt")
	if err := os.WriteFile(source, []byte("LAN file on clipboard session"), 0600); err != nil {
		t.Fatal(err)
	}
	queued, err := f.a.StartSend(f.bID.ID(), []string{source}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	finished := waitTask(t, f.a, queued.ID, func(task TaskSnapshot) bool { return isTerminal(task.State) })
	if finished.State != "completed" || finished.SessionID != sessions[0] || !finished.BilateralConfirmed {
		t.Fatalf("LAN clipboard session could not be reused for files: %+v", finished)
	}
	received, err := os.ReadFile(filepath.Join(bReceived, filepath.Base(source)))
	if err != nil || string(received) != "LAN file on clipboard session" {
		t.Fatalf("LAN received file mismatch: %v", err)
	}
	if refresh {
		f.a.directPoolMu.Lock()
		peer := f.a.directPool[directPoolKey(f.bID.ID(), 1)].peer
		f.a.directPoolMu.Unlock()
		f.b.closePooledSessions(f.aID.ID())
		select {
		case <-peer.Data.Conn.Context().Done():
		case <-ctx.Done():
			t.Fatal("remote connection close was not observed")
		}
		if err := f.a.NetworkChanged("address"); err != nil {
			t.Fatal(err)
		}
		f.a.directPoolMu.Lock()
		remaining := len(f.a.directPool)
		f.a.directPoolMu.Unlock()
		if remaining != 0 {
			t.Fatal("network refresh retained a closed idle session")
		}
	}
}
