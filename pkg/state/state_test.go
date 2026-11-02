package state_test

import (
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/state"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/testutil"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tx"
)

var P = testutil.P

func item(t *tx.Tx) state.Item {
	return state.Item{TID: t.TID(testutil.ExampleSeries().Hash()), Tx: t}
}

func snapshot(s *state.State) map[tx.Input]tx.Portion {
	m := map[tx.Input]tx.Portion{}
	s.ForEach(func(in tx.Input, p tx.Portion) { m[in] = p })
	return m
}

func equal(a, b map[tx.Input]tx.Portion) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func TestApplyRevert(t *testing.T) {
	k := testutil.NewKeys(testutil.ExampleSeries())
	st := state.New()
	claim := k.Claim(4, P("alice", 100))
	e1 := []state.Item{item(claim), item(k.Claim(5, P("bob", 100)))}
	u1 := st.Apply(e1)
	after1 := snapshot(st)

	split := k.Spend(4, []testutil.In{k.Out(claim, 0, "alice")}, P("carol", 30), P("alice2", 70))
	e2 := []state.Item{item(split)}
	u2 := st.Apply(e2)
	if st.Totals()[4] != 100 || st.Totals()[5] != 100 {
		t.Fatalf("totals %v", st.Totals())
	}
	if _, ok := st.Unspent(tx.Input{TID: claim.TID(testutil.ExampleSeries().Hash())}); ok {
		t.Fatal("spent portion still unspent")
	}
	if got := st.Portions(4); len(got) != 2 {
		t.Fatalf("mint 4 portions: %v", got)
	}

	st.Revert(e2, u2)
	if !equal(snapshot(st), after1) {
		t.Fatal("revert of E2 did not restore the state after E1")
	}
	st.Revert(e1, u1)
	if st.Len() != 0 || st.Claimed(4) || st.Claimed(5) {
		t.Fatal("revert of E1 did not empty the state")
	}
}

func TestOverlayLeavesBaseAlone(t *testing.T) {
	k := testutil.NewKeys(testutil.ExampleSeries())
	base := state.New()
	claim := k.Claim(4, P("alice", 100))
	base.Apply([]state.Item{item(claim)})
	before := snapshot(base)

	o := base.Overlay()
	split := k.Spend(4, []testutil.In{k.Out(claim, 0, "alice")}, P("carol", 30), P("alice2", 70))
	u := o.Apply([]state.Item{item(split)})
	if !equal(snapshot(base), before) {
		t.Fatal("overlay changed its base")
	}
	if o.Len() != 2 || o.Totals()[4] != 100 {
		t.Fatalf("overlay state wrong: %v", snapshot(o))
	}

	// Revert inside the overlay, then revert the base's own entry inside
	// another overlay: the overlay can undo entries that live in the base.
	o.Revert([]state.Item{item(split)}, u)
	if !equal(snapshot(o), before) {
		t.Fatal("overlay revert wrong")
	}
	o2 := base.Overlay()
	o2.Revert([]state.Item{item(claim)}, state.Undo{})
	if o2.Len() != 0 || o2.Claimed(4) {
		t.Fatal("overlay could not undo a base entry")
	}
	if !base.Claimed(4) || base.Len() != 1 {
		t.Fatal("base touched")
	}
	// Re-applying in that overlay puts it back.
	o2.Apply([]state.Item{item(claim)})
	if !equal(snapshot(o2), before) || !o2.Claimed(4) {
		t.Fatal("re-apply in overlay wrong")
	}
}

func TestApplyUncheckedPanics(t *testing.T) {
	k := testutil.NewKeys(testutil.ExampleSeries())
	defer func() {
		if recover() == nil {
			t.Fatal("want panic")
		}
	}()
	claim := k.Claim(4, P("alice", 100))
	state.New().Apply([]state.Item{item(k.Spend(4, []testutil.In{k.Out(claim, 0, "alice")}, P("b", 100)))})
}
