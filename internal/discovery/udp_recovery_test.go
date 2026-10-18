package discovery

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"net"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/Wen5555/LinkSend/internal/identity"
	"golang.org/x/net/ipv4"
)

func discoveryTestIdentity(t *testing.T) *identity.Identity {
	t.Helper()
	id, err := identity.Generate()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func discoveryTestUDP(t *testing.T, address string, port int) *net.UDPConn {
	t.Helper()
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.ParseIP(address), Port: port})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func startConflictedDiscovery(t *testing.T) (*Manager, *net.UDPConn, int) {
	t.Helper()
	blocker := discoveryTestUDP(t, "0.0.0.0", 0)
	port := blocker.LocalAddr().(*net.UDPAddr).Port
	m, err := Start(Config{Identity: discoveryTestIdentity(t), Name: "degraded", Port: port, AllowLoopback: true})
	if err != nil {
		t.Fatalf("occupied discovery port disabled other channels: %v", err)
	}
	t.Cleanup(func() { _ = m.Close() })
	m.mu.RLock()
	actualPort := m.udp.LocalAddr().(*net.UDPAddr).Port
	m.mu.RUnlock()
	if actualPort == port || m.cfg.Port != port || m.ChannelErrors()["listen"] != "fixed_port_unavailable" {
		t.Fatalf("temporary listener lost its target or degradation status: actual=%d target=%d cfg=%d status=%v", actualPort, port, m.cfg.Port, m.ChannelErrors())
	}
	return m, blocker, port
}

func waitDiscovery(t *testing.T, timeout time.Duration, check func() bool, failure string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if check() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal(failure)
}

func discoveryHeartbeat(t *testing.T, a, b *Manager) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	session, peer, _, err := a.Dial(ctx, b.cfg.Identity.ID())
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	var incoming Incoming
	select {
	case incoming = <-b.Incoming():
	case <-ctx.Done():
		t.Fatal("mutually pinned control handshake did not reach the listener")
	}
	defer incoming.Session.Close()
	if peer.ID != b.cfg.Identity.ID() || incoming.Peer.ID != a.cfg.Identity.ID() {
		t.Fatal("TLS control identities changed during UDP degradation")
	}
	if err := session.SendHeartbeat(ctx); err != nil {
		t.Fatal(err)
	}
	wire, err := incoming.Session.Read(ctx)
	if err != nil || wire.Type != "heartbeat" {
		t.Fatalf("authenticated control round trip failed: wire=%+v err=%v", wire, err)
	}
}

