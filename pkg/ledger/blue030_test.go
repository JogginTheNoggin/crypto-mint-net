package ledger_test

import (
	"fmt"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/entry"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/ledger"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/testutil"
)

// RED-086: a transaction that spends outputs made in many older entries
// costs what a cold revert of it reads: its view cost counts one unit for
// each distinct transaction it spends from, so the bodies read back to
// preview an entry below it stay under the cost charged.
func TestColdRevertCostCountsMakers(t *testing.T) {
	const m = 40
	f := newFixture(t, ledger.Config{K: ledger.Unlimited})
	tw := newTwin(f.s, ledger.Config{K: ledger.Unlimited})
	tw.cache.SetMax(0)
	step := 0
	add := func(e *entry.Entry) {
		t.Helper()
		tw.addEntry(t, step, e, nil)
		if _, err := tw.pool.AddLayer(layerOf(t, f.s, e.Hash(f.s), 1, uint64(step)+1)); err != nil {
			t.Fatal(err)
		}
		step++
	}
	// A claim of mint 4 in m outputs, each moved on in an entry of its own,
	// then one merge that spends all m moved outputs.
	amt := func(i int) uint64 { // the whole supply of 100, split m ways
		if i == m-1 {
			return 100 - (m - 1)
		}
		return 1
	}
	outs := make([]testutil.Pay, m)
	for i := range outs {
		outs[i] = P(fmt.Sprintf("a%d", i), amt(i))
	}
	claim := f.k.Claim(4, outs...)
	e := f.build(tw.pool.ID(), claim)
	add(e)
	var ins []testutil.In
	for i := 0; i < m; i++ {
		mv := f.k.Spend(4, []testutil.In{f.k.Out(claim, uint32(i), fmt.Sprintf("a%d", i))}, P(fmt.Sprintf("b%d", i), amt(i)))
		e = f.build(e.Hash(f.s), mv)
		add(e)
		ins = append(ins, f.k.Out(mv, 0, fmt.Sprintf("b%d", i)))
	}
	below := e.Hash(f.s)
	merge := f.k.Spend(4, ins, P("c", 100))
	e = f.build(below, merge)
	add(e)
	for i := 0; i < 3; i++ { // the merge falls out of the window
		c := f.k.Claim(uint64(5+i), P("d", 100))
		e = f.build(e.Hash(f.s), c)
		add(e)
	}
	tw.pool.ReleaseBodies()
	if tw.pool.Cold() < m {
		t.Fatalf("%d cold entries", tw.pool.Cold())
	}
	// A rival on the entry below the merge reverts the merge (cold) and the
	// three after it.
	rival := f.build(below, f.k.Claim(5, P("e", 100)))
	cost := tw.pool.ViewCost(rival, 1<<20)
	if cost < m {
		t.Fatalf("view cost %d, below the %d makers the merge's revert reads", cost, m)
	}
	before := tw.src.loads
	if _, _, err := tw.pool.EntryOwners(rival); err != nil {
		t.Fatal(err)
	}
	if loads := tw.src.loads - before; loads > cost {
		t.Fatalf("%d bodies read for a view cost of %d", loads, cost)
	} else {
		t.Logf("view cost %d, bodies read %d", cost, loads)
	}
}
