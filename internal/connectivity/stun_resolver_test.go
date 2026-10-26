package connectivity

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pion/ice/v4"
	"github.com/pion/stun/v4"
	"github.com/pion/transport/v4"
)

func fakeSystemSTUNLookup(context.Context, string, string) ([]net.IPAddr, error) {
	return []net.IPAddr{{IP: net.ParseIP("198.18.2.109")}}, nil
}

func dnsSTUNAnswer(host, ip string, ttl uint32) string {
	body, _ := json.Marshal(map[string]any{"Status": 0, "Question": []any{map[string]any{"name": host + ".", "type": 1}}, "Answer": []any{map[string]any{"name": host + ".", "type": 1, "TTL": ttl, "data": ip}}})
	return string(body)
}

func testSTUNDNS(t *testing.T, handler http.HandlerFunc, options stunResolverOptions) *stunResolverNet {
	t.Helper()
	server := httptest.NewTLSServer(handler)
	t.Cleanup(server.Close)
	options.client = server.Client()
	options.endpoints = []string{server.URL + "/dns-query"}
	if options.lookup == nil {
		options.lookup = fakeSystemSTUNLookup
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	n, err := newSTUNResolverNet(ctx, false, &options)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func testBindingServer(t *testing.T) (*net.UDPConn, *atomic.Int32) {
	t.Helper()
	u, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	count := new(atomic.Int32)
	done := make(chan struct{})
	go func() {
		defer close(done)
		buffer := make([]byte, 4096)
		for {
			n, remote, err := u.ReadFromUDP(buffer)
			if err != nil {
				return
			}
			request := &stun.Message{Raw: append([]byte(nil), buffer[:n]...)}
			if request.Decode() != nil || request.Type != stun.BindingRequest {
				continue
			}
			count.Add(1)
			response, err := stun.Build(stun.NewTransactionIDSetter(request.TransactionID), stun.BindingSuccess, stun.XORMappedAddress{IP: remote.IP, Port: remote.Port}, stun.Fingerprint)
			if err == nil {
				_, _ = u.WriteToUDP(response.Raw, remote)
			}
		}
	}()
	t.Cleanup(func() { _ = u.Close(); <-done })
	return u, count
}

func TestSTUNFakeDNSUsesHTTPSAndRealPionSharedSocket(t *testing.T) {
	u, bindings := testBindingServer(t)
	var queries, lookups atomic.Int32
	dns := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		queries.Add(1)
		if r.URL.Query().Get("name") != "stun.oooai.de" || r.URL.Query().Get("type") != "A" || r.Header.Get("Accept") != "application/dns-json" {
			t.Errorf("unexpected DNS request %s", r.URL)
		}
		w.Header().Set("Content-Type", "application/dns-json")
		_, _ = w.Write([]byte(dnsSTUNAnswer("stun.oooai.de", "127.0.0.1", 30)))
	}))
	defer dns.Close()
	port := u.LocalAddr().(*net.UDPAddr).Port
	e, err := New(Config{BindAddress: "127.0.0.1:0", AllowLoopback: true, Generation: 1, STUNURLs: []string{fmt.Sprintf("stun:stun.oooai.de:%d", port)}, stunResolver: &stunResolverOptions{
		lookup: func(ctx context.Context, network, host string) ([]net.IPAddr, error) {
			lookups.Add(1)
			return fakeSystemSTUNLookup(ctx, network, host)
		},
		client: dns.Client(), endpoints: []string{dns.URL + "/dns-query"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	if lookups.Load() != 0 || queries.Load() != 0 {
		t.Fatal("Endpoint.New performed network resolution before gathering")
	}
	if err = e.Gather(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, basePort, _ := net.SplitHostPort(e.BaseAddress())
	hostCount, srflxCount := 0, 0
	for {
		select {
		case <-ctx.Done():
			t.Fatal("production Pion gathering did not finish")
		case candidate, ok := <-e.Candidates():
			if !ok {
				if hostCount == 0 || srflxCount == 0 || lookups.Load() == 0 || queries.Load() != 1 || bindings.Load() == 0 || e.Stats().STUNResponsesReceived == 0 {
					t.Fatalf("missing actual candidates/traffic: host=%d srflx=%d DNS=%d bindings=%d stats=%+v", hostCount, srflxCount, queries.Load(), bindings.Load(), e.Stats())
				}
				return
			}
			parsed, err := ice.UnmarshalCandidate(candidate.Value)
			if err != nil {
				t.Fatal(err)
			}
			if fakeSTUNAddress(net.ParseIP(parsed.Address())) {
				t.Fatal("Fake-IP advertised as candidate")
			}
			switch parsed.Type() {
			case ice.CandidateTypeHost:
				hostCount++
			case ice.CandidateTypeServerReflexive:
				srflxCount++
				if strconv.Itoa(parsed.Port()) != basePort || parsed.Address() != "127.0.0.1" {
					t.Fatal("STUN mapping did not come from the actual shared ICE/QUIC socket")
				}
			case ice.CandidateTypeRelay:
				t.Fatal("relay candidate unexpectedly enabled")
			}
		}
	}
}

func TestSTUNPendingHTTPSDoesNotBlockHostAndCloseCancels(t *testing.T) {
	entered, stopped := make(chan struct{}), make(chan struct{})
	dns := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		<-r.Context().Done()
		close(stopped)
	}))
	defer dns.Close()
	e, err := New(Config{BindAddress: "127.0.0.1:0", AllowLoopback: true, Generation: 1, STUNURLs: []string{"stun:stun.oooai.de:3478"}, stunResolver: &stunResolverOptions{lookup: fakeSystemSTUNLookup, client: dns.Client(), endpoints: []string{dns.URL}}})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	if err = e.Gather(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("DoH request not observed")
	}
	select {
	case candidate := <-e.Candidates():
		parsed, err := ice.UnmarshalCandidate(candidate.Value)
		if err != nil || parsed.Type() != ice.CandidateTypeHost {
			t.Fatal("host not delivered independently of blocked HTTPS")
		}
	case <-ctx.Done():
		t.Fatal("host gathering waited for DoH completion")
	}
	closed := make(chan struct{})
	go func() { _ = e.Close(); close(closed) }()
	select {
	case <-closed:
	case <-ctx.Done():
		t.Fatal("Endpoint.Close did not cancel resolver/gathering")
	}
	select {
	case <-stopped:
	case <-ctx.Done():
		t.Fatal("HTTPS request survived endpoint close")
	}
}

type delegatedSTUNNet struct {
	transport.Net
	resolve func(string, string) (*net.UDPAddr, error)
}

func (n delegatedSTUNNet) ResolveUDPAddr(network, address string) (*net.UDPAddr, error) {
	return n.resolve(network, address)
}

func TestSTUNNormalDNSLiteralAndPrivateNamesKeepSystemPolicy(t *testing.T) {
	var queries, lookups, delegated atomic.Int32
	n := testSTUNDNS(t, func(w http.ResponseWriter, r *http.Request) {
		queries.Add(1)
		t.Error("normal resolution invoked public DoH")
	}, stunResolverOptions{lookup: func(context.Context, string, string) ([]net.IPAddr, error) {
		lookups.Add(1)
		return []net.IPAddr{{IP: net.ParseIP("198.18.0.1")}, {IP: net.ParseIP("9.9.9.9")}}, nil
	}})
	n.Net = delegatedSTUNNet{Net: n.Net, resolve: func(network, address string) (*net.UDPAddr, error) {
		delegated.Add(1)
		if address == "private.internal:3478" {
			return &net.UDPAddr{IP: net.ParseIP("192.168.20.1"), Port: 3478}, nil
		}
		return net.ResolveUDPAddr(network, address)
	}}
	for _, input := range []struct{ address, want string }{{"stun.oooai.de:3478", "9.9.9.9"}, {"stun.cloudflare.com:3478", "9.9.9.9"}, {"162.159.207.0:3478", "162.159.207.0"}, {"private.internal:3478", "192.168.20.1"}} {
		got, err := n.ResolveUDPAddr("udp4", input.address)
		if err != nil || got.IP.String() != input.want || got.Port != 3478 {
			t.Fatalf("resolution changed for %s: %v %v", input.address, got, err)
		}
	}
	if queries.Load() != 0 || lookups.Load() != 2 || delegated.Load() != 2 {
		t.Fatal("resolver did not preserve literal/private name scope")
	}
}

func TestSTUNHTTPSCacheAndConcurrentRequestsAreBounded(t *testing.T) {
	var queries atomic.Int32
	clock := time.Unix(100, 0)
	n := testSTUNDNS(t, func(w http.ResponseWriter, r *http.Request) {
		queries.Add(1)
		time.Sleep(30 * time.Millisecond)
		w.Header().Set("Content-Type", "application/dns-json")
		_, _ = w.Write([]byte(dnsSTUNAnswer("stun.oooai.de", "1.1.1.1", 30)))
	}, stunResolverOptions{now: func() time.Time { return clock }})
	var wg sync.WaitGroup
	errors := make(chan error, 16)
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ip, err := n.ResolveUDPAddr("udp4", "stun.oooai.de:3478")
			if err != nil {
				errors <- err
			} else if !ip.IP.Equal(net.ParseIP("1.1.1.1")) {
				errors <- fmt.Errorf("wrong IP %v", ip)
			}
		}()
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		t.Error(err)
	}
	if queries.Load() != 1 || len(n.cache) > stunDNSCacheLimit || len(n.inflight) != 0 {
		t.Fatal("duplicate or unbounded lookup work")
	}
	clock = clock.Add(31 * time.Second)
	if _, err := n.ResolveUDPAddr("udp4", "stun.oooai.de:3478"); err != nil {
		t.Fatal(err)
	}
	if queries.Load() != 2 {
		t.Fatal("expired DNS answer remained cached")
	}
}

