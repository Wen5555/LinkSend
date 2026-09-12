// Package discovery finds LinkSend peers on directly connected IPv4 networks.
// It carries only bounded, signed control metadata; file bytes never enter this
// package. Discovery establishes reachability, not trust or transfer consent.
package discovery

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/subtle"
	"crypto/tls"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/Wen5555/LinkSend/internal/identity"
	"github.com/Wen5555/LinkSend/internal/protocol"
	"github.com/Wen5555/LinkSend/internal/signaling"
	"golang.org/x/net/ipv4"
)

const (
	DefaultPort     = 53318
	maxPacketBytes  = 2048
	peerLifetime    = 15 * time.Second
	announceEvery   = 4 * time.Second
	maxFrameBytes   = protocol.MaxMessageBytes
	discoveryMethod = "LINKSEND-LAN"
	discoveryPath   = "/v1/discovery"
)

var multicastIP = net.IPv4(239, 255, 76, 83)

type Config struct {
	Identity           *identity.Identity
	Name               string
	Port               int
	ExcludedInterfaces []string
	AllowLoopback      bool
}

type Route struct {
	RemoteAddress string    `json:"remote_address"`
	ControlPort   int       `json:"control_port"`
	Interface     string    `json:"interface"`
	LocalAddress  string    `json:"local_address"`
	LastSeen      time.Time `json:"last_seen"`
}

type Device struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	PublicKey ed25519.PublicKey `json:"-"`
	Routes    []Route           `json:"routes"`
}

type Incoming struct {
	Session *Session
	Peer    Device
	Route   Route
}

type packet struct {
	Version   int    `json:"version"`
	Announce  bool   `json:"announce"`
	DeviceID  string `json:"device_id"`
	Name      string `json:"name"`
	PublicKey []byte `json:"public_key"`
	Port      int    `json:"port"`
	Nonce     string `json:"nonce"`
	IssuedAt  int64  `json:"issued_at"`
	Signature []byte `json:"signature"`
}

type unsignedPacket struct {
	Version   int    `json:"version"`
	Announce  bool   `json:"announce"`
	DeviceID  string `json:"device_id"`
	Name      string `json:"name"`
	PublicKey []byte `json:"public_key"`
	Port      int    `json:"port"`
}

type interfaceRoute struct {
	iface     *net.Interface
	address   net.IP
	network   *net.IPNet
	broadcast net.IP
}

type peerRecord struct {
	device Device
	routes map[string]Route
}

type Manager struct {
	cfg       Config
	ctx       context.Context
	cancel    context.CancelFunc
	udp       net.PacketConn
	packet    *ipv4.PacketConn
	tcp       net.Listener
	tlsCfg    *tls.Config
	port      int
	incoming  chan Incoming
	wg        sync.WaitGroup
	acceptWG  sync.WaitGroup
	close     sync.Once
	writeMu   sync.Mutex
	refreshMu sync.Mutex
	mu        sync.RWMutex
	peers     map[string]*peerRecord
	ifaces    map[int]interfaceRoute
	joined    map[int]bool
	replays   map[string]time.Time
	lastResp  map[string]time.Time
}

