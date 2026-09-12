package app

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Wen5555/LinkSend/internal/connectivity"
	"github.com/Wen5555/LinkSend/internal/identity"
	"github.com/Wen5555/LinkSend/internal/protocol"
	"github.com/Wen5555/LinkSend/internal/signaling"
)

type candidateSessionStub struct {
	send func(context.Context, protocol.Envelope) error
	read func(context.Context) (signaling.Wire, error)
}

func (s candidateSessionStub) SendEnvelope(ctx context.Context, env protocol.Envelope) error {
	return s.send(ctx, env)
}
func (s candidateSessionStub) Read(ctx context.Context) (signaling.Wire, error) {
	return s.read(ctx)
}

func lifecycleFixture(t *testing.T) (*identity.Identity, signaling.Device) {
	t.Helper()
	local, err := identity.Generate()
	if err != nil {
		t.Fatal(err)
	}
	peer, err := identity.Generate()
	if err != nil {
		t.Fatal(err)
	}
	return local, signaling.Device{ID: peer.ID(), PublicKey: peer.PublicKey(), Name: "peer"}
}

func TestExchangeCandidatesSendFailureCancelsReader(t *testing.T) {
	local, peer := lifecycleFixture(t)
	sendErr := errors.New("send failed")
	session := candidateSessionStub{
		send: func(context.Context, protocol.Envelope) error { return sendErr },
		read: func(ctx context.Context) (signaling.Wire, error) {
			<-ctx.Done()
			return signaling.Wire{}, ctx.Err()
		},
	}
	start := time.Now()
	err := exchangeCandidatesWithBudget(context.Background(), session, nil, nil, local, peer, protocol.RandomID(), firstGeneration, 100*time.Millisecond)
	if !errors.Is(err, sendErr) {
		t.Fatalf("error = %v, want send failure", err)
	}
	if time.Since(start) > time.Second {
		t.Fatalf("sibling reader was not cancelled promptly: %v", time.Since(start))
	}
}

func TestExchangeCandidatesReadFailureCancelsSender(t *testing.T) {
	local, peer := lifecycleFixture(t)
	readErr := errors.New("read failed")
	session := candidateSessionStub{
		send: func(ctx context.Context, _ protocol.Envelope) error {
			<-ctx.Done()
			return ctx.Err()
		},
		read: func(context.Context) (signaling.Wire, error) { return signaling.Wire{}, readErr },
	}
	err := exchangeCandidatesWithBudget(context.Background(), session, nil, []connectivity.Candidate{{Value: "candidate", Generation: 1}}, local, peer, protocol.RandomID(), firstGeneration, time.Second)
	if !errors.Is(err, readErr) {
		t.Fatalf("error = %v, want read failure", err)
	}
}

func TestExchangeCandidatesMissingEndTimesOut(t *testing.T) {
	local, peer := lifecycleFixture(t)
	session := candidateSessionStub{
		send: func(context.Context, protocol.Envelope) error { return nil },
		read: func(ctx context.Context) (signaling.Wire, error) {
			<-ctx.Done()
			return signaling.Wire{}, ctx.Err()
		},
	}
	start := time.Now()
	err := exchangeCandidatesWithBudget(context.Background(), session, nil, nil, local, peer, protocol.RandomID(), firstGeneration, 40*time.Millisecond)
	var protocolErr *protocol.Error
	if !errors.As(err, &protocolErr) || protocol.ErrorCode(err) != protocol.CandidateTimeout {
		t.Fatalf("error = %v, want candidate timeout", err)
	}
	if time.Since(start) > time.Second {
		t.Fatalf("missing end_of_candidates waited too long: %v", time.Since(start))
	}
}

