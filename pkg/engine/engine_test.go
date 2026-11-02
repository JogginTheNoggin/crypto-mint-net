package engine_test

import (
	"errors"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/engine"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/entry"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/layer"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/ledger"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/series"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/testutil"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tx"
)

var P = testutil.P

// network is an in-process set of engines that forward to each other the
// way the forwarding rules say: a kept, pushed item goes to every other
// node, which takes it in as pushed.
type network struct {
	t     *testing.T
	nodes []*engine.Engine
	key   engine.PoolKey
}

func newNetwork(t *testing.T, s *series.Series, n int) *network {
	net := &network{t: t, key: engine.PoolKey{Series: s.ID(), Index: 1}}
	for i := 0; i < n; i++ {
		e := engine.New(engine.DefaultConfig())
		e.AddSeries(s)
		if err := e.FollowSeries(s.ID()); err != nil {
			t.Fatal(err)
		}
		net.nodes = append(net.nodes, e)
	}
	return net
}

type item struct {
	tx    *tx.Tx
	entry *entry.Entry
	layer *layer.Layer
}

// flood delivers an item kept by node from to every other node, and so on
// until nothing new is forwarded.
func (n *network) flood(from int, it item) {
	type msg struct {
		to int
		it item
	}
	var queue []msg
	for i := range n.nodes {
		if i != from {
			queue = append(queue, msg{i, it})
		}
	}
	for len(queue) > 0 {
		m := queue[0]
		queue = queue[1:]
		node := n.nodes[m.to]
		var forward bool
		var err error
		switch {
		case m.it.tx != nil:
			var r engine.TxResult
			r, err = node.HandleTx(m.it.tx, engine.Pushed)
			forward = r.Forward
		case m.it.entry != nil:
			var r engine.EntryResult
			r, err = node.HandleEntry(n.key, m.it.entry, engine.Pushed)
			forward = r.Forward
		case m.it.layer != nil:
			var r engine.LayerResult
			r, err = node.HandleLayer(n.key, *m.it.layer, engine.Pushed)
			forward = r.Forward
		}
		if err != nil && !errors.Is(err, engine.ErrSeen) {
			n.t.Logf("node %d refused an item: %v", m.to, err)
		}
		if forward {
			for i := range n.nodes {
				if i != m.to {
					queue = append(queue, msg{i, m.it})
				}
			}
		}
	}
}

func (n *network) submit(node int, t *tx.Tx) {
	n.t.Helper()
	if _, err := n.nodes[node].HandleTx(t, engine.Pushed); err != nil {
		n.t.Fatalf("submit: %v", err)
	}
	n.flood(node, item{tx: t})
}

func (n *network) build(node int) *entry.Entry {
	n.t.Helper()
	e, _, err := n.nodes[node].Build(n.key, 0)
	if err != nil {
		n.t.Fatalf("build: %v", err)
	}
	n.flood(node, item{entry: e})
	return e
}

func (n *network) work(node int, e hash.Hash, tries uint64) {
	n.t.Helper()
	l, _, err := n.nodes[node].Work(n.key, e, uint64(node)<<40, tries)
	if err != nil {
		n.t.Fatalf("work: %v", err)
	}
	n.flood(node, item{layer: &l})
}

func (n *network) agree() engine.PoolStatus {
	n.t.Helper()
	first, _ := n.nodes[0].Status(n.key)
	for i, node := range n.nodes[1:] {
		st, _ := node.Status(n.key)
		if st.Tip != first.Tip || st.Weight.Cmp(first.Weight) != 0 {
			n.t.Fatalf("node %d disagrees: tip %s weight %s, node 0 tip %s weight %s",
				i+1, st.Tip.Short(), st.Weight, first.Tip.Short(), first.Weight)
		}
	}
	return first
}

func TestPaymentSpreadsAndSettles(t *testing.T) {
	s := testutil.ExampleSeries()
	k := testutil.NewKeys(s)
	net := newNetwork(t, s, 4)

	claim := k.Claim(4, P("alice", 100))
	net.submit(0, claim)
	e1 := net.build(1)
	net.work(2, e1.Hash(s), 2000)
	st := net.agree()
	if st.Height != 1 || st.Pending != 0 {
		t.Fatalf("after E1: %+v", st)
	}

	// Alice pays the store. Anyone adds layers, knowing only the hash.
	pay := k.Spend(4, []testutil.In{k.Out(claim, 0, "alice")}, P("store", 30), P("alice2", 70))
	net.submit(3, pay)
	e2 := net.build(0)
	for i := range net.nodes {
		net.work(i, e2.Hash(s), 500)
	}
	st = net.agree()
	if st.Height != 2 {
		t.Fatalf("after E2: %+v", st)
	}
	p, err := net.nodes[3].Payment(net.key, pay.TID(s.Hash()))
	if err != nil || !p.Selected || p.Entry != e2.Hash(s) || p.From.IsZero() {
		t.Fatalf("payment status %+v %v", p, err)
	}
	hs, _ := net.nodes[1].Holdings(s.ID(), 4)
	if len(hs) != 2 {
		t.Fatalf("holdings %+v", hs)
	}
}