func Start(cfg Config) (*Manager, error) {
	if cfg.Identity == nil {
		return nil, errors.New("LAN discovery identity is required")
	}
	cfg.Name = strings.TrimSpace(cfg.Name)
	if cfg.Name == "" || len(cfg.Name) > 128 || !utf8.ValidString(cfg.Name) {
		return nil, errors.New("invalid LAN discovery device name")
	}
	if cfg.Port == 0 {
		cfg.Port = DefaultPort
	}
	if cfg.Port < 1 || cfg.Port > 65535 {
		return nil, errors.New("invalid LAN discovery port")
	}

	tcp, err := net.Listen("tcp4", "0.0.0.0:0")
	if err != nil {
		return nil, fmt.Errorf("LAN control listen: %w", err)
	}
	udp, err := net.ListenPacket("udp4", net.JoinHostPort(net.IPv4zero.String(), fmt.Sprint(cfg.Port)))
	if err != nil {
		_ = tcp.Close()
		return nil, fmt.Errorf("LAN multicast listen: %w", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{cfg: cfg, ctx: ctx, cancel: cancel, udp: udp, packet: ipv4.NewPacketConn(udp), tcp: tcp, port: tcp.Addr().(*net.TCPAddr).Port, incoming: make(chan Incoming, 16), peers: map[string]*peerRecord{}, ifaces: map[int]interfaceRoute{}, joined: map[int]bool{}, replays: map[string]time.Time{}, lastResp: map[string]time.Time{}}
	m.tlsCfg, err = cfg.Identity.TLSServerConfig(m.allowedPeer)
	if err != nil {
		m.Close()
		return nil, err
	}
	// Darwin/Linux expose the receiving interface index. Windows does not
	// implement this control message, so routeForSource falls back to exact
	// local-subnet matching across the joined interfaces.
	_ = m.packet.SetControlMessage(ipv4.FlagInterface, true)
	if err = m.packet.SetMulticastTTL(1); err != nil {
		m.Close()
		return nil, fmt.Errorf("LAN multicast TTL: %w", err)
	}
	_ = m.packet.SetMulticastLoopback(false)
	if err = enableBroadcast(m.udp); err != nil {
		m.Close()
		return nil, fmt.Errorf("LAN broadcast socket: %w", err)
	}
	if err = m.refreshInterfaces(); err != nil {
		m.Close()
		return nil, err
	}
	m.wg.Add(3)
	go m.readLoop()
	go m.announceLoop()
	go m.acceptLoop()
	return m, nil
}

func (m *Manager) Incoming() <-chan Incoming { return m.incoming }

func (m *Manager) Close() error {
	var closeErr error
	m.close.Do(func() {
		if m.cancel != nil {
			m.cancel()
		}
		if m.udp != nil {
			closeErr = errors.Join(closeErr, m.udp.Close())
		}
		if m.tcp != nil {
			closeErr = errors.Join(closeErr, m.tcp.Close())
		}
		m.wg.Wait()
		m.acceptWG.Wait()
		close(m.incoming)
	})
	return closeErr
}

func (m *Manager) Refresh() error {
	if err := m.refreshInterfaces(); err != nil {
		return err
	}
	return m.sendAnnouncement(true, nil)
}

// ProbeAddress is the bounded fallback for networks that filter multicast and
// directed broadcast. Only an on-link unicast address is accepted; discovery
// can never use this method as an arbitrary network scanner.
func (m *Manager) ProbeAddress(raw string) error {
	address, err := netip.ParseAddr(strings.TrimSpace(raw))
	if err != nil || !address.Is4() || address.IsUnspecified() || address.IsMulticast() || address.IsLoopback() && !m.cfg.AllowLoopback {
		return protocol.Fail(protocol.LANProbeInvalid, "LAN probe requires an IPv4 address")
	}
	ip := net.IP(address.AsSlice())
	m.mu.RLock()
	onLink := false
	for _, route := range m.ifaces {
		if route.network.Contains(ip) && !route.address.Equal(ip) && (route.broadcast == nil || !route.broadcast.Equal(ip)) {
			onLink = true
			break
		}
	}
	m.mu.RUnlock()
	if !onLink {
		return protocol.Fail(protocol.LANProbeInvalid, "LAN probe address is not directly connected")
	}
	return m.sendAnnouncement(true, &net.UDPAddr{IP: ip, Port: m.cfg.Port})
}

func (m *Manager) Peers() []Device {
	now := time.Now()
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Device, 0, len(m.peers))
	for _, record := range m.peers {
		device := record.device
		device.PublicKey = bytes.Clone(record.device.PublicKey)
		device.Routes = nil
		for _, route := range record.routes {
			if now.Sub(route.LastSeen) <= peerLifetime {
				device.Routes = append(device.Routes, route)
			}
		}
		if len(device.Routes) == 0 {
			continue
		}
		sort.Slice(device.Routes, func(i, j int) bool { return device.Routes[i].LastSeen.After(device.Routes[j].LastSeen) })
		out = append(out, device)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].ID < out[j].ID
	})
	return out
}

