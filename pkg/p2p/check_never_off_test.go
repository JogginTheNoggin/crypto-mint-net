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
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tracking"
)

// TestCheckSecondsNeverOff is Black's B7 as node policy (D153): no
// check_seconds turns the weight check off. -1 and values over the most
// are read as the longest interval, so TestBlackCheckFindsLaterHeavier's
// path (a check finds a heavier ledger a no-notices peer holds) is always
// there; Black's own test of it with -1 would wait an hour.
func TestCheckSecondsNeverOff(t *testing.T) {
	for _, c := range []struct {
		set  int
		want time.Duration
	}{{-1, time.Hour}, {nodepolicy.MaxCheckSeconds + 1, time.Hour}, {1 << 30, time.Hour}, {30, 30 * time.Second}, {0, 0}} { // 0: New takes the default
		p := nodepolicy.Default()
		p.P2P.CheckSeconds = c.set
		if got := p2p.FromPolicy(p).CheckEvery; got != c.want {
			t.Errorf("check_seconds %d: CheckEvery %v, want %v", c.set, got, c.want)
		}
	}
	// Load reads it so too, and says so.
	dir := t.TempDir()
	p := nodepolicy.Default()
	p.P2P.CheckSeconds = -1
	must(t, nodepolicy.Save(filepath.Join(dir, nodepolicy.FileName), p))
	got, err := nodepolicy.Load(filepath.Join(dir, nodepolicy.FileName))
	must(t, err)
	if got.P2P.CheckSeconds != nodepolicy.MaxCheckSeconds || len(got.Lowered) != 1 || got.Lowered[0].Field != "p2p.check_seconds" {
		t.Fatalf("loaded: %d, %+v", got.P2P.CheckSeconds, got.Lowered)
	}
}

func startNodePolKeep(t *testing.T, name string, edit func(*nodepolicy.Policy)) *tnode {
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
	cfg.ManageEvery = -1
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

// From Black's Round 18 review (B7): with b checking every second: the check is what finds it.
func TestBlackCheckFindsLaterHeavier(t *testing.T) {
	served := func() *tracking.Tracker {
		tr, err := tracking.New(tracking.Config{LoopbackAsOne: true, Budgets: map[string]int{tracking.KindRequest: 1 << 30}}, "")
		must(t, err)
		return tr
	}
	c := startNode(t, "c", func(cf *p2p.Config) { cf.PartsOver = 128; cf.Tracker = served() })
	a := startNode(t, "a", func(cf *p2p.Config) { cf.PartsOver = 128; cf.Tracker = served(); cf.NoNotices = true })
	b := startNodePolKeep(t, "b", func(p *nodepolicy.Policy) {
		p.P2P.CheckSeconds, p.P2P.PingSeconds = 1, -1
		p.P2P.ChunkSize, p.P2P.EntryLimit, p.P2P.HashLimit, p.P2P.LayerLimit = 1, 1, 1, 1
	})
	last, owner := grow(t, c, nil, "alice", 3, 32)
	copyLedger(t, c, a)
	copyLedger(t, c, b)
	b.connect(a)
	time.Sleep(200 * time.Millisecond)
	growTagged(t, c, last, owner, "c", 4, 32)
	bareRun(t, c, 300)
	a.connect(c)
	eventuallyFor(t, 60*time.Second, "a to reach c's ledger", func() bool { return a.state() == c.state() }, a, c)
	eventuallyFor(t, 60*time.Second, "b to reach a's ledger", func() bool { return a.state() == b.state() }, a, b)
}

// With check -1, one new entry on a's ledger is enough: b chases it.
func TestBlackNoCheckCaughtByNextEntry(t *testing.T) {
	zero := 0
	served := func() *tracking.Tracker {
		tr, err := tracking.New(tracking.Config{LoopbackAsOne: true, Budgets: map[string]int{tracking.KindRequest: 1 << 30}}, "")
		must(t, err)
		return tr
	}
	c := startNode(t, "c", func(cf *p2p.Config) { cf.PartsOver = 128; cf.Tracker = served() })
	a := startNode(t, "a", func(cf *p2p.Config) { cf.PartsOver = 128; cf.Tracker = served(); cf.NoNotices = true })
	b := startNodePol(t, "b", func(p *nodepolicy.Policy) {
		p.K, p.Ties = &zero, 0
		p.P2P.CheckSeconds, p.P2P.PingSeconds = -1, -1
	})
	last, owner := grow(t, c, nil, "alice", 3, 32)
	copyLedger(t, c, a)
	copyLedger(t, c, b)
	b.connect(a)
	time.Sleep(200 * time.Millisecond)
	last, owner = growTagged(t, c, last, owner, "c", 4, 32)
	a.connect(c)
	eventuallyFor(t, 60*time.Second, "a to reach c's ledger", func() bool { return a.state() == c.state() }, a, c)
	time.Sleep(2 * time.Second)
	if a.state() == b.state() {
		t.Fatal("b reached it without a new entry")
	}
	growTagged(t, a, last, owner, "a", 1, 32)
	eventuallyFor(t, 60*time.Second, "b to reach a's ledger", func() bool { return a.state() == b.state() }, a, b)
}
