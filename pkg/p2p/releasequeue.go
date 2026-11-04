package p2p

import (
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

// releaseQueue holds, per series, the time the last release was taken and
// the highest one waiting for the window to end.
type releaseQueue struct {
	mu      sync.Mutex
	last    map[hash.Hash]time.Time
	held    map[hash.Hash]*heldRelease
	stopped bool
}

type heldRelease struct {
	r     series.Release
	from  *peer
	timer *time.Timer
}

// offer reports whether r is to be taken now: no release of its series was
// taken within the window and none is held. Otherwise r is held when it is
// higher than the one held (the peer that passed the highest is the one
// credited), or dropped, and flush takes the one held when the window
// ends.
func (q *releaseQueue) offer(r series.Release, from *peer, now time.Time, flush func(series.Release, *peer)) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.stopped {
		return false
	}
	if q.last == nil {
		q.last, q.held = map[hash.Hash]time.Time{}, map[hash.Hash]*heldRelease{}
	}
	id := r.Series
	if h := q.held[id]; h != nil {
		if r.Part > h.r.Part {
			h.r, h.from = r, from
		}
		return false
	}
	wait := releaseWindow - now.Sub(q.last[id])
	if _, ok := q.last[id]; !ok || wait <= 0 {
		q.last[id] = now
		return true
	}
	h := &heldRelease{r: r, from: from}
	q.held[id] = h
	h.timer = time.AfterFunc(wait, func() {
		q.mu.Lock()
		if q.held[id] != h || q.stopped {
			q.mu.Unlock()
			return
		}
		delete(q.held, id)
		q.last[id] = time.Now()
		r, from := h.r, h.from
		q.mu.Unlock()
		flush(r, from)
	})
	return false
}

// stopReleases drops the releases held, when the host stops.
func (h *Host) stopReleases() {
	q := &h.rel
	q.mu.Lock()
	defer q.mu.Unlock()
	q.stopped = true
	for id, held := range q.held {
		held.timer.Stop()
		delete(q.held, id)
	}
}
