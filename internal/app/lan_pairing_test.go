package app

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/Wen5555/LinkSend/internal/discovery"
	"github.com/Wen5555/LinkSend/internal/identity"
	"github.com/Wen5555/LinkSend/internal/protocol"
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

func TestLANPairingQueryRecoversReadyAfterTLSDisconnect(t *testing.T) {
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
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	go func() {
		for count := 0; count < 3; count++ {
			select {
			case incoming := <-bm.Incoming():
				b.receiveLANSession(ctx, incoming, "", DirectConfig{})
			case <-ctx.Done():
				return
			}
		}
	}()
	first, peer, _, err := am.Dial(ctx, b.identity.ID())
	if err != nil {
		t.Fatal(err)
	}
	requestID, nonce, expires := protocol.RandomID(), protocol.RandomID(), time.Now().Add(time.Minute).Unix()
	if err = first.SendLANPair(ctx, newLANPairFrame("request", requestID, nonce, a.identity.ID(), b.identity.ID(), 1, expires)); err != nil {
		t.Fatal(err)
	}
	var pending LANPairRequestInfo
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		if requests := b.PendingLANPairings(); len(requests) == 1 {
			pending = requests[0]
			break
		}
	}
	if pending.RequestID == "" || b.RespondLANPair(pending.RequestID, true) != nil {
		t.Fatal("receiver consent was not available")
	}
	if wire, readErr := first.Read(ctx); readErr != nil || wire.LANPair == nil || wire.LANPair.Phase != "accept" {
		t.Fatalf("accept wire=%+v err=%v", wire, readErr)
	}
	grant := identity.ProvisionalLANGrant{RequestID: requestID, Nonce: nonce, Peer: identity.TrustedPeer{ID: peer.ID, Name: peer.Name, PublicKey: peer.PublicKey}, Generation: 1, State: "accepted", ExpiresAt: time.Unix(expires, 0).UTC()}
	if err = identity.BeginProvisionalLAN(a.cfg.DataDir, grant); err != nil {
		t.Fatal(err)
	}
	if err = first.SendLANPair(ctx, newLANPairFrame("commit", requestID, nonce, a.identity.ID(), b.identity.ID(), 1, expires)); err != nil {
		t.Fatal(err)
	}
	if wire, readErr := first.Read(ctx); readErr != nil || wire.LANPair == nil || wire.LANPair.Phase != "ready" {
		t.Fatalf("ready wire=%+v err=%v", wire, readErr)
	}
	_ = first.Close() // Fault injection: ready arrived, confirm did not.
	second, _, _, err := am.Dial(ctx, b.identity.ID())
	if err != nil {
		t.Fatal(err)
	}
	if err = second.SendLANPair(ctx, newLANPairFrame("query", requestID, nonce, a.identity.ID(), b.identity.ID(), 1, expires)); err != nil {
		t.Fatal(err)
	}
	if wire, readErr := second.Read(ctx); readErr != nil || wire.LANPair == nil || wire.LANPair.Phase != "ready" {
		t.Fatalf("query ready wire=%+v err=%v", wire, readErr)
	}
	if err = second.SendLANPair(ctx, newLANPairFrame("confirm", requestID, nonce, a.identity.ID(), b.identity.ID(), 1, expires)); err != nil {
		t.Fatal(err)
	}
	_ = second.Close() // Fault injection: confirm may arrive, done is lost.
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		if status, _ := identity.LANPairStatus(b.cfg.DataDir, a.identity.ID(), requestID, nonce); status == "done" {
			break
		}
	}
	third, _, _, err := am.Dial(ctx, b.identity.ID())
	if err != nil {
		t.Fatal(err)
	}
	defer third.Close()
	if err = third.SendLANPair(ctx, newLANPairFrame("query", requestID, nonce, a.identity.ID(), b.identity.ID(), 1, expires)); err != nil {
		t.Fatal(err)
	}
	if wire, readErr := third.Read(ctx); readErr != nil || wire.LANPair == nil || wire.LANPair.Phase != "done" {
		t.Fatalf("query recovered done wire=%+v err=%v", wire, readErr)
	}
	if err = identity.CommitProvisionalLAN(a.cfg.DataDir, requestID, nonce); err != nil {
		t.Fatal(err)
	}
	for name, service := range map[string]*Service{"a": a, "b": b} {
		peers, loadErr := identity.LoadTrust(service.cfg.DataDir)
		if loadErr != nil || len(peers) != 1 {
			t.Fatalf("%s recovery trust=%+v err=%v", name, peers, loadErr)
		}
	}
}
