package connectivity

import (
	"context"
	"errors"
	"net"
	"os"
	"testing"
	"time"

	"github.com/pion/stun/v4"
)

func localEndpoint(t *testing.T) *Endpoint {
	t.Helper()
	e, err := New(Config{BindAddress: "127.0.0.1:0", AllowLoopback: true, Generation: 1})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = e.Close() })
	return e
}

func TestCandidatePolicy(t *testing.T) {
	for _, addr := range []string{"0.0.0.0:0", "[::]:0", "255.255.255.255:0", "[fe80::1]:0", "127.0.0.1:0"} {
		if e, err := New(Config{BindAddress: addr}); err == nil {
			_ = e.Close()
			t.Fatalf("accepted unsafe bind %q", addr)
		}
	}
	e := localEndpoint(t)
	for _, value := range []string{
		"1 1 udp 2130706431 224.0.0.1 1234 typ host",
		"1 1 udp 2130706431 255.255.255.255 1234 typ host",
		"1 1 udp 2130706431 0.0.0.0 1234 typ host",
		"1 1 udp 2130706431 fe80::1 1234 typ host",
		"1 1 tcp 2130706431 127.0.0.1 1234 typ host tcptype passive",
		"1 1 udp 2130706431 127.0.0.1 1234 typ relay raddr 127.0.0.1 rport 1234",
	} {
		if err := e.AddRemoteCandidate(Candidate{value, 1}); err == nil {
			t.Fatalf("accepted unsafe candidate: %s", value)
		}
	}
	if err := e.AddRemoteCandidate(Candidate{"1 1 udp 2130706431 127.0.0.1 1234 typ host", 2}); err == nil {
		t.Fatal("accepted stale generation")
	}
}

func TestICETimeoutAndClose(t *testing.T) {
	e := localEndpoint(t)
	if err := e.Gather(); err != nil {
		t.Fatal(err)
	}
	for range e.Candidates() {
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Millisecond)
	defer cancel()
	started := time.Now()
	if _, err := e.Connect(ctx, Credentials{"remote123", "01234567890123456789012345678901"}, true); err == nil {
		t.Fatal("connected with no remote")
	}
	if time.Since(started) > 2*time.Second {
		t.Fatal("timeout did not cancel checks")
	}
	if err := e.Close(); err != nil {
		t.Fatal(err)
	}
	if err := e.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestAdapterDatagramsDeadlinesAndClose(t *testing.T) {
	e := localEndpoint(t)
	// This isolated adapter test owns no ICE reads; create a second endpoint's
	// quic transport solely for its adapter and terminate its mux first is unsafe.
	// Instead exercise read deadlines against a stand-alone adapter below.
	u, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer u.Close()
	msg, err := stun.Build(stun.TransactionID, stun.BindingRequest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.packets.WriteTo([]byte("file bytes forbidden"), u.LocalAddr()); err == nil {
		t.Fatal("accepted non-STUN")
	}
	if _, err = e.packets.WriteTo(msg.Raw, u.LocalAddr()); err != nil {
		t.Fatal(err)
	}
	_ = u.SetReadDeadline(time.Now().Add(time.Second))
	buf := make([]byte, 4096)
	n, from, err := u.ReadFromUDP(buf)
	if err != nil {
		t.Fatal(err)
	}
	if from.String() != e.BaseAddress() || n != len(msg.Raw) {
		t.Fatalf("datagram/source changed: %d %v", n, from)
	}
	_ = e.packets.SetWriteDeadline(time.Now().Add(-time.Second))
	if _, err = e.packets.WriteTo(msg.Raw, u.LocalAddr()); !errors.Is(err, os.ErrDeadlineExceeded) {
		t.Fatalf("deadline: %v", err)
	}
	_ = e.packets.SetWriteDeadline(time.Time{})
	_ = e.packets.Close()
	if _, _, err = e.packets.ReadFrom(buf); !errors.Is(err, net.ErrClosed) {
		t.Fatalf("close did not release read: %v", err)
	}
}

func FuzzCandidateParser(f *testing.F) {
	f.Add("1 1 udp 2130706431 127.0.0.1 1234 typ host")
	f.Add("1 1 udp 2130706431 224.0.0.1 1234 typ host")
	f.Fuzz(func(t *testing.T, s string) {
		if len(s) > 2048 {
			return
		}
		_ = validSTUN([]byte(s))
	})
}
