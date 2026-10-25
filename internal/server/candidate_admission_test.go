package server

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/Wen5555/LinkSend/internal/identity"
	"github.com/Wen5555/LinkSend/internal/protocol"
	"github.com/Wen5555/LinkSend/internal/signaling"
)

func TestCandidateForwardAdmitsUnicastSuffixAndPreservesAuthorization(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	s, err := New(Config{Listen: "127.0.0.1:0", Database: filepath.Join(t.TempDir(), "control.db"), AllowInsecureLoopback: true})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	h := httptest.NewServer(s.Handler())
	defer h.Close()
	newClient := func() (*identity.Identity, *signaling.Client) {
		t.Helper()
		id, generateErr := identity.Generate()
		if generateErr != nil {
			t.Fatal(generateErr)
		}
		client, createErr := signaling.New(signaling.Config{ServerURL: h.URL, Identity: id, AllowInsecureLoopback: true})
		if createErr != nil {
			t.Fatal(createErr)
		}
		return id, client
	}
	a, ca := newClient()
	b, cb := newClient()
	other, co := newClient()
	if _, err = ca.Initialize(ctx, "candidate sender"); err != nil {
		t.Fatal(err)
	}
	invitation, err := ca.CreateInvitation(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = cb.Join(ctx, invitation.Token, "candidate recipient"); err != nil {
		t.Fatal(err)
	}
	if _, err = co.Initialize(ctx, "other group"); err != nil {
		t.Fatal(err)
	}
	sender, err := ca.Connect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer sender.Close()
	receiver, err := cb.Connect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer receiver.Close()
	session := protocol.RandomID()
	request, err := protocol.NewEnvelope("connect_request", a.ID(), b.ID(), session, 1, map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if err = sender.SendEnvelope(ctx, request); err != nil {
		t.Fatal(err)
	}
	if wire, readErr := receiver.Read(ctx); readErr != nil || wire.Message == nil || wire.Message.SessionID != session {
		t.Fatalf("authenticated session setup failed: %v", readErr)
	}
	s.mu.Lock()
	from := s.clients[a.ID()]
	s.mu.Unlock()
	if from == nil {
		t.Fatal("authenticated sender not registered")
	}
	makeCandidate := func(value string) protocol.Envelope {
		t.Helper()
		envelope, createErr := protocol.NewEnvelope("candidate", a.ID(), b.ID(), session, 1, protocol.Candidate{Candidate: value})
		if createErr != nil {
			t.Fatal(createErr)
		}
		return envelope
	}
	base := "1 1 udp 2130706431 10.0.0.255 3478 typ host"
	before := s.Counters().ForwardedMessages
	for _, fixture := range []struct {
		name   string
		change func(*protocol.Envelope)
		signer *identity.Identity
		code   protocol.Code
	}{
		{"wrong signature", func(*protocol.Envelope) {}, b, protocol.AuthenticationFailed},
		{"wrong sender", func(e *protocol.Envelope) { e.Sender = b.ID() }, b, protocol.AuthenticationFailed},
		{"unknown session", func(e *protocol.Envelope) { e.SessionID = protocol.RandomID() }, a, protocol.InvalidMessage},
		{"stale generation", func(e *protocol.Envelope) { e.Generation = 2 }, a, protocol.InvalidMessage},
		{"outside group", func(e *protocol.Envelope) { e.Recipient = other.ID() }, a, protocol.AuthenticationFailed},
		{"limited broadcast", func(e *protocol.Envelope) {
			e.Payload, _ = json.Marshal(protocol.Candidate{Candidate: "1 1 udp 2130706431 255.255.255.255 3478 typ host"})
		}, a, protocol.InvalidMessage},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			envelope := makeCandidate(base)
			fixture.change(&envelope)
			envelope.Signature = fixture.signer.Sign(envelope.SigningBytes())
			if forwardErr := s.forward(ctx, from, envelope); forwardErr == nil || protocol.ErrorCode(forwardErr) != fixture.code {
				t.Fatalf("candidate policy bypass: %v", forwardErr)
			}
			if got := s.Counters().ForwardedMessages; got != before {
				t.Fatalf("rejected candidate was forwarded: %d != %d", got, before)
			}
		})
	}
	for _, value := range []string{
		base,
		"1 1 udp 2130706431 172.16.0.255 3478 typ srflx",
		"1 1 udp 2130706431 203.0.112.255 3478 typ prflx",
	} {
		envelope := makeCandidate(value)
		envelope.Signature = a.Sign(envelope.SigningBytes())
		if err = s.forward(ctx, from, envelope); err != nil {
			t.Fatalf("signed unicast candidate rejected by production forward path: %v", err)
		}
		wire, readErr := receiver.Read(ctx)
		if readErr != nil || wire.Message == nil || wire.Message.MessageID != envelope.MessageID || wire.Message.SessionID != session || wire.Message.Generation != 1 {
			t.Fatalf("candidate forwarding lost original session binding: %v", readErr)
		}
		if err = wire.Message.Verify(a.PublicKey(), time.Now()); err != nil {
			t.Fatalf("forwarded candidate signature invalid: %v", err)
		}
		var got protocol.Candidate
		if err = json.Unmarshal(wire.Message.Payload, &got); err != nil || got.Candidate != value {
			t.Fatalf("forwarded candidate changed: %v", err)
		}
	}
}
