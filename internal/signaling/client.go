// Package signaling implements the authenticated HTTP and WSS control-plane
// client. It transfers only bounded control messages; file bytes never enter
// this package.
package signaling

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"net/url"
	"path"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Wen5555/LinkSend/internal/identity"
	"github.com/Wen5555/LinkSend/internal/protocol"
	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

const subprotocol = "linksend.signal.v1"

// Config separates the explicit development HTTP escape hatch from the normal
// HTTPS/WSS path. Plain HTTP is accepted only for a literal loopback address.
type Config struct {
	ServerURL             string
	Identity              *identity.Identity
	AllowInsecureLoopback bool
	HTTPClient            *http.Client
}

// Device mirrors the bounded public device representation returned by the
// rendezvous service. Membership is deliberately not equivalent to local trust.
type Device struct {
	ID        string `json:"id"`
	GroupID   string `json:"group_id"`
	Name      string `json:"name"`
	PublicKey []byte `json:"public_key"`
	Admin     bool   `json:"admin"`
	Revoked   bool   `json:"revoked"`
	Online    bool   `json:"online"`
}

func (d Device) Validate() error {
	if len(d.ID) != 64 || len(d.PublicKey) != ed25519.PublicKeySize || identity.DeviceID(d.PublicKey) != d.ID || d.Name == "" || len(d.Name) > 128 {
		return protocol.Fail(protocol.InvalidMessage, "invalid device response")
	}
	return nil
}

type Invitation struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

// Wire is the versioned websocket envelope shared with the server. It has no
// data-plane field by design.
type Wire struct {
	Type         string                 `json:"type"`
	Nonce        string                 `json:"nonce,omitempty"`
	DeviceID     string                 `json:"device_id,omitempty"`
	Signature    []byte                 `json:"signature,omitempty"`
	Message      *protocol.Envelope     `json:"message,omitempty"`
	Error        *protocol.Error        `json:"error,omitempty"`
	Capabilities *protocol.Capabilities `json:"capabilities,omitempty"`
}

// RemoteError retains the server's stable protocol code and HTTP status.
type RemoteError struct {
	Status int
	Cause  protocol.Error
}

func (e *RemoteError) Error() string {
	return fmt.Sprintf("signaling HTTP %d: %s", e.Status, e.Cause.Error())
}
func (e *RemoteError) Unwrap() error { return &e.Cause }

// Client owns the long-term device identity used to authenticate every HTTP
// and WSS control-plane action. It intentionally has no peer-trust policy.
type Client struct {
	base     *url.URL
	identity *identity.Identity
	http     *http.Client
}

func New(cfg Config) (*Client, error) {
	if cfg.Identity == nil {
		return nil, errors.New("device identity is required")
	}
	base, err := parseBaseURL(cfg.ServerURL, cfg.AllowInsecureLoopback)
	if err != nil {
		return nil, err
	}
	h := cfg.HTTPClient
	if h == nil {
		h = &http.Client{Timeout: 15 * time.Second}
	}
	clone := *h
	clone.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &Client{base: base, identity: cfg.Identity, http: &clone}, nil
}

func parseBaseURL(raw string, allowInsecureLoopback bool) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || u == nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, errors.New("invalid signaling server URL")
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return nil, errors.New("signaling server URL must use https or explicit loopback http")
	}
	if u.Path != "" && u.Path != "/" {
		return nil, errors.New("signaling server URL must not contain a path")
	}
	if u.Scheme == "http" {
		ip, parseErr := netip.ParseAddr(u.Hostname())
		if !allowInsecureLoopback || parseErr != nil || !ip.IsLoopback() {
			return nil, errors.New("plain HTTP is allowed only for explicit loopback development")
		}
	}
	u.Path = ""
	return u, nil
}

func (c *Client) ServerURL() string { return c.base.String() }

func (c *Client) endpoint(p string) (string, error) {
	if !strings.HasPrefix(p, "/") || path.Clean(p) != p {
		return "", errors.New("invalid signaling endpoint path")
	}
	u := *c.base
	u.Path = p
	return u.String(), nil
}

