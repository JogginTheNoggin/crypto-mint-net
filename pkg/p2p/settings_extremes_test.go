package p2p_test

import (
	"context"
	"math"
	"path/filepath"
	"testing"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/engine"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/node"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/nodepolicy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/p2p"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tracking"
)

// startNodePol is startNode with b's node.json edited first, as a bundle
// imported with -settings or -full would set it.
func startNodePol(t *testing.T, name string, edit func(*nodepolicy.Policy)) *tnode {
	t.Helper()
	s := exSeries
	id := s.ID()
	pol := nodepolicy.Default()
	pol.Series = []nodepolicy.SeriesInfo{{Params: s.Params()}}
	pol.Follow = []nodepolicy.Follow{{Series: &id, Pools: []uint64{1}}}
	pol.RPC.Listen = "off"
	pol.NoSync = true
	edit(&pol)
	dir := t.TempDir()
	must(t, nodepolicy.Save(filepath.Join(dir, nodepolicy.FileName), pol))
	n, err := node.Open(dir, node.Options{})
	must(t, err)
	tn := &tnode{t: t, name: name, n: n, done: make(chan struct{}), log: &syncBuf{}, key: engine.PoolKey{Series: id, Index: 1}}
	addr := freeAddr(t)
	cfg := p2p.FromPolicy(pol)
	cfg.Listen, cfg.Advertise, cfg.AllowLocalPeers = addr, addr, true
	cfg.CheckEvery, cfg.ManageEvery, cfg.PingEvery = -1, -1, -1
	cfg.RequestTimeout, cfg.FollowTimeout, cfg.Log = 5*time.Second, 500*time.Millisecond, tn.log
	cfg.ChaseSlice = 20 * time.Millisecond
	cfg.Chase.YieldWait = time.Millisecond
	cfg.RequestPace = -1
	tr, err := tracking.New(tracking.Config{LoopbackAsOne: true, Budgets: map[string]int{tracking.KindRevert: 1}}, "")
	must(t, err)
	cfg.Tracker = tr
	h, err := p2p.New(n, cfg)
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

// Black R17 (RED-043): every node setting a -settings bundle can carry,
// at its extremes, still lets b reach a's heavier ledger.
func TestBlackBundleSettingsCannotRefuseHeavier(t *testing.T) {
	zero := 0
	for _, c := range []struct {
		name string
		edit func(*nodepolicy.Policy)
	}{
		// The memory limits at their floors and ceilings (RED-063, D160).
		{"k0-tightest", func(p *nodepolicy.Policy) {
			p.K, p.Ties, p.MaxPending, p.SeenSize = &zero, nodepolicy.BoundTies.Min, nodepolicy.BoundMaxPending.Min, nodepolicy.BoundSeenSize.Min
			p.P2P.ChunkSize, p.P2P.EntryLimit, p.P2P.HashLimit, p.P2P.LayerLimit = 1, 1, 1, 1
		}},
		{"k0-largest", func(p *nodepolicy.Policy) {
			p.K, p.Ties, p.MaxPending, p.SeenSize = &zero, nodepolicy.BoundTies.Max, nodepolicy.BoundMaxPending.Max, nodepolicy.BoundSeenSize.Max
			p.P2P.ChunkSize, p.P2P.EntryLimit, p.P2P.HashLimit, p.P2P.LayerLimit = math.MaxUint32, math.MaxUint32, math.MaxUint32, math.MaxUint32
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			served, err := tracking.New(tracking.Config{LoopbackAsOne: true, Budgets: map[string]int{tracking.KindRequest: 1 << 30}}, "")
			must(t, err)
			a := startNode(t, "a", func(c *p2p.Config) { c.PartsOver = 128; c.Tracker = served })
			b := startNodePol(t, "b", c.edit)
			last, owner := grow(t, a, nil, "alice", 3, 32)
			copyLedger(t, a, b)
			growTagged(t, b, last, owner, "b", 8, 8)
			growTagged(t, a, last, owner, "a", 4, 32)
			bareRun(t, a, 600)
			if a.state() == b.state() {
				t.Fatal("setup: same ledger")
			}
			b.connect(a)
			eventuallyFor(t, 60*time.Second, "b to reach a's ledger", func() bool { return a.state() == b.state() }, a, b)
		})
	}
}
