package connectivity

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/pion/transport/v4"
	"github.com/pion/transport/v4/stdnet"
)

const stunResolveBudget = 2500 * time.Millisecond
const stunDNSMaxBytes = 16 << 10
const stunDNSCacheLimit = 4 // Two built-in public names, each with A/AAAA.

// Bound HTTPS fallback work across concurrent endpoint generations too.
var stunDNSQuerySlots = make(chan struct{}, 4)

var errSTUNDNS = errors.New("STUN hostname could not be safely resolved")

// This fallback is deliberately limited to built-in public infrastructure.
// User-selected private hostnames continue to use their system resolver.
func builtInSTUNHost(host string) bool {
	switch strings.TrimSuffix(strings.ToLower(host), ".") {
	case "stun.oooai.de", "stun.cloudflare.com":
		return true
	default:
		return false
	}
}

func fakeSTUNAddress(ip net.IP) bool {
	v4 := ip.To4()
	return v4 != nil && v4[0] == 198 && (v4[1] == 18 || v4[1] == 19)
}

// Tests can inject DNS and an HTTPS client without changing host networking.
// These hooks are package-private and cannot come from a peer or desktop IPC.
type stunResolverOptions struct {
	lookup    func(context.Context, string, string) ([]net.IPAddr, error)
	client    *http.Client
	endpoints []string
	budget    time.Duration
	now       func() time.Time
}

type stunDNSResult struct {
	ip      net.IP
	err     error
	expires time.Time
}

type stunResolverNet struct {
	transport.Net
	ctx           context.Context
	allowLoopback bool
	lookup        func(context.Context, string, string) ([]net.IPAddr, error)
	client        *http.Client
	endpoints     []string
	budget        time.Duration
	now           func() time.Time
	mu            sync.Mutex
	cache         map[string]stunDNSResult
	inflight      map[string]chan struct{}
}

var _ transport.Net = (*stunResolverNet)(nil)

func newSTUNResolverNet(ctx context.Context, loopback bool, options *stunResolverOptions) (*stunResolverNet, error) {
	base, err := stdnet.NewNet()
	if err != nil {
		return nil, err
	}
	n := &stunResolverNet{Net: base, ctx: ctx, allowLoopback: loopback, budget: stunResolveBudget,
		now:       time.Now,
		endpoints: []string{"https://cloudflare-dns.com/dns-query", "https://dns.alidns.com/resolve"},
		cache:     make(map[string]stunDNSResult), inflight: make(map[string]chan struct{})}
	// Use context-aware family selection without introducing another socket
	// owner. Tests inject the same bounded lookup boundary.
	n.lookup = func(ctx context.Context, network, host string) ([]net.IPAddr, error) {
		addresses, err := net.DefaultResolver.LookupNetIP(ctx, network, host)
		result := make([]net.IPAddr, 0, len(addresses))
		for _, ip := range addresses {
			result = append(result, net.IPAddr{IP: net.IP(ip.AsSlice()), Zone: ip.Zone()})
		}
		return result, err
	}
	client := http.Client{Transport: http.DefaultTransport}
	if options != nil {
		if options.lookup != nil {
			n.lookup = options.lookup
		}
		if options.client != nil {
			client = *options.client
		}
		if options.endpoints != nil {
			n.endpoints = append([]string(nil), options.endpoints...)
		}
		if options.budget > 0 && options.budget < n.budget {
			n.budget = options.budget
		}
		if options.now != nil {
			n.now = options.now
		}
	}
	if len(n.endpoints) == 0 || len(n.endpoints) > 2 {
		return nil, errSTUNDNS
	}
	for _, raw := range n.endpoints {
		u, err := url.Parse(raw)
		if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Fragment != "" {
			return nil, errSTUNDNS
		}
	}
	// Do not follow a resolver redirect to another trust boundary or plaintext.
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	if client.Timeout <= 0 || client.Timeout > n.budget {
		client.Timeout = n.budget
	}
	n.client = &client
	return n, nil
}

