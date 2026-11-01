package ledger_test

import (
	"reflect"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/entry"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/ledger"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/testutil"
)

// ORANGE-020: PageOwners finds each entry's owners against the state after
// the entries before it in the run, exactly as EntryOwners does once those
// entries are held, and stops before an entry it cannot preview.
func TestPageOwnersMatchesEntryOwners(t *testing.T) {
	f := newFixture(t, ledger.Config{})
	k := f.k
	c4 := k.Claim(4, P("a1", 100))
	c5 := k.Claim(5, P("b1", 100))
	e1 := f.build(f.pool.ID(), c4, c5)
	ac := k.Spend(4, []testutil.In{k.Out(c4, 0, "a1")}, P("carol", 30), P("a2", 70))
	bd := k.Spend(5, []testutil.In{k.Out(c5, 0, "b1")}, P("d1", 40), P("b2", 60))
	e2 := f.build(e1.Hash(f.s), ac, bd)
	ae := k.Spend(4, []testutil.In{k.Out(ac, 1, "a2"), k.Out(ac, 0, "carol")}, P("e2", 100))
	e3 := f.build(e2.Hash(f.s), ae)
	es := []*entry.Entry{e1, e2, e3}

	items, owners := f.pool.PageOwners(es)
	if len(items) != 3 || len(owners) != 3 {
		t.Fatalf("previewed %d of 3", len(items))
	}
	for i, e := range es {
		wantItems, wantOwners, err := f.pool.EntryOwners(e)
		if err != nil {
			t.Fatalf("entry %d: %v", i, err)
		}
		if !reflect.DeepEqual(items[i], wantItems) || !reflect.DeepEqual(owners[i], wantOwners) {
			t.Fatalf("entry %d: page owners %v, want %v", i, owners[i], wantOwners)
		}
		f.add(e)
	}

	g := newFixture(t, ledger.Config{})
	if items, _ := g.pool.PageOwners([]*entry.Entry{e1, e3}); len(items) != 1 {
		t.Fatalf("a run that does not link: previewed %d, want 1", len(items))
	}
	if items, _ := g.pool.PageOwners([]*entry.Entry{e2, e3}); len(items) != 0 {
		t.Fatalf("an unknown first parent: previewed %d, want 0", len(items))
	}
	g.add(e1)
	if items, _ := g.pool.PageOwners([]*entry.Entry{e1, e2}); len(items) != 0 {
		t.Fatalf("a held first entry: previewed %d, want 0", len(items))
	}
	if items, _ := g.pool.PageOwners([]*entry.Entry{e2, e3}); len(items) != 2 {
		t.Fatalf("from a held parent: previewed %d, want 2", len(items))
	}
	// A spend of an output the run never made stops the preview there.
	bad := f.build(e2.Hash(f.s), k.Spend(4, []testutil.In{k.Out(c4, 0, "a1")}, P("x", 100)))
	if items, _ := g.pool.PageOwners([]*entry.Entry{e2, bad}); len(items) != 1 {
		t.Fatalf("a spent input: previewed %d, want 1", len(items))
	}
}
