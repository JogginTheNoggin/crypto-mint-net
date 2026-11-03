package p2p

import (
	"sync"
	"time"
)

// sweepInFlight is how many weight requests one sweep keeps waiting for a
// reply from one peer at a time.
const sweepInFlight = 8

// sweepWeights asks p for its stated weight in each of pools, in order,
// as one sweep (ORANGE-029). A new peer is swept for every pool it covers
// and each check sweeps a priority peer's pools; before, each pool was
// asked at once, one goroutine each. A series of more pools than the
// peer's request budget (tracking budget "request", 600 a minute by
// default) followed whole then went over it at every sweep: the peer
// dropped the rest unanswered, and flagged this node once a window, so a
// check every minute had this node timed out by its peers within ten.
//
// A sweep sends at most sweepInFlight requests at a time and, over a
// RequestPaceWindow, at most half the pace to p (Config.RequestPace, or
// the pace learned from p, as chases use), so chases keep the other
// half. It waits for room rather than skipping a pool: every pool is
// still asked, a large series over more windows. While a sweep to p runs,
// a new one to p is not started; the next check starts it again. Nothing
// here changes what a node accepts.
func (h *Host) sweepWeights(p *peer, pools []*poolInfo) {
	if len(pools) == 0 || !p.sweeping.CompareAndSwap(false, true) {
		return
	}
	if !h.spawn(func() {
		defer p.sweeping.Store(false)
		sem := make(chan struct{}, sweepInFlight)
		var wg sync.WaitGroup
		defer wg.Wait()
		step := h.cfg.RequestPaceWindow / paceBuckets
		if step <= 0 {
			step = time.Second
		}
		for _, pi := range pools {
			for h.sweepPaced(p) {
				t := time.NewTimer(step)
				select {
				case <-p.done:
					t.Stop()
					return
				case <-h.ctx.Done():
					t.Stop()
					return
				case <-t.C:
				}
			}
			select {
			case sem <- struct{}{}:
			case <-p.done:
				return
			case <-h.ctx.Done():
				return
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer func() { <-sem }()
				h.askWeight(pi, p)
			}()
		}
	}) {
		p.sweeping.Store(false)
	}
}

// sweepPaced reports whether a weight sweep should wait before its next
// request to p: half the pace paced uses, so chases keep the rest.
func (h *Host) sweepPaced(p *peer) bool {
	now := time.Now()
	limit := h.cfg.RequestPace
	if l := p.reqs.learned(now); l > 0 && (limit <= 0 || l < limit) {
		limit = l
	}
	if limit <= 0 {
		return false
	}
	return p.reqs.count(now, h.cfg.RequestPaceWindow) >= max(limit/2, 1)
}
