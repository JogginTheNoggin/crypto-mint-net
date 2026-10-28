package engine_test

import (
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/engine"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/entry"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/testutil"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tx"
)

// TestPreverifyEntryNeedsShape: an entry that fails the cheap structural
// checks has none of its signatures verified ahead of HandleEntry, and a
// well-formed one has all of them (D105).
func TestPreverifyEntryNeedsShape(t *testing.T) {
	s := testutil.ExampleSeries()
	k := testutil.NewKeys(s)
	e := engine.New(engine.DefaultConfig())
	e.AddSeries(s)
	if err := e.FollowSeries(s.ID()); err != nil {
		t.Fatal(err)
	}
	first, _ := s.PoolRange(1)
	a, b := k.Claim(first, testutil.P("alice", 100)), k.Claim(first+1, testutil.P("bob", 100))
	key := engine.PoolKey{Series: s.ID(), Index: 1}
	bad := &entry.Entry{Parent: s.PoolID(1), Txs: []*tx.Tx{b, a}} // out of order
	e.PreverifyEntry(key, bad, engine.Pushed)
	if n := s.Remembered(); n != 0 {
		t.Fatalf("an entry out of order had %d witnesses verified", n)
	}
	good := &entry.Entry{Parent: s.PoolID(1), Txs: []*tx.Tx{a, b}}
	e.PreverifyEntry(key, good, engine.Pushed)
	if n := s.Remembered(); n != 2 {
		t.Fatalf("a well-formed entry had %d of 2 witnesses verified", n)
	}
}