func (m *Manager) Device(id string) (Device, bool) {
	for _, peer := range m.Peers() {
		if subtle.ConstantTimeCompare([]byte(peer.ID), []byte(id)) == 1 {
			return peer, true
		}
	}
	return Device{}, false
}

func (m *Manager) Dial(ctx context.Context, id string) (*Session, Device, Route, error) {
	peer, ok := m.Device(id)
	if !ok {
		return nil, Device{}, Route{}, protocol.Fail(protocol.PeerOffline, "LAN peer announcement expired")
	}
	tlsCfg, err := m.cfg.Identity.TLSConfig(peer.PublicKey, false)
	if err != nil {
		return nil, Device{}, Route{}, err
	}
	var dialErrors []error
	for _, route := range peer.Routes {
		localIP := net.ParseIP(route.LocalAddress)
		if localIP == nil {
			continue
		}
		dialer := net.Dialer{LocalAddr: &net.TCPAddr{IP: localIP}}
		dialCtx, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
		raw, dialErr := dialer.DialContext(dialCtx, "tcp4", net.JoinHostPort(route.RemoteAddress, fmt.Sprint(route.ControlPort)))
		cancel()
		if dialErr != nil {
			dialErrors = append(dialErrors, dialErr)
			continue
		}
		conn := tls.Client(raw, tlsCfg.Clone())
		handshakeCtx, stop := context.WithTimeout(ctx, 3*time.Second)
		dialErr = conn.HandshakeContext(handshakeCtx)
		stop()
		if dialErr != nil {
			_ = raw.Close()
			dialErrors = append(dialErrors, dialErr)
			continue
		}
		return newSession(conn, peer, m.cfg.Identity), peer, route, nil
	}
	return nil, Device{}, Route{}, protocol.Wrap(protocol.LANControlUnreachable, "LAN control connection failed", errors.Join(dialErrors...))
}

