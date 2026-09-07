// Package server is a bounded control plane: identity, membership and signed
// session signaling only. No file-transfer package is imported here.
package server

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"example.com/linksend/internal/protocol"
	"example.com/linksend/internal/store"
	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/pelletier/go-toml/v2"
)

type Config struct {
	Listen                  string `toml:"listen"`
	Database                string `toml:"database"`
	AllowInsecureLoopback   bool   `toml:"allow_insecure_loopback"`
	AllowLoopbackCandidates bool   `toml:"allow_loopback_candidates"`
	TLSCert                 string `toml:"tls_cert"`
	TLSKey                  string `toml:"tls_key"`
	BootstrapToken          string `toml:"-"`
}

func LoadConfig(path string) (Config, error) {
	var c Config
	b, err := os.ReadFile(path)
	if err != nil {
		return c, err
	}
	if err = toml.Unmarshal(b, &c); err != nil {
		return c, err
	}
	c.BootstrapToken = os.Getenv("LINKSEND_BOOTSTRAP_TOKEN")
	return c, c.Validate()
}
func (c Config) Validate() error {
	if c.Listen == "" || c.Database == "" {
		return errors.New("listen and database are required")
	}
	host, _, err := net.SplitHostPort(c.Listen)
	if err != nil {
		return err
	}
	ip, err := netip.ParseAddr(host)
	if c.AllowInsecureLoopback {
		if err != nil || !ip.IsLoopback() {
			return errors.New("insecure development server must bind a literal loopback address")
		}
	} else if c.TLSCert == "" || c.TLSKey == "" {
		return errors.New("public signaling requires tls_cert and tls_key")
	}
	if c.AllowLoopbackCandidates && !c.AllowInsecureLoopback {
		return errors.New("loopback candidates only allowed in explicit loopback development mode")
	}
	return nil
}

type JoinRequest struct {
	Token     string `json:"token"`
	Name      string `json:"name"`
	PublicKey []byte `json:"public_key"`
	Signature []byte `json:"signature"`
}
type Invitation struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}
type Wire struct {
	Type         string                 `json:"type"`
	Nonce        string                 `json:"nonce,omitempty"`
	DeviceID     string                 `json:"device_id,omitempty"`
	Signature    []byte                 `json:"signature,omitempty"`
	Message      *protocol.Envelope     `json:"message,omitempty"`
	Error        *protocol.Error        `json:"error,omitempty"`
	Capabilities *protocol.Capabilities `json:"capabilities,omitempty"`
}

func JoinBytes(kind string, r JoinRequest) []byte {
	return protocol.AuthBytes("POST", "/v1/"+kind, base64.RawStdEncoding.EncodeToString(r.PublicKey), r.Token, 0, []byte(r.Name))
}

type peer struct {
	device store.Device
	conn   *websocket.Conn
	send   chan Wire
	cancel context.CancelFunc
}
type negotiation struct {
	from, to   string
	generation uint64
	expires    time.Time
	accepted   bool
	counts     map[string]int
}
type bucket struct {
	tokens float64
	at     time.Time
}
type Counters struct {
	ReceivedMessages  uint64 `json:"received_messages"`
	ForwardedMessages uint64 `json:"forwarded_messages"`
	ReceivedBytes     uint64 `json:"received_bytes"`
	ForwardedBytes    uint64 `json:"forwarded_bytes"`
}
type Server struct {
	cfg               Config
	store             *store.Control
	ctx               context.Context
	cancel            context.CancelFunc
	mu                sync.Mutex
	clients           map[string]*peer
	sessions          map[string]*negotiation
	closed            bool
	buckets           map[string]bucket
	replays           *protocol.ReplayWindow
	slots             chan struct{}
	wg                sync.WaitGroup
	receivedMessages  atomic.Uint64
	forwardedMessages atomic.Uint64
	receivedBytes     atomic.Uint64
	forwardedBytes    atomic.Uint64
}