func compatibleSTUNFamily(network string, ip net.IP) bool {
	return ip != nil && (network == "udp" || network == "udp4" && ip.To4() != nil || network == "udp6" && ip.To4() == nil)
}

// Pion calls this from its srflx-gathering goroutine, in parallel with host
// candidates. The original transport still performs all STUN/ICE socket I/O.
func (n *stunResolverNet) ResolveUDPAddr(network, address string) (*net.UDPAddr, error) {
	host, portString, err := net.SplitHostPort(address)
	if err != nil || network != "udp" && network != "udp4" && network != "udp6" {
		return n.Net.ResolveUDPAddr(network, address)
	}
	if _, err := netip.ParseAddr(host); err == nil || !builtInSTUNHost(host) {
		return n.Net.ResolveUDPAddr(network, address)
	}
	port, err := strconv.Atoi(portString)
	if err != nil || port < 1 || port > 65535 {
		return nil, errSTUNDNS
	}
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	ctx, cancel := context.WithTimeout(n.ctx, n.budget)
	defer cancel()
	family := "ip4"
	if network == "udp6" {
		family = "ip6"
	}
	// Reserve time for HTTPS fallback if the system resolver does not reply.
	lookupCtx, lookupCancel := context.WithTimeout(ctx, min(750*time.Millisecond, n.budget/3))
	addresses, lookupErr := n.lookup(lookupCtx, family, host)
	lookupCancel()
	if lookupErr == nil {
		for _, address := range addresses {
			if address.Zone == "" && compatibleSTUNFamily(network, address.IP) && safeIP(address.IP, n.allowLoopback) && !fakeSTUNAddress(address.IP) {
				return &net.UDPAddr{IP: append(net.IP(nil), address.IP...), Port: port}, nil
			}
		}
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	ip, err := n.fallback(ctx, family, host)
	if err != nil {
		return nil, err
	}
	return &net.UDPAddr{IP: ip, Port: port}, nil
}

func (n *stunResolverNet) fallback(ctx context.Context, family, host string) (net.IP, error) {
	key := family + ":" + host
	for {
		n.mu.Lock()
		if cached, ok := n.cache[key]; ok && n.now().Before(cached.expires) {
			n.mu.Unlock()
			return append(net.IP(nil), cached.ip...), cached.err
		}
		if done := n.inflight[key]; done != nil {
			n.mu.Unlock()
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-done:
				continue
			}
		}
		if len(n.inflight) >= stunDNSCacheLimit {
			n.mu.Unlock()
			return nil, errSTUNDNS
		}
		done := make(chan struct{})
		n.inflight[key] = done
		n.mu.Unlock()
		ip, ttl, err := n.queryFallback(ctx, family, host)
		if err != nil {
			ttl = 2 * time.Second
		}
		n.mu.Lock()
		_, known := n.cache[key]
		if len(n.cache) < stunDNSCacheLimit || known {
			n.cache[key] = stunDNSResult{ip: append(net.IP(nil), ip...), err: err, expires: n.now().Add(ttl)}
		}
		delete(n.inflight, key)
		close(done)
		n.mu.Unlock()
		return append(net.IP(nil), ip...), err
	}
}

func (n *stunResolverNet) queryFallback(ctx context.Context, family, host string) (net.IP, time.Duration, error) {
	for index, endpoint := range n.endpoints {
		if err := ctx.Err(); err != nil {
			return nil, 0, err
		}
		// A stalled first provider must leave an opportunity for the other one.
		remaining := n.budget
		if deadline, ok := ctx.Deadline(); ok {
			remaining = time.Until(deadline)
		}
		queryCtx, cancel := context.WithTimeout(ctx, remaining/time.Duration(len(n.endpoints)-index))
		ip, ttl, err := n.query(queryCtx, endpoint, family, host)
		cancel()
		if err == nil {
			return ip, ttl, nil
		}
	}
	return nil, 0, errSTUNDNS
}