func (c *Client) signedRequest(ctx context.Context, method, p string, body []byte) (*http.Response, error) {
	endpoint, err := c.endpoint(p)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	if len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	nonce := protocol.RandomID()
	at := time.Now().Unix()
	id := c.identity.ID()
	sig := c.identity.Sign(protocol.AuthBytes(method, p, id, nonce, at, body))
	req.Header.Set("X-LinkSend-Device", id)
	req.Header.Set("X-LinkSend-Nonce", nonce)
	req.Header.Set("X-LinkSend-Time", fmt.Sprintf("%d", at))
	req.Header.Set("X-LinkSend-Signature", base64.RawStdEncoding.EncodeToString(sig))
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, protocol.Wrap(protocol.SignalingUnreachable, "request failed", err)
	}
	return resp, nil
}

func decodeError(resp *http.Response) error {
	defer resp.Body.Close()
	b, err := readLimitedJSON(resp.Body, protocol.MaxMessageBytes)
	if err != nil {
		return &RemoteError{Status: resp.StatusCode, Cause: protocol.Error{Code: protocol.InvalidMessage, Detail: "invalid signaling error response"}}
	}
	var e protocol.Error
	if json.Unmarshal(b, &e) == nil && e.Code != "" {
		return &RemoteError{Status: resp.StatusCode, Cause: e}
	}
	return &RemoteError{Status: resp.StatusCode, Cause: protocol.Error{Code: protocol.InvalidMessage, Detail: "unrecognized signaling error"}}
}

func decodeSuccess(resp *http.Response, out any) error {
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return decodeError(resp)
	}
	if out == nil {
		return nil
	}
	b, err := readLimitedJSON(resp.Body, protocol.MaxPayloadBytes)
	if err != nil {
		return protocol.Wrap(protocol.InvalidMessage, "invalid signaling response", err)
	}
	if err = json.Unmarshal(b, out); err != nil {
		return protocol.Wrap(protocol.InvalidMessage, "invalid signaling response", err)
	}
	return nil
}

func readLimitedJSON(r io.Reader, limit int) ([]byte, error) {
	if limit <= 0 {
		return nil, errors.New("invalid body limit")
	}
	b, err := io.ReadAll(io.LimitReader(r, int64(limit)+1))
	if err != nil {
		return nil, err
	}
	if len(b) == 0 || len(b) > limit || !json.Valid(b) {
		return nil, errors.New("invalid JSON response body")
	}
	return b, nil
}

type registration struct {
	Token     string `json:"token"`
	Name      string `json:"name"`
	PublicKey []byte `json:"public_key"`
	Signature []byte `json:"signature"`
}