func TestSTUNSystemLookupFailureUsesRestrictedHTTPS(t *testing.T) {
	n := testSTUNDNS(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(dnsSTUNAnswer("stun.cloudflare.com", "162.159.207.0", 20)))
	}, stunResolverOptions{lookup: func(context.Context, string, string) ([]net.IPAddr, error) {
		return nil, errors.New("controlled DNS failure")
	}})
	got, err := n.ResolveUDPAddr("udp4", "stun.cloudflare.com:3478")
	if err != nil || got.IP.String() != "162.159.207.0" {
		t.Fatalf("fallback failed: %v %v", got, err)
	}
}

func TestSTUNUnsafeSystemAnswersDoNotBypassAddressPolicy(t *testing.T) {
	for _, address := range []string{"0.0.0.0", "224.0.0.1", "127.0.0.1", "255.255.255.255"} {
		t.Run(address, func(t *testing.T) {
			n := testSTUNDNS(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/dns-json")
				_, _ = w.Write([]byte(dnsSTUNAnswer("stun.oooai.de", "1.1.1.1", 30)))
			}, stunResolverOptions{lookup: func(context.Context, string, string) ([]net.IPAddr, error) {
				return []net.IPAddr{{IP: net.ParseIP(address)}}, nil
			}})
			got, err := n.ResolveUDPAddr("udp4", "stun.oooai.de:3478")
			if err != nil || got.IP.String() != "1.1.1.1" {
				t.Fatal("unsafe system answer bypassed STUN address policy", got, err)
			}
		})
	}
}

