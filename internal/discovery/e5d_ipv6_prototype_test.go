package discovery

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/Wen5555/LinkSend/internal/identity"
	"golang.org/x/net/ipv6"
)

const e5DIPv6Multicast = "ff12::4c69:6e6b:7365"

func TestE5DIPv6DiscoveryPrototype(t *testing.T) {
	role := os.Getenv("LINKSEND_E5D_DISCOVERY_ROLE")
	if role == "" {
		t.Skip("set LINKSEND_E5D_DISCOVERY_ROLE=a|b only in the isolated E5-D namespace lab")
	}
	if role != "a" && role != "b" {
		t.Fatalf("invalid lab role %q", role)
	}
	ifaceName := os.Getenv("LINKSEND_E5D_DISCOVERY_INTERFACE")
	localRaw := strings.Split(strings.TrimSpace(os.Getenv("LINKSEND_E5D_DISCOVERY_LOCAL")), ",")
	if ifaceName == "" || len(localRaw) == 0 {
		t.Fatal("isolated lab interface and local IPv6 address are required")
	}
	var locals []net.IP
	for _, raw := range localRaw {
		ip := net.ParseIP(strings.TrimSpace(raw))
		if ip == nil || ip.To4() != nil || ip.IsUnspecified() || ip.IsMulticast() || ip.IsLinkLocalUnicast() {
			t.Fatalf("invalid lab ULA address %q", raw)
		}
		locals = append(locals, ip)
	}
	iface, err := net.InterfaceByName(ifaceName)
	if err != nil {
		t.Fatal(err)
	}
	identityValue, err := identity.Generate()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var listener *net.TCPListener
	if role == "a" {
		listener, err = net.ListenTCP("tcp6", &net.TCPAddr{IP: net.IPv6unspecified, Port: 0})
		if err != nil {
			t.Fatal(err)
		}
		defer listener.Close()
	}

	manager, err := newE5DIPv6Manager(identityValue, role, listener)
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	if role == "a" {
		manager.wg.Add(1)
		go manager.acceptLoop()
	}

	udp, packetConn, err := openE5DIPv6Socket(iface)
	if err != nil {
		t.Fatal(err)
	}
	defer udp.Close()
	controlPort := manager.port
	if controlPort == 0 {
		t.Fatal("prototype control listener port missing")
	}
	if role == "a" {
		fmt.Printf("E5D_DISCOVERY_READY role=a control_port=%d\n", controlPort)
	}

	wantRoutes := 1
	if role == "b" {
		wantRoutes = 2
	}
	peer, validPackets, err := discoverE5DIPv6(ctx, manager, packetConn, iface, locals, wantRoutes)
	if err != nil {
		t.Fatal(err)
	}
	if validPackets == 0 {
		t.Fatal("no signed IPv6 discovery packet was accepted")
	}

	if role == "a" {
		select {
		case incoming := <-manager.Incoming():
			defer incoming.Session.Close()
			readCtx, stop := context.WithTimeout(ctx, 3*time.Second)
			wire, readErr := incoming.Session.Read(readCtx)
			stop()
			if readErr != nil || wire.Type != "heartbeat" || incoming.Peer.ID != peer.ID {
				t.Fatalf("LAN TLS incoming peer=%s wire=%+v err=%v", incoming.Peer.ID, wire, readErr)
			}
			emitE5DDiscoveryResult(t, map[string]any{
				"result":               "PASS",
				"role":                 role,
				"peer_id":              peer.ID,
				"route_count":          len(peer.Routes),
				"valid_signed_packets": validPackets,
				"tls_version":          tls.VersionTLS13,
				"alpn":                 identity.ALPN,
				"family":               "ipv6",
			})
		case <-ctx.Done():
			t.Fatal("IPv6 LAN TLS control did not arrive")
		}
		return
	}

	if len(peer.Routes) != 2 {
		t.Fatalf("same DeviceID routes=%d want=2", len(peer.Routes))
	}
	remoteAddresses := make([]string, 0, len(peer.Routes))
	for _, route := range peer.Routes {
		if route.Family != "ipv6" {
			t.Fatalf("route family=%q", route.Family)
		}
		remoteAddresses = append(remoteAddresses, route.RemoteAddress)
	}
	sort.Strings(remoteAddresses)
	if len(remoteAddresses) != 2 || remoteAddresses[0] == remoteAddresses[1] {
		t.Fatalf("expected two distinct IPv6 routes: %v", remoteAddresses)
	}
	if err = dialE5DIPv6TLS(ctx, identityValue, peer, locals[0]); err != nil {
		t.Fatal(err)
	}
	emitE5DDiscoveryResult(t, map[string]any{
		"result":               "PASS",
		"role":                 role,
		"peer_id":              peer.ID,
		"route_count":          len(peer.Routes),
		"remote_addresses":     remoteAddresses,
		"valid_signed_packets": validPackets,
		"tls_version":          tls.VersionTLS13,
		"alpn":                 identity.ALPN,
		"family":               "ipv6",
	})
}

