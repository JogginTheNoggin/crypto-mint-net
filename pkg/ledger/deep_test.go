package ledger_test

import (
	"errors"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/entry"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/ledger"
)

// RED-011: EntryOwners does not build the state after a parent deeper
// than MaxPreviewCost below the selected tip (AddEntry builds it once),
// and ViewCost says what that state costs.
func TestEntryOwnersRefusesDeepParent(t *testing.T) {
	f := newFixture(t, ledger.Config{K: ledger.Unlimited})
	depth := ledger.MaxPreviewCost + 6
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

	on := func(p hash.Hash) *entry.Entry { return f.build(p, claim) }
	tip := on(parent)
	if c := f.pool.ViewCost(tip, ledger.MaxPreviewCost); c != 0 {
		t.Fatalf("on the tip: cost %d", c)
	}
	near := on(chain[depth-11])
	if c := f.pool.ViewCost(near, ledger.MaxPreviewCost); c != 10 {
		t.Fatalf("10 below the tip: cost %d", c)
	}
	if _, _, err := f.pool.EntryOwners(near); err != nil {
		t.Fatalf("near parent: %v", err)
	}
	deep := on(f.pool.ID())
	if c := f.pool.ViewCost(deep, ledger.MaxPreviewCost); c != depth {
		t.Fatalf("on the pool id: cost %d, want %d", c, depth)
	}
	if _, _, err := f.pool.EntryOwners(deep); !errors.Is(err, ledger.ErrDeepParent) {
		t.Fatalf("deep parent: %v", err)
	}
	if c := f.pool.ViewCost(f.build(hash.Hash{7}), ledger.MaxPreviewCost); c != 0 {
		t.Fatalf("unknown parent: cost %d", c)
	}
	// Not an offence: AddEntry still takes the entry.
	f.add(deep)

	// The off-chain path to a parent counts too.
	if c := f.pool.ViewCost(on(deep.Hash(f.s)), 3); c != depth+1 {
		t.Fatalf("on the fork: cost %d, want %d", c, depth+1)
	}
}
