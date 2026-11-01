package p2p

import (
	"sync"
	"time"
)

// paceBuckets is how many buckets a reqWindow splits its window into.
const paceBuckets = 60

// reqWindow counts the requests this node sent one peer over the last
// RequestPaceWindow, in paceBuckets buckets. The chase reads it to stay
// under the request budget a peer keeps for this node (tracking budget
// "request"): over it, the peer drops requests without a reply, and a
// chase that waits for one times out and loses what it took in
// (ORANGE-018).
type reqWindow struct {
	mu   sync.Mutex
	n    [paceBuckets]uint32
	slot [paceBuckets]int64
}

func (w *reqWindow) add(now time.Time, window time.Duration) {
	s := now.UnixNano() / int64(window/paceBuckets)
	i := s % paceBuckets
	w.mu.Lock()
	if w.slot[i] != s {
		w.slot[i], w.n[i] = s, 0
	}
	w.n[i]++
	w.mu.Unlock()
}

// count returns the requests sent in the last window, give or take one
// bucket.
func (w *reqWindow) count(now time.Time, window time.Duration) int {
	s := now.UnixNano() / int64(window/paceBuckets)
	w.mu.Lock()
	defer w.mu.Unlock()
	total := 0
	for i := range w.n {
		if s-w.slot[i] < paceBuckets {
			total += int(w.n[i])
		}
	}
	return total
}

// paced reports whether a chase should yield before its next request to
// p, because it has sent p RequestPace requests in the last
// RequestPaceWindow. The chase keeps what it took in and goes on when the
// window has room, so a long branch is reached at the rate peers serve
// it, never dropped by the sender's budget (D115).
func (h *Host) paced(p *peer) bool {
	return h.cfg.RequestPace > 0 && p.reqs.count(time.Now(), h.cfg.RequestPaceWindow) >= h.cfg.RequestPace
}
