package p2p_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/engine"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/node"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/nodepolicy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/p2p"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/series"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/testutil"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tracking"
)

// startWhole starts a node that follows every pool of s, with tracker tr
// and the request budget and pace both counted over window.
func startWhole(t *testing.T, name string, s *series.Series, tr *tracking.Tracker, window time.Duration) *tnode {
	t.Helper()
	id := s.ID()
	pol := nodepolicy.Default()
	pol.Series = []nodepolicy.SeriesInfo{{Params: s.Params()}}
	pol.Follow = []nodepolicy.Follow{{Series: &id}}
	pol.RPC.Listen = "off"
	pol.NoSync = true
	dir := t.TempDir()
	must(t, nodepolicy.Save(filepath.Join(dir, nodepolicy.FileName), pol))
	n, err := node.Open(dir, node.Options{})
	must(t, err)
	tn := &tnode{t: t, name: name, n: n, done: make(chan struct{}), log: &syncBuf{}, key: engine.PoolKey{Series: id, Index: 1}}
	addr := freeAddr(t)
	h, err := p2p.New(n, p2p.Config{Listen: addr, Advertise: addr, AllowLocalPeers: true, CheckEvery: -1, ManageEvery: -1, PingEvery: -1,
		RequestTimeout: 5 * time.Second, FollowTimeout: 2 * time.Second, Log: tn.log, Tracker: tr, RequestPaceWindow: window})
	must(t, err)
	tn.h = h
	ctx, cancel := context.WithCancel(context.Background())
	tn.cancel = cancel
	ready := make(chan struct{})
	go func() {
		defer close(tn.done)
		if err := h.Run(ctx, func() { close(ready) }); err != nil {
			t.Error(err)
			close(ready)
		}
	}()
	<-ready
	t.Cleanup(func() {
		tn.stop()
		n.Close()
		if t.Failed() {
			t.Logf("%s log:\n%s", name, tn.log)
		}
	})
	return tn
}

// ORANGE-029: two nodes that follow a series of more pools than a peer's
// request budget (600 a window by default) ask each other's weight for
// every pool, within the budget. Before, each pool was asked at once:
// the peer dropped what was over its budget unanswered and flagged the
// asker, and a check every window had it timed out within ten. Windows
// are 2 s here in place of a minute.
func TestWeightSweepWithinBudget(t *testing.T) {
	const pools = 700
	p := testutil.ExampleParams()
	p.PoolSize, p.Mints = 1, pools
	s := testutil.MustSeries(p)
	window := 2 * time.Second
	newTr := func() *tracking.Tracker {
		tr, err := tracking.New(tracking.Config{LoopbackAsOne: true, Windows: tracking.Windows{Budget: window}}, "")
		must(t, err)
		return tr
	}
	trA, trB := newTr(), newTr()
	a := startWhole(t, "a", s, trA, window)
	b := startWhole(t, "b", s, trB, window)
	b.connect(a)
	// Every pool is asked: b's tracker hears a for each pool section.
	asked := func(tr *tracking.Tracker) (answered, timedOut uint64, sections int) {
		for _, k := range tr.Keys() {
			r, _ := tr.Report(k, time.Now())
			for _, pr := range r.Policies {
				answered += pr.Answered
				timedOut += pr.TimedOut
				if pr.Answered > 0 {
					sections++
				}
			}
		}
		return
	}
	eventuallyFor(t, 60*time.Second, "every pool's weight asked", func() bool {
		_, _, nb := asked(trB)
		_, _, na := asked(trA)
		return nb >= pools && na >= pools
	}, a, b)
	for name, tr := range map[string]*tracking.Tracker{"a": trA, "b": trB} {
		_, to, _ := asked(tr)
		if to != 0 {
			t.Errorf("%s: %d requests unanswered", name, to)
		}
		for _, k := range tr.Keys() {
			if r, _ := tr.Report(k, time.Now()); len(r.Flags) != 0 {
				t.Errorf("%s flagged its peer: %+v", name, r.Flags)
			}
		}
	}
}