func newE5DIPv6Manager(id *identity.Identity, name string, listener *net.TCPListener) (*Manager, error) {
	ctx, cancel := context.WithCancel(context.Background())
	manager := &Manager{
		cfg:           Config{Identity: id, Name: "e5d-v6-" + name},
		ctx:           ctx,
		cancel:        cancel,
		tcp:           listener,
		incoming:      make(chan Incoming, 4),
		peers:         map[string]*peerRecord{},
		ifaces:        map[string]interfaceRoute{},
		joined:        map[int]*net.Interface{},
		replays:       map[string]time.Time{},
		lastResp:      map[string]time.Time{},
		acceptSlots:   make(chan struct{}, 4),
		responseSlots: make(chan struct{}, 4),
		channelErrors: map[string]string{},
		remembered:    map[string]rememberedProbe{},
	}
	if listener != nil {
		manager.port = listener.Addr().(*net.TCPAddr).Port
	} else {
		manager.port = 41001
	}
	var err error
	manager.tlsCfg, err = id.TLSServerConfig(manager.allowedPeer)
	if err != nil {
		cancel()
		return nil, err
	}
	return manager, nil
}

func openE5DIPv6Socket(iface *net.Interface) (*net.UDPConn, *ipv6.PacketConn, error) {
	group := net.ParseIP(e5DIPv6Multicast)
	if group == nil {
		return nil, nil, fmt.Errorf("invalid IPv6 multicast group")
	}
	socket, err := net.ListenUDP("udp6", &net.UDPAddr{IP: net.IPv6unspecified, Port: DefaultPort})
	if err != nil {
		return nil, nil, err
	}
	packetConn := ipv6.NewPacketConn(socket)
	if err = packetConn.SetControlMessage(ipv6.FlagInterface, true); err != nil {
		_ = socket.Close()
		return nil, nil, err
	}
	if err = packetConn.SetMulticastHopLimit(1); err != nil {
		_ = socket.Close()
		return nil, nil, err
	}
	if err = packetConn.SetMulticastLoopback(false); err != nil {
		_ = socket.Close()
		return nil, nil, err
	}
	if err = packetConn.JoinGroup(iface, &net.UDPAddr{IP: group}); err != nil {
		_ = socket.Close()
		return nil, nil, err
	}
	return socket, packetConn, nil
}

