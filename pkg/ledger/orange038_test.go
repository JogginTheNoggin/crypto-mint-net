package ledger_test

import (
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/entry"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/ledger"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/testutil"
)

// A released body leaves its entry's items without witnesses (ORANGE-038),
// so the skeleton no longer holds the body. A switch that displaces such
// an entry sorts its transactions as before, and carries a valid one back
// with its witnesses, read from the source. When the body cannot be read,
// the transaction is not carried, never carried without its witnesses,
// and the switch is otherwise the same.
func TestSwitchCarriesFromReleasedBodies(t *testing.T) {
	for _, fail := range []bool{false, true} {
		f := newFixture(t, ledger.Config{})
		src := newMapSource(f.s)
		p := ledger.New(f.s, 1, ledger.Config{Source: src, Bodies: ledger.NewBodyCache(0)})
		add := func(e *entry.Entry) ledger.AddResult {
			t.Helper()
			res, err := p.AddEntry(e)
			if err != nil {
				t.Fatal(err)
			}
			src.write(e)
			return res
		}
		c4 := f.k.Claim(4, P("alice", 100))
		e1 := f.build(f.pool.ID(), c4)
		add(e1)
		spend := f.k.Spend(4, []testutil.In{f.k.Out(c4, 0, "alice")}, P("bob", 100))
		e2 := f.build(e1.Hash(f.s), spend)
		add(e2)
		if _, err := p.AddLayer(layerOf(t, f.s, e1.Hash(f.s), 1, 0)); err != nil {
			t.Fatal(err)
		}
		if _, err := p.AddLayer(layerOf(t, f.s, e2.Hash(f.s), 1, 0)); err != nil {
			t.Fatal(err)
		}
		if tip, _, _ := p.Tip(); tip != e2.Hash(f.s) {
			t.Fatalf("tip %s, want e2", tip.Short())
		}
		p.ReleaseBodies()
		if p.InMemory() != 0 {
			t.Fatalf("%d bodies in memory", p.InMemory())
		}
		// A heavier rival on e1 displaces e2; the spend is still valid.
		r := f.build(e1.Hash(f.s))
		add(r)
		src.fail = fail
		res, err := p.AddLayer(layerOf(t, f.s, r.Hash(f.s), 3, 0))
		if err != nil {
			t.Fatal(err)
		}
		sw := res.Switch
		if sw == nil || len(sw.Reverted) != 1 || len(sw.Void) != 0 || len(sw.Unchanged) != 0 {
			t.Fatalf("fail %v: switch %+v", fail, sw)
		}
		switch {
		case fail && len(sw.Carried) != 0:
			t.Fatalf("a body that cannot be read: carried %d", len(sw.Carried))
		case !fail && (len(sw.Carried) != 1 || string(sw.Carried[0].Tx.Encode()) != string(spend.Encode())):
			t.Fatalf("carried %+v, want the spend with its witness", sw.Carried)
		}
		if tip, _, _ := p.Tip(); tip != r.Hash(f.s) {
			t.Fatalf("fail %v: tip %s", fail, tip.Short())
		}
	}
}
