package discovery

import (
	"errors"
	"fmt"
	"net"

	"golang.org/x/net/ipv4"
)

func listenDiscoveryUDP(port int) (net.PacketConn, bool, error) {
	udp, fixedErr := net.ListenPacket("udp4", net.JoinHostPort(net.IPv4zero.String(), fmt.Sprint(port)))
	if fixedErr == nil {
		return udp, false, nil
	}
	// Replies target the actual source port. Preserve outgoing discovery and
	// authenticated TCP control even when the well-known receive port is busy.
	udp, err := net.ListenPacket("udp4", "0.0.0.0:0")
	if err != nil {
		return nil, false, errors.Join(fixedErr, err)
	}
	return udp, true, nil
}

func configureDiscoveryUDP(udp net.PacketConn) (*ipv4.PacketConn, map[string]string) {
	packet := ipv4.NewPacketConn(udp)
	status := map[string]string{}
	// Windows falls back to subnet matching where receive interface metadata
	// is unavailable. Its writes continue to use bound per-source sockets.
	_ = packet.SetControlMessage(ipv4.FlagInterface, true)
	if err := packet.SetMulticastTTL(1); err != nil {
		status["multicast"] = "ttl_configuration_failed"
	}
	_ = packet.SetMulticastLoopback(false)
	if err := enableBroadcast(udp); err != nil {
		status["broadcast"] = "socket_configuration_failed"
	}
	return packet, status
}

// Caller holds writeMu. Socket replacement never acquires writeMu while
// holding refreshMu or mu; refresh retains the writeMu -> refreshMu -> mu order.
func (m *Manager) installUDPSocketLocked(udp net.PacketConn, temporary bool) {
	packet, status := configureDiscoveryUDP(udp)
	m.mu.Lock()
	previous := m.udp
	m.udp, m.packet = udp, packet
	m.joined = map[int]*net.Interface{}
	m.joinSignatures = map[int]string{}
	delete(m.channelErrors, "receive")
	delete(m.channelErrors, "multicast")
	delete(m.channelErrors, "broadcast")
	for provider, state := range status {
		m.channelErrors[provider] = state
	}
	if temporary {
		m.channelErrors["listen"] = "fixed_port_unavailable"
	} else {
		delete(m.channelErrors, "listen")
	}
	m.mu.Unlock()
	if previous != nil {
		_ = previous.Close() // Wake the sole readLoop so it follows the replacement.
	}
}

// restoreFixedUDPSocket never sacrifices a healthy temporary socket to retry
// the fixed port. Refresh and the existing announcement ticker drive retries.
func (m *Manager) restoreFixedUDPSocket() error {
	m.writeMu.Lock()
	defer m.writeMu.Unlock()
	if err := m.ctx.Err(); err != nil {
		return err
	}
	if m.udp != nil && m.udp.LocalAddr().(*net.UDPAddr).Port == m.cfg.Port {
		return nil
	}
	udp, err := net.ListenPacket("udp4", net.JoinHostPort(net.IPv4zero.String(), fmt.Sprint(m.cfg.Port)))
	if err != nil {
		return err
	}
	if err := m.ctx.Err(); err != nil {
		_ = udp.Close()
		return err
	}
	m.installUDPSocketLocked(udp, false)
	return nil
}

func (m *Manager) rebuildUDPSocket(failed *ipv4.PacketConn) error {
	m.writeMu.Lock()
	defer m.writeMu.Unlock()
	if err := m.ctx.Err(); err != nil {
		return err
	}
	// An old read can fail just as Refresh installs a healthy replacement.
	if failed != nil && m.packet != failed {
		return nil
	}
	if m.udp != nil {
		_ = m.udp.Close()
	}
	udp, temporary, err := listenDiscoveryUDP(m.cfg.Port)
	if err != nil {
		return err
	}
	if err := m.ctx.Err(); err != nil {
		_ = udp.Close()
		return err
	}
	m.installUDPSocketLocked(udp, temporary)
	return m.refreshInterfaces()
}