func discoverE5DIPv6(ctx context.Context, manager *Manager, packetConn *ipv6.PacketConn, iface *net.Interface, locals []net.IP, wantRoutes int) (Device, int, error) {
	group := net.ParseIP(e5DIPv6Multicast)
	nextSend := time.Time{}
	validPackets := 0
	buffer := make([]byte, maxPacketBytes+1)
	for {
		if !nextSend.After(time.Now()) {
			for _, local := range locals {
				if err := sendE5DIPv6DiscoveryPacket(manager, packetConn, iface, local, group, DefaultPort, true); err != nil {
					return Device{}, validPackets, err
				}
			}
			nextSend = time.Now().Add(250 * time.Millisecond)
		}
		if peer := firstE5DPeer(manager); peer.ID != "" && len(peer.Routes) >= wantRoutes {
			return peer, validPackets, nil
		}
		if err := packetConn.SetReadDeadline(time.Now().Add(100 * time.Millisecond)); err != nil {
			return Device{}, validPackets, err
		}
		n, control, source, err := packetConn.ReadFrom(buffer)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				select {
				case <-ctx.Done():
					return Device{}, validPackets, ctx.Err()
				default:
					continue
				}
			}
			return Device{}, validPackets, err
		}
		udpSource, ok := source.(*net.UDPAddr)
		if !ok || udpSource.IP == nil || udpSource.IP.To4() != nil || udpSource.IP.IsUnspecified() || udpSource.IP.IsMulticast() || udpSource.IP.IsLinkLocalUnicast() {
			continue
		}
		var received packet
		if json.Unmarshal(buffer[:n], &received) != nil || received.DeviceID == manager.cfg.Identity.ID() || !manager.validatePacket(received, udpSource.IP) {
			continue
		}
		if control == nil || control.IfIndex != iface.Index {
			continue
		}
		manager.remember(received, Route{
			RemoteAddress: udpSource.IP.String(),
			ControlPort:   received.Port,
			Interface:     iface.Name,
			LocalAddress:  locals[0].String(),
			LastSeen:      time.Now(),
			Family:        "ipv6",
			Generation:    manager.networkGeneration(),
		})
		validPackets++
		if received.Announce {
			for _, local := range locals {
				if err := sendE5DIPv6DiscoveryPacket(manager, packetConn, iface, local, udpSource.IP, udpSource.Port, false); err != nil {
					return Device{}, validPackets, err
				}
			}
		}
	}
}

func sendE5DIPv6DiscoveryPacket(manager *Manager, packetConn *ipv6.PacketConn, iface *net.Interface, local, destination net.IP, port int, announce bool) error {
	announcement, err := manager.newPacket(announce)
	if err != nil {
		return err
	}
	body, err := json.Marshal(announcement)
	if err != nil || len(body) > maxPacketBytes {
		return fmt.Errorf("invalid signed discovery packet")
	}
	zone := ""
	if destination.IsLinkLocalMulticast() || destination.IsLinkLocalUnicast() {
		zone = iface.Name
	}
	_, err = packetConn.WriteTo(body, &ipv6.ControlMessage{IfIndex: iface.Index, Src: local}, &net.UDPAddr{IP: destination, Port: port, Zone: zone})
	return err
}

func firstE5DPeer(manager *Manager) Device {
	peers := manager.Peers()
	if len(peers) == 0 {
		return Device{}
	}
	return peers[0]
}

func dialE5DIPv6TLS(ctx context.Context, local *identity.Identity, peer Device, bind net.IP) error {
	tlsConfig, err := local.TLSConfig(peer.PublicKey, false)
	if err != nil {
		return err
	}
	var lastErr error
	for _, route := range peer.Routes {
		dialer := net.Dialer{LocalAddr: &net.TCPAddr{IP: bind}}
		raw, dialErr := dialer.DialContext(ctx, "tcp6", net.JoinHostPort(route.RemoteAddress, fmt.Sprint(route.ControlPort)))
		if dialErr != nil {
			lastErr = dialErr
			continue
		}
		conn := tls.Client(raw, tlsConfig.Clone())
		handshakeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		dialErr = conn.HandshakeContext(handshakeCtx)
		cancel()
		if dialErr != nil {
			_ = raw.Close()
			lastErr = dialErr
			continue
		}
		session := newSession(conn, peer, local)
		sendCtx, stop := context.WithTimeout(ctx, 2*time.Second)
		dialErr = session.SendHeartbeat(sendCtx)
		stop()
		closeErr := session.Close()
		if dialErr == nil {
			dialErr = closeErr
		}
		if dialErr == nil {
			state := conn.ConnectionState()
			if state.Version != tls.VersionTLS13 || state.NegotiatedProtocol != identity.ALPN {
				return fmt.Errorf("unexpected LAN TLS parameters")
			}
			return nil
		}
		lastErr = dialErr
	}
	if lastErr == nil {
		lastErr = errors.New("no IPv6 discovery route")
	}
	return lastErr
}

func emitE5DDiscoveryResult(t *testing.T, result map[string]any) {
	t.Helper()
	body, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("E5D_DISCOVERY_RESULT=%s\n", body)
}
