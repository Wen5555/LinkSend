package discovery

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestRefreshMergesConcurrentRequestsAndThrottlesFailure(t *testing.T) {
	var gate refreshGate
	var calls atomic.Int32
	entered, release := make(chan struct{}), make(chan struct{})
	want := errors.New("provider unavailable")
	run := func() error {
		if calls.Add(1) == 1 {
			close(entered)
			<-release
		}
		return want
	}
	results := make(chan error, 65)
	go func() { results <- gate.run(t.Context(), time.Now(), run) }()
	<-entered
	var workers sync.WaitGroup
	for range 64 {
		workers.Go(func() { results <- gate.run(t.Context(), time.Now(), run) })
	}
	close(release)
	workers.Wait()
	for range 65 {
		if err := <-results; !errors.Is(err, want) {
			t.Fatalf("refresh lost provider failure: %v", err)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("refresh storm dispatched %d waves", calls.Load())
	}
	gate.mu.Lock()
	completed := gate.completed
	gate.mu.Unlock()
	_ = gate.run(t.Context(), completed.Add(refreshMinInterval-time.Nanosecond), run)
	if calls.Load() != 1 {
		t.Fatal("refresh retried before the interval elapsed")
	}
	_ = gate.run(t.Context(), completed.Add(refreshMinInterval), run)
	if calls.Load() != 2 {
		t.Fatal("failed provider was not retried after the interval")
	}
}

func TestRefreshWaiterCanCancelWithoutDispatchingAgain(t *testing.T) {
	var gate refreshGate
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	go func() {
		done <- gate.run(t.Context(), time.Now(), func() error { close(entered); <-release; return nil })
	}()
	<-entered
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	timer := time.AfterFunc(10*time.Millisecond, cancel)
	defer timer.Stop()
	if err := gate.run(ctx, time.Now(), func() error { t.Error("cancelled waiter dispatched"); return nil }); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled refresh = %v", err)
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestRetryJitterStaysBoundedAndVaries(t *testing.T) {
	values := map[time.Duration]bool{}
	for range 100 {
		duration := jitterInterval(announceEvery)
		if duration < announceEvery-announceEvery/8 || duration > announceEvery+announceEvery/8 {
			t.Fatalf("retry jitter escaped bounds: %v", duration)
		}
		values[duration] = true
	}
	if len(values) < 2 {
		t.Fatal("periodic retries remained in lockstep")
	}
}
