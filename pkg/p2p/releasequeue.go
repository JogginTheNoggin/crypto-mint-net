package p2p

import (
	"context"
	"sync"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/series"
)

// releaseWindow is how long a node waits after taking a release of a
// series before it takes the next one, holding only the highest that
// arrives meanwhile. A release proves every part before it, so the ones
// it covers add nothing: a peer that replays releases oldest first costs
// a node at most two saves, relays, reloads and credits a window, not one
// each (RED-078, D169). A variable only so tests can make it short.
var releaseWindow = 2 * time.Second

// releaseQueue holds, per series, the time the last release was taken,
// whether one is being checked, and the highest one waiting. A release is
// checked in full by one worker per series, never in a peer's read loop,
// so a far part does not stop the peer's other messages, and the host's
// context ends the check when the node stops (RED-082, D171).
type releaseQueue struct {
	mu      sync.Mutex
	last    map[hash.Hash]time.Time
	busy    map[hash.Hash]bool
	held    map[hash.Hash]*heldRelease
	stopped bool
	wg      sync.WaitGroup
}

type heldRelease struct {
	r     series.Release
	from  *peer
	timer *time.Timer
}

// take is what the queue does with a release it lets through: the host's
// full check, keep and pass on.
type take func(series.Release, *peer)

// offer takes r in a worker now when no release of its series was taken
// within the window, none is being checked and none is held. Otherwise r
// is held when it is higher than the one held (the peer that passed the
// highest is the one credited), or dropped; the one held is taken when the
// window ends and no check runs.
func (q *releaseQueue) offer(r series.Release, from *peer, now time.Time, do take) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.stopped {
		return
	}
	if q.last == nil {
		q.last, q.busy, q.held = map[hash.Hash]time.Time{}, map[hash.Hash]bool{}, map[hash.Hash]*heldRelease{}
	}
	id := r.Series
	if h := q.held[id]; h != nil {
		if r.Part > h.r.Part {
			h.r, h.from = r, from
		}
		return
	}
	last, seen := q.last[id]
	if !q.busy[id] && (!seen || now.Sub(last) >= releaseWindow) {
		q.startLocked(id, r, from, now, do)
		return
	}
	q.held[id] = &heldRelease{r: r, from: from}
	if !q.busy[id] {
		q.armLocked(id, now, do)
	}
}

// startLocked runs do(r, from) in the series' worker.
func (q *releaseQueue) startLocked(id hash.Hash, r series.Release, from *peer, now time.Time, do take) {
	q.busy[id], q.last[id] = true, now
	q.wg.Add(1)
	go func() {
		defer q.wg.Done()
		do(r, from)
		q.mu.Lock()
		defer q.mu.Unlock()
		q.busy[id] = false
		if !q.stopped && q.held[id] != nil {
			q.armLocked(id, time.Now(), do)
		}
	}()
}

// armLocked takes the release held for id when the window since the last
// one taken ends.
func (q *releaseQueue) armLocked(id hash.Hash, now time.Time, do take) {
	h := q.held[id]
	if h.timer != nil {
		return
	}
	wait := max(releaseWindow-now.Sub(q.last[id]), 0)
	h.timer = time.AfterFunc(wait, func() {
		q.mu.Lock()
		defer q.mu.Unlock()
		if q.held[id] != h || q.stopped || q.busy[id] {
			return
		}
		delete(q.held, id)
		q.startLocked(id, h.r, h.from, time.Now(), do)
	})
}

// stopReleases drops the releases held and waits for the checks running,
// which the host's context ends, when the host stops.
func (h *Host) stopReleases() {
	q := &h.rel
	q.mu.Lock()
	q.stopped = true
	for id, held := range q.held {
		if held.timer != nil {
			held.timer.Stop()
		}
		delete(q.held, id)
	}
	q.mu.Unlock()
	q.wg.Wait()
}

// hostContext is the context Run was given, or Background before Run.
func (h *Host) hostContext() context.Context {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.ctx == nil {
		return context.Background()
	}
	return h.ctx
}
