package connectivity

import (
	"errors"
	"net"
	"sort"
	"strings"
)

// InterfaceAddress is an eligible local unicast address. Interface names are
// labels for explicit user policy only; LinkSend never infers network nature
// from a name, private prefix, or candidate type.
type InterfaceAddress struct {
	Interface    string `json:"interface"`
	Address      string `json:"address"`
	Family       string `json:"family"`
	Index        int    `json:"index"`
	MTU          int    `json:"mtu"`
	Loopback     bool   `json:"loopback"`
	PointToPoint bool   `json:"point_to_point"`
}

func DiscoverInterfaceAddresses(allowLoopback bool) ([]InterfaceAddress, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	result := make([]InterfaceAddress, 0, len(interfaces))
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 {
			continue
		}
		addresses, addrErr := iface.Addrs()
		if addrErr != nil {
			continue
		}
		for _, address := range addresses {
			ip, _, parseErr := net.ParseCIDR(address.String())
			if parseErr != nil || !safeIP(ip, allowLoopback) {
				continue
			}
			family := "ipv6"
			if ip.To4() != nil {
				family = "ipv4"
				ip = ip.To4()
			}
			result = append(result, InterfaceAddress{Interface: iface.Name, Address: ip.String(), Family: family, Index: iface.Index, MTU: iface.MTU, Loopback: ip.IsLoopback(), PointToPoint: iface.Flags&net.FlagPointToPoint != 0})
		}
	}
	return result, nil
}

// ResolveInterfaceAddress applies the same deterministic safety policy used by
// Endpoint automatic binding. Callers may cache the result only while
// InterfaceAddressPresent continues to confirm the exact interface and IP.
func ResolveInterfaceAddress(allowLoopback bool, priority, excluded []string) (InterfaceAddress, error) {
	addresses, err := DiscoverInterfaceAddresses(allowLoopback)
	if err != nil {
		return InterfaceAddress{}, err
	}
	return selectInterfaceAddress(addresses, priority, excluded)
}

func InterfaceAddressPresent(interfaceName, address string) bool {
	return localAddressPresent(interfaceName, net.ParseIP(address))
}

func interfaceListed(name string, values []string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), name) {
			return true
		}
	}
	return false
}

func selectInterfaceAddress(addresses []InterfaceAddress, priority, excluded []string) (InterfaceAddress, error) {
	eligible := make([]InterfaceAddress, 0, len(addresses))
	for _, address := range addresses {
		if !interfaceListed(address.Interface, excluded) {
			eligible = append(eligible, address)
		}
	}
	if len(eligible) == 0 {
		return InterfaceAddress{}, errors.New("E_NO_CANDIDATE: no eligible local interface address")
	}
	priorityRank := func(name string) int {
		for index, preferred := range priority {
			if strings.EqualFold(strings.TrimSpace(preferred), name) {
				return index
			}
		}
		return len(priority)
	}
	sort.SliceStable(eligible, func(i, j int) bool {
		left, right := eligible[i], eligible[j]
		if a, b := priorityRank(left.Interface), priorityRank(right.Interface); a != b {
			return a < b
		}
		if left.Loopback != right.Loopback {
			return !left.Loopback
		}
		// Abnormally large MTUs are common on capture/TUN adapters. Prefer a
		// normal physical-link range for automatic selection, while explicit
		// user priority above still permits any adapter.
		if a, b := ordinaryLinkMTU(left.MTU), ordinaryLinkMTU(right.MTU); a != b {
			return a
		}
		if left.PointToPoint != right.PointToPoint {
			return !left.PointToPoint
		}
		if left.Family != right.Family {
			return left.Family == "ipv4"
		}
		if left.Index != right.Index {
			return left.Index < right.Index
		}
		return left.Address < right.Address
	})
	return eligible[0], nil
}

func ordinaryLinkMTU(mtu int) bool { return mtu >= 1280 && mtu <= 9000 }

func interfaceForIP(ip net.IP) string {
	interfaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	for _, iface := range interfaces {
		addresses, addrErr := iface.Addrs()
		if addrErr != nil {
			continue
		}
		for _, address := range addresses {
			candidate, _, parseErr := net.ParseCIDR(address.String())
			if parseErr == nil && candidate.Equal(ip) {
				return iface.Name
			}
		}
	}
	return ""
}

func localAddressPresent(interfaceName string, ip net.IP) bool {
	if interfaceName == "" || ip == nil {
		return false
	}
	iface, err := net.InterfaceByName(interfaceName)
	if err != nil || iface.Flags&net.FlagUp == 0 {
		return false
	}
	addresses, err := iface.Addrs()
	if err != nil {
		return false
	}
	for _, address := range addresses {
		candidate, _, parseErr := net.ParseCIDR(address.String())
		if parseErr == nil && candidate.Equal(ip) {
			return true
		}
	}
	return false
}

func directlyConnected(interfaceName string, remote net.IP) bool {
	if interfaceName == "" || remote == nil || !safeIP(remote, false) {
		return false
	}
	iface, err := net.InterfaceByName(interfaceName)
	if err != nil || iface.Flags&net.FlagUp == 0 {
		return false
	}
	addresses, err := iface.Addrs()
	if err != nil {
		return false
	}
	for _, address := range addresses {
		local, network, parseErr := net.ParseCIDR(address.String())
		if parseErr == nil && !local.Equal(remote) && network.Contains(remote) {
			return true
		}
	}
	return false
}
