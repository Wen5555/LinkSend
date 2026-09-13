package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/Wen5555/LinkSend/internal/protocol"
	quic "github.com/quic-go/quic-go"
)

func TestSessionReuseRequiresBilateralCapability(t *testing.T) {
	old, err := protocol.NewEnvelope("connect_response", "sender", "recipient",
		"0123456789abcdef0123456789abcdef", 1, map[string]string{"ufrag": "u", "password": "p"})
	if err != nil {
		t.Fatal(err)
	}
	if envelopeSupportsSessionReuse(old) {
		t.Fatal("legacy response was treated as reusable")
	}
	modern, err := protocol.NewEnvelope("connect_response", "sender", "recipient",
		"0123456789abcdef0123456789abcdef", 1, iceDescription{Ufrag: "u", Password: "p", SessionReuse: true})
	if err != nil {
		t.Fatal(err)
	}
	if !envelopeSupportsSessionReuse(modern) {
		t.Fatal("bilateral reuse capability was lost")
	}
	var legacy struct {
		Ufrag    string `json:"ufrag"`
		Password string `json:"password"`
	}
	if err := json.Unmarshal(modern.Payload, &legacy); err != nil || legacy.Ufrag != "u" || legacy.Password != "p" {
		t.Fatalf("new optional field broke legacy ICE parsing: %+v %v", legacy, err)
	}
}

func TestDirectPoolKeyLockSerializesSamePeer(t *testing.T) {
	service := &Service{directPoolLocks: make(map[string]*sync.Mutex)}
	lockA := service.directPoolKeyLock("peer/1")
	lockB := service.directPoolKeyLock("peer/1")
	if lockA != lockB {
		t.Fatal("same pool key received different connect locks")
	}
	lockA.Lock()
	entered := make(chan struct{})
	go func() {
		lockB.Lock()
		close(entered)
		lockB.Unlock()
	}()
	select {
	case <-entered:
		t.Fatal("same-key connect was not serialized")
	case <-time.After(25 * time.Millisecond):
	}
	lockA.Unlock()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("serialized connect did not resume")
	}
}

func TestReusablePeerStreamErrorsDoNotCloseHealthyConnection(t *testing.T) {
	for _, err := range []error{
		protocol.Fail(protocol.ReceiveRejected, "declined"),
		&quic.StreamError{StreamID: 4, ErrorCode: 0x100, Remote: true},
	} {
		if !reusablePeerStreamResult(err) {
			t.Fatalf("single-stream result closed session: %v", err)
		}
	}
	if reusablePeerStreamResult(protocol.Fail(protocol.ConnectionInterrupted, "connection lost")) {
		t.Fatal("connection-level failure was treated as reusable")
	}
}

func TestDialPublishCannotRaceShutdown(t *testing.T) {
	f := newDirectFixtureServices(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	waiting := make(chan struct{})
	receiver := make(chan *PeerSession, 1)
	go func() {
		peer, _ := f.b.AcceptDirect(ctx, f.aID.ID(), DirectConfig{
			AllowLoopback: true, CheckTimeout: 5 * time.Second, WaitTimeout: 5 * time.Second,
			onPhase: func(phase string) {
				if phase == "waiting" {
					select {
					case <-waiting:
					default:
						close(waiting)
					}
				}
			},
		})
		receiver <- peer
	}()
	<-waiting
	reached, release := make(chan struct{}), make(chan struct{})
	f.a.directPoolBeforePublish = func(kind string) {
		if kind == "dial" {
			close(reached)
			<-release
		}
	}
	result := make(chan error, 1)
	go func() {
		_, err := f.a.acquirePeerSession(ctx, f.bID.ID(), DirectConfig{AllowLoopback: true, CheckTimeout: 5 * time.Second})
		result <- err
	}()
	<-reached
	shutdown := make(chan error, 1)
	go func() { shutdown <- f.a.ShutdownContext(ctx) }()
	waitFor(t, time.Second, f.a.isClosing, "shutdown did not close the publish gate")
	close(release)
	if err := <-result; err == nil || err.Error() != "APP_CLOSING" {
		t.Fatalf("late dial was published during shutdown: %v", err)
	}
	if peer := <-receiver; peer != nil {
		_ = peer.Close()
	}
	if err := <-shutdown; err != nil {
		t.Fatal(err)
	}
	f.a.directPoolMu.Lock()
	remaining := len(f.a.directPool)
	f.a.directPoolMu.Unlock()
	if remaining != 0 {
		t.Fatalf("shutdown retained %d late pooled sessions", remaining)
	}
	history := filepath.Join(f.a.cfg.DataDir, "task-history.sqlite")
	if err := os.Rename(history, history+".closed"); err != nil {
		t.Fatalf("profile remained owned after blocked dial shutdown: %v", err)
	}
}

func TestInboundAdoptCannotRaceShutdown(t *testing.T) {
	f := newDirectFixtureServices(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	sender, receiver := connectDirectPair(t, f, ctx)
	defer sender.Close()
	reached, release := make(chan struct{}), make(chan struct{})
	f.b.directPoolBeforePublish = func(kind string) {
		if kind == "adopt" {
			close(reached)
			<-release
		}
	}
	done := make(chan struct{})
	go func() {
		f.b.adoptInboundPeerSession(receiver)
		close(done)
	}()
	<-reached
	shutdown := make(chan error, 1)
	go func() { shutdown <- f.b.ShutdownContext(ctx) }()
	waitFor(t, time.Second, f.b.isClosing, "shutdown did not close the inbound publish gate")
	close(release)
	<-done
	if err := <-shutdown; err != nil {
		t.Fatal(err)
	}
	f.b.directPoolMu.Lock()
	remaining := len(f.b.directPool)
	f.b.directPoolMu.Unlock()
	if remaining != 0 {
		t.Fatalf("shutdown retained %d late inbound sessions", remaining)
	}
	history := filepath.Join(f.b.cfg.DataDir, "task-history.sqlite")
	if err := os.Rename(history, history+".closed"); err != nil {
		t.Fatalf("profile remained owned after blocked adopt shutdown: %v", err)
	}
}