func (m *Manager) refreshInterfaces() error {
	m.refreshMu.Lock()
	defer m.refreshMu.Unlock()
	interfaces, err := net.Interfaces()
	if err != nil {
		return err
	}
	next := map[int]interfaceRoute{}
	for index := range interfaces {
		iface := interfaces[index]
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagMulticast == 0 || listed(iface.Name, m.cfg.ExcludedInterfaces) {
			continue
		}
		addresses, addrErr := iface.Addrs()
		if addrErr != nil {
			continue
		}
		for _, address := range addresses {
			ip, network, parseErr := net.ParseCIDR(address.String())
			if parseErr != nil || ip.To4() == nil || ip.IsUnspecified() || ip.IsMulticast() || ip.IsLinkLocalUnicast() || (!m.cfg.AllowLoopback && ip.IsLoopback()) {
				continue
			}
			next[iface.Index] = interfaceRoute{iface: &iface, address: ip.To4(), network: network, broadcast: subnetBroadcast(ip.To4(), network.Mask)}
			break
		}
	}
	if len(next) == 0 {
		return errors.New("LAN discovery found no multicast-capable IPv4 interface")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	active := make(map[int]interfaceRoute, len(next))
	for index, previous := range m.ifaces {
		if _, ok := next[index]; !ok {
			_ = m.packet.LeaveGroup(previous.iface, &net.UDPAddr{IP: multicastIP})
			delete(m.joined, index)
		}
	}
	for index, route := range next {
		if !m.joined[index] {
			if err = m.packet.JoinGroup(route.iface, &net.UDPAddr{IP: multicastIP}); err != nil {
				continue
			}
			m.joined[index] = true
		}
		active[index] = route
	}
	if len(active) == 0 {
		return errors.New("LAN discovery could not join multicast on an eligible interface")
	}
	m.ifaces = active
	return nil
}

func (m *Manager) announceLoop() {
	defer m.wg.Done()
	_ = m.sendAnnouncement(true, nil)
	fast := time.NewTimer(350 * time.Millisecond)
	ticker := time.NewTicker(announceEvery)
	cleanup := time.NewTicker(3 * time.Second)
	defer fast.Stop()
	defer ticker.Stop()
	defer cleanup.Stop()
	for {
		select {
		case <-m.ctx.Done():
			return
		case <-fast.C:
			_ = m.sendAnnouncement(true, nil)
		case <-ticker.C:
			_ = m.refreshInterfaces()
			_ = m.sendAnnouncement(true, nil)
			m.probeKnownPeers()
		case <-cleanup.C:
			m.expirePeers()
		}
	}
}

func (m *Manager) probeKnownPeers() {
	seen := map[string]bool{}
	for _, peer := range m.Peers() {
		for _, route := range peer.Routes {
			if seen[route.RemoteAddress] {
				continue
			}
			seen[route.RemoteAddress] = true
			ip := net.ParseIP(route.RemoteAddress)
			if ip != nil {
				_ = m.sendAnnouncement(true, &net.UDPAddr{IP: ip, Port: m.cfg.Port})
			}
		}
	}
}

func (m *Manager) readLoop() {
	defer m.wg.Done()
	buffer := make([]byte, maxPacketBytes+1)
	for {
		n, control, source, err := m.packet.ReadFrom(buffer)
		if err != nil {
			if m.ctx.Err() != nil {
				return
			}
			continue
		}
		if n == 0 || n > maxPacketBytes {
			continue
		}
		udpSource, ok := source.(*net.UDPAddr)
		if !ok || udpSource.IP == nil || udpSource.IP.To4() == nil || udpSource.IP.IsUnspecified() || udpSource.IP.IsMulticast() || udpSource.IP.IsLoopback() && !m.cfg.AllowLoopback {
			continue
		}
		route, ok := m.routeForSource(control, udpSource.IP)
		if !ok {
			continue
		}
		var received packet
		if json.Unmarshal(buffer[:n], &received) != nil || !m.validatePacket(received, udpSource.IP) {
			continue
		}
		if received.DeviceID == m.cfg.Identity.ID() {
			continue
		}
		seen := Route{RemoteAddress: udpSource.IP.String(), ControlPort: received.Port, Interface: route.iface.Name, LocalAddress: route.address.String(), LastSeen: time.Now()}
		m.remember(received, seen)
		if received.Announce && m.shouldRespond(received.DeviceID, udpSource.IP) {
			destination := &net.UDPAddr{IP: udpSource.IP, Port: udpSource.Port}
			_ = m.sendAnnouncement(false, destination)
		}
	}
}

func (m *Manager) acceptLoop() {
	defer m.wg.Done()
	for {
		raw, err := m.tcp.Accept()
		if err != nil {
			if m.ctx.Err() != nil {
				return
			}
			continue
		}
		m.acceptWG.Add(1)
		go func() {
			defer m.acceptWG.Done()
			m.acceptOne(raw)
		}()
	}
}

func (m *Manager) acceptOne(raw net.Conn) {
	remote, ok := raw.RemoteAddr().(*net.TCPAddr)
	if !ok || remote.IP == nil {
		_ = raw.Close()
		return
	}
	conn := tls.Server(raw, m.tlsCfg.Clone())
	ctx, cancel := context.WithTimeout(m.ctx, 4*time.Second)
	err := conn.HandshakeContext(ctx)
	cancel()
	if err != nil {
		_ = raw.Close()
		return
	}
	state := conn.ConnectionState()
	if len(state.PeerCertificates) != 1 {
		_ = conn.Close()
		return
	}
	key, ok := state.PeerCertificates[0].PublicKey.(ed25519.PublicKey)
	if !ok {
		_ = conn.Close()
		return
	}
	id := identity.DeviceID(key)
	peer, route, ok := m.peerFromAddress(id, remote.IP)
	if !ok || !bytes.Equal(peer.PublicKey, key) {
		_ = conn.Close()
		return
	}
	session := newSession(conn, peer, m.cfg.Identity)
	select {
	case m.incoming <- Incoming{Session: session, Peer: peer, Route: route}:
	case <-m.ctx.Done():
		_ = session.Close()
	default:
		_ = session.Close()
	}
}

func (m *Manager) sendAnnouncement(announce bool, destination *net.UDPAddr) error {
	p, err := m.newPacket(announce)
	if err != nil {
		return err
	}
	body, err := json.Marshal(p)
	if err != nil || len(body) > maxPacketBytes {
		return errors.New("LAN discovery packet exceeds limit")
	}
	m.writeMu.Lock()
	defer m.writeMu.Unlock()
	if destination != nil {
		_, err = m.packet.WriteTo(body, nil, destination)
		return err
	}
	m.mu.RLock()
	routes := make([]interfaceRoute, 0, len(m.ifaces))
	for _, route := range m.ifaces {
		routes = append(routes, route)
	}
	m.mu.RUnlock()
	var errs []error
	sent := false
	for _, route := range routes {
		if err = m.packet.SetMulticastInterface(route.iface); err != nil {
			errs = append(errs, err)
		} else {
			destination := &net.UDPAddr{IP: multicastIP, Port: m.cfg.Port}
			if _, err = m.packet.WriteTo(body, nil, destination); err != nil {
				errs = append(errs, err)
			} else {
				sent = true
			}
		}
		if route.broadcast != nil && !route.broadcast.Equal(route.address) {
			destination := &net.UDPAddr{IP: route.broadcast, Port: m.cfg.Port}
			if _, err = m.packet.WriteTo(body, nil, destination); err != nil {
				errs = append(errs, err)
			} else {
				sent = true
			}
		}
	}
	if sent {
		return nil
	}
	return errors.Join(errs...)
}

func (m *Manager) newPacket(announce bool) (packet, error) {
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return packet{}, err
	}
	p := packet{Version: protocol.Version, Announce: announce, DeviceID: m.cfg.Identity.ID(), Name: m.cfg.Name, PublicKey: m.cfg.Identity.PublicKey(), Port: m.port, Nonce: hex.EncodeToString(random[:]), IssuedAt: time.Now().Unix()}
	p.Signature = m.cfg.Identity.Sign(packetBytes(p))
	if len(p.Signature) != ed25519.SignatureSize {
		return packet{}, errors.New("LAN discovery signature failed")
	}
	return p, nil
}