func TestSTUNHTTPSRejectsUnsafeAndMalformedAnswers(t *testing.T) {
	valid := dnsSTUNAnswer("stun.oooai.de", "1.1.1.1", 30)
	cases := map[string]string{
		"fake":               dnsSTUNAnswer("stun.oooai.de", "198.19.2.10", 30),
		"loopback":           dnsSTUNAnswer("stun.oooai.de", "127.0.0.1", 30),
		"private":            dnsSTUNAnswer("stun.oooai.de", "10.20.30.40", 30),
		"multicast":          dnsSTUNAnswer("stun.oooai.de", "224.0.0.1", 30),
		"wrong family":       dnsSTUNAnswer("stun.oooai.de", "2606:4700::1111", 30),
		"wrong question":     dnsSTUNAnswer("unrelated.example", "1.1.1.1", 30),
		"unrelated answer":   strings.Replace(valid, `"Answer":[{"TTL":30,"data":"1.1.1.1","name":"stun.oooai.de."`, `"Answer":[{"TTL":30,"data":"1.1.1.1","name":"unrelated.example."`, 1),
		"truncated response": strings.Replace(valid, `"Status":0`, `"Status":0,"TC":true`, 1),
		"nxdomain":           strings.Replace(valid, `"Status":0`, `"Status":3`, 1),
		"invalid JSON":       "{",
		"trailing JSON":      valid + valid,
		"oversized":          strings.Repeat(" ", stunDNSMaxBytes+1) + valid,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			n := testSTUNDNS(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/dns-json")
				_, _ = w.Write([]byte(body))
			}, stunResolverOptions{})
			if _, err := n.ResolveUDPAddr("udp4", "stun.oooai.de:3478"); err == nil {
				t.Fatal("unsafe DNS answer accepted")
			}
		})
	}
}

