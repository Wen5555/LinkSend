package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"example.com/linksend/internal/connectivity"
	"example.com/linksend/internal/identity"
	"example.com/linksend/internal/protocol"
	"example.com/linksend/internal/signaling"
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
	err := exchangeCandidatesWithBudget(context.Background(), session, nil, nil, local, peer, protocol.RandomID(), 100*time.Millisecond)
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
	err := exchangeCandidatesWithBudget(context.Background(), session, nil, []connectivity.Candidate{{Value: "candidate", Generation: 1}}, local, peer, protocol.RandomID(), time.Second)
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
	err := exchangeCandidatesWithBudget(context.Background(), session, nil, nil, local, peer, protocol.RandomID(), 40*time.Millisecond)
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
	err := exchangeCandidatesWithBudget(ctx, session, nil, nil, local, peer, protocol.RandomID(), time.Second)
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
	err := exchangeCandidatesWithBudget(context.Background(), session, nil, nil, local, peer, protocol.RandomID(), time.Second)
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
	if err = exchangeCandidatesWithBudget(context.Background(), session, nil, nil, local, peer, firstSession, time.Second); err != nil {
		t.Fatal(err)
	}
	err = exchangeCandidatesWithBudget(context.Background(), session, nil, nil, local, peer, secondSession, time.Second)
	if protocol.ErrorCode(err) != protocol.InvalidMessage {
		t.Fatalf("late end error = %v, want invalid message for the new session", err)
	}
}