func TestExchangeCandidatesParentCancellation(t *testing.T) {
	local, peer := lifecycleFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	session := candidateSessionStub{
		send: func(ctx context.Context, _ protocol.Envelope) error {
			<-ctx.Done()
			return ctx.Err()
		},
		read: func(ctx context.Context) (signaling.Wire, error) {
			<-ctx.Done()
			return signaling.Wire{}, ctx.Err()
		},
	}
	cancel()
	err := exchangeCandidatesWithBudget(ctx, session, nil, nil, local, peer, protocol.RandomID(), firstGeneration, time.Second)
	if protocol.ErrorCode(err) != protocol.Cancelled {
		t.Fatalf("error = %v, want cancellation", err)
	}
}

func TestExchangeCandidatesMalformedMessageFailsImmediately(t *testing.T) {
	local, peer := lifecycleFixture(t)
	session := candidateSessionStub{
		send: func(context.Context, protocol.Envelope) error { return nil },
		read: func(context.Context) (signaling.Wire, error) {
			return signaling.Wire{Type: "signal"}, nil
		},
	}
	err := exchangeCandidatesWithBudget(context.Background(), session, nil, nil, local, peer, protocol.RandomID(), firstGeneration, time.Second)
	if protocol.ErrorCode(err) != protocol.InvalidMessage {
		t.Fatalf("error = %v, want invalid message", err)
	}
}

func TestExchangeCandidatesLateEndDoesNotEnterNextSession(t *testing.T) {
	local, err := identity.Generate()
	if err != nil {
		t.Fatal(err)
	}
	peerIdentity, err := identity.Generate()
	if err != nil {
		t.Fatal(err)
	}
	peer := signaling.Device{ID: peerIdentity.ID(), PublicKey: peerIdentity.PublicKey(), Name: "peer"}
	firstSession := protocol.RandomID()
	secondSession := protocol.RandomID()
	makeEnd := func(sessionID string) signaling.Wire {
		env, newErr := protocol.NewEnvelope("end_of_candidates", peer.ID, local.ID(), sessionID, 1, map[string]any{})
		if newErr != nil {
			t.Fatal(newErr)
		}
		env.Signature = peerIdentity.Sign(env.SigningBytes())
		return signaling.Wire{Type: "signal", Message: &env}
	}
	wires := []signaling.Wire{makeEnd(firstSession), makeEnd(firstSession)}
	readIndex := 0
	session := candidateSessionStub{
		send: func(context.Context, protocol.Envelope) error { return nil },
		read: func(context.Context) (signaling.Wire, error) {
			wire := wires[readIndex]
			readIndex++
			return wire, nil
		},
	}
	if err = exchangeCandidatesWithBudget(context.Background(), session, nil, nil, local, peer, firstSession, firstGeneration, time.Second); err != nil {
		t.Fatal(err)
	}
	err = exchangeCandidatesWithBudget(context.Background(), session, nil, nil, local, peer, secondSession, firstGeneration, time.Second)
	if protocol.ErrorCode(err) != protocol.InvalidMessage {
		t.Fatalf("late end error = %v, want invalid message for the new session", err)
	}
}

