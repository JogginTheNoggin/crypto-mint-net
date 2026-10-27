package mempool_test

import (
	"errors"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/entry"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/layer"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/ledger"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/mempool"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/testutil"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tx"
)

var P = testutil.P

type view struct{ p *ledger.Pool }

func (v view) Unspent(in tx.Input) (tx.Portion, bool) { return v.p.State().Unspent(in) }
func (v view) Claimed(s uint64) bool                  { return v.p.State().Claimed(s) }
func (v view) Locate(t hash.Hash) (hash.Hash, bool)   { return v.p.Locate(t) }

func setup(t *testing.T) (*testutil.Keys, *ledger.Pool, *mempool.Pool, *tx.Tx) {
	s := testutil.ExampleSeries()
	k := testutil.NewKeys(s)
	lp := ledger.New(s, 1, ledger.Config{K: ledger.Unlimited})
	claim := k.Claim(4, P("alice", 100))
	e, h, _ := entry.Build(s, lp.ID(), []*tx.Tx{claim}, 0)
	if _, err := lp.AddEntry(e); err != nil {
		t.Fatal(err)
	}
	l, _ := layer.GrindTo(s.Hash(), h, 0, 1, 1000)
	if _, err := lp.AddLayer(l); err != nil {
		t.Fatal(err)
	}
	return k, lp, mempool.New(s, 0), claim
}

func tid(x *tx.Tx) hash.Hash { return x.TID(testutil.ExampleSeries().Hash()) }

func TestFirstComeFirstServed(t *testing.T) {
	k, lp, mp, claim := setup(t)
	v := view{lp}
	toBob := k.Spend(4, []testutil.In{k.Out(claim, 0, "alice")}, P("bob", 100))
	toEve := k.Spend(4, []testutil.In{k.Out(claim, 0, "alice")}, P("eve", 100))
	if err := mp.Add(v, toBob, tid(toBob)); err != nil {
		t.Fatal(err)
	}
	if err := mp.Add(v, toBob, tid(toBob)); !errors.Is(err, mempool.ErrKnown) {
		t.Errorf("want ErrKnown, got %v", err)
	}
	if err := mp.Add(v, toEve, tid(toEve)); !errors.Is(err, mempool.ErrConflict) {
		t.Errorf("want ErrConflict, got %v", err)
	}
	// A spend of a pending output is refused.
	onward := k.Spend(4, []testutil.In{k.Out(toBob, 0, "bob")}, P("carol", 100))
	if err := mp.Add(v, onward, tid(onward)); !errors.Is(err, mempool.ErrDependsOnPending) {
		t.Errorf("want ErrDependsOnPending, got %v", err)
	}
	// A claim of an already claimed mint is invalid at the tip.
	if err := mp.Add(v, k.Claim(4, P("x", 100)), tid(k.Claim(4, P("x", 100)))); !errors.Is(err, mempool.ErrInvalid) {
		t.Errorf("want ErrInvalid, got %v", err)
	}
	// Two pending claims of one unclaimed mint conflict.
	c5a, c5b := k.Claim(5, P("a", 100)), k.Claim(5, P("b", 100))
	if err := mp.Add(v, c5a, tid(c5a)); err != nil {
		t.Fatal(err)
	}
	if err := mp.Add(v, c5b, tid(c5b)); !errors.Is(err, mempool.ErrConflict) {
		t.Errorf("want ErrConflict for second claim, got %v", err)
	}
	if got := mp.Ready(); len(got) != 2 || got[0].TID != tid(toBob) {
		t.Fatalf("ready %v", got)
	}
}

func TestLimitAndInSelected(t *testing.T) {
	k, lp, _, claim := setup(t)
	mp := mempool.New(testutil.ExampleSeries(), 1)
	v := view{lp}
	if err := mp.Add(v, claim, tid(claim)); !errors.Is(err, mempool.ErrInSelected) {
		t.Errorf("want ErrInSelected, got %v", err)
	}
	a := k.Claim(5, P("a", 100))
	b := k.Claim(6, P("b", 100))
	if err := mp.Add(v, a, tid(a)); err != nil {
		t.Fatal(err)
	}
	if err := mp.Add(v, b, tid(b)); !errors.Is(err, mempool.ErrFull) {
		t.Errorf("want ErrFull, got %v", err)
	}
}

func TestRevalidateAfterInclusion(t *testing.T) {
	k, lp, mp, claim := setup(t)
	s := testutil.ExampleSeries()
	v := view{lp}
	toBob := k.Spend(4, []testutil.In{k.Out(claim, 0, "alice")}, P("bob", 100))
	c5 := k.Claim(5, P("pat", 100))
	for _, x := range []*tx.Tx{toBob, c5} {
		if err := mp.Add(v, x, tid(x)); err != nil {
			t.Fatal(err)
		}
	}
	tip, _, _ := lp.Tip()
	// A rival claim of mint 5 gets into an entry first.
	rival := k.Claim(5, P("quinn", 100))
	e, h, _ := entry.Build(s, tip, []*tx.Tx{toBob, rival}, 0)
	lp.AddEntry(e)
	l, _ := layer.GrindTo(s.Hash(), h, 0, 1, 1000)
	lp.AddLayer(l)
	dropped := mp.Revalidate(v)
	if mp.Len() != 0 || len(dropped) != 1 || dropped[0] != tid(c5) {
		t.Fatalf("pending %d, dropped %v", mp.Len(), dropped)
	}
}

