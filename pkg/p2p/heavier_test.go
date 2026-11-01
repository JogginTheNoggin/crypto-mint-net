package p2p_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/engine"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/entry"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/ledger"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/p2p"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/testutil"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tracking"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/wire"
)

// The rule behind the consensus audit (docs/consensus-audit.md, P0): a
// node must end up on a heavier valid ledger whatever its settings. Here
// the heavier ledger forks deeper than the revert budget, splits deeper
// than 16 hash chain pages, holds more than 4,096 entries in a row without
// layers, and holds entries larger than the size the sender sends whole,
// which come in parts. The node is run with the default policy and with
// the tightest values every limit allows. TestHeavierLedgerHuge adds an
// entry over 16 MiB holding a transaction of more than 65,536 outputs.
func TestHeavierLedgerAlwaysReached(t *testing.T) {
	const bare = 4100
	for _, c := range []struct {
		name string
		edit func(*p2p.Config)
	}{
		{"default", nil},
		{"tightest", func(c *p2p.Config) {
			c.Limits.HashLimit, c.Limits.EntryLimit, c.Limits.LayerLimit = 1, 1, 1
			c.Limits.ChunkSize = 1
			c.ChaseSlice = 20 * time.Millisecond
			c.Chase.YieldWait = time.Millisecond
			// One hash a page takes thousands of requests, which a serves
			// without a budget: pacing them to a's budget is tested in
			// TestLongChaseIsPaced (ORANGE-018).
			c.RequestPace = -1
			tr, err := tracking.New(tracking.Config{LoopbackAsOne: true, Budgets: map[string]int{tracking.KindRevert: 1}}, "")
			must(t, err)
			c.Tracker = tr
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			// a serves as much as b asks for: its own request budget is
			// not what is tested here.
			served, err := tracking.New(tracking.Config{LoopbackAsOne: true, Budgets: map[string]int{tracking.KindRequest: 1 << 30}}, "")
			must(t, err)
			a := startNode(t, "a", func(c *p2p.Config) { c.PartsOver = 128; c.Tracker = served })
			b := startNode(t, "b", c.edit)
			last, owner := grow(t, a, nil, "alice", 3, 32)
			copyLedger(t, a, b)
			growTagged(t, b, last, owner, "b", 8, 8)
			growTagged(t, a, last, owner, "a", 4, 32) // entries with a transaction go in parts
			if e := a.tipEntry(); len(wire.EncodeItem(wire.EntryItem{Entry: e})) <= 128 {
				t.Fatalf("a's entry is only %d bytes: it would not go in parts", len(e.Encode()))
			}
			bareRun(t, a, bare)
			b.connect(a)
			eventuallyFor(t, 30*time.Second, "b to reach a's ledger", func() bool { return a.state() == b.state() }, a, b)
			if h := b.status().Height; h != 3+4+bare {
				t.Fatalf("b's height %d", h)
			}
		})
	}
}

func (tn *tnode) tipEntry() *entry.Entry {
	var e *entry.Entry
	tn.n.Engine().Ledger(tn.key, func(l *ledger.Pool) {
		tip, _, _ := l.Tip()
		e, _ = l.Entry(tip)
	})
	return e
}

// bareRun adds n entries without layers on tn's tip, then one layer on
// the last, so the run is heavier than the tip it builds on.
func bareRun(t *testing.T, tn *tnode, n int) {
	t.Helper()
	parent := tn.status().Tip
	for i := 0; i < n; i++ {
		e, _, err := entry.Build(exSeries, parent, nil, 0)
		must(t, err)
		_, err = tn.n.HandleEntry(exKey, e, engine.Local)
		must(t, err)
		parent = e.Hash(exSeries)
	}
	_, _, err := tn.n.Work(exKey, parent, 0, 64)
	must(t, err)
}

