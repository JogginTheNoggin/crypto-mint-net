package watch_test

import (
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/engine"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/testutil"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/watch"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/wire"
)

// D188: an export read a page at a time by following Next yields the
// selected ledger oldest first, each entry once, parent after parent, and
// another node takes every item in through its full checks; a changed
// item that is not an entry item is refused, and so is an entry whose
// parent the node lacks.
func TestExportPagesAndImports(t *testing.T) {
	defer watch.SetExportLimit(2)()
	src, dst := newFixture(t), newFixture(t)
	s, k := src.s, src.k
	key := engine.PoolKey{Series: s.ID(), Index: s.PoolOf(4)}
	prev := k.Claim(4, testutil.P("alice", 100))
	const height = 5
	for i := 0; i < height; i++ {
		if i > 0 {
			prev = k.Spend(4, []testutil.In{k.Out(prev, 0, "alice")}, testutil.P("alice", 100))
		}
		_, err := src.n.HandleTx(prev, engine.Local)
		must(t, err)
		e, _, err := src.n.Build(key, 0)
		must(t, err)
		_, _, err = src.n.Work(key, e.Hash(s), 0, 20)
		must(t, err)
	}

	var items [][]byte
	var entries []watch.ExportEntry
	pages := 0
	for from := 1; from != 0; pages++ {
		p, err := watch.Export(src.n.Engine(), key, from, 0)
		must(t, err)
		if p.Height != height || len(p.Items) != len(p.Entries) || len(p.Items) > 2 {
			t.Fatalf("page from %d: %+v", from, p)
		}
		items = append(items, p.Items...)
		entries = append(entries, p.Entries...)
		from = p.Next
	}
	if pages != 3 || len(entries) != height {
		t.Fatalf("%d pages, %d entries", pages, len(entries))
	}
	for i, e := range entries {
		if e.Height != i+1 || (i > 0 && e.Parent != entries[i-1].Hash) {
			t.Fatalf("entry %d: %+v", i, e)
		}
	}
	// A range stops at to, and one past the tip is empty.
	p, err := watch.Export(src.n.Engine(), key, 2, 3)
	must(t, err)
	if len(p.Entries) != 2 || p.Entries[1].Height != 3 || p.Next != 0 {
		t.Fatalf("2..3: %+v", p)
	}
	p, err = watch.Export(src.n.Engine(), key, height+1, 0)
	must(t, err)
	if len(p.Entries) != 0 || p.Next != 0 {
		t.Fatalf("past the tip: %+v", p)
	}

	// An item that is not an entry item, and the second entry without
	// the first: both refused, nothing kept.
	it, err := wire.DecodeItem(items[0])
	must(t, err)
	if !it.HasLayers || len(it.Nonces) == 0 {
		t.Fatal("no layer nonces exported")
	}
	res, err := dst.n.ImportItems(key, [][]byte{{1, 2, 3}, items[1]})
	must(t, err)
	if res.Kept != 0 || len(res.Refused) != 2 || res.Refused[1].Index != 1 || res.Refused[1].Hash != entries[1].Hash {
		t.Fatalf("bad import %+v", res)
	}
	res, err = dst.n.ImportItems(key, items)
	must(t, err)
	if res.Kept != height || res.Known != 0 || len(res.Refused) != 0 || res.LayersKept != height {
		t.Fatalf("import %+v", res)
	}
	q, err := watch.Export(dst.n.Engine(), key, 1, 0)
	must(t, err)
	if q.Height != height || q.Tip != p.Tip {
		t.Fatalf("imported ledger %d %s, want %d %s", q.Height, q.Tip, height, p.Tip)
	}
}
