package connectivity

import (
	"net"
	"testing"
	"time"
)

func TestInterfaceSelectionHonoursExplicitPriorityAndExclusion(t *testing.T) {
	addresses := []InterfaceAddress{
		{Interface: "Virtual", Address: "10.0.0.2", Family: "ipv4", Index: 1},
		{Interface: "Ethernet", Address: "192.168.10.2", Family: "ipv4", Index: 2},
		{Interface: "Wi-Fi", Address: "192.168.20.2", Family: "ipv4", Index: 3},
		{Interface: "Ethernet", Address: "2001:db8::2", Family: "ipv6", Index: 2},
	}
	selected, err := selectInterfaceAddress(addresses, []string{"Wi-Fi", "Ethernet"}, []string{"wi-fi"})
	if err != nil {
		t.Fatal(err)
	}
	if selected.Interface != "Ethernet" || selected.Family != "ipv4" {
		t.Fatalf("unexpected selection: %+v", selected)
	}
	if _, err = selectInterfaceAddress(addresses, nil, []string{"virtual", "ethernet", "wi-fi"}); err == nil {
		t.Fatal("all explicitly excluded interfaces still produced a candidate")
	}
}

func TestInterfaceSelectionPrefersOrdinaryLinkMTUWithoutNameGuessing(t *testing.T) {
	addresses := []InterfaceAddress{
		{Interface: "Tunnel", Address: "198.18.0.1", Family: "ipv4", Index: 1, MTU: 65535},
		{Interface: "Physical", Address: "10.20.30.40", Family: "ipv4", Index: 2, MTU: 1500},
	}
	selected, err := selectInterfaceAddress(addresses, nil, nil)
	if err != nil || selected.Interface != "Physical" {
		t.Fatalf("ordinary link was not preferred: %+v %v", selected, err)
	}
	selected, err = selectInterfaceAddress(addresses, []string{"Tunnel"}, nil)
	if err != nil || selected.Interface != "Tunnel" {
		t.Fatalf("explicit tunnel priority was ignored: %+v %v", selected, err)
	}
}

func TestInterfaceSelectionPrefersNonPointToPointAtEqualMTU(t *testing.T) {
	addresses := []InterfaceAddress{
		{Interface: "Tunnel", Address: "198.18.0.1", Family: "ipv4", Index: 1, MTU: 1500, PointToPoint: true},
		{Interface: "Physical", Address: "10.20.30.40", Family: "ipv4", Index: 2, MTU: 1500},
	}
	selected, err := selectInterfaceAddress(addresses, nil, nil)
	if err != nil || selected.Interface != "Physical" {
		t.Fatalf("non-point-to-point link was not preferred: %+v %v", selected, err)
	}
	selected, err = selectInterfaceAddress(addresses, []string{"Tunnel"}, nil)
	if err != nil || selected.Interface != "Tunnel" {
		t.Fatalf("explicit point-to-point priority was ignored: %+v %v", selected, err)
	}
}

func TestAddressChangeInvalidatesSelectedEndpoint(t *testing.T) {
	e := &Endpoint{
		done:            make(chan struct{}),
		pathChanged:     make(chan struct{}),
		interfaceName:   "test-interface",
		boundIP:         net.ParseIP("192.0.2.10"),
		presenceProbe:   func(string, net.IP) bool { return false },
		monitorInterval: time.Millisecond,
	}
	e.monitorWG.Add(1)
	go e.monitorLocalAddress()
	select {
	case <-e.pathChanged:
	case <-time.After(time.Second):
		t.Fatal("address removal did not invalidate the endpoint")
	}
	close(e.done)
	e.monitorWG.Wait()
}

func TestInterfaceDiscoveryExcludesUnsupportedAddresses(t *testing.T) {
	addresses, err := DiscoverInterfaceAddresses(true)
	if err != nil {
		t.Fatal(err)
	}
	for _, address := range addresses {
		ip := net.ParseIP(address.Address)
		if ip == nil || ip.IsUnspecified() || ip.IsMulticast() || ip.IsLinkLocalUnicast() {
			t.Fatalf("unsafe discovered address: %+v", address)
		}
		if address.Family != "ipv4" && address.Family != "ipv6" {
			t.Fatalf("unknown address family: %+v", address)
		}
	}
}

func TestLocalAddressPresenceRequiresExactInterfaceAndAddress(t *testing.T) {
	addresses, err := DiscoverInterfaceAddresses(true)
	if err != nil {
		t.Fatal(err)
	}
	if len(addresses) == 0 {
		t.Skip("host has no eligible local address")
	}
	selected := addresses[0]
	if !localAddressPresent(selected.Interface, net.ParseIP(selected.Address)) {
		t.Fatalf("discovered address was not present: %+v", selected)
	}
	if localAddressPresent("linksend-interface-that-does-not-exist", net.ParseIP(selected.Address)) {
		t.Fatal("missing interface reported as present")
	}
}