func eventuallyFor(t *testing.T, d time.Duration, what string, cond func() bool, nodes ...*tnode) {
	t.Helper()
	deadline := time.Now().Add(d)
	for !cond() {
		if time.Now().After(deadline) {
			for _, n := range nodes {
				t.Logf("%s: %s", n.name, n.state())
			}
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// TestHeavierLedgerHuge is the P0 test's large items: an entry over the
// 16 MiB frame, holding one transaction of more than 65,536 outputs, is
// reached in parts. It signs and checks 150,000 outputs, so it is skipped
// with -short and under the race detector; CI runs it on its own.
func TestHeavierLedgerHuge(t *testing.T) {
	if testing.Short() || raceOn {
		t.Skip("large: run without -short and -race")
	}
	const outs = 150000
	p := testutil.ExampleParams()
	p.Supply = outs
	s := testutil.MustSeries(p)
	keys := testutil.NewKeys(s)
	key := engine.PoolKey{Series: s.ID(), Index: 1}
	a := startNodeIn(t, "a", s, nil)
	b := startNodeIn(t, "b", s, nil)
	pays := make([]testutil.Pay, outs)
	for i := range pays {
		pays[i] = P(fmt.Sprintf("o%d", i), 1)
	}
	big := keys.Claim(4, pays...)
	if len(big.Encode()) <= 16<<20 {
		t.Fatalf("the claim is only %d bytes", len(big.Encode()))
	}
	_, err := a.n.HandleTx(big, engine.Local)
	must(t, err)
	e, _, err := a.n.Build(key, 0)
	must(t, err)
	if len(e.Txs) != 1 {
		t.Fatalf("the entry holds %d transactions", len(e.Txs))
	}
	_, _, err = a.n.Work(key, e.Hash(s), 0, 32)
	must(t, err)
	b.connect(a)
	eventuallyFor(t, 5*time.Minute, "b to take the large entry", func() bool { return a.state() == b.state() }, a, b)
}

// Black's B1 (D121): b holds every entry of a's heavier branch but none
// of its layers, so the sender's newest hashes are all held off b's
// selected ledger. The chase walks back to where the branch leaves it,
// checks the sender's layers on it, and switches.
func TestHeldBranchWithoutLayers(t *testing.T) {
	a := startNode(t, "a", nil)
	b := startNode(t, "b", nil)
	last, owner := grow(t, a, nil, "alice", 3, 32)
	copyLedger(t, a, b)
	growTagged(t, b, last, owner, "b", 2, 8)
	growTagged(t, a, last, owner, "a", 4, 64)
	pushEntries(t, a, b, 4)
	if a.state() == b.state() {
		t.Fatal("setup: b already on a's ledger")
	}
	b.connect(a)
	eventuallyFor(t, 30*time.Second, "b to reach a's ledger", func() bool { return a.state() == b.state() }, a, b)
}

// B1's other half: the sender's ledger is a prefix of b's selected one,
// or the same entries, with more layers deep down. b asks for the layers
// on the sender's own hashes, page by page from its tip, until it is as
// heavy as stated.
func TestDeepLayersAreToppedUp(t *testing.T) {
	small := func(c *p2p.Config) { c.Limits.HashLimit = 2 }
	a := startNode(t, "a", small)
	b := startNode(t, "b", small)
	grow(t, a, nil, "alice", 12, 8)
	copyLedger(t, a, b)
	// Heavy new layers on a's third entry only.
	var third hash.Hash
	a.n.Engine().Ledger(exKey, func(l *ledger.Pool) { third = l.Chain(3, 3)[0] })
	_, _, err := a.n.Work(exKey, third, 1<<20, 1<<12)
	must(t, err)
	if a.state() == b.state() {
		t.Fatal("setup: no heavier layers")
	}
	b.connect(a)
	eventuallyFor(t, 30*time.Second, "b to take a's layers", func() bool { return a.state() == b.state() }, a, b)
}

// pushEntries hands dst src's selected entries from height from on,
// without their layers.
func pushEntries(t *testing.T, src, dst *tnode, from int) {
	t.Helper()
	var hs []hash.Hash
	src.n.Engine().Ledger(src.key, func(l *ledger.Pool) { hs = l.Chain(from, l.Height()) })
	for _, h := range hs {
		src.n.Engine().Ledger(src.key, func(l *ledger.Pool) {
			e, _ := l.Entry(h)
			_, err := dst.n.HandleEntry(dst.key, e, engine.Pushed)
			must(t, err)
		})
	}
}

// Black's C-3 (D121): the branch's work sits in more layers on one entry
// than the sender's layers reply carries whole. The chase pages them all
// before it decides, so it reaches the heavier ledger.
func TestManyLayersOnOneEntryAreCounted(t *testing.T) {
	a := startNode(t, "a", func(c *p2p.Config) { c.LayersWhole = 1 })
	b := startNode(t, "b", nil)
	last, owner := grow(t, a, nil, "alice", 3, 32)
	copyLedger(t, a, b)
	growTagged(t, b, last, owner, "b", 2, 32)
	growTagged(t, a, last, owner, "a", 1, 8)
	tip := a.status().Tip
	for i := uint64(1); i <= 12; i++ {
		_, _, err := a.n.Work(exKey, tip, i<<20, 32)
		must(t, err)
	}
	if !a.status().Weight.Greater(b.status().Weight) {
		t.Fatal("setup: a is not heavier")
	}
	b.connect(a)
	eventuallyFor(t, 30*time.Second, "b to reach a's ledger", func() bool { return a.state() == b.state() }, a, b)
}
