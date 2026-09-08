// Package connectivity integrates Pion ICE with quic-go on a live UDP endpoint.
package connectivity

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"sync"
	"time"

	"github.com/pion/ice/v4"
	"github.com/pion/stun/v4"
	quic "github.com/quic-go/quic-go"
)

const MaxCandidates = 32

var (
	ErrCheckTimeout      = errors.New("ICE_CHECK_TIMEOUT")
	ErrNoViableCandidate = errors.New("NO_VIABLE_CANDIDATE")
	ErrICEFailed         = errors.New("ICE_FAILED")
)

type Config struct {
	// A concrete local address is required; unspecified binds are rejected.
	BindAddress   string
	STUNURLs      []string
	AllowLoopback bool
	Generation    uint64
	CheckTimeout  time.Duration
}

type Credentials struct {
	Ufrag    string `json:"ufrag"`
	Password string `json:"password"`
}
type Candidate struct {
	Value      string `json:"value"`
	Generation uint64 `json:"generation"`
}
type Path struct {
	Generation        uint64 `json:"generation"`
	BaseSocket        string `json:"base_socket"`
	LocalCandidate    string `json:"local_candidate"`
	RemoteCandidate   string `json:"remote_candidate"`
	LocalType         string `json:"local_type"`
	RemoteType        string `json:"remote_type"`
	RemoteAddress     string `json:"remote_address"`
	ConnectionMethod  string `json:"connection_method"`
	TransportProtocol string `json:"transport_protocol"`
	Relay             bool   `json:"relay"`
}
type Stats struct {
	STUNBytesSent     uint64 `json:"stun_bytes_sent"`
	STUNBytesReceived uint64 `json:"stun_bytes_received"`
	RejectedPackets   uint64 `json:"rejected_packets"`
}

type Endpoint struct {
	cfg                            Config
	udp                            *net.UDPConn
	tr                             *quic.Transport
	packets                        *stunPacketConn
	mux                            *ice.UniversalUDPMuxDefault
	agent                          *ice.Agent
	creds                          Credentials
	candidates                     chan Candidate
	done                           chan struct{}
	pathChanged                    chan struct{}
	once, candidatesOnce, pathOnce sync.Once
	mu                             sync.Mutex
	remote                         map[string]bool
	selected                       string
	connecting                     bool
}

