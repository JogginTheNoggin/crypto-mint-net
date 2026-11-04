package node_test

import (
	"bytes"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/engine"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/entry"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/ledger"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/node"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/nodepolicy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/testutil"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tx"
)

// bodies returns every held entry of the fixture's pool, encoded, in
// replay order.
func (f *fixture) bodies(n *node.Node) map[hash.Hash][]byte {
	f.t.Helper()
	out := map[hash.Hash][]byte{}
	missing := 0
	n.Engine().Ledger(f.key, func(p *ledger.Pool) {
		for _, h := range p.ReplayOrder() {
			e, ok := p.Entry(h)
			if !ok {
				missing++
				continue
			}
			out[h] = e.Encode()
		}
	})
	if missing > 0 {
		f.t.Fatalf("%d held entries could not be read back", missing)
	}
	return out
}

func sameBodies(t *testing.T, step string, got, want map[hash.Hash][]byte) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: %d entries, want %d", step, len(got), len(want))
	}
	for h, b := range want {
		if !bytes.Equal(got[h], b) {
			t.Fatalf("%s: entry %s reads back differently", step, h.Short())
		}
	}
}

// TestBodiesFromDisk is D172's node-level differential: a node whose
// bodies are all read back from its log, with a cache of no bytes,
// selects, holds and serves exactly what it did with every body in
// memory, through a compaction and a restart.
func TestBodiesFromDisk(t *testing.T) {
	f := newFixture(t)
	n := f.open()
	f.history(n)
	want := f.snap(n)
	wantBodies := f.bodies(n)

	n.BodyCache().SetMax(0)
	n.Engine().Ledger(f.key, func(p *ledger.Pool) { p.ReleaseBodies() })
	st := n.Bodies()
	if st.InMemory != 0 || st.Held != want.entries || st.Located < st.Held {
		t.Fatalf("after release: %+v", st)
	}
	sameBodies(t, "from disk", f.bodies(n), wantBodies)
	if got := f.snap(n); got != want {
		t.Fatalf("from disk\n got %+v\nwant %+v", got, want)
	}
	if st := n.Bodies(); st.Loads == 0 || st.Errors != 0 || st.Bodies != 0 {
		t.Fatalf("stats %+v: want loads, no errors, nothing cached", st)
	}

	// A compaction copies the records and moves the locations.
	if err := n.Compact(f.key); err != nil {
		t.Fatal(err)
	}
	sameBodies(t, "after compaction", f.bodies(n), wantBodies)
	if st := n.Bodies(); st.Located != st.Held || st.Errors != 0 {
		t.Fatalf("after compaction: %+v", st)
	}

	// A new entry after the compaction is found at its new place.
	c6 := f.k.Claim(6, P("gina", 100))
	if _, err := n.HandleTx(c6, engine.Local); err != nil {
		t.Fatal(err)
	}
	if _, _, err := n.Build(f.key, 0); err != nil {
		t.Fatal(err)
	}
	want = f.snap(n)
	wantBodies = f.bodies(n)
	if err := n.Close(); err != nil {
		t.Fatal(err)
	}

	// After a restart every body is on disk, and reads back.
	n = f.open()
	defer n.Close()
	if got := f.snap(n); got != want {
		t.Fatalf("after restart\n got %+v\nwant %+v", got, want)
	}
	if st := n.Bodies(); st.InMemory != 0 || st.Held != want.entries {
		t.Fatalf("after restart: %+v", st)
	}
	n.BodyCache().SetMax(0)
	sameBodies(t, "after restart", f.bodies(n), wantBodies)

	// And entries taken in after the restart too.
	c7 := f.k.Claim(7, P("hal", 100))
	if _, err := n.HandleTx(c7, engine.Local); err != nil {
		t.Fatal(err)
	}
	e, _, err := n.Build(f.key, 0)
	if err != nil {
		t.Fatal(err)
	}
	r, _, err := entry.Build(f.s, e.Hash(f.s), []*tx.Tx{f.k.Spend(7, []testutil.In{f.k.Out(c7, 0, "hal")}, P("ivy", 100))}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := n.HandleEntry(f.key, r, engine.Pushed); err != nil {
		t.Fatal(err)
	}
	if st := n.Bodies(); st.InMemory > 1 {
		t.Fatalf("more than the newest body in memory: %+v", st)
	}
	got := f.bodies(n)
	if !bytes.Equal(got[e.Hash(f.s)], e.Encode()) || !bytes.Equal(got[r.Hash(f.s)], r.Encode()) {
		t.Fatal("an entry taken in after the restart reads back differently")
	}
}

// TestEntryCacheLimit: entry_cache_mib is a bounded limit, applied to the
// node's cache.
func TestEntryCacheLimit(t *testing.T) {
	f := newFixture(t, func(p *nodepolicy.Policy) { p.EntryCacheMiB = 3 })
	n := f.open()
	defer n.Close()
	if got := n.Bodies().MaxBytes; got != 3<<20 {
		t.Fatalf("cache bound %d, want 3 MiB", got)
	}
}
