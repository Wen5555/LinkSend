package transfer

import (
	"bytes"
	"context"
	"errors"
	"math"
	"net"
	"sync"
	"testing"
	"time"
)

type budgetTestTimer struct {
	at      time.Time
	ch      chan time.Time
	stopped bool
}

func TestFileBodyBudgetBillsWireRetransmissionAndSkipsControl(t *testing.T) {
	p := fixture(t)
	budget := new(budgetRecorder)
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()
	done := make(chan error, 1)
	go func() {
		_, err := Receive(t.Context(), b, t.TempDir(), "peer", func(Manifest) bool { return true }, nil)
		done <- err
	}()
	var retransmitted int64
	result, err := SendWithOptions(t.Context(), a, p, SendOptions{BodyBudget: budget, Hooks: SendHooks{PreviouslySent: func(uint32, int) bool { return true }, ChunkSent: func(sent ChunkTransmission) {
		if sent.Retransmitted {
			retransmitted += sent.Bytes
		}
	}}})
	if err != nil {
		t.Fatal(err)
	}
	if err = <-done; err != nil {
		t.Fatal(err)
	}
	if int64(budget.total) != result.Bytes || retransmitted != result.Bytes || result.Bytes == 0 {
		t.Fatal("wire body/retransmission charging differs", budget.total, result.Bytes, retransmitted)
	}
}

type budgetTestClock struct {
	mu     sync.Mutex
	now    time.Time
	timers []*budgetTestTimer
	armed  chan struct{}
}

func newBudgetTestClock() *budgetTestClock {
	return &budgetTestClock{now: time.Unix(1, 0), armed: make(chan struct{}, 32)}
}
func (c *budgetTestClock) Now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.now }
func (c *budgetTestClock) Timer(d time.Duration) (<-chan time.Time, func()) {
	c.mu.Lock()
	timer := &budgetTestTimer{at: c.now.Add(d), ch: make(chan time.Time, 1)}
	c.timers = append(c.timers, timer)
	c.mu.Unlock()
	c.armed <- struct{}{}
	return timer.ch, func() { c.mu.Lock(); timer.stopped = true; c.mu.Unlock() }
}
func (c *budgetTestClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
	for _, timer := range c.timers {
		if !timer.stopped && !timer.at.After(c.now) {
			timer.ch <- c.now
			timer.stopped = true
		}
	}
}
func (c *budgetTestClock) waitTimer(t *testing.T) {
	t.Helper()
	select {
	case <-c.armed:
	case <-time.After(time.Second):
		t.Fatal("budget did not arm timer")
	}
}

func TestFileBodyBudgetFakeClockBurstSharedAndDynamic(t *testing.T) {
	clock := newBudgetTestClock()
	budget := newFileBodyLimiter(clock)
	if err := budget.SetRate(10 << 10); err != nil {
		t.Fatal(err)
	}
	if n, err := budget.Acquire(t.Context(), 64<<10); n != 1024 || err != nil {
		t.Fatalf("initial burst %d %v", n, err)
	}
	done := make(chan int, 1)
	go func() { n, _ := budget.Acquire(t.Context(), 64<<10); done <- n }()
	clock.waitTimer(t)
	clock.advance(50 * time.Millisecond)
	select {
	case <-done:
		t.Fatal("second sender received premature budget")
	default:
	}
	clock.advance(50 * time.Millisecond)
	select {
	case n := <-done:
		if n != 1024 {
			t.Fatal(n)
		}
	case <-time.After(time.Second):
		t.Fatal("refill stalled")
	}
	clock.advance(time.Hour)
	if n, err := budget.Acquire(t.Context(), 64<<10); n != 1024 || err != nil {
		t.Fatal("idle accumulated unbounded credit", n, err)
	}
	// Reapplying an unchanged value and changing to a nonzero value do not mint credit.
	_ = budget.SetRate(10 << 10)
	_ = budget.SetRate(20 << 10)
	go func() { n, _ := budget.Acquire(t.Context(), 64<<10); done <- n }()
	clock.waitTimer(t)
	select {
	case <-done:
		t.Fatal("rate update reset burst")
	default:
	}
	_ = budget.SetRate(0)
	select {
	case n := <-done:
		if n != 64<<10 {
			t.Fatal(n)
		}
	case <-time.After(time.Second):
		t.Fatal("unlimited did not wake waiter")
	}
	for _, timer := range clock.timers {
		if !timer.stopped {
			t.Fatal("timer leaked")
		}
	}
}

func TestFileBodyBudgetCancelAndInvalidValues(t *testing.T) {
	clock := newBudgetTestClock()
	budget := newFileBodyLimiter(clock)
	for _, rate := range []int64{-1, 1, 1023, MaxFileSendBytesPerSecond + 1, math.MaxInt64} {
		if !errors.Is(budget.SetRate(rate), ErrFileSendLimit) {
			t.Fatal("invalid limit", rate)
		}
	}
	_ = budget.SetRate(1024)
	_, _ = budget.Acquire(t.Context(), 1024)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { _, err := budget.Acquire(ctx, 1024); done <- err }()
	clock.waitTimer(t)
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancel blocked on budget")
	}
	if budget.Rate() != 1024 {
		t.Fatal("invalid updates mutated rate")
	}
	if _, err := budget.Acquire(t.Context(), 0); err == nil {
		t.Fatal("zero request accepted")
	}
	if err := budget.SetRate(MaxFileSendBytesPerSecond); err != nil {
		t.Fatal(err)
	}
	clock.advance(time.Hour)
	if n, err := budget.Acquire(t.Context(), 1<<30); n != maxBodyWrite || err != nil {
		t.Fatal("maximum burst", n, err)
	}
}

type budgetRecorder struct {
	total  int
	grants []int
}

func (b *budgetRecorder) Acquire(_ context.Context, n int) (int, error) {
	n = min(n, 317)
	b.total += n
	b.grants = append(b.grants, n)
	return n, nil
}
func TestFileBodyFrameCompatibilityAndRepeatedBodyBilling(t *testing.T) {
	payload := bytes.Repeat([]byte("body"), 50000)
	budget := new(budgetRecorder)
	for range 2 { // The same application chunk sent again must pay again.
		var expected, actual bytes.Buffer
		if err := writeFrame(&expected, payload, len(payload)); err != nil {
			t.Fatal(err)
		}
		if err := writeBodyFrame(t.Context(), &actual, payload, len(payload), budget); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(expected.Bytes(), actual.Bytes()) {
			t.Fatal("body budgeting changed wire frame")
		}
	}
	if budget.total != 2*len(payload) {
		t.Fatal("body/headers billed incorrectly", budget.total)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	var actual bytes.Buffer
	if err := writeBodyFrame(ctx, &actual, payload, len(payload), NewFileBodyLimiter()); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if actual.Len() != 4 {
		t.Fatal("body emitted after cancellation")
	}
}
