package engine_test

import (
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/engine"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/entry"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/series"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/testutil"
)

// RED-049, D154: an item that is refused leaves no state behind, a query
// makes none, and a kept one keeps its pool's state.
func TestRefusedItemsLeaveNoState(t *testing.T) {
	p := testutil.ExampleParams()
	p.PoolSize, p.Mints = 1, 1<<40
	s, err := series.New(p)
	if err != nil {
		t.Fatal(err)
	}
	e := engine.New(engine.DefaultConfig())
	e.AddSeries(s)
	if err := e.FollowSeries(s.ID()); err != nil {
		t.Fatal(err)
	}
	listed := func() int { return len(e.ListedPools()) }
	for i := uint64(0); i < 100; i++ {
		k := engine.PoolKey{Series: s.ID(), Index: 1<<30 + i}
		if _, err := e.HandleEntry(k, &entry.Entry{Parent: hash.Hash{byte(i), 1}}, engine.Pushed); err == nil {
			t.Fatal("an entry with an unknown parent was kept")
		}
		if _, _, _, err := e.Tip(k); err != nil {
			t.Fatal(err)
		}
	}
	keys := testutil.NewKeys(s)
	bad := keys.Claim(5, testutil.P("alice", p.Supply))
	bad.Witnesses = nil
	if _, err := e.HandleTx(bad, engine.Pushed); err == nil {
		t.Fatal("a claim with no witness was taken")
	}
	if n := listed(); n != 0 {
		t.Fatalf("%d pools have state after refused items and queries", n)
	}
	if _, err := e.HandleTx(keys.Claim(5, testutil.P("alice", p.Supply)), engine.Pushed); err != nil {
		t.Fatal(err)
	}
	if n := listed(); n != 1 {
		t.Fatalf("%d pools have state after one kept claim", n)
	}
}

// RED-050, D154: a whole follow but some pools follows every other pool;
// a pool followed by its index or another whole follow puts one back.
func TestFollowSeriesExcept(t *testing.T) {
	s := testutil.ExampleSeries() // two pools
	q := testutil.ExampleParams()
	q.Mints = 40 // ten pools
	big := testutil.MustSeries(q)
	e := engine.New(engine.DefaultConfig())
	e.AddSeries(s)
	e.AddSeries(big)
	k := func(i uint64) engine.PoolKey { return engine.PoolKey{Series: big.ID(), Index: i} }
	if err := e.FollowSeriesExcept(big.ID(), []uint64{3, 7}); err != nil {
		t.Fatal(err)
	}
	if e.Follows(k(3)) || e.Follows(k(7)) || !e.Follows(k(4)) || !e.FollowsWhole(big.ID()) {
		t.Fatal("except not applied")
	}
	if n := e.FollowedCount(); n != 8 {
		t.Fatalf("followed %d", n)
	}
	if err := e.FollowPool(big.ID(), 3); err != nil || !e.Follows(k(3)) || e.Follows(k(7)) {
		t.Fatalf("a pool followed by index: %v", err)
	}
	if err := e.FollowSeriesExcept(big.ID(), []uint64{7, 8}); err != nil || e.Follows(k(7)) || !e.Follows(k(8)) {
		t.Fatalf("two excepts follow what either follows: %v", err)
	}
	if err := e.FollowSeries(big.ID()); err != nil || !e.Follows(k(7)) {
		t.Fatalf("a whole follow: %v", err)
	}
	if err := e.FollowSeriesExcept(big.ID(), []uint64{40}); err == nil {
		t.Fatal("a pool out of range was taken")
	}
	for _, l := range e.ListedPools() {
		if l.Series == big.ID() && !e.Follows(l) {
			t.Fatalf("listed a pool not followed: %v", l)
		}
	}
}
