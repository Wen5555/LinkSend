//go:build windows

package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wen5555/LinkSend/apps/desktop/nativeclipboard"
	coreapp "github.com/Wen5555/LinkSend/internal/app"
	"github.com/Wen5555/LinkSend/internal/clipboardsync"
	"github.com/Wen5555/LinkSend/internal/identity"
	"github.com/Wen5555/LinkSend/internal/protocol"
	"github.com/Wen5555/LinkSend/internal/server"
)

type temporaryClipboardFixtureState struct {
	APresent, ASendReady, AReceiveReady, AWaiting, AError bool
	BPresent, BSendReady, BReceiveReady, BWaiting, BError bool
	ASignaling, AListening, BSignaling, BListening        bool
}

func temporaryClipboardFixtureStatus(a, b *coreapp.Service, aID, bID string) temporaryClipboardFixtureState {
	var state temporaryClipboardFixtureState
	for _, status := range a.ClipboardPeerStatuses() {
		if status.PeerID == bID {
			state.APresent, state.ASendReady, state.AReceiveReady = true, status.SendReady, status.ReceiveReady
			state.AWaiting, state.AError = status.Waiting != "", status.Error != ""
		}
	}
	for _, status := range b.ClipboardPeerStatuses() {
		if status.PeerID == aID {
			state.BPresent, state.BSendReady, state.BReceiveReady = true, status.SendReady, status.ReceiveReady
			state.BWaiting, state.BError = status.Waiting != "", status.Error != ""
		}
	}
	aInbox, bInbox := a.InboxStatus(), b.InboxStatus()
	state.ASignaling, state.AListening = aInbox.SignalingConnected, aInbox.Listening
	state.BSignaling, state.BListening = bInbox.SignalingConnected, bInbox.Listening
	return state
}