func packetBytes(p packet) []byte {
	body, _ := json.Marshal(unsignedPacket{Version: p.Version, Announce: p.Announce, DeviceID: p.DeviceID, Name: p.Name, PublicKey: p.PublicKey, Port: p.Port})
	return protocol.AuthBytes(discoveryMethod, discoveryPath, p.DeviceID, p.Nonce, p.IssuedAt, body)
}

func (m *Manager) validatePacket(p packet, source net.IP) bool {
	now := time.Now()
	if p.Version != protocol.Version || p.DeviceID == "" || len(p.Name) == 0 || len(p.Name) > 128 || !utf8.ValidString(p.Name) || len(p.PublicKey) != ed25519.PublicKeySize || identity.DeviceID(p.PublicKey) != p.DeviceID || p.Port < 1 || p.Port > 65535 || len(p.Nonce) != 32 || len(p.Signature) != ed25519.SignatureSize || p.IssuedAt < now.Add(-30*time.Second).Unix() || p.IssuedAt > now.Add(15*time.Second).Unix() || !ed25519.Verify(p.PublicKey, packetBytes(p), p.Signature) {
		return false
	}
	if _, err := hex.DecodeString(p.Nonce); err != nil {
		return false
	}
	replayKey := p.DeviceID + "|" + p.Nonce + "|" + source.String()
	m.mu.Lock()
	defer m.mu.Unlock()
	for key, expiry := range m.replays {
		if !expiry.After(now) {
			delete(m.replays, key)
		}
	}
	if _, exists := m.replays[replayKey]; exists || len(m.replays) >= 1024 {
		return false
	}
	m.replays[replayKey] = now.Add(45 * time.Second)
	return true
}

