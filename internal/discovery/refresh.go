package discovery

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"sync"
	"time"
)

const refreshMinInterval = 750 * time.Millisecond

type refreshFlight struct {
	done chan struct{}
	err  error
}

// All entry points share one refresh, including the periodic retry. Suppressed
// requests return the last dispatch result, never evidence of peer reachability.
type refreshGate struct {
	mu        sync.Mutex
	flight    *refreshFlight
	completed time.Time
	lastError error
}

func (g *refreshGate) run(ctx context.Context, now time.Time, refresh func() error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	g.mu.Lock()
	if flight := g.flight; flight != nil {
		g.mu.Unlock()
		select {
		case <-flight.done:
			return flight.err
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if !g.completed.IsZero() && now.Sub(g.completed) < refreshMinInterval {
		err := g.lastError
		g.mu.Unlock()
		return err
	}
	flight := &refreshFlight{done: make(chan struct{})}
	g.flight = flight
	g.mu.Unlock()
	err := refresh()
	g.mu.Lock()
	flight.err = err
	g.lastError, g.completed, g.flight = err, time.Now(), nil
	close(flight.done)
	g.mu.Unlock()
	return err
}

// Bounded jitter prevents peers and remembered-address retries from remaining
// in lockstep. Randomness affects scheduling only, never identity or consent.
func jitterInterval(base time.Duration) time.Duration {
	var bytes [8]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return base
	}
	spread := base / 8
	return base - spread + time.Duration(binary.LittleEndian.Uint64(bytes[:])%uint64(2*spread+1))
}
