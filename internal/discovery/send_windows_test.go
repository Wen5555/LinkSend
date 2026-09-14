//go:build windows

package discovery

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/Wen5555/LinkSend/internal/identity"
)

func TestWindowsBroadcastUsesBoundSourceWithoutMulticastFlag(t *testing.T) {
	id, err := identity.Generate()
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	route := interfaceRoute{iface: &net.Interface{Index: 1, Name: "source-test", Flags: net.FlagUp}, address: net.ParseIP("127.0.0.2").To4(), network: &net.IPNet{IP: net.ParseIP("127.0.0.0").To4(), Mask: net.CIDRMask(8, 32)}, broadcast: net.ParseIP("127.0.0.1").To4()}
	m := &Manager{cfg: Config{Identity: id, Name: "test", Port: listener.LocalAddr().(*net.UDPAddr).Port, AllowLoopback: true}, ctx: ctx, ifaces: map[string]interfaceRoute{"route": route}, replays: map[string]time.Time{}, peers: map[string]*peerRecord{}, responseSlots: make(chan struct{}, 2)}
	if err = m.sendAnnouncement(true, nil); err != nil {
		t.Fatal(err)
	}
	_ = listener.SetReadDeadline(time.Now().Add(time.Second))
	buffer := make([]byte, maxPacketBytes)
	_, source, err := listener.ReadFromUDP(buffer)
	if err != nil {
		t.Fatal(err)
	}
	if got := source.IP.String(); got != "127.0.0.2" {
		t.Fatalf("source address=%s want 127.0.0.2", got)
	}
}