func (m *Manager) routeForSource(control *ipv4.ControlMessage, source net.IP) (interfaceRoute, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if control != nil && control.IfIndex != 0 {
		route, ok := m.ifaces[control.IfIndex]
		if ok && route.network.Contains(source) && !route.address.Equal(source) {
			return route, true
		}
		return interfaceRoute{}, false
	}
	for _, route := range m.ifaces {
		if route.network.Contains(source) && !route.address.Equal(source) {
			return route, true
		}
	}
	return interfaceRoute{}, false
}

func (m *Manager) remember(p packet, route Route) {
	key := route.Interface + "|" + route.RemoteAddress + "|" + fmt.Sprint(route.ControlPort)
	m.mu.Lock()
	defer m.mu.Unlock()
	record := m.peers[p.DeviceID]
	if record == nil {
		record = &peerRecord{routes: map[string]Route{}}
		m.peers[p.DeviceID] = record
	}
	record.device = Device{ID: p.DeviceID, Name: p.Name, PublicKey: bytes.Clone(p.PublicKey)}
	record.routes[key] = route
}

func (m *Manager) expirePeers() {
	now := time.Now()
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, record := range m.peers {
		for key, route := range record.routes {
			if now.Sub(route.LastSeen) > peerLifetime {
				delete(record.routes, key)
			}
		}
		if len(record.routes) == 0 {
			delete(m.peers, id)
		}
	}
}

func (m *Manager) shouldRespond(id string, source net.IP) bool {
	now := time.Now()
	key := id + "|" + source.String()
	m.mu.Lock()
	defer m.mu.Unlock()
	if last := m.lastResp[key]; !last.IsZero() && now.Sub(last) < time.Second {
		return false
	}
	m.lastResp[key] = now
	return true
}

func (m *Manager) allowedPeer(id string, key ed25519.PublicKey) bool {
	peer, ok := m.Device(id)
	return ok && bytes.Equal(peer.PublicKey, key)
}

func (m *Manager) peerFromAddress(id string, address net.IP) (Device, Route, bool) {
	peer, ok := m.Device(id)
	if !ok {
		return Device{}, Route{}, false
	}
	for _, route := range peer.Routes {
		if net.ParseIP(route.RemoteAddress).Equal(address) {
			return peer, route, true
		}
	}
	return Device{}, Route{}, false
}

func listed(name string, values []string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), name) {
			return true
		}
	}
	return false
}

func subnetBroadcast(ip net.IP, mask net.IPMask) net.IP {
	ip = ip.To4()
	if ip == nil || len(mask) != net.IPv4len {
		return nil
	}
	out := make(net.IP, net.IPv4len)
	for index := range out {
		out[index] = ip[index] | ^mask[index]
	}
	return out
}

type readResult struct {
	wire signaling.Wire
	err  error
}

// Session is a TLS 1.3 LAN control channel with one background read owner.
// Its interface mirrors the bounded WSS session used by application signaling.
type Session struct {
	conn      net.Conn
	peer      Device
	identity  *identity.Identity
	reader    *bufio.Reader
	readCh    chan readResult
	closed    chan struct{}
	writeMu   sync.Mutex
	closeOnce sync.Once
	sent      atomic.Uint64
	received  atomic.Uint64
}