func TestCarriedChainsWait(t *testing.T) {
	k, lp, mp, claim := setup(t)
	v := view{lp}
	// Suppose a switch displaced two linked payments: a, then b spending a.
	a := k.Spend(4, []testutil.In{k.Out(claim, 0, "alice")}, P("bob", 100))
	b := k.Spend(4, []testutil.In{k.Out(a, 0, "bob")}, P("carol", 100))
	mp.AddCarried(v, []mempool.Item{{TID: tid(a), Tx: a}, {TID: tid(b), Tx: b}})
	if mp.Len() != 2 || mp.Waiting() != 1 {
		t.Fatalf("len %d waiting %d", mp.Len(), mp.Waiting())
	}
	if r := mp.Ready(); len(r) != 1 || r[0].TID != tid(a) {
		t.Fatalf("only a is ready, got %v", r)
	}
	// Once a is in the selected ledger, b becomes ready.
	s := testutil.ExampleSeries()
	tip, _, _ := lp.Tip()
	e, h, _ := entry.Build(s, tip, []*tx.Tx{a}, 0)
	lp.AddEntry(e)
	l, _ := layer.GrindTo(s.Hash(), h, 0, 1, 1000)
	lp.AddLayer(l)
	mp.Revalidate(v)
	if r := mp.Ready(); len(r) != 1 || r[0].TID != tid(b) || mp.Waiting() != 0 {
		t.Fatalf("b should be ready, got %v", r)
	}
}

// TestOlderChildOfCarriedParent is a regression test found by the network
// simulator: b was pending, spending a, which was in the selected ledger.
// A switch displaced a and carried it back, so b waited on it; b is older
// in the pool than a. When a is selected again, the same Revalidate must
// make b ready, though b comes first in the pool's order.
func TestOlderChildOfCarriedParent(t *testing.T) {
	k, lp, mp, claim := setup(t)
	v := view{lp}
	s := testutil.ExampleSeries()
	a := k.Spend(4, []testutil.In{k.Out(claim, 0, "alice")}, P("bob", 100))
	b := k.Spend(4, []testutil.In{k.Out(a, 0, "bob")}, P("carol", 100))
	// a in the selected ledger, b pending on it.
	tip, _, _ := lp.Tip()
	ea, ha, _ := entry.Build(s, tip, []*tx.Tx{a}, 0)
	lp.AddEntry(ea)
	l, _ := layer.GrindTo(s.Hash(), ha, 0, 1, 1000)
	lp.AddLayer(l)
	if err := mp.Add(v, b, tid(b)); err != nil {
		t.Fatal(err)
	}
	// A switch away from a (to its parent, here simulated by a rival
	// sibling) carries a back: b now waits on it.
	rival, hr, _ := entry.Build(s, tip, nil, 0)
	lp.AddEntry(rival)
	for n := uint64(0); lp.InSelected(ha); n += 1000 {
		l, _ := layer.GrindTo(s.Hash(), hr, n, 4, 1000)
		lp.AddLayer(l)
	}
	mp.AddCarried(v, []mempool.Item{{TID: tid(a), Tx: a}})
	mp.Revalidate(v)
	if mp.Waiting() != 1 {
		t.Fatalf("b should wait on a: waiting %d", mp.Waiting())
	}
	// a is bundled again and selected.
	tip, _, _ = lp.Tip()
	ea2, ha2, _ := entry.Build(s, tip, []*tx.Tx{a}, 0)
	lp.AddEntry(ea2)
	for n := uint64(0); !lp.InSelected(ha2); n += 1000 {
		l, _ := layer.GrindTo(s.Hash(), ha2, n, 4, 1000)
		lp.AddLayer(l)
	}
	mp.Revalidate(v)
	if r := mp.Ready(); len(r) != 1 || r[0].TID != tid(b) || mp.Waiting() != 0 {
		t.Fatalf("b should be ready once a is selected again: ready %v, waiting %d", r, mp.Waiting())
	}
}

func TestCarriedEvictsConflictingPending(t *testing.T) {
	k, lp, mp, claim := setup(t)
	v := view{lp}
	pendingToEve := k.Spend(4, []testutil.In{k.Out(claim, 0, "alice")}, P("eve", 100))
	if err := mp.Add(v, pendingToEve, tid(pendingToEve)); err != nil {
		t.Fatal(err)
	}
	carried := k.Spend(4, []testutil.In{k.Out(claim, 0, "alice")}, P("bob", 100))
	mp.AddCarried(v, []mempool.Item{{TID: tid(carried), Tx: carried}})
	if mp.Has(tid(pendingToEve)) || !mp.Has(tid(carried)) {
		t.Fatal("a carried transaction should replace a pending one it conflicts with")
	}
	mp.Remove(tid(carried))
	if mp.Len() != 0 {
		t.Fatal("remove")
	}
}
