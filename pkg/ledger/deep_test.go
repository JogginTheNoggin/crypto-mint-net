package ledger_test

import (
	"errors"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/entry"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/ledger"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/testutil"
)

// RED-011, RED-012: EntryOwners does not build the state after a parent
// that costs more than MaxPreviewCost (AddEntry builds it once), and
// ViewCost prices entries and their transactions.
func TestEntryOwnersRefusesDeepParent(t *testing.T) {
	f := newFixture(t, ledger.Config{K: ledger.Unlimited})
	depth := ledger.MaxPreviewCost + 6 // empty entries: one unit each
	parent := f.pool.ID()
	var chain []hash.Hash
	for i := 0; i < depth; i++ {
		e := f.build(parent)
		f.add(e)
		parent = e.Hash(f.s)
		f.layers(parent, 1)
		chain = append(chain, parent)
	}
	if h := f.pool.Height(); h != depth {
		t.Fatalf("height %d, want %d", h, depth)
	}
	first, _ := f.s.PoolRange(1)
	claim := f.k.Claim(first, P("alice", 100))
	cost := func(e *entry.Entry) int { return f.pool.ViewCost(e, 1<<20) }

	on := func(p hash.Hash) *entry.Entry { return f.build(p, claim) }
	if c := cost(on(parent)); c != 0 {
		t.Fatalf("on the tip: cost %d", c)
	}
	near := on(chain[depth-11])
	if c := cost(near); c != 10 {
		t.Fatalf("10 below the tip: cost %d", c)
	}
	if _, _, err := f.pool.EntryOwners(near); err != nil {
		t.Fatalf("near parent: %v", err)
	}
	deep := on(f.pool.ID())
	if c := cost(deep); c != depth {
		t.Fatalf("on the pool id: cost %d, want %d", c, depth)
	}
	if _, _, err := f.pool.EntryOwners(deep); !errors.Is(err, ledger.ErrDeepParent) {
		t.Fatalf("deep parent: %v", err)
	}
	if c := cost(f.build(hash.Hash{7})); c != 0 {
		t.Fatalf("unknown parent: cost %d", c)
	}
	// Not an offence: AddEntry still takes the entry, and keeps the state
	// after it, so the next entry on it costs nothing.
	f.add(deep)
	if c := cost(f.build(deep.Hash(f.s))); c != 0 {
		t.Fatalf("on the kept view: cost %d", c)
	}
	if c := f.pool.ViewCost(f.build(deep.Hash(f.s)), 3); c != 0 {
		t.Fatalf("on the kept view, limit 3: cost %d", c)
	}
	// Once another entry is added, the fork costs its entry and its claim
	// on top of the revert.
	f.add(near)
	if c := cost(f.build(deep.Hash(f.s))); c != depth+2 {
		t.Fatalf("on the fork: cost %d, want %d", c, depth+2)
	}
	if c := f.pool.ViewCost(f.build(deep.Hash(f.s)), 1); c != 2 {
		t.Fatalf("on the fork, limit 1: cost %d, want the count to stop at 2", c)
	}
}

// RED-012: the state AddEntry keeps for the next entry is the state after
// exactly the entry it was kept for. A sibling, or an entry on an older
// entry of the branch, is checked against its own parent's state.
func TestKeptViewIsTheParentsState(t *testing.T) {
	f := newFixture(t, ledger.Config{K: ledger.Unlimited})
	claim := f.k.Claim(4, P("alice", 100))
	e1 := f.build(f.pool.ID(), claim)
	f.add(e1) // no layers: the branch stays off the selected ledger
	toBob := f.k.Spend(4, []testutil.In{f.k.Out(claim, 0, "alice")}, P("bob", 100))
	e2 := f.build(e1.Hash(f.s), toBob)
	f.add(e2)
	// On e1 again: alice's portion is unspent there.
	toCarol := f.k.Spend(4, []testutil.In{f.k.Out(claim, 0, "alice")}, P("carol", 100))
	f.add(f.build(e1.Hash(f.s), toCarol))
	// On e2: bob's portion exists, alice's is spent.
	if _, err := f.pool.AddEntry(f.build(e2.Hash(f.s), toCarol)); !errors.Is(err, ledger.ErrInvalidEntry) {
		t.Fatalf("a spent portion on e2: %v", err)
	}
	fromBob := f.k.Spend(4, []testutil.In{f.k.Out(toBob, 0, "bob")}, P("dave", 100))
	e4 := f.build(e2.Hash(f.s), fromBob)
	f.add(e4)
	// A refused entry leaves the kept state as it was.
	if _, err := f.pool.AddEntry(f.build(e4.Hash(f.s), fromBob)); !errors.Is(err, ledger.ErrInvalidEntry) {
		t.Fatalf("a spent portion on e4: %v", err)
	}
	fromDave := f.k.Spend(4, []testutil.In{f.k.Out(fromBob, 0, "dave")}, P("erin", 100))
	f.add(f.build(e4.Hash(f.s), fromDave))
	if err := ledger.CheckOff(f.pool); err != nil {
		t.Fatal(err)
	}
}

// RED-012: a branch with no layers, taken in parent first, costs the same
// per entry however long it is.
func TestLongBranchIsLinear(t *testing.T) {
	f := newFixture(t, ledger.Config{K: ledger.DefaultK})
	parent := f.pool.ID()
	for i := 0; i < 4000; i++ {
		e := f.build(parent)
		if c := f.pool.ViewCost(e, 1<<20); c != 0 && i > 0 {
			t.Fatalf("entry %d: cost %d, want 0 on the kept state", i, c)
		}
		f.add(e)
		parent = e.Hash(f.s)
	}
	if f.pool.Height() != 0 {
		t.Fatal("a branch with no weight was selected")
	}
}