func TestDoubleSpendResolvedByWeight(t *testing.T) {
	s := testutil.ExampleSeries()
	k := testutil.NewKeys(s)
	net := newNetwork(t, s, 3)

	claim := k.Claim(5, P("alice", 100))
	net.submit(0, claim)
	e1 := net.build(0)
	net.work(0, e1.Hash(s), 1000)

	honest := k.Spend(5, []testutil.In{k.Out(claim, 0, "alice")}, P("store", 100))
	net.submit(0, honest)
	e2 := net.build(0)
	net.work(1, e2.Hash(s), 300)

	// Alice builds a rival on E1 that pays herself, and grinds hard on it.
	rivalTx := k.Spend(5, []testutil.In{k.Out(claim, 0, "alice")}, P("alice-other", 100))
	// The honest spend is pending nowhere now, so nodes refuse the rival as a
	// pending transaction: it conflicts with the selected ledger.
	if _, err := net.nodes[2].HandleTx(rivalTx, engine.Pushed); err == nil {
		t.Fatal("a spend of a spent portion must be refused as pending")
	}
	rival, _, err := entry.Build(s, e1.Hash(s), []*tx.Tx{rivalTx}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := net.nodes[2].HandleEntry(net.key, rival, engine.Pushed); err != nil {
		t.Fatalf("a valid rival entry must be kept: %v", err)
	}
	net.flood(2, item{entry: rival})
	before := net.agree()
	if before.Tip != e2.Hash(s) {
		t.Fatal("the rival has no layers yet")
	}
	// Grind until the rival is heavier.
	for i := uint64(0); ; i++ {
		l, _, err := net.nodes[2].Work(net.key, rival.Hash(s), i*100000, 100000)
		if err != nil {
			t.Fatal(err)
		}
		net.flood(2, item{layer: &l})
		if st := net.agree(); st.Tip == rival.Hash(s) {
			break
		}
	}
	// Every node reports the store's payment as reversed.
	for _, node := range net.nodes {
		p, _ := node.Payment(net.key, honest.TID(s.Hash()))
		if p.Selected {
			t.Fatal("the honest payment should be out of the selected ledger")
		}
	}
	// The store's side restores the honest ledger by adding layers to it.
	for i := uint64(0); ; i++ {
		l, _, err := net.nodes[1].Work(net.key, e2.Hash(s), (i+1)<<32, 100000)
		if err != nil {
			t.Fatal(err)
		}
		net.flood(1, item{layer: &l})
		if st := net.agree(); st.Tip == e2.Hash(s) {
			break
		}
	}
}

func TestFollowingAndOrigins(t *testing.T) {
	s := testutil.ExampleSeries()
	k := testutil.NewKeys(s)
	e := engine.New(engine.DefaultConfig())
	claim := k.Claim(4, P("alice", 100))
	if _, err := e.HandleTx(claim, engine.Pushed); !errors.Is(err, engine.ErrUnknownSeries) {
		t.Fatalf("want ErrUnknownSeries, got %v", err)
	}
	e.AddSeries(s)
	if _, err := e.HandleTx(claim, engine.Pushed); !errors.Is(err, engine.ErrNotFollowed) {
		t.Fatalf("want ErrNotFollowed, got %v", err)
	}
	if err := e.FollowMint(s.ID(), 6); err != nil { // mint 6 is in pool 1, with mint 4
		t.Fatal(err)
	}
	if !e.Follows(engine.PoolKey{Series: s.ID(), Index: 1}) || e.Follows(engine.PoolKey{Series: s.ID(), Index: 0}) {
		t.Fatal("following a mint follows exactly its pool")
	}
	if r, err := e.HandleTx(claim, engine.Pushed); err != nil || !r.Forward {
		t.Fatalf("followed pool: %v %+v", err, r)
	}
	if _, err := e.HandleTx(claim, engine.Pushed); !errors.Is(err, engine.ErrSeen) {
		t.Fatalf("want ErrSeen, got %v", err)
	}
	en, res, err := e.Build(engine.PoolKey{Series: s.ID(), Index: 1}, 0)
	if err != nil || !res.Forward {
		t.Fatalf("build: %v", err)
	}
	// Requested data is kept but never forwarded.
	other := engine.New(engine.DefaultConfig())
	other.AddSeries(s)
	other.FollowSeries(s.ID())
	r, err := other.HandleEntry(res.Key, en, engine.Requested)
	if err != nil || r.Forward {
		t.Fatalf("requested entry: forward=%v err=%v", r.Forward, err)
	}
	// The entry has no credit yet, so the claim is still pending and
	// building again gives the same entry.
	if _, _, err := e.Build(res.Key, 0); !errors.Is(err, engine.ErrAlreadyBuilt) {
		t.Fatalf("want ErrAlreadyBuilt, got %v", err)
	}
	if _, _, err := e.Work(res.Key, en.Hash(s), 0, 100); err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.Build(res.Key, 0); !errors.Is(err, engine.ErrNothingPending) {
		t.Fatalf("want ErrNothingPending, got %v", err)
	}
	if _, _, err := e.Work(res.Key, hash.Zero, 0, 10); !errors.Is(err, ledger.ErrUnknownEntry) {
		t.Fatalf("want ErrUnknownEntry, got %v", err)
	}
	if err := e.FollowPool(s.ID(), 99); err == nil {
		t.Fatal("pool out of range must fail")
	}
}

func TestSeen(t *testing.T) {
	s := engine.NewSeen(2)
	a, b, c := hash.Hash{1}, hash.Hash{2}, hash.Hash{3}
	s.Add(a)
	s.Add(b)
	if !s.Has(a) || !s.Has(b) || s.Len() != 2 {
		t.Fatal("seen")
	}
	s.Add(c)
	if s.Has(a) || !s.Has(c) || s.Len() != 2 {
		t.Fatal("oldest should drop out")
	}
	s.Add(c)
	s.Add(c)
	if !s.Has(c) || s.Has(b) {
		t.Fatal("repeated adds")
	}
}
