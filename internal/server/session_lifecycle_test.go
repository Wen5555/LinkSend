package server

import (
	"context"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/Wen5555/LinkSend/internal/identity"
	"github.com/Wen5555/LinkSend/internal/protocol"
	"github.com/Wen5555/LinkSend/internal/signaling"
)

func TestReconnectAllowsFreshNegotiationImmediately(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	token := "isolated-session-test-bootstrap-0123456789"
	s, err := New(Config{Listen: "127.0.0.1:0", Database: filepath.Join(t.TempDir(), "control.db"), BootstrapToken: token, AllowInsecureLoopback: true})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	h := httptest.NewServer(s.Handler())
	defer h.Close()
	a, _ := identity.Generate()
	b, _ := identity.Generate()
	ca, err := signaling.New(signaling.Config{ServerURL: h.URL, Identity: a, AllowInsecureLoopback: true})
	if err != nil {
		t.Fatal(err)
	}
	cb, err := signaling.New(signaling.Config{ServerURL: h.URL, Identity: b, AllowInsecureLoopback: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = ca.Bootstrap(ctx, token, "a"); err != nil {
		t.Fatal(err)
	}
	inv, err := ca.CreateInvitation(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = cb.Join(ctx, inv.Token, "b"); err != nil {
		t.Fatal(err)
	}
	receiver, err := cb.Connect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer receiver.Close()
	oldSender, err := ca.Connect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	oldPeer := s.clients[a.ID()]
	s.mu.Unlock()
	oldRequest, err := protocol.NewEnvelope("connect_request", a.ID(), b.ID(), protocol.RandomID(), 1, map[string]string{})
	if err != nil {
		t.Fatal(err)
	}
	if err = oldSender.SendEnvelope(ctx, oldRequest); err != nil {
		t.Fatal(err)
	}
	if wire, readErr := receiver.Read(ctx); readErr != nil || wire.Message == nil || wire.Message.SessionID != oldRequest.SessionID {
		t.Fatalf("initial negotiation rejected: %v", readErr)
	}

	newSender, err := ca.Connect(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer newSender.Close()
	select {
	case <-oldPeer.done:
	case <-ctx.Done():
		t.Fatal("replaced signaling handler did not terminate")
	}
	s.mu.Lock()
	replacement := s.clients[a.ID()]
	s.mu.Unlock()
	if replacement == nil || replacement == oldPeer {
		t.Fatal("old connection cleanup removed or retained the wrong client")
	}

	freshRequest, err := protocol.NewEnvelope("connect_request", a.ID(), b.ID(), protocol.RandomID(), 1, map[string]string{})
	if err != nil {
		t.Fatal(err)
	}
	if err = newSender.SendEnvelope(ctx, freshRequest); err != nil {
		t.Fatal(err)
	}
	if wire, readErr := receiver.Read(ctx); readErr != nil || wire.Message == nil || wire.Message.SessionID != freshRequest.SessionID {
		t.Fatalf("fresh negotiation rejected after old callback: %v", readErr)
	}
	end, err := protocol.NewEnvelope("end_of_candidates", a.ID(), b.ID(), freshRequest.SessionID, 1, map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if err = newSender.SendEnvelope(ctx, end); err != nil {
		t.Fatal(err)
	}
	if wire, readErr := receiver.Read(ctx); readErr != nil || wire.Message == nil || wire.Message.SessionID != freshRequest.SessionID {
		t.Fatalf("old callback deleted fresh negotiation state: %v", readErr)
	}
	receiverEnd, err := protocol.NewEnvelope("end_of_candidates", b.ID(), a.ID(), freshRequest.SessionID, 1, map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if err = receiver.SendEnvelope(ctx, receiverEnd); err != nil {
		t.Fatal(err)
	}
	if wire, readErr := newSender.Read(ctx); readErr != nil || wire.Message == nil || wire.Message.SessionID != freshRequest.SessionID {
		t.Fatalf("second end_of_candidates was not forwarded: %v", readErr)
	}
	reusedRequest, err := protocol.NewEnvelope("connect_request", a.ID(), b.ID(), protocol.RandomID(), 1, map[string]string{})
	if err != nil {
		t.Fatal(err)
	}
	if err = newSender.SendEnvelope(ctx, reusedRequest); err != nil {
		t.Fatal(err)
	}
	if wire, readErr := receiver.Read(ctx); readErr != nil || wire.Message == nil || wire.Message.SessionID != reusedRequest.SessionID {
		t.Fatalf("completed candidate exchange prevented WSS reuse: %v", readErr)
	}
}
