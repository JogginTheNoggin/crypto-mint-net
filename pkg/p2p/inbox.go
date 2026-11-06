package p2p

import (
	"context"
	"fmt"
	"runtime/debug"
	"sync"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/peers"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/wire"
)

// job is one message waiting to be fully checked.
type job struct {
	p    *peer
	m    wire.Message
	size int
}

// inbox holds the requests and data that passed the cheap checks, in one
// queue per category of the sender: priority, backup, candidate,
// unlisted. The queues are handled top to bottom, each lower one getting
// its share of every round, so nothing waits forever.
type inbox struct {
	mu       sync.Mutex
	qs       [4][]job
	bytes    [4]int // the frame bytes waiting in each queue
	shares   [4]int
	limit    int
	maxBytes int
	ready    chan struct{}
	done     bool
}

// DefaultQueueBytes is how many bytes of messages may wait in each inbox
// queue (RED-103): with four queues, at most 128 MiB in all, where a
// count alone allowed 1,024 messages of up to 16 MiB in each.
const DefaultQueueBytes = 32 << 20

func newInbox(shares [4]int, limit, maxBytes int) *inbox {
	for i := range shares {
		if shares[i] <= 0 {
			shares[i] = 1
		}
	}
	if maxBytes <= 0 {
		maxBytes = DefaultQueueBytes
	}
	return &inbox{shares: shares, limit: limit, maxBytes: maxBytes, ready: make(chan struct{}, 1)}
}

// put queues j; a queue at its count, or that j would take over its bytes,
// drops it. An empty queue takes any one message, so one larger than the
// byte bound still gets through.
func (b *inbox) put(c peers.Category, j job) bool {
	if c < peers.CatPriority || c > peers.CatUnlisted {
		c = peers.CatUnlisted
	}
	b.mu.Lock()
	if b.done || len(b.qs[c]) >= b.limit || len(b.qs[c]) > 0 && b.bytes[c]+j.size > b.maxBytes {
		b.mu.Unlock()
		return false
	}
	b.qs[c] = append(b.qs[c], j)
	b.bytes[c] += j.size
	b.mu.Unlock()
	select {
	case b.ready <- struct{}{}:
	default:
	}
	return true
}

// round takes up to each queue's share, top to bottom, waiting until there
// is something. It returns nil once closed.
func (b *inbox) round(ctx context.Context) []job {
	for {
		b.mu.Lock()
		if b.done {
			b.mu.Unlock()
			return nil
		}
		var out []job
		for i := range b.qs {
			n := b.shares[i]
			if n > len(b.qs[i]) {
				n = len(b.qs[i])
			}
			for _, j := range b.qs[i][:n] {
				b.bytes[i] -= j.size
			}
			out = append(out, b.qs[i][:n]...)
			b.qs[i] = append([]job(nil), b.qs[i][n:]...)
		}
		b.mu.Unlock()
		if len(out) > 0 {
			return out
		}
		select {
		case <-ctx.Done():
			return nil
		case <-b.ready:
		}
	}
}

// close drops whatever waits and stops the queues.
func (b *inbox) close() {
	b.mu.Lock()
	b.done = true
	b.qs = [4][]job{}
	b.bytes = [4]int{}
	b.mu.Unlock()
	select {
	case b.ready <- struct{}{}:
	default:
	}
}

// workLoop handles queued messages one at a time: the engine checks one
// item at a time anyway, and one worker keeps each queue in order.
func (h *Host) workLoop(ctx context.Context) {
	defer h.wg.Done()
	for {
		jobs := h.inbox.round(ctx)
		if jobs == nil {
			return
		}
		for _, j := range jobs {
			if j.p.closed() {
				continue
			}
			if err := h.handleSafe(j.p, j.m, j.size); err != nil {
				h.misbehave(j.p, err)
			}
		}
	}
}

// handleSafe is handle, with a panic turned into closing that peer: a
// fault in this node's own handling of one message (a nil left on a
// serving path, RED-084) costs one connection, never the process. The
// peer is not counted against, since the fault is this node's (D173). A
// panic while the engine was changing a pool fails the engine first
// (engine.ErrFailed): the daemon then stops the node, which rebuilds from
// its stores on the next start, so nothing builds on a half-changed pool
// (D174).
func (h *Host) handleSafe(p *peer, m wire.Message, size int) (err error) {
	defer func() {
		if v := recover(); v != nil {
			h.log.Error("handling a peer message failed; the peer is disconnected", "peer", p.short(), "type", m.Type.String(), "panic", fmt.Sprint(v), "stack", string(debug.Stack()))
			p.close(fmt.Errorf("p2p: internal error handling %s", m.Type))
			err = nil
		}
	}()
	return h.handle(p, m, size)
}