func TestDiscoveryFixedPortConflictKeepsTLSAndRestores(t *testing.T) {
	m, blocker, port := startConflictedDiscovery(t)
	m.mu.RLock()
	temporary, oldReader := m.udp, m.packet
	m.mu.RUnlock()
	controlPort := m.ControlPort()
	peer := newTCPTestManager(t, discoveryTestIdentity(t), "known-peer")
	rememberLoopbackPeer(m, peer)
	rememberLoopbackPeer(peer, m)
	discoveryHeartbeat(t, peer, m)
	discoveryHeartbeat(t, m, peer)
	wrongPin, err := peer.cfg.Identity.TLSConfig(discoveryTestIdentity(t).PublicKey(), false)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	dialer := tls.Dialer{Config: wrongPin}
	conn, err := dialer.DialContext(ctx, "tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(controlPort)))
	if conn != nil {
		_ = conn.Close()
	}
	if err == nil {
		t.Fatal("degraded listener accepted the wrong expected identity pin")
	}
	for range 2 {
		_ = m.Refresh()
		m.mu.RLock()
		preserved := m.udp == temporary
		m.mu.RUnlock()
		if !preserved {
			t.Fatal("failed fixed-port retry replaced the healthy temporary socket")
		}
		if m.ChannelErrors()["listen"] != "fixed_port_unavailable" {
			t.Fatal("refresh hid the occupied fixed port")
		}
	}
	discoveryHeartbeat(t, peer, m)
	_ = blocker.Close()
	// A user refresh storm is coalesced; the released port recovers on the
	// next eligible wave rather than forcing a new probe for every click.
	time.Sleep(refreshMinInterval)
	_ = m.Refresh()
	m.mu.RLock()
	restored := m.udp.LocalAddr().(*net.UDPAddr).Port == port
	m.mu.RUnlock()
	if !restored || m.ChannelErrors()["listen"] != "" || m.ControlPort() != controlPort {
		t.Fatalf("fixed listener did not recover independently: restored=%t status=%v control=%d", restored, m.ChannelErrors(), m.ControlPort())
	}
	m.mu.RLock()
	current := m.udp
	m.mu.RUnlock()
	if err := m.rebuildUDPSocket(oldReader); err != nil {
		t.Fatal(err)
	}
	m.mu.RLock()
	preserved := m.udp == current
	m.mu.RUnlock()
	if !preserved {
		t.Fatal("late read error replaced the newly restored socket")
	}
	discoveryHeartbeat(t, peer, m)
}

func TestDiscoveryFixedPortRestoresOnExistingTicker(t *testing.T) {
	m, blocker, port := startConflictedDiscovery(t)
	_ = blocker.Close()
	waitDiscovery(t, announceEvery+2*time.Second, func() bool {
		m.mu.RLock()
		defer m.mu.RUnlock()
		return m.udp.LocalAddr().(*net.UDPAddr).Port == port && m.channelErrors["listen"] == ""
	}, "existing announcement ticker did not restore the fixed listener")
}

func TestDiscoveryLateReaderFailureCannotRestoreDegradedStatus(t *testing.T) {
	m, blocker, _ := startConflictedDiscovery(t)
	m.mu.RLock()
	oldReader := m.packet
	m.mu.RUnlock()
	if !m.markReceiveDegraded(oldReader) || m.ChannelErrors()["receive"] != "socket_read_degraded" {
		t.Fatal("current reader failure was not exposed")
	}
	_ = blocker.Close()
	_ = m.Refresh()
	m.mu.RLock()
	currentReader := m.packet
	m.mu.RUnlock()
	if currentReader == oldReader || m.ChannelErrors()["receive"] != "" || m.ChannelErrors()["listen"] != "" {
		t.Fatal("real fixed-port replacement did not clear the old provider failure")
	}
	// An old ReadFrom may already have passed readLoop's first socket check.
	// Its delayed status write and conditional rebuild must not poison the
	// replacement, even when no later read error would clear that status.
	lateAccepted := m.markReceiveDegraded(oldReader)
	if err := m.rebuildUDPSocket(oldReader); err != nil {
		t.Fatal(err)
	}
	if lateAccepted || m.ChannelErrors()["receive"] != "" {
		t.Fatalf("late old-reader failure survived socket replacement: accepted=%t status=%v", lateAccepted, m.ChannelErrors())
	}
	if !m.markReceiveDegraded(currentReader) || m.ChannelErrors()["receive"] != "socket_read_degraded" {
		t.Fatal("replacement reader's own failure was incorrectly suppressed")
	}
	if err := m.rebuildUDPSocket(currentReader); err != nil {
		t.Fatal(err)
	}
	if m.ChannelErrors()["receive"] != "" {
		t.Fatal("successful reader rebuild retained degraded receive status")
	}
	peer := newTCPTestManager(t, discoveryTestIdentity(t), "still-pinned")
	rememberLoopbackPeer(m, peer)
	rememberLoopbackPeer(peer, m)
	discoveryHeartbeat(t, peer, m)
}

func TestDiscoveryTemporaryRepliesAndReplacedReader(t *testing.T) {
	requireDiscoveryLoopbackSources(t, "127.0.0.2")
	m, blocker, port := startConflictedDiscovery(t)
	remote := discoveryTestUDP(t, "127.0.0.2", 0)
	if err := m.ProbeAddress("127.0.0.2"); err != nil {
		t.Fatal(err)
	}
	_ = blocker.SetReadDeadline(time.Now().Add(2 * time.Second))
	buffer := make([]byte, maxPacketBytes+1)
	var n int
	var source *net.UDPAddr
	for {
		var err error
		n, source, err = blocker.ReadFromUDP(buffer)
		if err != nil {
			t.Fatal(err)
		}
		if source.IP.Equal(net.ParseIP("127.0.0.1")) {
			break
		}
	}
	var announcement packet
	if json.Unmarshal(buffer[:n], &announcement) != nil || announcement.Port != m.ControlPort() || source.Port == port {
		t.Fatalf("discovery did not use the temporary source while advertising TLS: source=%v packet=%+v", source, announcement)
	}
	peer := &Manager{cfg: Config{Identity: discoveryTestIdentity(t), Name: "reply-peer"}, port: 45678}
	response, err := peer.newPacket(false)
	if err != nil {
		t.Fatal(err)
	}
	untampered := response
	response.Name = "tampered"
	body, _ := json.Marshal(response)
	if _, err := remote.WriteToUDP(body, source); err != nil {
		t.Fatal(err)
	}
	// Rejecting the tampered packet must not consume the genuine packet's nonce.
	body, _ = json.Marshal(untampered)
	if _, err := remote.WriteToUDP(body, source); err != nil {
		t.Fatal(err)
	}
	waitDiscovery(t, time.Second, func() bool {
		got, ok := m.Device(peer.cfg.Identity.ID())
		return ok && got.Name == "reply-peer"
	}, "temporary-source response was not authenticated and remembered")
	if m.ChannelErrors()["listen"] != "fixed_port_unavailable" {
		t.Fatal("successful reply concealed fixed-port degradation")
	}
	_ = remote.Close()
	_ = blocker.Close()
	_ = m.Refresh()
	remote = discoveryTestUDP(t, "127.0.0.2", 0)
	peer.cfg.Name = "after-socket-replacement"
	response, _ = peer.newPacket(false)
	body, _ = json.Marshal(response)
	if _, err := remote.WriteToUDP(body, &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: port}); err != nil {
		t.Fatal(err)
	}
	waitDiscovery(t, time.Second, func() bool {
		got, ok := m.Device(peer.cfg.Identity.ID())
		return ok && got.Name == peer.cfg.Name
	}, "readLoop did not follow the restored socket")
}