func New(cfg Config) (*Endpoint, error) {
	addr, err := net.ResolveUDPAddr("udp", cfg.BindAddress)
	if err != nil {
		return nil, err
	}
	if addr.IP == nil || addr.IP.IsUnspecified() || !safeIP(addr.IP, cfg.AllowLoopback) || addr.Zone != "" {
		return nil, errors.New("E_NO_CANDIDATE: bind a concrete unicast IP; IPv6 link-local unsupported")
	}
	if len(cfg.STUNURLs) > 4 {
		return nil, errors.New("STUN server limit exceeded")
	}
	if cfg.CheckTimeout <= 0 {
		cfg.CheckTimeout = 20 * time.Second
	}
	var urls []*stun.URI
	for _, raw := range cfg.STUNURLs {
		u, err := stun.ParseURI(raw)
		if err != nil {
			return nil, err
		}
		if u.Scheme != stun.SchemeTypeSTUN || u.Proto != stun.ProtoTypeUDP {
			return nil, errors.New("only stun: over UDP supported; relay=false")
		}
		urls = append(urls, u)
	}
	network, nt := "udp4", ice.NetworkTypeUDP4
	if addr.IP.To4() == nil {
		network = "udp6"
		nt = ice.NetworkTypeUDP6
	}
	u, err := net.ListenUDP(network, addr)
	if err != nil {
		return nil, err
	}
	e := &Endpoint{cfg: cfg, udp: u, tr: &quic.Transport{Conn: u, DisableVersionNegotiationPackets: true}, candidates: make(chan Candidate, MaxCandidates), done: make(chan struct{}), pathChanged: make(chan struct{}), remote: make(map[string]bool)}
	// Only this Endpoint owns and ultimately closes the socket.
	e.packets, err = newSTUNPacketConn(e.tr, u.LocalAddr())
	if err != nil {
		_ = e.tr.Close()
		_ = u.Close()
		return nil, err
	}
	e.mux = ice.NewUniversalUDPMuxDefault(ice.UniversalUDPMuxParams{UDPConn: e.packets})
	types := []ice.CandidateType{ice.CandidateTypeHost}
	if len(urls) > 0 {
		types = append(types, ice.CandidateTypeServerReflexive)
	}
	opts := []ice.AgentOption{ice.WithUrls(urls), ice.WithNetworkTypes([]ice.NetworkType{nt}), ice.WithCandidateTypes(types), ice.WithMulticastDNSMode(ice.MulticastDNSModeDisabled), ice.WithUDPMux(e.mux), ice.WithUDPMuxSrflx(e.mux), ice.WithRemoteIPFilter(func(ip net.IP) bool { return safeIP(ip, cfg.AllowLoopback) }), ice.WithHostAcceptanceMinWait(0), ice.WithSrflxAcceptanceMinWait(500 * time.Millisecond), ice.WithMaxBindingRequests(7), ice.WithCheckInterval(100 * time.Millisecond), ice.WithSTUNGatherTimeout(3 * time.Second)}
	if cfg.AllowLoopback {
		opts = append(opts, ice.WithIncludeLoopback())
	}
	e.agent, err = ice.NewAgentWithOptions(opts...)
	if err != nil {
		_ = e.Close()
		return nil, err
	}
	e.creds.Ufrag, e.creds.Password, err = e.agent.GetLocalUserCredentials()
	if err != nil {
		_ = e.Close()
		return nil, err
	}
	err = e.agent.OnCandidate(func(c ice.Candidate) {
		if c == nil {
			e.candidatesOnce.Do(func() { close(e.candidates) })
			return
		}
		select {
		case e.candidates <- Candidate{Value: c.Marshal(), Generation: cfg.Generation}:
		case <-e.done:
		default:
		}
	})
	if err != nil {
		_ = e.Close()
		return nil, err
	}
	err = e.agent.OnSelectedCandidatePairChange(func(local, remote ice.Candidate) {
		key := local.Marshal() + "|" + remote.Marshal()
		e.mu.Lock()
		changed := e.selected != "" && e.selected != key
		e.selected = key
		e.mu.Unlock()
		if changed {
			e.pathOnce.Do(func() { close(e.pathChanged) })
		}
	})
	if err != nil {
		_ = e.Close()
		return nil, err
	}
	return e, nil
}

func safeIP(ip net.IP, loopback bool) bool {
	if ip == nil || ip.IsUnspecified() || ip.IsMulticast() || ip.IsLinkLocalUnicast() || ip.Equal(net.IPv4bcast) {
		return false
	}
	if ip.IsLoopback() {
		return loopback
	}
	if v4 := ip.To4(); v4 != nil && v4[0] == 0 {
		return false
	}
	return true
}

func (e *Endpoint) Credentials() Credentials     { return e.creds }
func (e *Endpoint) Candidates() <-chan Candidate { return e.candidates }
func (e *Endpoint) Gather() error                { return e.agent.GatherCandidates() }
func (e *Endpoint) QUIC() *quic.Transport        { return e.tr }
func (e *Endpoint) PathChanged() <-chan struct{} { return e.pathChanged }
func (e *Endpoint) Done() <-chan struct{}        { return e.done }
func (e *Endpoint) BaseAddress() string          { return e.udp.LocalAddr().String() }
func (e *Endpoint) Stats() Stats {
	return Stats{e.packets.sent.Load(), e.packets.received.Load(), e.packets.rejected.Load()}
}

