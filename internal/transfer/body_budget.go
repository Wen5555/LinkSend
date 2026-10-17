package transfer

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"math"
	"sync"
	"time"
)

const MaxFileSendBytesPerSecond int64 = 1 << 30
const maxBodyWrite = 64 << 10

var ErrFileSendLimit = errors.New("INVALID_FILE_SEND_LIMIT: use 0 or an integer from 1024 to 1073741824 bytes per second")

func ValidateFileSendLimit(rate int64) error {
	if rate != 0 && (rate < 1024 || rate > MaxFileSendBytesPerSecond) {
		return ErrFileSendLimit
	}
	return nil
}

// BodyBudget grants bounded body writes; control frames never acquire it.
type BodyBudget interface {
	Acquire(context.Context, int) (int, error)
}

type budgetClock interface {
	Now() time.Time
	Timer(time.Duration) (<-chan time.Time, func())
}
type realBudgetClock struct{}

func (realBudgetClock) Now() time.Time { return time.Now() }
func (realBudgetClock) Timer(d time.Duration) (<-chan time.Time, func()) {
	timer := time.NewTimer(d)
	return timer.C, func() { timer.Stop() }
}

// FileBodyLimiter is shared by all file senders in one application process.
// It does not budget QUIC packet overhead, transport retransmissions or clipboard.
type FileBodyLimiter struct {
	mu      sync.Mutex
	clock   budgetClock
	rate    int64
	tokens  float64
	last    time.Time
	changed chan struct{}
}

func NewFileBodyLimiter() *FileBodyLimiter { return newFileBodyLimiter(realBudgetClock{}) }
func newFileBodyLimiter(clock budgetClock) *FileBodyLimiter {
	return &FileBodyLimiter{clock: clock, last: clock.Now(), changed: make(chan struct{})}
}
func fileBodyBurst(rate int64) int {
	return int(max(int64(1024), min(int64(maxBodyWrite), rate/10)))
}
func (b *FileBodyLimiter) refill(now time.Time) {
	if b.rate > 0 && now.After(b.last) {
		b.tokens = math.Min(float64(fileBodyBurst(b.rate)), b.tokens+now.Sub(b.last).Seconds()*float64(b.rate))
	}
	b.last = now
}
func (b *FileBodyLimiter) Rate() int64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.rate
}
func (b *FileBodyLimiter) SetRate(rate int64) error {
	if err := ValidateFileSendLimit(rate); err != nil {
		return err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.refill(b.clock.Now())
	if b.rate == rate {
		return nil
	}
	if b.rate == 0 {
		b.tokens = float64(fileBodyBurst(rate))
	}
	b.rate = rate
	b.tokens = math.Min(b.tokens, float64(fileBodyBurst(rate)))
	close(b.changed)
	b.changed = make(chan struct{})
	return nil
}

func (b *FileBodyLimiter) Acquire(ctx context.Context, maximum int) (int, error) {
	if maximum <= 0 {
		return 0, errors.New("INVALID_BODY_BUDGET_REQUEST")
	}
	maximum = min(maximum, maxBodyWrite)
	for {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		b.mu.Lock()
		b.refill(b.clock.Now())
		if b.rate == 0 {
			b.mu.Unlock()
			return maximum, nil
		}
		need := min(maximum, fileBodyBurst(b.rate))
		if b.tokens >= float64(need) {
			b.tokens -= float64(need)
			b.mu.Unlock()
			return need, nil
		}
		delay := time.Duration(math.Ceil((float64(need) - b.tokens) / float64(b.rate) * float64(time.Second)))
		changed := b.changed
		alarm, stop := b.clock.Timer(max(time.Nanosecond, delay))
		b.mu.Unlock()
		select {
		case <-ctx.Done():
			stop()
			return 0, ctx.Err()
		case <-changed:
		case <-alarm:
		}
		stop()
	}
}

// Preserve the existing length-prefixed frame, budgeting only its body. Every
// requested chunk, including an application-level retransmission, calls here.
func writeBodyFrame(ctx context.Context, w io.Writer, body []byte, limit int, budget BodyBudget) error {
	if budget == nil {
		return writeFrame(w, body, limit)
	}
	if limit <= 0 || len(body) > limit {
		return errors.New("INVALID_BODY_FRAME")
	}
	var header [4]byte
	binary.BigEndian.PutUint32(header[:], uint32(len(body)))
	if err := writeAll(w, header[:]); err != nil {
		return err
	}
	for len(body) > 0 {
		n, err := budget.Acquire(ctx, min(len(body), maxBodyWrite))
		if err != nil {
			return err
		}
		if n <= 0 || n > min(len(body), maxBodyWrite) {
			return errors.New("INVALID_BODY_BUDGET_GRANT")
		}
		if err = ctx.Err(); err != nil {
			return err
		}
		if err = writeAll(w, body[:n]); err != nil {
			return err
		}
		body = body[n:]
	}
	return nil
}
