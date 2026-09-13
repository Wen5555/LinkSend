package app

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/Wen5555/LinkSend/internal/discovery"
	"github.com/Wen5555/LinkSend/internal/identity"
)

func unusedUDPPort(t *testing.T) int {
	t.Helper()
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	if err != nil {
		t.Fatal(err)
	}
	port := conn.LocalAddr().(*net.UDPAddr).Port
	_ = conn.Close()
	return port
}

func TestLANPairingRequiresRemoteConsentAndCommitsBothPins(t *testing.T) {
	a, err := New(Config{DataDir: t.TempDir(), AllowInsecureLoopback: true, Name: "a"})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Shutdown()
	b, err := New(Config{DataDir: t.TempDir(), AllowInsecureLoopback: true, Name: "b"})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Shutdown()
	am, err := discovery.Start(discovery.Config{Identity: a.identity, Name: "a", Port: unusedUDPPort(t), AllowLoopback: true})
	if err != nil {
		t.Fatal(err)
	}
	defer am.Close()
	bm, err := discovery.Start(discovery.Config{Identity: b.identity, Name: "b", Port: unusedUDPPort(t), AllowLoopback: true})
	if err != nil {
		t.Fatal(err)
	}
	defer bm.Close()
	routeToB := discovery.Route{RemoteAddress: "127.0.0.1", ControlPort: bm.ControlPort(), Interface: "loopback", LocalAddress: "127.0.0.1", LastSeen: time.Now(), Family: "ipv4", Generation: 1}
	routeToA := discovery.Route{RemoteAddress: "127.0.0.1", ControlPort: am.ControlPort(), Interface: "loopback", LocalAddress: "127.0.0.1", LastSeen: time.Now(), Family: "ipv4", Generation: 1}
	if err = am.RememberVerifiedPeer(discovery.Device{ID: b.identity.ID(), Name: "b", PublicKey: b.identity.PublicKey()}, routeToB); err != nil {
		t.Fatal(err)
	}
	if err = bm.RememberVerifiedPeer(discovery.Device{ID: a.identity.ID(), Name: "a", PublicKey: a.identity.PublicKey()}, routeToA); err != nil {
		t.Fatal(err)
	}
	a.lan = &lanRuntime{manager: am}
	defer func() { a.lanMu.Lock(); a.lan = nil; a.lanMu.Unlock() }()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go func() {
		select {
		case incoming := <-bm.Incoming():
			b.receiveLANSession(ctx, incoming, "", DirectConfig{})
		case <-ctx.Done():
		}
	}()
	type pairResult struct {
		result LANPairResult
		err    error
	}
	done := make(chan pairResult, 1)
	go func() { result, pairErr := a.RequestLANPair(ctx, b.identity.ID()); done <- pairResult{result, pairErr} }()
	var request LANPairRequestInfo
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		pending := b.PendingLANPairings()
		if len(pending) == 1 {
			request = pending[0]
			break
		}
	}
	if request.RequestID == "" {
		t.Fatal("receiver did not expose pending LAN consent")
	}
	if err = b.RespondLANPair(request.RequestID, true); err != nil {
		t.Fatal(err)
	}
	paired := <-done
	if paired.err != nil || paired.result.State != "lan_paired" {
		t.Fatalf("pair result=%+v err=%v", paired.result, paired.err)
	}
	for name, service := range map[string]*Service{"a": a, "b": b} {
		peers, loadErr := identity.LoadTrust(service.cfg.DataDir)
		if loadErr != nil || len(peers) != 1 || peers[0].GrantKind != "lan" || peers[0].AutoAccept {
			t.Fatalf("%s trust=%+v err=%v", name, peers, loadErr)
		}
	}
}

func TestLANPairingRejectLeavesNoAuthorization(t *testing.T) {
	a, err := New(Config{DataDir: t.TempDir(), AllowInsecureLoopback: true, Name: "a"})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Shutdown()
	b, err := New(Config{DataDir: t.TempDir(), AllowInsecureLoopback: true, Name: "b"})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Shutdown()
	am, err := discovery.Start(discovery.Config{Identity: a.identity, Name: "a", Port: unusedUDPPort(t), AllowLoopback: true})
	if err != nil {
		t.Fatal(err)
	}
	defer am.Close()
	bm, err := discovery.Start(discovery.Config{Identity: b.identity, Name: "b", Port: unusedUDPPort(t), AllowLoopback: true})
	if err != nil {
		t.Fatal(err)
	}
	defer bm.Close()
	if err = am.RememberVerifiedPeer(discovery.Device{ID: b.identity.ID(), Name: "b", PublicKey: b.identity.PublicKey()}, discovery.Route{RemoteAddress: "127.0.0.1", ControlPort: bm.ControlPort(), Interface: "loopback", LocalAddress: "127.0.0.1", LastSeen: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if err = bm.RememberVerifiedPeer(discovery.Device{ID: a.identity.ID(), Name: "a", PublicKey: a.identity.PublicKey()}, discovery.Route{RemoteAddress: "127.0.0.1", ControlPort: am.ControlPort(), Interface: "loopback", LocalAddress: "127.0.0.1", LastSeen: time.Now()}); err != nil {
		t.Fatal(err)
	}
	a.lan = &lanRuntime{manager: am}
	defer func() { a.lan = nil }()
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	go func() { incoming := <-bm.Incoming(); b.receiveLANSession(ctx, incoming, "", DirectConfig{}) }()
	done := make(chan error, 1)
	go func() { _, requestErr := a.RequestLANPair(ctx, b.identity.ID()); done <- requestErr }()
	var request LANPairRequestInfo
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		pending := b.PendingLANPairings()
		if len(pending) == 1 {
			request = pending[0]
			break
		}
	}
	if request.RequestID == "" {
		t.Fatal("pair request did not arrive")
	}
	if err = b.RespondLANPair(request.RequestID, false); err != nil {
		t.Fatal(err)
	}
	if err = <-done; err == nil {
		t.Fatal("rejected LAN pairing reported success")
	}
	for _, service := range []*Service{a, b} {
		peers, loadErr := identity.LoadTrust(service.cfg.DataDir)
		if loadErr != nil || len(peers) != 0 {
			t.Fatalf("rejected pairing left authorization: %+v %v", peers, loadErr)
		}
	}
}
