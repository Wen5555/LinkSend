package discovery

import (
	"context"
	"encoding/binary"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/Wen5555/LinkSend/internal/identity"
	"github.com/Wen5555/LinkSend/internal/protocol"
)

func newTCPTestManager(t *testing.T, id *identity.Identity, name string) *Manager {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{cfg: Config{Identity: id, Name: name, AllowLoopback: true}, ctx: ctx, cancel: cancel, tcp: listener, port: listener.Addr().(*net.TCPAddr).Port, incoming: make(chan Incoming, 4), peers: map[string]*peerRecord{}, ifaces: map[int]interfaceRoute{}, joined: map[int]bool{}, replays: map[string]time.Time{}, lastResp: map[string]time.Time{}}
	m.tlsCfg, err = id.TLSServerConfig(m.allowedPeer)
	if err != nil {
		t.Fatal(err)
	}
	m.wg.Add(1)
	go m.acceptLoop()
	t.Cleanup(func() { _ = m.Close() })
	return m
}

func rememberLoopbackPeer(m *Manager, peer *Manager) {
	m.remember(packet{DeviceID: peer.cfg.Identity.ID(), Name: peer.cfg.Name, PublicKey: peer.cfg.Identity.PublicKey(), Port: peer.port}, Route{RemoteAddress: "127.0.0.1", ControlPort: peer.port, Interface: "loopback-test", LocalAddress: "127.0.0.1", LastSeen: time.Now()})
}

func TestSignedPacketValidationAndReplay(t *testing.T) {
	id, err := identity.Generate()
	if err != nil {
		t.Fatal(err)
	}
	m := &Manager{cfg: Config{Identity: id, Name: "peer"}, port: 45678, replays: map[string]time.Time{}}
	p, err := m.newPacket(true)
	if err != nil {
		t.Fatal(err)
	}
	source := net.ParseIP("192.0.2.44")
	if !m.validatePacket(p, source) {
		t.Fatal("valid signed announcement was rejected")
	}
	if m.validatePacket(p, source) {
		t.Fatal("replayed announcement was accepted")
	}
	p, err = m.newPacket(true)
	if err != nil {
		t.Fatal(err)
	}
	p.Name = "tampered"
	if m.validatePacket(p, source) {
		t.Fatal("tampered signed announcement was accepted")
	}
}

func TestSubnetBroadcast(t *testing.T) {
	for _, test := range []struct {
		ip, cidr, want string
	}{
		{"10.234.232.205", "10.234.0.0/16", "10.234.255.255"},
		{"192.168.8.24", "192.168.8.0/24", "192.168.8.255"},
	} {
		_, network, err := net.ParseCIDR(test.cidr)
		if err != nil {
			t.Fatal(err)
		}
		if got := subnetBroadcast(net.ParseIP(test.ip), network.Mask); got.String() != test.want {
			t.Fatalf("broadcast(%s,%s)=%s want %s", test.ip, test.cidr, got, test.want)
		}
	}
}

func TestLANSessionCarriesOnlyBoundedFrames(t *testing.T) {
	left, right := net.Pipe()
	aIdentity, err := identity.Generate()
	if err != nil {
		t.Fatal(err)
	}
	bIdentity, err := identity.Generate()
	if err != nil {
		t.Fatal(err)
	}
	a := newSession(left, Device{ID: bIdentity.ID()}, aIdentity)
	b := newSession(right, Device{ID: aIdentity.ID()}, bIdentity)
	defer a.Close()
	defer b.Close()
	envelope, err := protocol.NewEnvelope("status", aIdentity.ID(), bIdentity.ID(), "cccccccccccccccccccccccccccccccc", 1, map[string]string{"state": "ready"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err = a.SendEnvelope(ctx, envelope); err != nil {
		t.Fatal(err)
	}
	wire, err := b.Read(ctx)
	if err != nil || wire.Message == nil || wire.Message.MessageID != envelope.MessageID {
		t.Fatalf("frame round trip: %+v %v", wire, err)
	}
	if err = wire.Message.Verify(aIdentity.PublicKey(), time.Now()); err != nil {
		t.Fatalf("LAN envelope was not signed by the local identity: %v", err)
	}
	if a.Stats().BytesSent == 0 || b.Stats().BytesReceived == 0 {
		t.Fatal("LAN signaling accounting was not updated")
	}
}

func TestLANSessionRejectsOversizedFrame(t *testing.T) {
	left, right := net.Pipe()
	id, err := identity.Generate()
	if err != nil {
		t.Fatal(err)
	}
	session := newSession(left, Device{}, id)
	defer session.Close()
	defer right.Close()
	done := make(chan error, 1)
	go func() {
		var header [4]byte
		binary.BigEndian.PutUint32(header[:], maxFrameBytes+1)
		_, err := right.Write(header[:])
		done <- err
	}()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err = session.Read(ctx)
	if protocol.ErrorCode(err) != protocol.InvalidMessage {
		t.Fatalf("oversized frame error=%v", err)
	}
	if writeErr := <-done; writeErr != nil && !errors.Is(writeErr, net.ErrClosed) {
		t.Fatal(writeErr)
	}
}

func TestManagersEstablishMutuallyPinnedLANControl(t *testing.T) {
	aID, err := identity.Generate()
	if err != nil {
		t.Fatal(err)
	}
	bID, err := identity.Generate()
	if err != nil {
		t.Fatal(err)
	}
	a := newTCPTestManager(t, aID, "a")
	b := newTCPTestManager(t, bID, "b")
	rememberLoopbackPeer(a, b)
	rememberLoopbackPeer(b, a)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	type dialResult struct {
		session *Session
		err     error
	}
	dialed := make(chan dialResult, 1)
	go func() {
		session, _, _, dialErr := a.Dial(ctx, bID.ID())
		dialed <- dialResult{session: session, err: dialErr}
	}()
	var incoming Incoming
	select {
	case incoming = <-b.Incoming():
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	client := <-dialed
	if client.err != nil {
		t.Fatal(client.err)
	}
	defer client.session.Close()
	defer incoming.Session.Close()
	if incoming.Peer.ID != aID.ID() {
		t.Fatalf("authenticated peer=%s", incoming.Peer.ID)
	}
	if err = client.session.SendHeartbeat(ctx); err != nil {
		t.Fatal(err)
	}
	wire, err := incoming.Session.Read(ctx)
	if err != nil || wire.Type != "heartbeat" {
		t.Fatalf("heartbeat wire=%+v err=%v", wire, err)
	}
}