// A local reset cannot mint the remote send lease. Wait for the real owners
// under the fixture's existing total deadline, including the normal remote
// renewal after resume, rather than imposing a shorter three-second contract.
func waitTemporaryClipboardFixtureReady(t *testing.T, ctx context.Context, phase string, a, b *coreapp.Service, aID, bID string) {
	t.Helper()
	started := time.Now()
	if _, bounded := ctx.Deadline(); !bounded {
		t.Fatal("clipboard fixture requires a bounded parent context")
	}
	t.Logf("clipboard fixture phase=%s started", phase)
	for {
		state := temporaryClipboardFixtureStatus(a, b, aID, bID)
		if state.ASendReady && state.AReceiveReady {
			t.Logf("clipboard fixture phase=%s ready elapsed_ms=%d state=%+v", phase, time.Since(started).Milliseconds(), state)
			return
		}
		if ctx.Err() != nil {
			t.Fatalf("clipboard fixture phase=%s not ready elapsed_ms=%d state=%+v", phase, time.Since(started).Milliseconds(), state)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func waitTemporaryClipboardFixtureInbox(t *testing.T, ctx context.Context, receiver *coreapp.Service) {
	t.Helper()
	started := time.Now()
	if _, bounded := ctx.Deadline(); !bounded {
		t.Fatal("clipboard fixture requires a bounded parent context")
	}
	for {
		status := receiver.InboxStatus()
		if status.SignalingConnected && status.Listening {
			t.Logf("clipboard fixture phase=receiver_registration ready elapsed_ms=%d", time.Since(started).Milliseconds())
			return
		}
		if ctx.Err() != nil {
			t.Fatalf("clipboard fixture phase=receiver_registration not ready signaling=%t listening=%t error_present=%t", status.SignalingConnected, status.Listening, status.LastError != "")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestDesktopTemporaryClipboardFixtureWaitsForInboxRegistration(t *testing.T) {
	root := t.TempDir()
	backend, err := server.New(server.Config{Listen: "127.0.0.1:0", Database: filepath.Join(root, "server.sqlite"), BootstrapToken: "memory-inbox-fixture-token-0123456789", AllowInsecureLoopback: true, AllowLoopbackCandidates: true})
	if err != nil {
		t.Fatal("fixture control unavailable")
	}
	t.Cleanup(func() { _ = backend.Close() })
	registrationStarted, allowRegistration := make(chan struct{}), make(chan struct{})
	var started, released sync.Once
	release := func() { released.Do(func() { close(allowRegistration) }) }
	handler := backend.Handler()
	h := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/ws" {
			started.Do(func() { close(registrationStarted) })
			select {
			case <-allowRegistration:
			case <-r.Context().Done():
				return
			}
		}
		handler.ServeHTTP(w, r)
	}))
	t.Cleanup(h.Close)
	receiver, err := coreapp.New(coreapp.Config{DataDir: filepath.Join(root, "receiver"), ServerURL: h.URL, AllowInsecureLoopback: true})
	if err != nil {
		t.Fatal("fixture receiver unavailable")
	}
	t.Cleanup(receiver.Shutdown)
	t.Cleanup(release)
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	t.Cleanup(cancel)
	if _, err = receiver.Bootstrap(ctx, "memory-inbox-fixture-token-0123456789", "memory-receiver"); err != nil {
		t.Fatal("fixture bootstrap", protocol.ErrorCode(err))
	}
	if err = receiver.StartInbox(filepath.Join(root, "received"), coreapp.DirectConfig{BindAddress: "127.0.0.1:0", AllowLoopback: true, CheckTimeout: 5 * time.Second}); err != nil {
		t.Fatal("fixture inbox", protocol.ErrorCode(err))
	}
	select {
	case <-registrationStarted:
	case <-ctx.Done():
		t.Fatal("fixture did not reach controlled receiver registration")
	}
	status := receiver.InboxStatus()
	if !status.Enabled || status.SignalingConnected || status.Listening {
		t.Fatal("StartInbox return incorrectly implied receiver registration")
	}
	t.Log("clipboard fixture phase=receiver_registration held enabled=true signaling=false listening=false")
	release()
	waitTemporaryClipboardFixtureInbox(t, ctx, receiver)
}

type temporaryMemoryClipboard struct {
	mu         sync.Mutex
	generation uint64
	body       string
	reads      atomic.Int32
}

func (b *temporaryMemoryClipboard) current() uint64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.generation
}
func (b *temporaryMemoryClipboard) write(body string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.generation++
	b.body = body
	return true
}
func (b *temporaryMemoryClipboard) read(_ context.Context, _ clipboardsync.Kind, generation uint64) ([]byte, uint64, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.reads.Add(1)
	if generation != b.generation {
		return nil, b.generation, clipboardsync.ErrStale
	}
	return []byte(b.body), b.generation, nil
}

// This exercises the real core lease owners and desktop copy/delivery bridge.
// Every clipboard byte and generation belongs to an in-memory adapter; no
// native clipboard API, watcher, opt-in flag or user clipboard is involved.
func TestDesktopTemporaryClipboardFixtureResumeWaitsForRemoteLease(t *testing.T) {
	root := t.TempDir()
	backend, err := server.New(server.Config{Listen: "127.0.0.1:0", Database: filepath.Join(root, "server.sqlite"), BootstrapToken: "memory-clipboard-fixture-token-0123456789", AllowInsecureLoopback: true, AllowLoopbackCandidates: true, ExperimentalShareControl: true})
	if err != nil {
		t.Fatal("fixture control unavailable")
	}
	t.Cleanup(func() { _ = backend.Close() })
	var signalingConnections atomic.Int32
	handler := backend.Handler()
	h := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/ws" {
			signalingConnections.Add(1)
		}
		handler.ServeHTTP(w, r)
	}))
	t.Cleanup(h.Close)
	aID, _ := identity.Generate()
	bID, _ := identity.Generate()
	a, err := coreapp.New(coreapp.Config{DataDir: filepath.Join(root, "a"), ServerURL: h.URL, Identity: aID, AllowInsecureLoopback: true})
	if err != nil {
		t.Fatal("fixture sender unavailable")
	}
	t.Cleanup(a.Shutdown)
	b, err := coreapp.New(coreapp.Config{DataDir: filepath.Join(root, "b"), ServerURL: h.URL, Identity: bID, AllowInsecureLoopback: true})
	if err != nil {
		t.Fatal("fixture receiver unavailable")
	}
	t.Cleanup(b.Shutdown)
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	t.Cleanup(cancel)
	if _, err = a.Bootstrap(ctx, "memory-clipboard-fixture-token-0123456789", "memory-sender"); err != nil {
		t.Fatal("fixture bootstrap", protocol.ErrorCode(err))
	}
	invitation, err := a.CreateInvitation(ctx)
	if err != nil {
		t.Fatal("fixture invitation", protocol.ErrorCode(err))
	}
	if _, err = b.Join(ctx, invitation.Token, "memory-receiver"); err != nil {
		t.Fatal("fixture join", protocol.ErrorCode(err))
	}
	if _, err = a.Devices(ctx); err != nil {
		t.Fatal("fixture sender directory", protocol.ErrorCode(err))
	}
	if _, err = b.Devices(ctx); err != nil {
		t.Fatal("fixture receiver directory", protocol.ErrorCode(err))
	}
	board := &temporaryMemoryClipboard{generation: 10}
	a.ConfigureClipboard(coreapp.ClipboardAdapter{Generation: board.current, Read: board.read})
	var receiverGeneration atomic.Uint64
	receiverGeneration.Store(10)
	received := make(chan string, 4)
	b.ConfigureClipboard(coreapp.ClipboardAdapter{Generation: receiverGeneration.Load, Validate: nativeclipboard.ValidatePayload, Write: func(_ clipboardsync.Kind, body []byte, expected uint64, before time.Time) (uint64, error) {
		if receiverGeneration.Load() != expected || !before.IsZero() && !time.Now().Before(before) {
			return expected, clipboardsync.ErrStale
		}
		received <- string(body)
		return receiverGeneration.Add(1), nil
	}})
	for _, item := range []struct {
		service         *coreapp.Service
		peer, direction string
	}{{a, bID.ID(), "send"}, {a, bID.ID(), "receive"}, {b, aID.ID(), "send"}, {b, aID.ID(), "receive"}} {
		if _, err = item.service.SetClipboardGrant(ctx, coreapp.ClipboardGrantPatch{PeerID: item.peer, Direction: item.direction, Kind: "text", Enabled: true}); err != nil {
			t.Fatal("fixture grant", protocol.ErrorCode(err))
		}
	}
	a.ResetClipboard(false, board.current())
	b.ResetClipboard(false, receiverGeneration.Load())
	cfg := coreapp.DirectConfig{BindAddress: "127.0.0.1:0", AllowLoopback: true, CheckTimeout: 5 * time.Second}
	if err = b.StartInbox(filepath.Join(root, "received"), cfg); err != nil {
		t.Fatal("fixture inbox", protocol.ErrorCode(err))
	}
	waitTemporaryClipboardFixtureInbox(t, ctx, b)
	if err = a.EnsureClipboardSessions(ctx, cfg); err != nil {
		t.Fatal("fixture initial connect", protocol.ErrorCode(err))
	}
	waitTemporaryClipboardFixtureReady(t, ctx, "initial_leases", a, b, aID.ID(), bID.ID())
	source := filepath.Join(root, "fixture.txt")
	if err = os.WriteFile(source, []byte("memory share-code fixture"), 0600); err != nil {
		t.Fatal("fixture source unavailable")
	}
	created, err := a.CreateTemporaryShare(ctx, []string{source}, nil)
	if err != nil {
		t.Fatal("fixture create", protocol.ErrorCode(err))
	}
	app := NewApp()
	app.ctx, app.core = ctx, a
	for _, paused := range []bool{false, true} {
		connectionsBefore := signalingConnections.Load()
		if paused {
			a.ResetClipboard(true, board.current())
		}
		before := board.reads.Load()
		if err = app.copyTemporaryShareCode(created.Share.ID, created.Code, board.write, board.current); err != nil {
			t.Fatal("fixture protected copy", protocol.ErrorCode(err))
		}
		app.deliverClipboardChange(nativeclipboard.Change{Sequence: board.current(), Text: true})
		if paused {
			a.ResetClipboard(false, board.current())
			if err = a.EnsureClipboardSessions(ctx, cfg); err != nil {
				t.Fatal("fixture resume ensure", protocol.ErrorCode(err))
			}
			app.deliverClipboardChange(nativeclipboard.Change{Sequence: board.current(), Text: true})
			// Reset discards A's outgoing leases. The existing remote owner
			// renews B's lease on its normal seven-second schedule; a local
			// Ensure on this healthy connection does not forge that proof.
			waitTemporaryClipboardFixtureReady(t, ctx, "resumed_leases", a, b, aID.ID(), bID.ID())
			if signalingConnections.Load() != connectionsBefore {
				t.Fatal("resume redialed instead of using the existing lease owners")
			}
		}
		if board.reads.Load() != before {
			t.Fatal("protected generation reached memory payload reader")
		}
		select {
		case <-received:
			t.Fatal("protected code reached receiver")
		default:
		}
		text := "new memory copy after protected code"
		if paused {
			text += " and resume"
		}
		board.write(text)
		app.deliverClipboardChange(nativeclipboard.Change{Sequence: board.current(), Text: true})
		select {
		case got := <-received:
			if got != text {
				t.Fatal("new memory copy differed")
			}
		case <-ctx.Done():
			t.Fatal("new memory copy did not cross authenticated QUIC")
		}
	}
}
