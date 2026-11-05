package ledger_test

import (
	"errors"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/entry"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/ledger"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/testutil"
)

// chainOf builds a selected ledger of claims and spends of mint 4 on both
// pools, so the cold entries' undo records hold real spends.
func chainOf(t *testing.T, f *fixture, tw *twin, n int) []*entry.Entry {
	t.Helper()
	select1 := func(e *entry.Entry) {
		l := layerOf(t, f.s, e.Hash(f.s), 1, 1)
		if _, err := f.pool.AddLayer(l); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.pool.AddLayer(l); err != nil {
			t.Fatal(err)
		}
	}
	parent := f.pool.ID()
	claim := f.k.Claim(4, P("o0", 100))
	e := f.build(parent, claim)
	f.add(e)
	tw.addEntry(t, 0, e, nil)
	select1(e)
	es := []*entry.Entry{e}
	prev, owner := claim, "o0"
	for i := 1; i < n; i++ {
		to := "o1"
		if owner == "o1" {
			to = "o0"
		}
		sp := f.k.Spend(4, []testutil.In{f.k.Out(prev, 0, owner)}, P(to, 100))
		e = f.build(e.Hash(f.s), sp)
		f.add(e)
		tw.addEntry(t, i, e, nil)
		select1(e)
		es = append(es, e)
		prev, owner = sp, to
	}
	return es
}

// A switch below the window reads the items and undo back and ends where
// a pool with everything in memory ends (D175).
func TestDeepSwitchReadsBack(t *testing.T) {
	f := newFixture(t, ledger.Config{K: ledger.Unlimited})
	tw := newTwin(f.s, ledger.Config{K: ledger.Unlimited})
	es := chainOf(t, f, tw, 12)
	if _, h, _ := f.pool.Tip(); h != 12 {
		t.Fatalf("height %d", h)
	}
	tw.compare(t, 12, f.pool)
	if c := tw.pool.Cold(); c < 8 {
		t.Fatalf("%d cold entries, want most of 12", c)
	}
	// A heavier branch from the second entry: it reverts ten entries, all
	// but the newest cold, and spends mint 4's claim output differently.
	parent := es[0].Hash(f.s)
	claim := es[0].Txs[0]
	rival := f.build(parent, f.k.Spend(4, []testutil.In{f.k.Out(claim, 0, "o0")}, P("o5", 100)))
	f.add(rival)
	tw.addEntry(t, 13, rival, nil)
	h := rival.Hash(f.s)
	for i := 0; i < 64; i++ {
		l := layerOf(t, f.s, h, 1, uint64(i)+1)
		_, err := f.pool.AddLayer(l)
		_, terr := tw.pool.AddLayer(l)
		if (err == nil) != (terr == nil) {
			t.Fatalf("layer %d: pool %v, twin %v", i, err, terr)
		}
		tw.compare(t, 14+i, f.pool)
		if tip, _, _ := f.pool.Tip(); tip == h {
			break
		}
	}
	if tip, _, _ := tw.pool.Tip(); tip != h {
		t.Fatal("the heavier branch was not selected")
	}
	// The reverted entries' transactions read back whole.
	if tx, ok := tw.pool.Tx(es[5].Hash(f.s), es[5].Txs[0].TID(f.s.Hash())); !ok || len(tx.Inputs) != 1 {
		t.Fatal("a reverted entry's transaction is not found")
	}
}

// A read that fails while a switch needs it panics with a ReadFault before
// the pool changes, so the engine fails and the node rebuilds from its
// stores (D174, D175).
func TestDeepSwitchReadFault(t *testing.T) {
	f := newFixture(t, ledger.Config{K: ledger.Unlimited})
	tw := newTwin(f.s, ledger.Config{K: ledger.Unlimited})
	es := chainOf(t, f, tw, 10)
	tw.pool.ReleaseBodies()
	tw.cache.SetMax(0)
	tw.src.fail = true
	before, height, _ := tw.pool.Tip()
	// An entry on a deep parent needs the state after it, which reverts
	// cold entries on an overlay: the read fails before anything changes.
	rival := f.build(es[0].Hash(f.s), f.k.Spend(4, []testutil.In{f.k.Out(es[0].Txs[0], 0, "o0")}, P("o5", 100)))
	var fault ledger.ReadFault
	func() {
		defer func() {
			v := recover()
			rf, ok := v.(ledger.ReadFault)
			if !ok {
				t.Fatalf("recovered %v, want a ReadFault", v)
			}
			fault = rf
		}()
		tw.pool.AddEntry(rival)
	}()
	if tw.pool.Has(rival.Hash(f.s)) {
		t.Fatal("the entry was kept although its check could not finish")
	}
	if !errors.Is(fault.Err, ledger.ErrUnreadable) || fault.Entry == (hash.Hash{}) {
		t.Fatalf("fault %+v", fault)
	}
	if tip, ht, _ := tw.pool.Tip(); tip != before || ht != height {
		t.Fatalf("the pool changed before the fault: %s at %d", tip.Short(), ht)
	}
}