func TestSTUNHTTPSRedirectIsNotFollowed(t *testing.T) {
	var redirected atomic.Int32
	target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirected.Add(1)
		w.Header().Set("Content-Type", "application/dns-json")
		_, _ = w.Write([]byte(dnsSTUNAnswer("stun.oooai.de", "1.1.1.1", 30)))
	}))
	defer target.Close()
	n := testSTUNDNS(t, func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, http.StatusFound) }, stunResolverOptions{})
	if _, err := n.ResolveUDPAddr("udp4", "stun.oooai.de:3478"); err == nil || redirected.Load() != 0 {
		t.Fatal("resolver redirect followed")
	}
}

func TestSTUNHTTPSCNAMEUsesLinkedAnswerAndShortestTTL(t *testing.T) {
	var queries atomic.Int32
	clock := time.Unix(100, 0)
	n := testSTUNDNS(t, func(w http.ResponseWriter, r *http.Request) {
		queries.Add(1)
		w.Header().Set("Content-Type", "application/dns-json")
		_, _ = w.Write([]byte(`{"Status":0,"Question":[{"name":"stun.oooai.de.","type":1}],"Answer":[{"name":"unrelated.example.","type":1,"TTL":300,"data":"9.9.9.9"},{"name":"stun.oooai.de.","type":5,"TTL":1,"data":"edge.example."},{"name":"edge.example.","type":1,"TTL":300,"data":"1.1.1.1"}]}`))
	}, stunResolverOptions{now: func() time.Time { return clock }})
	got, err := n.ResolveUDPAddr("udp4", "stun.oooai.de:3478")
	if err != nil || got.IP.String() != "1.1.1.1" {
		t.Fatal("CNAME chain did not bind the A record", got, err)
	}
	clock = clock.Add(2 * time.Second)
	if _, err := n.ResolveUDPAddr("udp4", "stun.oooai.de:3478"); err != nil || queries.Load() != 2 {
		t.Fatal("cache outlived CNAME TTL", err)
	}
}

func TestSTUNHTTPSSecondProviderAndTLSVerification(t *testing.T) {
	first := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "controlled failure", http.StatusServiceUnavailable)
	}))
	defer first.Close()
	var hits atomic.Int32
	second := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/dns-json")
		_, _ = w.Write([]byte(dnsSTUNAnswer("stun.oooai.de", "1.1.1.1", 30)))
	}))
	defer second.Close()
	n, err := newSTUNResolverNet(context.Background(), false, &stunResolverOptions{lookup: fakeSystemSTUNLookup, client: first.Client(), endpoints: []string{first.URL, second.URL}})
	if err != nil {
		t.Fatal(err)
	}
	got, err := n.ResolveUDPAddr("udp4", "stun.oooai.de:3478")
	if err != nil || got.IP.String() != "1.1.1.1" || hits.Load() != 1 {
		t.Fatal("independent second resolver did not recover", got, err)
	}
	// No custom CA is installed for this client, so the controlled self-signed
	// HTTPS server must fail certificate verification before an HTTP request.
	untrusted, err := newSTUNResolverNet(context.Background(), false, &stunResolverOptions{lookup: fakeSystemSTUNLookup, client: &http.Client{}, endpoints: []string{second.URL}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = untrusted.ResolveUDPAddr("udp4", "stun.oooai.de:3478"); err == nil || hits.Load() != 1 {
		t.Fatal("untrusted HTTPS certificate accepted")
	}
}

func TestSTUNHTTPSFailuresBackOffAndRemainTimeBounded(t *testing.T) {
	var queries atomic.Int32
	clock := time.Unix(100, 0)
	n := testSTUNDNS(t, func(w http.ResponseWriter, r *http.Request) {
		queries.Add(1)
		http.Error(w, "controlled failure", http.StatusServiceUnavailable)
	}, stunResolverOptions{now: func() time.Time { return clock }})
	for i := 0; i < 2; i++ {
		if _, err := n.ResolveUDPAddr("udp4", "stun.oooai.de:3478"); err == nil {
			t.Fatal("resolver failure ignored")
		}
	}
	if queries.Load() != 1 {
		t.Fatal("negative cache did not bound failure retries")
	}
	clock = clock.Add(3 * time.Second)
	if _, err := n.ResolveUDPAddr("udp4", "stun.oooai.de:3478"); err == nil || queries.Load() != 2 {
		t.Fatal("negative cache did not release retry")
	}
	blocked := testSTUNDNS(t, func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }, stunResolverOptions{budget: 80 * time.Millisecond})
	start := time.Now()
	if _, err := blocked.ResolveUDPAddr("udp4", "stun.oooai.de:3478"); err == nil || time.Since(start) > time.Second {
		t.Fatal("HTTPS deadline did not bound resolution")
	}
}