func TestExchangeCandidatesRejectsStaleGeneration(t *testing.T) {
	local, err := identity.Generate()
	if err != nil {
		t.Fatal(err)
	}
	peerIdentity, err := identity.Generate()
	if err != nil {
		t.Fatal(err)
	}
	peer := signaling.Device{ID: peerIdentity.ID(), PublicKey: peerIdentity.PublicKey(), Name: "peer"}
	sessionID := protocol.RandomID()
	stale, err := protocol.NewEnvelope("end_of_candidates", peer.ID, local.ID(), sessionID, firstGeneration+1, map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	stale.Signature = peerIdentity.Sign(stale.SigningBytes())
	session := candidateSessionStub{
		send: func(context.Context, protocol.Envelope) error { return nil },
		read: func(context.Context) (signaling.Wire, error) {
			return signaling.Wire{Type: "signal", Message: &stale}, nil
		},
	}
	err = exchangeCandidatesWithBudget(context.Background(), session, nil, nil, local, peer, sessionID, firstGeneration, time.Second)
	if protocol.ErrorCode(err) != protocol.InvalidMessage {
		t.Fatalf("stale generation error = %v, want invalid message", err)
	}
}

func TestPeerSessionFailureRequiresSignedSessionBinding(t *testing.T) {
	local, peer := lifecycleFixture(t)
	peerIdentity, err := identity.Generate()
	if err != nil {
		t.Fatal(err)
	}
	peer = signaling.Device{ID: peerIdentity.ID(), PublicKey: peerIdentity.PublicKey(), Name: "peer"}
	sessionID := protocol.RandomID()
	env, err := protocol.NewEnvelope("status", peer.ID, local.ID(), sessionID, firstGeneration, sessionStatus{State: "failed", Code: protocol.NoCandidates})
	if err != nil {
		t.Fatal(err)
	}
	env.Signature = peerIdentity.Sign(env.SigningBytes())
	if got := peerSessionFailure(&env, peer, local.ID(), sessionID, firstGeneration); protocol.ErrorCode(got) != protocol.NoCandidates {
		t.Fatalf("signed peer failure = %v, want %s", got, protocol.NoCandidates)
	}

	tampered := env
	tampered.SessionID = protocol.RandomID()
	if got := peerSessionFailure(&tampered, peer, local.ID(), sessionID, firstGeneration); protocol.ErrorCode(got) != protocol.InvalidMessage {
		t.Fatalf("tampered peer failure = %v, want %s", got, protocol.InvalidMessage)
	}

	unknown, err := protocol.NewEnvelope("status", peer.ID, local.ID(), sessionID, firstGeneration, sessionStatus{State: "failed", Code: protocol.PermissionDenied})
	if err != nil {
		t.Fatal(err)
	}
	unknown.Signature = peerIdentity.Sign(unknown.SigningBytes())
	if got := peerSessionFailure(&unknown, peer, local.ID(), sessionID, firstGeneration); protocol.ErrorCode(got) != protocol.InvalidMessage {
		t.Fatalf("unsupported peer failure = %v, want %s", got, protocol.InvalidMessage)
	}
}

func TestCandidateStreamCanFinishLANBeforeGatherCompletes(t *testing.T) {
	local, err := identity.Generate()
	if err != nil {
		t.Fatal(err)
	}
	peerIdentity, err := identity.Generate()
	if err != nil {
		t.Fatal(err)
	}
	peer := signaling.Device{ID: peerIdentity.ID(), PublicKey: peerIdentity.PublicKey(), Name: "peer"}
	sessionID := protocol.RandomID()
	remoteEnd, err := protocol.NewEnvelope("end_of_candidates", peer.ID, local.ID(), sessionID, firstGeneration, map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	remoteEnd.Signature = peerIdentity.Sign(remoteEnd.SigningBytes())

	localCandidates := make(chan connectivity.Candidate, 1)
	localCandidates <- connectivity.Candidate{Value: "candidate", Generation: firstGeneration}
	firstCandidate := make(chan struct{})
	stop := make(chan struct{})
	var once sync.Once
	session := candidateSessionStub{
		send: func(_ context.Context, env protocol.Envelope) error {
			if env.Type == "candidate" {
				once.Do(func() { close(firstCandidate) })
			}
			return nil
		},
		read: func(context.Context) (signaling.Wire, error) {
			return signaling.Wire{Type: "signal", Message: &remoteEnd}, nil
		},
	}
	done := make(chan error, 1)
	go func() {
		done <- exchangeCandidateStreamWithBudget(context.Background(), session, nil, localCandidates, local, peer, sessionID, firstGeneration, stop, time.Second)
	}()
	select {
	case <-firstCandidate:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("first host candidate waited for gathering completion")
	}
	select {
	case err = <-done:
		t.Fatalf("candidate exchange ended before gathering completed: %v", err)
	default:
	}
	close(stop)
	if err = <-done; err != nil {
		t.Fatal(err)
	}
}
