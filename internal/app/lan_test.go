package app

import (
	"context"
	"fmt"
	"net"
	"strings"
	"testing"

	"github.com/Wen5555/LinkSend/internal/discovery"
	"github.com/Wen5555/LinkSend/internal/identity"
)

func TestRememberLANPeerUpdatesCurrentManagerAfterPersistence(t *testing.T) {
	for _, mode := range []string{"allowed", "denied", "invalid_identity", "capacity_full"} {
		t.Run(mode, func(t *testing.T) {
			s, err := New(Config{DataDir: t.TempDir()})
			if err != nil {
				t.Fatal(err)
			}
			defer s.Shutdown()
			peer := authorizationPeer(t, s)
			interfaces, err := net.Interfaces()
			if err != nil {
				t.Fatal(err)
			}
			var excluded []string
			for _, iface := range interfaces {
				excluded = append(excluded, iface.Name)
			}
			manager, err := discovery.Start(discovery.Config{Identity: s.identity, Name: "memory", Port: unusedUDPPort(t), ExcludedInterfaces: excluded})
			if err != nil {
				t.Fatal(err)
			}
			defer manager.Close()
			s.lan = &lanRuntime{manager: manager}
			defer func() { s.lanMu.Lock(); s.lan = nil; s.lanMu.Unlock() }()
			if status := s.InboxStatus(); !status.LANAvailable || status.LANProviderErrors["interfaces"] != "no_eligible_address" {
				t.Fatalf("inbox hid the unavailable interface provider: %+v", status)
			}
			if status := s.Diagnostics(context.Background()); status.LANProviderErrors["interfaces"] != "no_eligible_address" {
				t.Fatal("exportable diagnostics hid provider degradation")
			}
			if mode == "denied" {
				if err := identity.RevokePeer(s.cfg.DataDir, peer.ID()); err != nil {
					t.Fatal(err)
				}
			}
			if mode == "capacity_full" {
				for index := 1; index <= 64; index++ {
					if err := manager.RememberAddress(fmt.Sprintf("192.0.2.%d", index)); err != nil {
						t.Fatal(err)
					}
				}
			}
			session := &PeerSession{PeerID: peer.ID(), peerName: "peer", peerPublicKey: peer.PublicKey(), localPeer: true, lanAddress: "192.0.2.100"}
			if mode == "invalid_identity" {
				session.PeerID = strings.Repeat("0", 64)
			}
			err = s.rememberLANPeer(session)
			if (err != nil) != (mode == "denied") {
				t.Fatalf("remember result for %s: %v", mode, err)
			}
			if mode == "allowed" {
				requireCurrentLANMemory(t, manager, session.lanAddress)
			}
			if mode == "denied" || mode == "invalid_identity" {
				for index := 1; index <= 64; index++ {
					if err := manager.RememberAddress(fmt.Sprintf("192.0.2.%d", index)); err != nil {
						t.Fatalf("uncommitted identity consumed remembered-address capacity: %v", err)
					}
				}
			}
			if mode == "allowed" || mode == "capacity_full" {
				peers, err := identity.LoadTrust(s.cfg.DataDir)
				if err != nil || len(peers) != 1 || peers[0].LastLANAddress != session.lanAddress || peers[0].AutoAccept {
					t.Fatalf("discovery availability altered durable grant/address: peers=%+v err=%v", peers, err)
				}
			}
		})
	}
}

func TestLANConfigurationDefersRestartWhileTaskIsActive(t *testing.T) {
	manager := newTaskManager()
	_, cancel := context.WithCancel(context.Background())
	if _, err := manager.create(TaskSnapshot{Direction: "receive"}, cancel); err != nil {
		t.Fatal(err)
	}
	runtime := &lanRuntime{directory: "old", cfg: DirectConfig{BindAddress: "192.0.2.1:0"}}
	s := &Service{tasks: manager, lan: runtime}
	s.startLANDiscovery("new", DirectConfig{BindAddress: "192.0.2.2:0"})
	runtime.mu.RLock()
	defer runtime.mu.RUnlock()
	if !runtime.restartPending || runtime.directory != "new" || runtime.cfg.BindAddress != "192.0.2.2:0" {
		t.Fatalf("deferred LAN configuration=%+v", runtime)
	}
}

func TestNetworkChangeInvalidatesFutureSelectionWithoutCancellingActiveTask(t *testing.T) {
	s, err := New(Config{DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Shutdown()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	record, err := s.tasks.create(TaskSnapshot{Direction: "send", PeerID: strings.Repeat("a", 64)}, cancel)
	if err != nil {
		t.Fatal(err)
	}
	s.network = cachedNetworkSelection{key: "stale"}
	if err = s.NetworkChanged("wake"); err != nil {
		t.Fatal(err)
	}
	if ctx.Err() != nil || record.snapshot().State == "cancel_requested" || s.network.key != "" {
		t.Fatalf("network refresh cancelled healthy task or retained cache: task=%+v cache=%+v", record.snapshot(), s.network)
	}
}