func New(cfg Config) (*Server, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	db, err := store.OpenControl(cfg.Database)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	s := &Server{cfg: cfg, store: db, ctx: ctx, cancel: cancel, clients: map[string]*peer{}, sessions: map[string]*negotiation{}, buckets: map[string]bucket{}, replays: protocol.NewReplayWindow(65536), slots: make(chan struct{}, 128)}
	return s, nil
}
func (s *Server) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	s.cancel()
	for _, p := range s.clients {
		p.cancel()
		p.conn.CloseNow()
	}
	s.mu.Unlock()
	s.wg.Wait()
	return s.store.Close()
}
func (s *Server) Counters() Counters {
	return Counters{s.receivedMessages.Load(), s.forwardedMessages.Load(), s.receivedBytes.Load(), s.forwardedBytes.Load()}
}
func (s *Server) Handler() http.Handler {
	m := http.NewServeMux()
	m.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		respond(w, 200, map[string]any{"status": "ok", "capabilities": protocol.Supported()})
	})
	m.HandleFunc("POST /v1/bootstrap", s.bootstrap)
	m.HandleFunc("POST /v1/pairing/join", s.join)
	m.HandleFunc("POST /v1/pairing/invitations", s.invite)
	m.HandleFunc("GET /v1/devices", s.devices)
	m.HandleFunc("DELETE /v1/devices/{id}", s.revoke)
	m.HandleFunc("GET /v1/ws", s.websocket)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		ip, _ := netip.ParseAddr(host)
		if r.TLS == nil && (!s.cfg.AllowInsecureLoopback || err != nil || !ip.IsLoopback()) {
			reject(w, 403, protocol.AuthenticationFailed, "HTTPS/WSS required")
			return
		}
		if !s.admit(host) {
			reject(w, 429, protocol.RateLimited, "request rate limit")
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		m.ServeHTTP(w, r)
	})
}
func (s *Server) admit(host string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	if len(s.buckets) >= 1024 {
		for k, v := range s.buckets {
			if now.Sub(v.at) > time.Minute*5 {
				delete(s.buckets, k)
			}
		}
	}
	b, ok := s.buckets[host]
	if !ok {
		if len(s.buckets) >= 1024 {
			return false
		}
		b = bucket{tokens: 60, at: now}
	}
	b.tokens = min(60, b.tokens+now.Sub(b.at).Seconds()*2)
	b.at = now
	allowed := b.tokens >= 1
	if allowed {
		b.tokens--
	}
	s.buckets[host] = b
	return allowed
}
func respond(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func reject(w http.ResponseWriter, status int, code protocol.Code, detail string) {
	respond(w, status, &protocol.Error{Code: code, Detail: detail})
}
func readBody(w http.ResponseWriter, r *http.Request) ([]byte, error) {
	return io.ReadAll(http.MaxBytesReader(w, r.Body, protocol.MaxPayloadBytes))
}
func (s *Server) registration(w http.ResponseWriter, r *http.Request) (JoinRequest, bool) {
	var req JoinRequest
	b, err := readBody(w, r)
	if err != nil || json.Unmarshal(b, &req) != nil || len(req.PublicKey) != 32 || len(req.Signature) != 64 || len(req.Name) == 0 || len(req.Name) > 128 {
		reject(w, 400, protocol.InvalidMessage, "invalid registration")
		return req, false
	}
	kind := "pairing/join"
	if r.URL.Path == "/v1/bootstrap" {
		kind = "bootstrap"
	}
	if !ed25519.Verify(req.PublicKey, JoinBytes(kind, req), req.Signature) {
		reject(w, 401, protocol.AuthenticationFailed, "registration proof invalid")
		return req, false
	}
	return req, true
}
func (s *Server) bootstrap(w http.ResponseWriter, r *http.Request) {
	req, ok := s.registration(w, r)
	if !ok {
		return
	}
	expected := sha256.Sum256([]byte(s.cfg.BootstrapToken))
	got := sha256.Sum256([]byte(req.Token))
	if len(s.cfg.BootstrapToken) < 32 || subtle.ConstantTimeCompare(expected[:], got[:]) != 1 {
		reject(w, 401, protocol.AuthenticationFailed, "bootstrap token invalid or not configured")
		return
	}
	d, err := s.store.Bootstrap(r.Context(), req.Name, req.PublicKey)
	if err != nil {
		reject(w, 409, protocol.AuthenticationFailed, "bootstrap unavailable")
		return
	}
	respond(w, 201, d)
}
func (s *Server) join(w http.ResponseWriter, r *http.Request) {
	req, ok := s.registration(w, r)
	if !ok {
		return
	}
	d, err := s.store.Join(r.Context(), req.Token, req.Name, req.PublicKey)
	if err != nil {
		reject(w, 403, protocol.AuthenticationFailed, "invitation invalid, expired or used")
		return
	}
	respond(w, 201, d)
}
func (s *Server) authenticate(w http.ResponseWriter, r *http.Request, body []byte) (store.Device, bool) {
	id := r.Header.Get("X-LinkSend-Device")
	nonce := r.Header.Get("X-LinkSend-Nonce")
	at, err := strconv.ParseInt(r.Header.Get("X-LinkSend-Time"), 10, 64)
	sig, decodeErr := base64.RawStdEncoding.DecodeString(r.Header.Get("X-LinkSend-Signature"))
	now := time.Now()
	if err != nil || decodeErr != nil || len(nonce) != 32 || len(sig) != 64 || at < now.Add(-time.Minute).Unix() || at > now.Add(15*time.Second).Unix() {
		reject(w, 401, protocol.AuthenticationFailed, "invalid request authentication")
		return store.Device{}, false
	}
	d, err := s.store.Device(r.Context(), id)
	if err != nil || !ed25519.Verify(d.PublicKey, protocol.AuthBytes(r.Method, r.URL.RequestURI(), id, nonce, at, body), sig) {
		reject(w, 401, protocol.AuthenticationFailed, "request signature or membership invalid")
		return store.Device{}, false
	}
	if err = s.replays.Use("http:"+id+":"+nonce, now.Add(2*time.Minute), now); err != nil {
		reject(w, 409, protocol.Replay, "request replayed")
		return store.Device{}, false
	}
	return d, true
}
func (s *Server) invite(w http.ResponseWriter, r *http.Request) {
	body, err := readBody(w, r)
	if err != nil {
		reject(w, 400, protocol.InvalidMessage, "body too large")
		return
	}
	d, ok := s.authenticate(w, r, body)
	if !ok {
		return
	}
	token, expires, err := s.store.Invitation(r.Context(), d)
	if err != nil {
		reject(w, 403, protocol.AuthenticationFailed, "administrator required")
		return
	}
	respond(w, 201, Invitation{token, expires})
}
func (s *Server) devices(w http.ResponseWriter, r *http.Request) {
	d, ok := s.authenticate(w, r, nil)
	if !ok {
		return
	}
	devices, err := s.store.Devices(r.Context(), d.GroupID)
	if err != nil {
		reject(w, 500, protocol.InvalidMessage, "database unavailable")
		return
	}
	s.mu.Lock()
	for i := range devices {
		_, devices[i].Online = s.clients[devices[i].ID]
	}
	s.mu.Unlock()
	respond(w, 200, devices)
}
func (s *Server) revoke(w http.ResponseWriter, r *http.Request) {
	d, ok := s.authenticate(w, r, nil)
	if !ok {
		return
	}
	id := r.PathValue("id")
	if err := s.store.Revoke(r.Context(), d, id); err != nil {
		reject(w, 403, protocol.AuthenticationFailed, "revocation not authorized")
		return
	}
	s.mu.Lock()
	if p := s.clients[id]; p != nil {
		p.cancel()
		p.conn.CloseNow()
	}
	for sid, n := range s.sessions {
		if n.from == id || n.to == id {
			delete(s.sessions, sid)
		}
	}
	s.mu.Unlock()
	respond(w, 200, map[string]bool{"revoked": true})
}

func (s *Server) websocket(w http.ResponseWriter, r *http.Request) {
	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	default:
		reject(w, 429, protocol.RateLimited, "connection limit")
		return
	}
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{Subprotocols: []string{"linksend.signal.v1"}})
	if err != nil {
		return
	}
	defer c.CloseNow()
	c.SetReadLimit(protocol.MaxMessageBytes)
	if c.Subprotocol() != "linksend.signal.v1" {
		_ = c.Close(websocket.StatusPolicyViolation, "protocol required")
		return
	}
	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()
	authCtx, authCancel := context.WithTimeout(ctx, 5*time.Second)
	defer authCancel()
	nonce := protocol.RandomID() + protocol.RandomID()
	caps := protocol.Supported()
	if err = wsjson.Write(authCtx, c, Wire{Type: "hello", Nonce: nonce, Capabilities: &caps}); err != nil {
		return
	}
	var auth Wire
	if err = wsjson.Read(authCtx, c, &auth); err != nil {
		return
	}
	d, err := s.store.Device(authCtx, auth.DeviceID)
	if err != nil || auth.Type != "authenticate" || !ed25519.Verify(d.PublicKey, protocol.ChallengeBytes(auth.DeviceID, nonce), auth.Signature) {
		_ = c.Close(websocket.StatusPolicyViolation, "authentication failed")
		return
	}
	p := &peer{device: d, conn: c, send: make(chan Wire, 64), cancel: cancel}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	if old := s.clients[d.ID]; old != nil {
		old.cancel()
		old.conn.CloseNow()
	}
	s.clients[d.ID] = p
	s.wg.Add(1)
	s.mu.Unlock()
	defer s.wg.Done()
	defer func() {
		s.mu.Lock()
		if s.clients[d.ID] == p {
			delete(s.clients, d.ID)
		}
		s.mu.Unlock()
	}()
	if err = wsjson.Write(authCtx, c, Wire{Type: "authenticated", DeviceID: d.ID, Capabilities: &caps}); err != nil {
		return
	}
	authCancel()
	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		for {
			select {
			case <-ctx.Done():
				return
			case msg := <-p.send:
				wc, cc := context.WithTimeout(ctx, 5*time.Second)
				err := wsjson.Write(wc, c, msg)
				cc()
				if err != nil {
					cancel()
					c.CloseNow()
					return
				}
			}
		}
	}()
	defer func() { cancel(); c.CloseNow(); <-writerDone }()
	budget := 60
	window := time.Now()
	for {
		rc, cc := context.WithTimeout(ctx, 65*time.Second)
		var wire Wire
		err = wsjson.Read(rc, c, &wire)
		cc()
		if err != nil {
			return
		}
		if time.Since(window) >= time.Second {
			window = time.Now()
			budget = 60
		}
		budget--
		if budget < 0 {
			_ = c.Close(websocket.StatusPolicyViolation, "message rate limit")
			return
		}
		if _, err = s.store.Device(ctx, d.ID); err != nil {
			return
		}
		if wire.Type == "heartbeat" {
			select {
			case p.send <- Wire{Type: "heartbeat"}:
			default:
				return
			}
			continue
		}
		if wire.Type != "signal" || wire.Message == nil {
			s.sendError(p, protocol.InvalidMessage, "unknown critical message")
			continue
		}
		encoded, _ := json.Marshal(wire.Message)
		s.receivedMessages.Add(1)
		s.receivedBytes.Add(uint64(len(encoded)))
		if err = s.forward(ctx, p, *wire.Message); err != nil {
			var pe *protocol.Error
			if !errors.As(err, &pe) {
				pe = &protocol.Error{Code: protocol.InvalidMessage, Detail: "control message rejected"}
			}
			s.sendError(p, pe.Code, pe.Detail)
		}
	}
}
func (s *Server) sendError(p *peer, code protocol.Code, detail string) {
	select {
	case p.send <- Wire{Type: "error", Error: &protocol.Error{Code: code, Detail: detail}}:
	default:
		p.cancel()
		p.conn.CloseNow()
	}
}
func (s *Server) forward(ctx context.Context, from *peer, e protocol.Envelope) error {
	now := time.Now()
	if e.Sender != from.device.ID {
		return protocol.Fail(protocol.AuthenticationFailed, "sender mismatch")
	}
	if err := e.Verify(from.device.PublicKey, now); err != nil {
		return err
	}
	if e.Type == "candidate" {
		var c protocol.Candidate
		if json.Unmarshal(e.Payload, &c) != nil {
			return protocol.Fail(protocol.InvalidMessage, "invalid candidate payload")
		}
		if err := protocol.ValidateCandidate(c.Candidate, s.cfg.AllowLoopbackCandidates); err != nil {
			return err
		}
	}
	toDevice, err := s.store.Device(ctx, e.Recipient)
	if err != nil || toDevice.GroupID != from.device.GroupID {
		return protocol.Fail(protocol.AuthenticationFailed, "recipient outside authorized group")
	}
	if err = s.replays.Use("signal:"+e.Sender+":"+e.MessageID, time.Unix(e.ExpiresAt, 0).Add(time.Second), now); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for sid, n := range s.sessions {
		if !n.expires.After(now) {
			delete(s.sessions, sid)
		}
	}
	to := s.clients[e.Recipient]
	if to == nil {
		return protocol.Fail(protocol.PeerOffline, "recipient is offline")
	}
	n := s.sessions[e.SessionID]
	if e.Type == "connect_request" {
		if n != nil {
			return protocol.Fail(protocol.Replay, "session identifier already active")
		}
		count := 0
		for _, other := range s.sessions {
			if other.from == e.Sender || other.to == e.Sender {
				count++
			}
			if (other.from == e.Sender && other.to == e.Recipient) || (other.from == e.Recipient && other.to == e.Sender) {
				return protocol.Fail(protocol.InvalidMessage, "a negotiation for this device pair is already active")
			}
		}
		if count >= protocol.MaxSessionsPerDevice {
			return protocol.Fail(protocol.RateLimited, "device session limit")
		}
		recipientCount := 0
		for _, other := range s.sessions {
			if other.from == e.Recipient || other.to == e.Recipient {
				recipientCount++
			}
		}
		if recipientCount >= protocol.MaxSessionsPerDevice {
			return protocol.Fail(protocol.RateLimited, "recipient session limit")
		}
		n = &negotiation{from: e.Sender, to: e.Recipient, generation: e.Generation, expires: now.Add(2 * time.Minute), counts: map[string]int{}}
		s.sessions[e.SessionID] = n
	} else {
		if n == nil || n.generation != e.Generation || !((n.from == e.Sender && n.to == e.Recipient) || (n.from == e.Recipient && n.to == e.Sender)) {
			return protocol.Fail(protocol.InvalidMessage, "unknown or stale session generation")
		}
		if e.Type == "connect_response" {
			if e.Sender != n.to || n.accepted {
				return protocol.Fail(protocol.InvalidMessage, "duplicate or wrong-role response")
			}
			n.accepted = true
		}
		if e.Type == "candidate" {
			n.counts[e.Sender]++
			if n.counts[e.Sender] > protocol.MaxCandidates {
				return protocol.Fail(protocol.RateLimited, "candidate limit")
			}
		}
	}
	copyEnvelope := e
	select {
	case to.send <- Wire{Type: "signal", Message: &copyEnvelope}:
		encoded, _ := json.Marshal(e)
		s.forwardedMessages.Add(1)
		s.forwardedBytes.Add(uint64(len(encoded)))
	default:
		return protocol.Fail(protocol.RateLimited, "recipient queue full")
	}
	return nil
}

// HTTPServer applies transport time limits; the caller owns Listen/Shutdown.
func (s *Server) HTTPServer() *http.Server {
	return &http.Server{Addr: s.cfg.Listen, Handler: s.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 * 1024}
}
func PublicURL(c Config) string {
	scheme := "https"
	if c.AllowInsecureLoopback {
		scheme = "http"
	}
	return fmt.Sprintf("%s://%s", scheme, strings.TrimSpace(c.Listen))
}
