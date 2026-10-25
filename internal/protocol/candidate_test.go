package protocol

import (
	"fmt"
	"net/netip"
	"strings"
	"testing"
)

func candidateAt(address, kind string) string {
	return fmt.Sprintf("1 1 udp 2130706431 %s 3478 typ %s", address, kind)
}

func TestCandidateIPv4SuffixIsNotBroadcastEvidence(t *testing.T) {
	// Neither the sender's interface prefix nor the public mapping's prefix
	// travels in an ICE candidate. In particular these are ordinary host
	// addresses in the indicated networks despite their final octet.
	for _, fixture := range []struct {
		address, network, broadcast string
	}{
		{"10.0.0.255", "10.0.0.0/23", "10.0.1.255"},
		{"172.16.0.255", "172.16.0.0/16", "172.16.255.255"},
		{"192.168.0.255", "192.168.0.0/23", "192.168.1.255"},
		{"203.0.112.255", "203.0.112.0/23", "203.0.113.255"},
	} {
		if address := netip.MustParseAddr(fixture.address); !netip.MustParsePrefix(fixture.network).Contains(address) || fixture.address == fixture.broadcast {
			t.Fatalf("invalid unicast fixture: %+v", fixture)
		}
		for _, kind := range []string{"host", "srflx", "prflx"} {
			t.Run(fixture.address+"/"+kind, func(t *testing.T) {
				if err := ValidateCandidate(candidateAt(fixture.address, kind), false); err != nil {
					t.Fatalf("legal unicast candidate rejected based on last octet: %v", err)
				}
			})
		}
	}
}

func TestCandidateUnsafeAddressesRemainRejected(t *testing.T) {
	for _, address := range []string{
		"0.0.0.0", "0.1.2.3", "224.0.0.1", "239.255.255.250",
		"240.0.0.1", "254.1.2.3", "255.255.255.255", "169.254.1.2",
		"::", "ff02::1", "fe80::1", "fe80::1%test",
		"::ffff:0.0.0.0", "::ffff:0.1.2.3", "::ffff:169.254.1.2", "::ffff:224.0.0.1", "::ffff:240.0.0.1", "::ffff:255.255.255.255", "::ffff:10.0.0.1%test",
	} {
		t.Run(address, func(t *testing.T) {
			if err := ValidateCandidate(candidateAt(address, "host"), false); err == nil {
				t.Fatal("unsafe candidate admitted")
			}
		})
	}
	for _, address := range []string{"127.0.0.1", "::1", "::ffff:127.0.0.1"} {
		t.Run(address+"/loopback", func(t *testing.T) {
			if err := ValidateCandidate(candidateAt(address, "host"), false); err == nil {
				t.Fatal("loopback admitted without explicit test configuration")
			}
			if err := ValidateCandidate(candidateAt(address, "host"), true); err != nil {
				t.Fatalf("explicit loopback test configuration rejected: %v", err)
			}
		})
	}
}

func TestCandidateDirectTransportAndBoundsRemainEnforced(t *testing.T) {
	for _, candidate := range []string{
		candidateAt("10.0.0.255", "host") + strings.Repeat(" ", 2049),
		"1 1 tcp 2130706431 10.0.0.255 3478 typ host",
		"1 1 udp 2130706431 10.0.0.255 3478 typ relay",
		"1 1 udp 2130706431 10.0.0.255 3478 typ unsupported",
		"1 1 udp 2130706431 10.0.0.255 0 typ host",
		"1 1 udp 2130706431 10.0.0.255 65536 typ host",
		"1 1 udp 2130706431 host.example 3478 typ host",
	} {
		if err := ValidateCandidate(candidate, false); err == nil {
			t.Fatalf("invalid candidate admitted: %q", candidate)
		}
	}
	for _, address := range []string{"10.0.0.255", "::ffff:10.0.0.255", "2001:db8::1"} {
		if err := ValidateCandidate(candidateAt(address, "host"), false); err != nil {
			t.Fatalf("safe unicast candidate rejected: %v", err)
		}
	}
}