// AddRemoteCandidate must only be called after the application verifies the
// signed session envelope and trust pin. ICE credentials alone are not identity.
func (e *Endpoint) AddRemoteCandidate(c Candidate) error {
	if c.Generation != e.cfg.Generation {
		return errors.New("E_STALE_GENERATION")
	}
	if len(c.Value) > 2048 {
		return errors.New("candidate too large")
	}
	v, err := ice.UnmarshalCandidate(c.Value)
	if err != nil {
		return fmt.Errorf("invalid candidate: %w", err)
	}
	ip, err := netip.ParseAddr(v.Address())
	if err != nil || ip.Zone() != "" || !safeIP(net.IP(ip.AsSlice()), e.cfg.AllowLoopback) {
		return errors.New("E_UNSAFE_CANDIDATE")
	}
	if v.Port() < 1 || v.Port() > 65535 || v.Component() != ice.ComponentRTP || !v.NetworkType().IsUDP() {
		return errors.New("E_UNSAFE_CANDIDATE")
	}
	if v.Type() != ice.CandidateTypeHost && v.Type() != ice.CandidateTypeServerReflexive && v.Type() != ice.CandidateTypePeerReflexive {
		return errors.New("E_RELAY_NOT_IMPLEMENTED")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.remote[c.Value] {
		return nil
	}
	if len(e.remote) >= MaxCandidates {
		return errors.New("candidate limit exceeded")
	}
	if err = e.agent.AddRemoteCandidate(v); err != nil {
		return err
	}
	e.remote[c.Value] = true
	return nil
}

func (e *Endpoint) Connect(ctx context.Context, remote Credentials, controlling bool) (Path, error) {
	e.mu.Lock()
	if e.connecting {
		e.mu.Unlock()
		return Path{}, errors.New("endpoint already connecting")
	}
	e.connecting = true
	e.mu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, e.cfg.CheckTimeout)
	defer cancel()
	var err error
	if controlling {
		_, err = e.agent.Dial(ctx, remote.Ufrag, remote.Password)
	} else {
		_, err = e.agent.Accept(ctx, remote.Ufrag, remote.Password)
	}
	if err != nil {
		if errors.Is(ctx.Err(), context.Canceled) {
			return Path{}, fmt.Errorf("%w: %v", context.Canceled, err)
		}
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return Path{}, fmt.Errorf("%w: %v", ErrCheckTimeout, err)
		}
		if errors.Is(err, ice.ErrNoCandidatePairs) {
			return Path{}, fmt.Errorf("%w: %v", ErrNoViableCandidate, err)
		}
		return Path{}, fmt.Errorf("%w: %v", ErrICEFailed, err)
	}
	p, err := e.agent.GetSelectedCandidatePair()
	if err != nil {
		return Path{}, err
	}
	if p == nil {
		return Path{}, ErrNoViableCandidate
	}
	method := "direct_unknown"
	if p.Local.Type() == ice.CandidateTypeServerReflexive || p.Remote.Type() == ice.CandidateTypeServerReflexive {
		method = "internet_p2p"
	}
	return Path{Generation: e.cfg.Generation, BaseSocket: e.BaseAddress(), LocalCandidate: p.Local.Marshal(), RemoteCandidate: p.Remote.Marshal(), LocalType: p.Local.Type().String(), RemoteType: p.Remote.Type().String(), RemoteAddress: net.JoinHostPort(p.Remote.Address(), strconv.Itoa(p.Remote.Port())), ConnectionMethod: method, TransportProtocol: "quic", Relay: false}, nil
}

func (e *Endpoint) Close() error {
	var result error
	e.once.Do(func() {
		close(e.done)
		if e.packets != nil {
			_ = e.packets.Close()
		}
		if e.agent != nil {
			_ = e.agent.Close()
		}
		if e.mux != nil {
			_ = e.mux.Close()
		}
		if e.tr != nil {
			result = e.tr.Close()
		}
		if e.udp != nil {
			_ = e.udp.Close()
		}
		if e.packets != nil {
			e.packets.wg.Wait()
		}
	})
	return result
}
