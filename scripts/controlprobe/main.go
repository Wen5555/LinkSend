// controlprobe exercises the production HTTPS/WSS membership client against an
// explicitly selected service. It never emits invitation secrets or file data.
package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/Wen5555/LinkSend/internal/identity"
	"github.com/Wen5555/LinkSend/internal/protocol"
	"github.com/Wen5555/LinkSend/internal/signaling"
)

type state struct {
	AID, BID, GroupID, AIncarnation, BIncarnation string
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	serverURL := flag.String("server", "", "explicit HTTPS service URL")
	root := flag.String("fixture", "", "dedicated fixture directory (keys stay here)")
	phase := flag.String("phase", "", "setup, verify, revoke or verify-revoked")
	caFile := flag.String("ca-file", "", "explicit CA for origin/isolated restore checks only")
	dialIP := flag.String("dial-ip", "", "test-client-only authority override; requires ca-file")
	flag.Parse()
	u, err := url.Parse(*serverURL)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || *root == "" || (*phase != "setup" && *phase != "verify" && *phase != "revoke" && *phase != "verify-revoked") || (*dialIP != "" && (*caFile == "" || net.ParseIP(*dialIP) == nil)) {
		return errors.New("require explicit HTTPS, fixture, known phase and verified CA for any dial override")
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	defer transport.CloseIdleConnections()
	if *caFile != "" {
		pem, readErr := os.ReadFile(*caFile)
		if readErr != nil {
			return readErr
		}
		roots := x509.NewCertPool()
		if !roots.AppendCertsFromPEM(pem) {
			return errors.New("explicit CA contains no certificates")
		}
		transport.TLSClientConfig = &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}
	}
	if *dialIP != "" {
		transport.Proxy = nil
		transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, splitErr := net.SplitHostPort(address)
			if splitErr != nil || host != u.Hostname() {
				return nil, errors.New("unexpected authority in scoped dial override")
			}
			return (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, network, net.JoinHostPort(*dialIP, port))
		}
	}
	statePath := filepath.Join(*root, "probe-state.json")
	if *phase == "setup" {
		if _, statErr := os.Lstat(statePath); statErr == nil || !errors.Is(statErr, os.ErrNotExist) {
			return errors.New("setup state already exists; use verify instead")
		}
	}
	if err = os.MkdirAll(*root, 0700); err != nil {
		return err
	}
	if *phase != "setup" {
		for _, peer := range []string{"a", "b"} {
			if info, statErr := os.Lstat(filepath.Join(*root, peer, "identity.key")); statErr != nil || !info.Mode().IsRegular() {
				return errors.New("fixture identity is missing; verify must not create replacement keys")
			}
		}
	}
	a, err := identity.LoadOrCreate(filepath.Join(*root, "a"))
	if err != nil {
		return err
	}
	b, err := identity.LoadOrCreate(filepath.Join(*root, "b"))
	if err != nil {
		return err
	}
	client := func(id *identity.Identity) (*signaling.Client, error) {
		return signaling.New(signaling.Config{ServerURL: *serverURL, Identity: id, HTTPClient: &http.Client{Transport: transport, Timeout: 10 * time.Second}})
	}
	ca, err := client(a)
	if err != nil {
		return err
	}
	cb, err := client(b)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	capabilities, err := ca.Health(ctx)
	if err != nil {
		return fmt.Errorf("verified health check failed: %w", err)
	}
	if capabilities.Relay || capabilities.MembershipVersion != 2 {
		return errors.New("unsupported relay/membership capabilities")
	}
	var saved state
	replayRejected := false
	if *phase == "setup" {
		first, initErr := ca.Initialize(ctx, "AE-control-A")
		if initErr != nil {
			return fmt.Errorf("initialize: %w", initErr)
		}
		invitation, inviteErr := ca.CreateInvitation(ctx)
		if inviteErr != nil {
			return fmt.Errorf("invitation: %w", inviteErr)
		}
		second, joinErr := cb.Join(ctx, invitation.Token, "AE-control-B")
		if joinErr != nil {
			return fmt.Errorf("join: %w", joinErr)
		}
		third, keyErr := identity.Generate()
		if keyErr != nil {
			return keyErr
		}
		cc, keyErr := client(third)
		if keyErr != nil {
			return keyErr
		}
		if _, replayErr := cc.Join(ctx, invitation.Token, "AE-replay-rejected"); protocol.ErrorCode(replayErr) != protocol.PairingCodeUsed {
			return errors.New("consumed invitation did not return PAIRING_CODE_USED")
		}
		replayRejected = true
		saved = state{AID: first.ID, BID: second.ID, GroupID: first.GroupID, AIncarnation: first.Incarnation, BIncarnation: second.Incarnation}
		if first.GroupID != second.GroupID || first.ID != a.ID() || second.ID != b.ID() {
			return errors.New("initialized/joined group identity mismatch")
		}
		data, encodeErr := json.Marshal(saved)
		if encodeErr != nil {
			return encodeErr
		}
		file, openErr := os.OpenFile(statePath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if openErr != nil {
			return openErr
		}
		_, writeErr := file.Write(data)
		if err = errors.Join(writeErr, file.Sync(), file.Close()); err != nil {
			return err
		}
	} else {
		body, readErr := os.ReadFile(statePath)
		if readErr != nil || len(body) > 4096 {
			return errors.New("missing/bounded fixture state")
		}
		if err = json.Unmarshal(body, &saved); err != nil {
			return err
		}
	}
	if saved.AID != a.ID() || saved.BID != b.ID() {
		return errors.New("fixture keys differ from saved identities")
	}
	if *phase == "revoke" {
		if err = ca.Revoke(ctx, saved.BID); err != nil {
			return fmt.Errorf("revoke: %w", err)
		}
	}
	revoked := *phase == "revoke" || *phase == "verify-revoked"
	devices, err := ca.Devices(ctx)
	if err != nil {
		return err
	}
	expected := 2
	if revoked {
		expected = 1
	}
	if len(devices) != expected {
		return fmt.Errorf("unexpected visible membership count: %d", len(devices))
	}
	for _, device := range devices {
		if device.GroupID != saved.GroupID || device.ID == saved.AID && (device.Incarnation != saved.AIncarnation || !bytes.Equal(device.PublicKey, a.PublicKey())) || device.ID == saved.BID && (device.Incarnation != saved.BIncarnation || !bytes.Equal(device.PublicKey, b.PublicKey())) || device.ID != saved.AID && device.ID != saved.BID {
			return errors.New("persisted membership, incarnation or identity pin changed")
		}
	}
	if revoked {
		if _, err = cb.Devices(ctx); protocol.ErrorCode(err) != protocol.AuthenticationFailed {
			return fmt.Errorf("revoked identity was not rejected: %w", err)
		}
	} else {
		sa, connectErr := ca.Connect(ctx)
		if connectErr != nil {
			return fmt.Errorf("WSS A: %w", connectErr)
		}
		defer sa.Close()
		sb, connectErr := cb.Connect(ctx)
		if connectErr != nil {
			return fmt.Errorf("WSS B: %w", connectErr)
		}
		defer sb.Close()
		if err = errors.Join(sa.SendHeartbeat(ctx), sb.SendHeartbeat(ctx)); err != nil {
			return err
		}
		devices, err = ca.Devices(ctx)
		if err != nil || len(devices) != 2 || !devices[0].Online || !devices[1].Online {
			return errors.New("authenticated WSS presence did not become online")
		}
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{"phase": *phase, "tls_verified": true, "members": expected, "membership_identity_unchanged": true, "relay": false, "wss_two_authenticated": !revoked, "consumed_invitation_rejected": replayRejected, "revocation_verified": revoked})
}