func newSession(conn net.Conn, peer Device, localIdentity *identity.Identity) *Session {
	s := &Session{conn: conn, peer: peer, identity: localIdentity, reader: bufio.NewReaderSize(conn, maxFrameBytes+4), readCh: make(chan readResult, 8), closed: make(chan struct{})}
	go s.readPump()
	return s
}

func (s *Session) Peer() Device { return s.peer }

func (s *Session) RemoteAddress() string {
	if address, ok := s.conn.RemoteAddr().(*net.TCPAddr); ok {
		return address.IP.String()
	}
	return ""
}

func (s *Session) SendEnvelope(ctx context.Context, envelope protocol.Envelope) error {
	if s.identity == nil || envelope.Sender != s.identity.ID() || len(envelope.Signature) != 0 {
		return protocol.Fail(protocol.AuthenticationFailed, "outbound LAN envelope sender or signature invalid")
	}
	envelope.Signature = s.identity.Sign(envelope.SigningBytes())
	if err := envelope.Verify(s.identity.PublicKey(), time.Now()); err != nil {
		return err
	}
	return s.write(ctx, signaling.Wire{Type: "signal", Message: &envelope})
}

func (s *Session) SendHeartbeat(ctx context.Context) error {
	return s.write(ctx, signaling.Wire{Type: "heartbeat"})
}

func (s *Session) Read(ctx context.Context) (signaling.Wire, error) {
	select {
	case <-ctx.Done():
		return signaling.Wire{}, ctx.Err()
	case result, ok := <-s.readCh:
		if !ok {
			return signaling.Wire{}, io.EOF
		}
		return result.wire, result.err
	}
}

func (s *Session) Stats() signaling.SessionStats {
	return signaling.SessionStats{BytesSent: s.sent.Load(), BytesReceived: s.received.Load()}
}

func (s *Session) Close() error {
	var err error
	s.closeOnce.Do(func() {
		close(s.closed)
		err = s.conn.Close()
	})
	return err
}

func (s *Session) write(ctx context.Context, wire signaling.Wire) error {
	body, err := json.Marshal(wire)
	if err != nil || len(body) > maxFrameBytes {
		return protocol.Fail(protocol.InvalidMessage, "LAN signaling frame exceeds limit")
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	deadline := time.Now().Add(5 * time.Second)
	if value, ok := ctx.Deadline(); ok && value.Before(deadline) {
		deadline = value
	}
	if err = s.conn.SetWriteDeadline(deadline); err != nil {
		return err
	}
	var header [4]byte
	binary.BigEndian.PutUint32(header[:], uint32(len(body)))
	buffers := net.Buffers{header[:], body}
	n, err := buffers.WriteTo(s.conn)
	s.sent.Add(uint64(max(n, 0)))
	_ = s.conn.SetWriteDeadline(time.Time{})
	return err
}

func (s *Session) readPump() {
	defer close(s.readCh)
	for {
		var header [4]byte
		if _, err := io.ReadFull(s.reader, header[:]); err != nil {
			s.deliver(readResult{err: err})
			return
		}
		size := binary.BigEndian.Uint32(header[:])
		if size == 0 || size > maxFrameBytes {
			s.deliver(readResult{err: protocol.Fail(protocol.InvalidMessage, "invalid LAN signaling frame length")})
			_ = s.Close()
			return
		}
		body := make([]byte, int(size))
		if _, err := io.ReadFull(s.reader, body); err != nil {
			s.deliver(readResult{err: err})
			return
		}
		s.received.Add(uint64(len(header) + len(body)))
		var wire signaling.Wire
		if err := json.Unmarshal(body, &wire); err != nil || wire.Type != "signal" && wire.Type != "heartbeat" {
			s.deliver(readResult{err: protocol.Fail(protocol.InvalidMessage, "invalid LAN signaling frame")})
			_ = s.Close()
			return
		}
		if !s.deliver(readResult{wire: wire}) {
			return
		}
	}
}

func (s *Session) deliver(result readResult) bool {
	select {
	case s.readCh <- result:
		return true
	case <-s.closed:
		return false
	}
}
