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
	Interface string `json:"interface"`
	Address   string `json:"address"`
	Family    string `json:"family"`
	Index     int    `json:"index"`
	Loopback  bool   `json:"loopback"`
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
			result = append(result, InterfaceAddress{Interface: iface.Name, Address: ip.String(), Family: family, Index: iface.Index, Loopback: ip.IsLoopback()})
		}
	}
	return result, nil
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
