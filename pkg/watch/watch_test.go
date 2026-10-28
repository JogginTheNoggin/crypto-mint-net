package watch_test

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/engine"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/entry"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/filter"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/node"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/nodepolicy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/series"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/testutil"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tx"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/watch"
)

var P = testutil.P

type fixture struct {
	t   *testing.T
	n   *node.Node
	s   *series.Series
	k   *testutil.Keys
	key engine.PoolKey
}

// newFixture opens a node that follows the whole example series.
func newFixture(t *testing.T) *fixture {
	t.Helper()
	s := testutil.ExampleSeries()
	id := s.ID()
	p := nodepolicy.Default()
	p.Series = []nodepolicy.SeriesInfo{{Params: s.Params()}}
	p.Follow = []nodepolicy.Follow{{Series: &id}}
	p.RPC.Listen = "off"
	dir := t.TempDir()
	must(t, nodepolicy.Save(filepath.Join(dir, nodepolicy.FileName), p))
	n, err := node.Open(dir, node.Options{})
	must(t, err)
	t.Cleanup(func() { n.Close() })
	return &fixture{t: t, n: n, s: s, k: testutil.NewKeys(s), key: engine.PoolKey{Series: id, Index: 1}}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func drain(s *watch.Sub) []watch.Event {
	var out []watch.Event
	for {
		select {
		case e := <-s.C:
			out = append(out, e)
		default:
			return out
		}
	}
}

func kinds(evs []watch.Event) map[filter.Kind]int {
	m := map[filter.Kind]int{}
	for _, e := range evs {
		m[e.Kind]++
	}
	return m
}

func parse(t *testing.T, args ...string) filter.Filter {
	t.Helper()
	var b filter.Builder
	for i := 0; i < len(args); i += 2 {
		must(t, b.Add(args[i], args[i+1]))
	}
	return b.Filter()
}

// A claim, a spend and a rival double spend that wins: every watcher sees
// what its filter selects, in order, and the views agree with the ledger.
func TestWatchAndViews(t *testing.T) {
	f := newFixture(t)
	n, s, k := f.n, f.s, f.k
	hub := watch.NewHub(n)
	all := hub.Subscribe(filter.Filter{}, 0)
	alice := hub.Subscribe(parse(t, "address", k.Addr("alice").String()), 0)
	dave := hub.Subscribe(parse(t, "address", k.Addr("dave").String()), 0)
	other := hub.Subscribe(parse(t, "pool", "0"), 0)
	selected := hub.Subscribe(parse(t, "kind", "selected,reverted"), 0)
	defer hub.Close()

	c4 := k.Claim(4, P("alice", 100))
	_, err := n.HandleTx(c4, engine.Local)
	must(t, err)
	e1, _, err := n.Build(f.key, 0)
	must(t, err)
	_, _, err = n.Work(f.key, e1.Hash(s), 0, 100)
	must(t, err)
	spend := k.Spend(4, []testutil.In{k.Out(c4, 0, "alice")}, P("dave", 30), P("alice2", 70))
	_, err = n.HandleTx(spend, engine.Local)
	must(t, err)
	e2, _, err := n.Build(f.key, 0)
	must(t, err)
	_, _, err = n.Work(f.key, e2.Hash(s), 0, 20)
	must(t, err)
	// A rival spending the same portion, worked until it is selected.
	rival := k.Spend(4, []testutil.In{k.Out(c4, 0, "alice")}, P("eve", 100))
	r, _, err := entry.Build(s, e1.Hash(s), []*tx.Tx{rival}, 0)
	must(t, err)
	_, err = n.HandleEntry(f.key, r, engine.Pushed)
	must(t, err)
	for start := uint64(0); ; start += 1 << 12 {
		_, _, err = n.Work(f.key, r.Hash(s), start, 1<<12)
		must(t, err)
		if st, _ := n.Engine().Status(f.key); st.Tip == r.Hash(s) {
			break
		}
	}

	evs := drain(all)
	got := kinds(evs)
	if got[filter.KindTx] != 2 || got[filter.KindEntry] != 3 || got[filter.KindSwitch] != 1 || got[filter.KindReverted] != 1 || got[filter.KindSelected] < 3 {
		t.Fatalf("all: %v", got)
	}
	for i := 1; i < len(evs); i++ {
		if evs[i].Seq <= evs[i-1].Seq {
			t.Fatal("events out of order")
		}
	}
	var sw *watch.Event
	for i := range evs {
		if evs[i].Kind == filter.KindSwitch {
			sw = &evs[i]
		}
	}
	if sw.Switch.From != e2.Hash(s) || sw.Switch.To != r.Hash(s) || len(sw.Switch.Void) != 1 || sw.Switch.Void[0] != spend.TID(s.Hash()) {
		t.Fatalf("switch %+v", sw.Switch)
	}
	// Alice is in every transaction; dave only in the spend.
	if got := kinds(drain(alice)); got[filter.KindTx] != 2 || got[filter.KindEntry] != 3 || got[filter.KindSwitch] != 1 || got[filter.KindReverted] != 1 {
		t.Fatalf("alice: %v", got)
	}
	for _, e := range drain(dave) {
		if e.Kind == filter.KindLayer && e.Layer.Entry != e2.Hash(s) {
			t.Fatalf("dave got a layer on another entry: %+v", e.Layer)
		}
		if e.Kind == filter.KindEntry && e.Entry.Hash != e2.Hash(s) {
			t.Fatalf("dave got another entry: %+v", e.Entry)
		}
	}
	if evs := drain(other); len(evs) != 0 {
		t.Fatalf("pool 0 watcher got %d events", len(evs))
	}
	sel := drain(selected)
	if kinds(sel)[filter.KindReverted] != 1 || len(sel) != kinds(sel)[filter.KindSelected]+1 {
		t.Fatalf("selected watcher: %v", kinds(sel))
	}
	for _, e := range sel {
		if e.Kind == filter.KindReverted && (!e.Void || *e.TID != spend.TID(s.Hash()) || e.Tx == nil) {
			t.Fatalf("reverted %+v", e)
		}
	}
	// Every event is JSON.
	if _, err := json.Marshal(evs); err != nil {
		t.Fatal(err)
	}

	eng := n.Engine()
	if ps := watch.Pools(eng, filter.Filter{}); len(ps) != 2 || ps[1].Height != 2 {
		t.Fatalf("pools %+v", ps)
	}
	if ps := watch.Pools(eng, parse(t, "serial", "5")); len(ps) != 1 || ps[0].Pool != 1 {
		t.Fatalf("pools of serial 5 %+v", ps)
	}
	lp, err := watch.LedgerPage(eng, f.key, filter.Filter{}, 0, 1)
	must(t, err)
	if len(lp.Entries) != 1 || lp.Entries[0].Hash != r.Hash(s) || lp.Next != 1 {
		t.Fatalf("ledger page %+v", lp)
	}
	lp, _ = watch.LedgerPage(eng, f.key, parse(t, "address", k.Addr("eve").String()), 0, 10)
	if len(lp.Entries) != 1 || len(lp.Entries[0].Txs) != 1 || lp.Next != 0 {
		t.Fatalf("eve's entries %+v", lp)
	}
	st, err := watch.TxOfSeries(eng, s.ID(), rival.TID(s.Hash()), nil)
	must(t, err)
	if !st.Selected || st.Height != 2 || st.Tx == nil || st.Tx.Raw == "" || *st.Tx.Inputs[0].Address != k.Addr("alice") {
		t.Fatalf("tx status %+v", st)
	}
	if _, err := watch.TxOfSeries(eng, s.ID(), spend.TID(s.Hash()), nil); err != watch.ErrNotFound {
		t.Fatalf("void spend: %v", err)
	}
	a, err := watch.AddressOf(eng, s.ID(), k.Addr("alice"), filter.Filter{}, 0)
	must(t, err)
	if a.Balance != 0 || len(a.History) != 2 || a.History[0].Spends != 1 || a.History[1].Received != 100 {
		t.Fatalf("alice %+v", a)
	}
	eve, _ := watch.AddressOf(eng, s.ID(), k.Addr("eve"), filter.Filter{}, 0)
	if eve.Balance != 100 || len(eve.Holdings) != 1 {
		t.Fatalf("eve %+v", eve)
	}

	// Pending, by policy.
	c5 := k.Claim(5, P("bob", 60), P("carol", 40))
	_, err = n.HandleTx(c5, engine.Pushed)
	must(t, err)
	bob := parse(t, "policy", hexOf(k.Get("bob").Policy.Encode()))
	if pend := watch.Pending(eng, bob); len(pend) != 1 || pend[0].TID != c5.TID(s.Hash()) || pend[0].Pool != 1 {
		t.Fatalf("pending %+v", pend)
	}
	if pend := watch.Pending(eng, parse(t, "address", k.Addr("dave").String())); len(pend) != 0 {
		t.Fatalf("dave pending %+v", pend)
	}
}

// A watcher that falls behind loses events and is told how many.
func TestSlowWatcherMisses(t *testing.T) {
	f := newFixture(t)
	hub := watch.NewHub(f.n)
	slow := hub.Subscribe(filter.Filter{}, 1)
	for serial := uint64(4); serial < 7; serial++ {
		_, err := f.n.HandleTx(f.k.Claim(serial, P("a", 100)), engine.Local)
		must(t, err)
	}
	if got := len(drain(slow)); got != 1 {
		t.Fatalf("%d events", got)
	}
	if m := slow.Missed(); m != 2 {
		t.Fatalf("missed %d", m)
	}
	slow.Close()
	slow.Close()
	if _, ok := <-slow.C; ok {
		t.Fatal("closed subscriber still open")
	}
}

func hexOf(b []byte) string { return hex.EncodeToString(b) }

// RED-008: the address view looks at no more than 50*limit entries per
// request, however tall the ledger, says where to go on, and stops when
// its context is done.
func TestAddressPageIsBounded(t *testing.T) {
	f := newFixture(t)
	n, s, k := f.n, f.s, f.k
	prev := k.Claim(4, P("alice", 100))
	const height = 120
	for i := 0; i < height; i++ {
		if i > 0 {
			prev = k.Spend(4, []testutil.In{k.Out(prev, 0, "alice")}, P("alice", 100))
		}
		_, err := n.HandleTx(prev, engine.Local)
		must(t, err)
		e, _, err := n.Build(f.key, 0)
		must(t, err)
		_, _, err = n.Work(f.key, e.Hash(s), 0, 20)
		must(t, err)
	}
	if st, _ := n.Engine().Status(f.key); st.Height != height {
		t.Fatalf("height %d", st.Height)
	}
	eng := n.Engine()
	scope := parse(t, "pool", "1")
	// An address with no history: each page looks at 50 entries.
	var walked []int
	below := 0
	for {
		a, err := watch.AddressPage(context.Background(), eng, s.ID(), k.Addr("zed"), scope, below, 1)
		must(t, err)
		if len(a.History) != 0 {
			t.Fatalf("zed has history %+v", a.History)
		}
		if len(a.Next) == 0 {
			break
		}
		if len(a.Next) != 1 || a.Next[0].Pool != 1 {
			t.Fatalf("next %+v", a.Next)
		}
		below = a.Next[0].Below
		walked = append(walked, below)
	}
	if len(walked) != 2 || walked[0] != height-50 || walked[1] != height-100 {
		t.Fatalf("pages ended at %v, want [%d %d]", walked, height-50, height-100)
	}
	// An address with history: a page stops at limit items, newest first,
	// and the next page carries on below them.
	a, err := watch.AddressPage(context.Background(), eng, s.ID(), k.Addr("alice"), scope, 0, 3)
	must(t, err)
	if len(a.History) != 3 || a.History[0].Height != height || len(a.Next) != 1 || a.Next[0].Below != height-3 {
		t.Fatalf("alice page %d items, next %+v", len(a.History), a.Next)
	}
	b, err := watch.AddressPage(context.Background(), eng, s.ID(), k.Addr("alice"), scope, a.Next[0].Below, 3)
	must(t, err)
	if len(b.History) != 3 || b.History[0].Height != height-3 {
		t.Fatalf("second page %+v", b.History)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := watch.AddressPage(ctx, eng, s.ID(), k.Addr("zed"), scope, 0, 10); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled: %v", err)
	}
}

// TestSubscribeAfterClose: a subscriber that arrives after the hub closed,
// as a watch request racing a reload can, is closed at once instead of
// waiting forever (and holding the interface's shutdown open).
func TestSubscribeAfterClose(t *testing.T) {
	f := newFixture(t)
	hub := watch.NewHub(f.n)
	hub.Close()
	s := hub.Subscribe(filter.Filter{}, 0)
	select {
	case _, ok := <-s.C:
		if ok {
			t.Fatal("an event on a closed hub")
		}
	case <-time.After(time.Second):
		t.Fatal("a subscriber of a closed hub stays open")
	}
	s.Close() // and closing it again is fine
}
