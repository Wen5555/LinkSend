package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"github.com/pion/mdns/v2"
	"golang.org/x/net/ipv6"
)

type result struct {
	Result     string   `json:"result"`
	Mode       string   `json:"mode"`
	Service    string   `json:"service"`
	Instance   string   `json:"instance,omitempty"`
	Address    string   `json:"address,omitempty"`
	Port       uint16   `json:"port,omitempty"`
	TXTKeys    []string `json:"txt_keys,omitempty"`
	TrustBasis string   `json:"trust_basis"`
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}

func newConn(ifaceName, address, host string) (*mdns.Conn, error) {
	iface, err := net.InterfaceByName(ifaceName)
	if err != nil {
		return nil, err
	}
	listen, err := net.ResolveUDPAddr("udp6", mdns.DefaultAddressIPv6)
	if err != nil {
		return nil, err
	}
	socket, err := net.ListenUDP("udp6", listen)
	if err != nil {
		return nil, err
	}
	localName := strings.TrimSuffix(host, ".")
	conn, err := mdns.NewServer(nil, ipv6.NewPacketConn(socket),
		mdns.WithInterfaces(*iface),
		mdns.WithLocalAddress(net.ParseIP(address)),
		mdns.WithLocalNames(localName),
		mdns.WithName("linksend-e5d-mdns"),
	)
	if err != nil {
		_ = socket.Close()
		return nil, err
	}
	return conn, nil
}

func main() {
	mode := flag.String("mode", "", "publish or browse")
	iface := flag.String("interface", "", "IPv6 multicast interface")
	address := flag.String("address", "", "ULA address to publish")
	service := flag.String("service", "_linksend._udp", "DNS-SD service type")
	instance := flag.String("instance", "LinkSend E5D Candidate", "DNS-SD instance")
	host := flag.String("host", "linksend-e5d.local.", "mDNS host name")
	port := flag.Uint("port", 41001, "candidate UDP port")
	timeout := flag.Duration("timeout", 10*time.Second, "bounded lifetime")
	flag.Parse()
	if *mode != "publish" && *mode != "browse" {
		fatal(fmt.Errorf("mode must be publish or browse"))
	}
	if *iface == "" || net.ParseIP(*address) == nil || *service == "" || *host == "" || *port == 0 || *port > 65535 {
		fatal(fmt.Errorf("valid interface, ULA address, service, host, and port are required"))
	}
	conn, err := newConn(*iface, *address, *host)
	if err != nil {
		fatal(err)
	}
	defer conn.Close()

	switch *mode {
	case "publish":
		err = conn.Register(mdns.ServiceInstance{
			Instance: *instance,
			Service:  *service,
			Domain:   "local",
			Host:     *host,
			Port:     uint16(*port),
			Text: []mdns.TXTEntry{
				mdns.NewTXTString("candidate", "untrusted"),
				mdns.NewTXTString("proto", "linksend-v1"),
			},
		})
		if err != nil {
			fatal(err)
		}
		fmt.Println("MDNS_PUBLISH_READY")
		time.Sleep(*timeout)
		_ = json.NewEncoder(os.Stdout).Encode(result{Result: "PASS", Mode: "publish", Service: *service, Instance: *instance, Address: *address, Port: uint16(*port), TrustBasis: "none"})
	case "browse":
		ctx, cancel := context.WithTimeout(context.Background(), *timeout)
		defer cancel()
		events := make(chan mdns.ServiceEvent, 1)
		conn.OnServiceDiscovered(func(event mdns.ServiceEvent) {
			select {
			case events <- event:
			default:
			}
		})
		if err = conn.Browse(ctx, *service); err != nil {
			fatal(err)
		}
		select {
		case event := <-events:
			if event.Instance.Instance != *instance || event.Addr.String() == "" || event.Instance.Port != uint16(*port) {
				fatal(fmt.Errorf("unexpected DNS-SD event"))
			}
			keys := make([]string, 0, len(event.Instance.Text))
			for _, entry := range event.Instance.Text {
				keys = append(keys, entry.Key)
			}
			if err = json.NewEncoder(os.Stdout).Encode(result{Result: "PASS", Mode: "browse", Service: *service, Instance: event.Instance.Instance, Address: event.Addr.String(), Port: event.Instance.Port, TXTKeys: keys, TrustBasis: "none"}); err != nil {
				fatal(err)
			}
		case <-ctx.Done():
			fatal(fmt.Errorf("DNS_SD_BROWSE_TIMEOUT"))
		}
	}
}