func discoveryLoopbackInterface(t *testing.T) *net.Interface {
	t.Helper()
	interfaces, err := net.Interfaces()
	if err != nil {
		t.Fatal(err)
	}
	for _, iface := range interfaces {
		addresses, _ := iface.Addrs()
		for _, address := range addresses {
			ip, _, _ := net.ParseCIDR(address.String())
			if ip.Equal(net.ParseIP("127.0.0.1")) {
				return &iface
			}
		}
	}
	t.Fatal("IPv4 loopback interface missing")
	return nil
}

func requireDiscoveryLoopbackSources(t *testing.T, addresses ...string) {
	t.Helper()
	for _, address := range addresses {
		conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.ParseIP(address)})
		if err != nil {
			// Some hosts only expose 127.0.0.1. Do not add interface aliases or
			// change routes just to manufacture a multi-source fixture.
			t.Skipf("additional loopback source %s is unavailable: %v", address, err)
		}
		_ = conn.Close()
	}
}

func TestDirectedAnnouncementAttemptsEveryMatchingSource(t *testing.T) {
	requireDiscoveryLoopbackSources(t, "127.0.0.2", "127.0.0.3")
	listener := discoveryTestUDP(t, "127.0.0.3", 0)
	sender := discoveryTestUDP(t, "0.0.0.0", 0)
	iface := discoveryLoopbackInterface(t)
	_, network, _ := net.ParseCIDR("127.0.0.0/8")
	ctx, cancel := context.WithCancel(t.Context())
	m := &Manager{cfg: Config{Identity: discoveryTestIdentity(t), Name: "multi-source", AllowLoopback: true}, ctx: ctx, cancel: cancel, udp: sender, packet: ipv4.NewPacketConn(sender), port: 45678, incoming: make(chan Incoming), ifaces: map[string]interfaceRoute{
		"first":  {iface: iface, address: net.ParseIP("127.0.0.1").To4(), network: network},
		"second": {iface: iface, address: net.ParseIP("127.0.0.2").To4(), network: network},
	}, responseSlots: make(chan struct{}, 32)}
	t.Cleanup(func() { _ = m.Close() })
	if err := m.sendAnnouncement(true, listener.LocalAddr().(*net.UDPAddr)); err != nil {
		t.Fatal(err)
	}
	_ = listener.SetReadDeadline(time.Now().Add(time.Second))
	seen := map[string]bool{}
	for len(seen) < 2 {
		_, source, err := listener.ReadFromUDP(make([]byte, maxPacketBytes))
		if err != nil {
			t.Fatalf("first successful UDP write hid another source route: sources=%v err=%v", seen, err)
		}
		seen[source.IP.String()] = true
	}
	if !seen["127.0.0.1"] || !seen["127.0.0.2"] {
		t.Fatalf("unexpected source addresses: %v", seen)
	}
	// Too many overlapping routes must be rejected explicitly, without an
	// unbounded fan-out or silently selecting only the first successful write.
	m.mu.Lock()
	for index := 0; index <= maxProbeRoutes; index++ {
		m.ifaces[strconv.Itoa(index)] = interfaceRoute{iface: iface, address: net.IPv4(127, 0, 1, byte(index+1)).To4(), network: network}
	}
	m.mu.Unlock()
	if err := m.sendAnnouncement(true, listener.LocalAddr().(*net.UDPAddr)); err == nil || err.Error() != "LAN discovery on-link route limit reached" {
		t.Fatalf("excessive route fan-out was not rejected: %v", err)
	}
}

func TestDiscoveryCloseRejectsLateSendsAndRecovery(t *testing.T) {
	m, blocker, port := startConflictedDiscovery(t)
	m.mu.RLock()
	temporaryPort := m.udp.LocalAddr().(*net.UDPAddr).Port
	m.mu.RUnlock()
	_ = blocker.Close()
	m.writeMu.Lock()
	var workers sync.WaitGroup
	for range 4 {
		workers.Go(func() { _ = m.Refresh() })
	}
	closed := make(chan struct{})
	go func() { _ = m.Close(); close(closed) }()
	<-m.ctx.Done()
	m.writeMu.Unlock()
	workers.Wait()
	select {
	case <-closed:
	case <-time.After(3 * time.Second):
		t.Fatal("close and fixed-port recovery deadlocked")
	}
	if err := m.Refresh(); !errors.Is(err, context.Canceled) {
		t.Fatalf("refresh accepted after close: %v", err)
	}
	if err := m.ProbeAddress("127.0.0.2"); !errors.Is(err, context.Canceled) {
		t.Fatalf("source socket could be created after close: %v", err)
	}
	if len(m.responseSlots) != 0 {
		t.Fatal("source response workers survived close")
	}
	_ = discoveryTestUDP(t, "0.0.0.0", port)
	_ = discoveryTestUDP(t, "0.0.0.0", temporaryPort)
}
