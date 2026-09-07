package connectivity

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pion/stun/v4"
	quic "github.com/quic-go/quic-go"
)

const maxSTUNPacket = 4096

type packet struct {
	data []byte
	addr net.Addr
	err  error
}
type writeRequest struct {
	packet
	result chan packet
}

// stunPacketConn gives Pion only decoded STUN datagrams. QUIC remains the only
// reader/writer of the original UDPConn. This adapter never carries file bytes.
// Its bounded queues own their buffers, including after deadline cancellation.
type stunPacketConn struct {
	transport                   *quic.Transport
	local                       net.Addr
	ctx                         context.Context
	cancel                      context.CancelFunc
	reads                       chan packet
	writes                      chan writeRequest
	mu                          sync.Mutex
	readDeadline, writeDeadline time.Time
	changed                     chan struct{}
	closeOnce                   sync.Once
	wg                          sync.WaitGroup
	received, sent, rejected    atomic.Uint64
}

func newSTUNPacketConn(t *quic.Transport, local net.Addr) (*stunPacketConn, error) {
	ctx, cancel := context.WithCancel(context.Background())
	p := &stunPacketConn{transport: t, local: local, ctx: ctx, cancel: cancel,
		reads: make(chan packet, 32), writes: make(chan writeRequest, 32), changed: make(chan struct{})}
	// A single, synchronous first call initializes quic-go's non-QUIC reader.
	// Concurrent first calls would race in the selected upstream version.
	initCtx, stop := context.WithCancel(ctx)
	stop()
	_, _, err := t.ReadNonQUICPacket(initCtx, make([]byte, maxSTUNPacket))
	if err != nil && !errors.Is(err, context.Canceled) {
		cancel()
		return nil, err
	}
	p.wg.Add(2)
	go p.readLoop()
	go p.writeLoop()
	return p, nil
}

func validSTUN(b []byte) bool {
	if len(b) > maxSTUNPacket || !stun.IsMessage(b) {
		return false
	}
	m := &stun.Message{Raw: b}
	return m.Decode() == nil && len(b) == 20+int(m.Length) && m.Type.Method == stun.MethodBinding
}

func (p *stunPacketConn) readLoop() {
	defer p.wg.Done()
	// Read at maximum UDP size so truncation can never masquerade as valid STUN.
	buf := make([]byte, 65535)
	for {
		n, addr, err := p.transport.ReadNonQUICPacket(p.ctx, buf)
		if err != nil {
			select {
			case p.reads <- packet{err: err}:
			case <-p.ctx.Done():
			}
			return
		}
		if !validSTUN(buf[:n]) {
			p.rejected.Add(1)
			continue
		}
		p.received.Add(uint64(n))
		q := packet{data: append([]byte(nil), buf[:n]...), addr: addr}
		select {
		case p.reads <- q:
		case <-p.ctx.Done():
			return
		default:
			p.rejected.Add(1)
		}
	}
}

func (p *stunPacketConn) writeLoop() {
	defer p.wg.Done()
	for {
		select {
		case <-p.ctx.Done():
			return
		case q := <-p.writes:
			n, err := p.transport.WriteTo(q.data, q.addr)
			if n > 0 {
				p.sent.Add(uint64(n))
			}
			q.result <- packet{data: q.data[:n], err: err}
		}
	}
}

func (p *stunPacketConn) deadline(read bool) (time.Time, <-chan struct{}) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if read {
		return p.readDeadline, p.changed
	}
	return p.writeDeadline, p.changed
}

func deadlineTimer(d time.Time) (<-chan time.Time, func()) {
	if d.IsZero() {
		return nil, func() {}
	}
	t := time.NewTimer(time.Until(d))
	return t.C, func() { t.Stop() }
}

func (p *stunPacketConn) ReadFrom(b []byte) (int, net.Addr, error) {
	for {
		d, changed := p.deadline(true)
		if !d.IsZero() && !time.Now().Before(d) {
			return 0, nil, os.ErrDeadlineExceeded
		}
		timer, stop := deadlineTimer(d)
		select {
		case <-p.ctx.Done():
			stop()
			return 0, nil, net.ErrClosed
		case <-changed:
			stop()
			continue
		case <-timer:
			stop()
			return 0, nil, os.ErrDeadlineExceeded
		case q := <-p.reads:
			stop()
			if q.err != nil {
				return 0, nil, q.err
			}
			if len(b) < len(q.data) {
				return 0, q.addr, io.ErrShortBuffer
			}
			return copy(b, q.data), q.addr, nil
		}
	}
}

func (p *stunPacketConn) WriteTo(b []byte, addr net.Addr) (int, error) {
	if !validSTUN(b) {
		return 0, errors.New("only valid STUN binding datagrams allowed")
	}
	q := writeRequest{packet: packet{data: append([]byte(nil), b...), addr: addr}, result: make(chan packet, 1)}
	queued := false
	for {
		d, changed := p.deadline(false)
		if !d.IsZero() && !time.Now().Before(d) {
			return 0, os.ErrDeadlineExceeded
		}
		timer, stop := deadlineTimer(d)
		var out chan writeRequest
		if !queued {
			out = p.writes
		}
		select {
		case <-p.ctx.Done():
			stop()
			return 0, net.ErrClosed
		case <-changed:
			stop()
		case <-timer:
			stop()
			return 0, os.ErrDeadlineExceeded
		case out <- q:
			stop()
			queued = true
		case result := <-q.result:
			stop()
			return len(result.data), result.err
		}
	}
}

func (p *stunPacketConn) LocalAddr() net.Addr { return p.local }
func (p *stunPacketConn) Close() error        { p.closeOnce.Do(p.cancel); return nil }
func (p *stunPacketConn) SetDeadline(t time.Time) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.readDeadline = t
	p.writeDeadline = t
	close(p.changed)
	p.changed = make(chan struct{})
	return nil
}
func (p *stunPacketConn) SetReadDeadline(t time.Time) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.readDeadline = t
	close(p.changed)
	p.changed = make(chan struct{})
	return nil
}
func (p *stunPacketConn) SetWriteDeadline(t time.Time) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.writeDeadline = t
	close(p.changed)
	p.changed = make(chan struct{})
	return nil
}
