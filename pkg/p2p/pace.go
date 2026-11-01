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
	// limit, when above 0 and until passes, is the pace learned from the
	// peer: three quarters of what was sent in the window when a second
	// request in a row went unanswered (D133). misses counts those in a
	// row, and reply is when the peer last replied to anything.
	limit  int
	until  time.Time
	misses int
	reply  time.Time
	// probing marks a learned pace not yet shown to be the peer's budget:
	// it doubles each window in which the peer replied (raised is when it
	// last did), and ends at Config.RequestPace, so a peer that was only
	// slow, or lost two requests, is soon served at the pace it allows. A
	// second learning while probing shows the budget: the pace then holds
	// as D133 says, and known keeps it, so a later drop with as much sent
	// holds at once (RED-028, D136).
	probing bool
	raised  time.Time
	known   int
}

// paceLearn is how many windows a learned pace lasts after the last
// unanswered request. The pace then goes back to Config.RequestPace and
// is learned again if the peer still drops requests: one flag at most
// each paceLearn windows, too few to time this node out (D133).
const paceLearn = 30

// dropped notes a request, sent at sent, that the peer left unanswered.
// A second in a row, with no reply from the peer since either was sent,
// is a peer that drops what is over its budget (one alone is as likely
// lost or slow): its budget is below what was sent in the window, so the
// pace to it becomes three quarters of that, for paceLearn windows. A
// lower learned pace replaces a higher one; it never rises while it
// lasts.
func (w *reqWindow) dropped(sent, now time.Time, window time.Duration) {
	n := w.count(now, window)
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.reply.After(sent) {
		return // the peer replied to a later request: not dropping
	}
	w.misses++
	if w.misses < 2 {
		return
	}
	limit := max(1, n*3/4)
	active := w.limit > 0 && !now.After(w.until)
	switch {
	case active && !w.probing:
		// Held: a lower pace replaces it only when as much as half of it
		// was sent, since drops with little sent say nothing of the
		// budget (RED-028).
		if limit < w.limit && n >= w.limit/2 {
			w.limit = limit
		}
	case (active && w.probing) || (w.known > 0 && n >= w.known):
		// The peer drops again while probed, or at what it dropped at
		// before: its budget, which holds.
		w.limit, w.probing, w.known = limit, false, limit
	default:
		// The first drops say little about the budget: a pace to probe
		// up from, not to hold.
		w.limit, w.probing, w.raised = limit, true, now
	}
	w.until = now.Add(paceLearn * window)
}

// answered notes a reply from the peer at now: the misses in a row start
// again, and a pace being probed doubles once a window, ending once it
// reaches pace (Config.RequestPace; 0 or less for none).
func (w *reqWindow) answered(now time.Time, pace int, window time.Duration) {
	w.mu.Lock()
	w.misses, w.reply = 0, now
	if w.probing && w.limit > 0 && !now.After(w.until) && now.Sub(w.raised) >= window {
		w.limit, w.raised = w.limit*2, now
		if pace > 0 && w.limit >= pace {
			w.limit, w.probing = 0, false
		}
	}
	w.mu.Unlock()
}

// learned returns the learned pace, or 0 when there is none.
func (w *reqWindow) learned(now time.Time) int {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.limit == 0 || now.After(w.until) {
		return 0
	}
	return w.limit
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
// RequestPaceWindow, or the pace learned from p (reqWindow.dropped). The
// chase keeps what it took in and goes on when the window has room, so a
// long branch is reached at the rate peers serve it, never dropped by the
// sender's budget (D115). The learned pace is Black's Round 10 note on
// C-6 (D133): a sender whose budget is below this node's pace drops
// requests, and flags this node once a window for it; without learning,
// the flags add up to a time-out, the sender refuses this node for the
// time-out's length, and a branch longer than ten windows is not reached.
func (h *Host) paced(p *peer) bool {
	now := time.Now()
	limit := h.cfg.RequestPace
	if l := p.reqs.learned(now); l > 0 && (limit <= 0 || l < limit) {
		limit = l
	}
	return limit > 0 && p.reqs.count(now, h.cfg.RequestPaceWindow) >= limit
}