func canonicalDNSName(name string) string {
	name = strings.TrimSuffix(strings.ToLower(name), ".")
	if name == "" || len(name) > 253 {
		return ""
	}
	for _, label := range strings.Split(name, ".") {
		if label == "" || len(label) > 63 {
			return ""
		}
		for _, c := range label {
			if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {
				return ""
			}
		}
	}
	return name
}

func (n *stunResolverNet) publicAnswer(ip net.IP) bool {
	if ip == nil || fakeSTUNAddress(ip) {
		return false
	}
	if ip.IsLoopback() {
		return n.allowLoopback
	}
	if !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLinkLocalUnicast() {
		return false
	}
	v4 := ip.To4()
	if v4 != nil {
		if v4[0] == 0 || v4[0] >= 224 || v4[0] == 100 && v4[1]&0xc0 == 64 {
			return false
		}
		if v4[0] == 192 && v4[1] == 0 && (v4[2] == 0 || v4[2] == 2) || v4[0] == 198 && v4[1] == 51 && v4[2] == 100 || v4[0] == 203 && v4[1] == 0 && v4[2] == 113 {
			return false
		}
	}
	return true
}

func (n *stunResolverNet) query(ctx context.Context, endpoint, family, host string) (net.IP, time.Duration, error) {
	select {
	case stunDNSQuerySlots <- struct{}{}:
		defer func() { <-stunDNSQuerySlots }()
	case <-ctx.Done():
		return nil, 0, ctx.Err()
	}
	u, _ := url.Parse(endpoint)
	typeNumber, typeName := 1, "A"
	if family == "ip6" {
		typeNumber, typeName = 28, "AAAA"
	}
	q := u.Query()
	q.Set("name", host)
	q.Set("type", typeName)
	u.RawQuery = q.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, 0, errSTUNDNS
	}
	request.Header.Set("Accept", "application/dns-json")
	response, err := n.client.Do(request)
	if err != nil {
		return nil, 0, err
	}
	defer response.Body.Close()
	media, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if response.StatusCode != http.StatusOK || err != nil || media != "application/dns-json" && media != "application/json" {
		return nil, 0, errSTUNDNS
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, stunDNSMaxBytes+1))
	if err != nil || len(raw) > stunDNSMaxBytes {
		return nil, 0, errSTUNDNS
	}
	var message struct {
		Status   int
		TC       bool
		Question []struct {
			Name string
			Type int
		}
		Answer []struct {
			Name string
			Type int
			TTL  uint32
			Data string
		}
	}
	if err := json.Unmarshal(raw, &message); err != nil || message.Status != 0 || message.TC || len(message.Question) != 1 || message.Question[0].Type != typeNumber || canonicalDNSName(message.Question[0].Name) != host || len(message.Answer) > 64 {
		return nil, 0, errSTUNDNS
	}
	allowed := map[string]uint32{host: 300}
	for rounds := 0; rounds < 8; rounds++ {
		changed := false
		for _, answer := range message.Answer {
			parentTTL, linked := allowed[canonicalDNSName(answer.Name)]
			if answer.Type == 5 && linked {
				alias := canonicalDNSName(answer.Data)
				ttl := min(parentTTL, answer.TTL)
				previous, known := allowed[alias]
				if alias != "" && (!known || ttl < previous) {
					allowed[alias] = ttl
					changed = true
				}
			}
		}
		if !changed {
			break
		}
	}
	for _, answer := range message.Answer {
		parentTTL, linked := allowed[canonicalDNSName(answer.Name)]
		if answer.Type != typeNumber || !linked {
			continue
		}
		ip := net.ParseIP(answer.Data)
		if ip == nil || family == "ip4" && ip.To4() == nil || family == "ip6" && ip.To4() != nil || !n.publicAnswer(ip) {
			continue
		}
		return append(net.IP(nil), ip...), time.Duration(min(answer.TTL, parentTTL)) * time.Second, nil
	}
	return nil, 0, fmt.Errorf("%w: no usable address", errSTUNDNS)
}
