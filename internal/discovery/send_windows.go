package discovery

import (
	"encoding/json"
	"errors"
	"net"
	"time"
)

// x/net/ipv4 ignores ControlMessage source selection on Windows. A bounded
// per-source socket both emits the selected local address and receives the
// unicast response sent back to its ephemeral source port.
func (m *Manager) writeOnRoute(body []byte, destination *net.UDPAddr, route interfaceRoute) error {
	select {
	case m.responseSlots <- struct{}{}:
	default:
		return errors.New("LAN discovery source socket limit reached")
	}
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: route.address, Port: 0})
	if err != nil {
		<-m.responseSlots
		return err
	}
	if _, err = conn.WriteToUDP(body, destination); err != nil {
		_ = conn.Close()
		<-m.responseSlots
		return err
	}
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		defer func() { <-m.responseSlots }()
		defer conn.Close()
		buffer := make([]byte, maxPacketBytes+1)
		_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		for {
			n, source, readErr := conn.ReadFromUDP(buffer)
			if readErr != nil {
				return
			}
			if n == 0 || n > maxPacketBytes || source.IP == nil || source.IP.To4() == nil {
				continue
			}
			var received packet
			if json.Unmarshal(buffer[:n], &received) != nil || !m.validatePacket(received, source.IP) || received.DeviceID == m.cfg.Identity.ID() {
				continue
			}
			seen := Route{RemoteAddress: source.IP.String(), ControlPort: received.Port, Interface: route.iface.Name, LocalAddress: route.address.String(), LastSeen: time.Now(), Family: "ipv4", Generation: m.networkGeneration()}
			m.remember(received, seen)
		}
	}()
	return nil
}
