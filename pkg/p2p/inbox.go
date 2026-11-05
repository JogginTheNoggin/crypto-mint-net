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
	mu     sync.Mutex
	qs     [4][]job
	shares [4]int
	limit  int
	ready  chan struct{}
	done   bool
}

func newInbox(shares [4]int, limit int) *inbox {
	for i := range shares {
		if shares[i] <= 0 {
			shares[i] = 1
		}
	}
	return &inbox{shares: shares, limit: limit, ready: make(chan struct{}, 1)}
}

// put queues j; a full queue drops it.
func (b *inbox) put(c peers.Category, j job) bool {
	if c < peers.CatPriority || c > peers.CatUnlisted {
		c = peers.CatUnlisted
	}
	b.mu.Lock()
	if b.done || len(b.qs[c]) >= b.limit {
		b.mu.Unlock()
		return false
	}
	b.qs[c] = append(b.qs[c], j)
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
