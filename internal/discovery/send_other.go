//go:build !windows

package discovery

import (
	"net"

	"golang.org/x/net/ipv4"
)

func (m *Manager) writeOnRoute(body []byte, destination *net.UDPAddr, route interfaceRoute) error {
	_, err := m.packet.WriteTo(body, &ipv4.ControlMessage{IfIndex: route.iface.Index, Src: route.address}, destination)
	return err
}
