package app

import (
	"context"
	"crypto/tls"
	"sync"
	"testing"
	"time"

	"github.com/Wen5555/LinkSend/internal/identity"
	"github.com/Wen5555/LinkSend/internal/protocol"
)

func requireDeviceConnectionState(t *testing.T, svc *Service, peerID, want string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	devices, err := svc.Devices(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, device := range devices {
		if device.ID == peerID {
			if device.ConnectionState != want {
				t.Fatalf("connection state = %q, want %q (trusted=%t blocked=%t)", device.ConnectionState, want, device.Trusted, device.Blocked)
			}
			return
		}
	}
	t.Fatal("peer missing from device directory")
}

func TestDevicesSessionIDIsNotConnection(t *testing.T) {
	for _, state := range []string{"requesting_peer", "ice_checking", "quic_handshake", "paused", "recovering", "transferring", "blocked"} {
		t.Run(state, func(t *testing.T) {
			svc, err := New(Config{DataDir: t.TempDir()})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(svc.Shutdown)
			peer := authorizationPeer(t, svc)
			task, err := svc.tasks.create(TaskSnapshot{Direction: "send", PeerID: peer.ID(), SessionID: protocol.RandomID()}, nil)
			if err != nil {
				t.Fatal(err)
			}
			task.update(func(snapshot *TaskSnapshot) {
				snapshot.State, snapshot.Phase = state, state
				// Historical evidence must not become a live connection signal.
				if state == "paused" || state == "recovering" || state == "transferring" {
					snapshot.TLSVersion = tls.VersionTLS13
				}
			})
			want := "not_connected"
			if state == "blocked" {
				// Cover the interval after durable denial but before task cancellation.
				if err := identity.RevokePeer(svc.cfg.DataDir, peer.ID()); err != nil {
					t.Fatal(err)
				}
				want = "blocked"
			}
			requireDeviceConnectionState(t, svc, peer.ID(), want)
		})
	}
}

func TestDevicesAuthenticatedConnectionLifetime(t *testing.T) {
	for _, mode := range []string{"direct", "legacy_non_reuse", "idle_reusable"} {
		t.Run(mode, func(t *testing.T) {
			f := newDirectFixtureServices(t)
			ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
			defer cancel()
			waiting := make(chan struct{})
			type result struct {
				peer *PeerSession
				err  error
			}
			received := make(chan result, 1)
			cfg := DirectConfig{AllowLoopback: true, CheckTimeout: 5 * time.Second, WaitTimeout: 5 * time.Second, disableSessionReuse: mode == "legacy_non_reuse", disableClipboardSync: true}
			receiverCfg := cfg
			receiverCfg.onPhase = func(phase string) {
				if phase == "waiting" {
					close(waiting)
				}
			}
			go func() {
				peer, err := f.b.AcceptDirect(ctx, f.aID.ID(), receiverCfg)
				received <- result{peer, err}
			}()
			select {
			case <-waiting:
			case <-ctx.Done():
				t.Fatal("receiver did not enter signaling wait")
			}
			var sender *PeerSession
			var err error
			if mode == "idle_reusable" {
				sender, err = f.a.acquirePeerSession(ctx, f.bID.ID(), cfg)
			} else {
				sender, err = f.a.ConnectDirect(ctx, f.bID.ID(), cfg)
			}
			if err != nil {
				t.Fatal(err)
			}
			defer sender.Close()
			got := <-received
			if got.err != nil {
				t.Fatal(got.err)
			}
			receiver := got.peer
			defer receiver.Close()
			for _, peer := range []*PeerSession{sender, receiver} {
				state := peer.Data.Conn.ConnectionState().TLS
				if !state.HandshakeComplete || state.Version != tls.VersionTLS13 || state.NegotiatedProtocol != identity.ALPN {
					t.Fatal("fixture did not establish authenticated TLS 1.3 QUIC")
				}
				if (mode == "legacy_non_reuse") == peer.SessionReuse {
					t.Fatal("fixture negotiated the wrong session reuse capability")
				}
			}
			if mode == "idle_reusable" {
				f.b.adoptInboundPeerSession(receiver)
				if err := f.a.releasePeerSession(sender, nil); err != nil {
					t.Fatal(err)
				}
				f.a.directPoolMu.Lock()
				pooled := f.a.directPool[directPoolKey(sender.PeerID, sender.AuthorizationGeneration)]
				idle := pooled != nil && !pooled.inUse && !pooled.receiving && pooled.timer != nil
				f.a.directPoolMu.Unlock()
				if !idle || sender.Data.Conn.Context().Err() != nil {
					t.Fatal("fixture did not retain a healthy idle reusable connection")
				}
			}
			if len(f.a.Tasks()) != 0 || len(f.b.Tasks()) != 0 {
				t.Fatal("connection fixture unexpectedly created transfer tasks")
			}
			requireDeviceConnectionState(t, f.a, f.bID.ID(), "connected")
			requireDeviceConnectionState(t, f.b, f.aID.ID(), "connected")
			// Direct callers and legacy peers need not be in the reusable pool.
			if err := f.a.BlockPeer(f.bID.ID()); err != nil {
				t.Fatal(err)
			}
			requireDeviceConnectionState(t, f.a, f.bID.ID(), "blocked")
			_ = sender.Close()
			_ = receiver.Close()
			requireDeviceConnectionState(t, f.b, f.aID.ID(), "not_connected")
		})
	}
}

func TestDevicesSimultaneousAuthenticatedConnections(t *testing.T) {
	f := newDirectFixtureServices(t)
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	ready := sync.WaitGroup{}
	ready.Add(2)
	type result struct {
		peer *PeerSession
		err  error
	}
	results := make(chan result, 2)
	for _, side := range []struct {
		svc  *Service
		peer string
	}{{f.a, f.bID.ID()}, {f.b, f.aID.ID()}} {
		go func() {
			var once sync.Once
			peer, err := side.svc.ConnectDirect(ctx, side.peer, DirectConfig{AllowLoopback: true, CheckTimeout: 5 * time.Second,
				onSession: func(string, string) { once.Do(func() { ready.Done(); ready.Wait() }) },
			})
			results <- result{peer, err}
		}()
	}
	for range 2 {
		got := <-results
		if got.err != nil {
			t.Fatal(got.err)
		}
		defer got.peer.Close()
	}
	// The higher identity took ConnectDirect's simultaneous responder branch.
	// Neither side has a task or a reusable-pool entry to stand in for this connection.
	requireDeviceConnectionState(t, f.a, f.bID.ID(), "connected")
	requireDeviceConnectionState(t, f.b, f.aID.ID(), "connected")
}

func TestDevicesLiveConnectionRequiresCurrentAuthorization(t *testing.T) {
	f := newDirectFixtureServices(t)
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	sender, receiver := connectDirectPair(t, f, ctx)
	defer sender.Close()
	defer receiver.Close()
	// Losing signaling must not hide an existing authenticated data path.
	f.http.Close()
	requireDeviceConnectionState(t, f.a, f.bID.ID(), "connected")
	requireDeviceConnectionState(t, f.b, f.aID.ID(), "connected")
	if err := f.a.BlockPeer(f.bID.ID()); err != nil {
		t.Fatal(err)
	}
	requireDeviceConnectionState(t, f.a, f.bID.ID(), "blocked")
	if err := f.a.UnblockPeer(f.bID.ID()); err != nil {
		t.Fatal(err)
	}
	if err := identity.TrustPairedPeer(f.a.cfg.DataDir, identity.TrustedPeer{ID: f.bID.ID(), Name: "paired again", PublicKey: f.bID.PublicKey()}); err != nil {
		t.Fatal(err)
	}
	generation, err := identity.AuthorizationGeneration(f.a.cfg.DataDir, f.bID.ID())
	if err != nil || generation == sender.AuthorizationGeneration || sender.Data.Conn.Context().Err() != nil {
		t.Fatalf("fixture did not preserve a live connection with an old grant: generation=%d previous=%d err=%v", generation, sender.AuthorizationGeneration, err)
	}
	requireDeviceConnectionState(t, f.a, f.bID.ID(), "not_connected")
	// Observation owns neither the connection nor shutdown: caller cleanup can
	// happen after the service closes without touching its files or workers.
	f.a.Shutdown()
	_ = sender.Close()
	waitFor(t, time.Second, func() bool {
		f.a.deviceConnections.mu.Lock()
		defer f.a.deviceConnections.mu.Unlock()
		return len(f.a.deviceConnections.live) == 0
	}, "closed connection remained registered after service shutdown")
}
