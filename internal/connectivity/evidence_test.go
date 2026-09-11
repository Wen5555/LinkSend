package connectivity

import (
	"strings"
	"testing"

	"github.com/pion/ice/v4"
)

func TestCandidateEvidenceOmitsExtensions(t *testing.T) {
	for _, address := range []string{"192.0.2.1", "2001:db8::1"} {
		candidate, err := ice.UnmarshalCandidate("1 1 udp 2130706431 " + address + " 1234 typ host ufrag secretfragment password secretpassword x-private secretextension")
		if err != nil {
			t.Fatal(err)
		}
		got := candidateEvidence(candidate)
		if strings.Contains(got, "secret") || strings.Contains(got, "ufrag") || strings.Contains(got, "password") {
			t.Fatal("candidate evidence exposed extensions")
		}
		if !strings.Contains(got, address) || !strings.Contains(got, "1234") || !strings.Contains(got, "host") {
			t.Fatal("candidate evidence lost path fields")
		}
	}
}