func (c *Client) register(ctx context.Context, p, token, name string) (Device, error) {
	if token == "" || name == "" || len(name) > 128 {
		return Device{}, protocol.Fail(protocol.InvalidMessage, "registration token and name are required")
	}
	req := registration{Token: token, Name: name, PublicKey: c.identity.PublicKey()}
	req.Signature = c.identity.Sign(protocol.AuthBytes(http.MethodPost, p, base64.RawStdEncoding.EncodeToString(req.PublicKey), req.Token, 0, []byte(req.Name)))
	body, err := json.Marshal(req)
	if err != nil {
		return Device{}, err
	}
	endpoint, err := c.endpoint(p)
	if err != nil {
		return Device{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return Device{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(httpReq)
	if err != nil {
		return Device{}, protocol.Wrap(protocol.SignalingUnreachable, "registration request failed", err)
	}
	var d Device
	if err = decodeSuccess(resp, &d); err != nil {
		return Device{}, err
	}
	if err = d.Validate(); err != nil {
		return Device{}, err
	}
	if d.ID != c.identity.ID() {
		return Device{}, protocol.Fail(protocol.AuthenticationFailed, "server returned another device identity")
	}
	return d, nil
}

func (c *Client) Bootstrap(ctx context.Context, token, name string) (Device, error) {
	return c.register(ctx, "/v1/bootstrap", token, name)
}

func (c *Client) Join(ctx context.Context, token, name string) (Device, error) {
	return c.register(ctx, "/v1/pairing/join", token, name)
}

func (c *Client) CreateInvitation(ctx context.Context) (Invitation, error) {
	resp, err := c.signedRequest(ctx, http.MethodPost, "/v1/pairing/invitations", nil)
	if err != nil {
		return Invitation{}, err
	}
	var invitation Invitation
	if err = decodeSuccess(resp, &invitation); err != nil {
		return Invitation{}, err
	}
	if len(invitation.Token) != 43 || invitation.ExpiresAt.Before(time.Now()) {
		return Invitation{}, protocol.Fail(protocol.InvalidMessage, "invalid invitation response")
	}
	return invitation, nil
}

func (c *Client) Devices(ctx context.Context) ([]Device, error) {
	resp, err := c.signedRequest(ctx, http.MethodGet, "/v1/devices", nil)
	if err != nil {
		return nil, err
	}
	var devices []Device
	if err = decodeSuccess(resp, &devices); err != nil {
		return nil, err
	}
	for _, d := range devices {
		if err = d.Validate(); err != nil {
			return nil, err
		}
	}
	return devices, nil
}

func (c *Client) Revoke(ctx context.Context, deviceID string) error {
	if len(deviceID) != 64 {
		return protocol.Fail(protocol.InvalidMessage, "invalid device id")
	}
	resp, err := c.signedRequest(ctx, http.MethodDelete, "/v1/devices/"+deviceID, nil)
	if err != nil {
		return err
	}
	return decodeSuccess(resp, nil)
}

func (c *Client) Health(ctx context.Context) (protocol.Capabilities, error) {
	endpoint, err := c.endpoint("/healthz")
	if err != nil {
		return protocol.Capabilities{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return protocol.Capabilities{}, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return protocol.Capabilities{}, protocol.Wrap(protocol.SignalingUnreachable, "health request failed", err)
	}
	var body struct {
		Status       string                `json:"status"`
		Capabilities protocol.Capabilities `json:"capabilities"`
	}
	if err = decodeSuccess(resp, &body); err != nil {
		return protocol.Capabilities{}, err
	}
	if body.Status != "ok" || !supported(body.Capabilities) {
		return protocol.Capabilities{}, protocol.Fail(protocol.VersionIncompatible, "server capabilities incompatible")
	}
	return body.Capabilities, nil
}

func supported(c protocol.Capabilities) bool {
	return c.ProtocolVersion == protocol.Version && !c.Relay && c.Transport == "quic"
}

func (c *Client) websocketURL() string {
	u := *c.base
	if u.Scheme == "http" {
		u.Scheme = "ws"
	} else {
		u.Scheme = "wss"
	}
	u.Path = "/v1/ws"
	return u.String()
}

// Connect opens a WSS connection and proves possession of the enrolled device
// private key. It does not itself accept remote device identities as trusted.
func (c *Client) Connect(ctx context.Context) (*Session, error) {
	conn, response, err := websocket.Dial(ctx, c.websocketURL(), &websocket.DialOptions{HTTPClient: c.http, Subprotocols: []string{subprotocol}, CompressionMode: websocket.CompressionDisabled})
	if err != nil {
		if response != nil {
			defer response.Body.Close()
			return nil, decodeError(response)
		}
		return nil, protocol.Wrap(protocol.SignalingUnreachable, "WSS connection failed", err)
	}
	conn.SetReadLimit(protocol.MaxMessageBytes)
	closeOnError := func(e error) (*Session, error) {
		_ = conn.Close(websocket.StatusPolicyViolation, "authentication failed")
		return nil, e
	}
	handshakeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var hello Wire
	if err = wsjson.Read(handshakeCtx, conn, &hello); err != nil {
		return closeOnError(protocol.Wrap(protocol.SignalingUnreachable, "WSS hello missing", err))
	}
	if hello.Type != "hello" || len(hello.Nonce) != 64 || hello.Capabilities == nil || !supported(*hello.Capabilities) {
		return closeOnError(protocol.Fail(protocol.VersionIncompatible, "WSS server capabilities incompatible"))
	}
	auth := Wire{Type: "authenticate", DeviceID: c.identity.ID(), Signature: c.identity.Sign(protocol.ChallengeBytes(c.identity.ID(), hello.Nonce))}
	if err = wsjson.Write(handshakeCtx, conn, auth); err != nil {
		return closeOnError(protocol.Wrap(protocol.SignalingUnreachable, "WSS authentication write failed", err))
	}
	var accepted Wire
	if err = wsjson.Read(handshakeCtx, conn, &accepted); err != nil {
		return closeOnError(protocol.Wrap(protocol.SignalingUnreachable, "WSS authentication response missing", err))
	}
	if accepted.Type != "authenticated" || accepted.DeviceID != c.identity.ID() || accepted.Capabilities == nil || !supported(*accepted.Capabilities) {
		return closeOnError(protocol.Fail(protocol.AuthenticationFailed, "WSS device authentication rejected"))
	}
	return &Session{conn: conn, identity: c.identity, capabilities: *accepted.Capabilities}, nil
}

// Session has one reader at a time. Writes are serialized to preserve control
// message order even though coder/websocket permits concurrent Write calls.
type Session struct {
	conn          *websocket.Conn
	identity      *identity.Identity
	capabilities  protocol.Capabilities
	writeMu       sync.Mutex
	closeOnce     sync.Once
	bytesSent     atomic.Uint64
	bytesReceived atomic.Uint64
}

type SessionStats struct {
	BytesSent     uint64 `json:"bytes_sent"`
	BytesReceived uint64 `json:"bytes_received"`
}

func (s *Session) Capabilities() protocol.Capabilities { return s.capabilities }
func (s *Session) Stats() SessionStats {
	return SessionStats{BytesSent: s.bytesSent.Load(), BytesReceived: s.bytesReceived.Load()}
}

func (s *Session) SendEnvelope(ctx context.Context, env protocol.Envelope) error {
	if env.Sender != s.identity.ID() || len(env.Signature) != 0 {
		return protocol.Fail(protocol.AuthenticationFailed, "outbound envelope sender or signature invalid")
	}
	env.Signature = s.identity.Sign(env.SigningBytes())
	if err := env.Verify(s.identity.PublicKey(), time.Now()); err != nil {
		return err
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	wire := Wire{Type: "signal", Message: &env}
	if err := wsjson.Write(ctx, s.conn, wire); err != nil {
		return protocol.Wrap(protocol.SignalingUnreachable, "WSS signal write failed", err)
	}
	if encoded, err := json.Marshal(wire); err == nil {
		s.bytesSent.Add(uint64(len(encoded)))
	}
	return nil
}

func (s *Session) SendHeartbeat(ctx context.Context) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	wire := Wire{Type: "heartbeat"}
	if err := wsjson.Write(ctx, s.conn, wire); err != nil {
		return protocol.Wrap(protocol.SignalingUnreachable, "WSS heartbeat write failed", err)
	}
	if encoded, err := json.Marshal(wire); err == nil {
		s.bytesSent.Add(uint64(len(encoded)))
	}
	return nil
}

func (s *Session) Read(ctx context.Context) (Wire, error) {
	var wire Wire
	if err := wsjson.Read(ctx, s.conn, &wire); err != nil {
		return wire, protocol.Wrap(protocol.SignalingUnreachable, "WSS signal read failed", err)
	}
	if encoded, err := json.Marshal(wire); err == nil {
		s.bytesReceived.Add(uint64(len(encoded)))
	}
	switch wire.Type {
	case "heartbeat":
		return wire, nil
	case "signal":
		if wire.Message == nil {
			return wire, protocol.Fail(protocol.InvalidMessage, "missing WSS signal envelope")
		}
		return wire, nil
	case "error":
		if wire.Error == nil {
			return wire, protocol.Fail(protocol.InvalidMessage, "missing WSS error")
		}
		return wire, wire.Error
	default:
		return wire, protocol.Fail(protocol.InvalidMessage, "unknown WSS message type")
	}
}

func (s *Session) Close() error {
	var err error
	s.closeOnce.Do(func() { err = s.conn.Close(websocket.StatusNormalClosure, "closed") })
	return err
}
