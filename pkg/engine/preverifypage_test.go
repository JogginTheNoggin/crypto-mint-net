package engine_test

import (
	"errors"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/engine"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/entry"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/ledger"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/series"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/testutil"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tx"
)

// chainOf builds n entries from the pool_id, the first with a claim and
// each later one spending the one before, without taking any in.
func chainOf(t *testing.T, s *series.Series, k *testutil.Keys, n int) ([]*entry.Entry, []*tx.Tx) {
	t.Helper()
	first, _ := s.PoolRange(1)
	parent := s.PoolID(1)
	var es []*entry.Entry
	var txs []*tx.Tx
	var prev *tx.Tx
	owner := "o0"
	for i := 0; i < n; i++ {
		var x *tx.Tx
		next := owner
		if prev == nil {
			x = k.Claim(first, testutil.P(owner, 100))
		} else {
			next = owner + "x"
			x = k.Spend(first, []testutil.In{k.Out(prev, 0, owner)}, testutil.P(next, 100))
		}
		e, _, err := entry.Build(s, parent, []*tx.Tx{x}, 0)
		if err != nil {
			t.Fatal(err)
		}
		es, txs = append(es, e), append(txs, x)
		parent, prev, owner = e.Hash(s), x, next
	}
	return es, txs
}

// ORANGE-020: PreverifyPage verifies a run's witnesses ahead, each entry
// against the state after the ones before it, so taking them in order
// verifies nothing more; it never changes what HandleEntry returns.
func TestPreverifyPage(t *testing.T) {
	e, s, k, n := countingEngine(t)
	key := engine.PoolKey{Series: s.ID(), Index: 1}
	es, _ := chainOf(t, s, k, 12)
	e.PreverifyPage(key, es)
	if got := n.Load(); got != 12 {
		t.Fatalf("%d satisfy calls ahead, want 12", got)
	}
	n.Store(0)
	for i, en := range es {
		if _, err := e.HandleEntry(key, en, engine.Requested); err != nil {
			t.Fatalf("entry %d: %v", i, err)
		}
	}
	if got := n.Load(); got != 0 {
		t.Fatalf("%d satisfy calls taking the run in, want 0", got)
	}

	// A bad witness in the fourth entry: the preview stops the workers,
	// and taking the run in fails at that entry as it would without it.
	e2, s2, k2, n2 := countingEngine(t)
	key2 := engine.PoolKey{Series: s2.ID(), Index: 1}
	es2, txs2 := chainOf(t, s2, k2, 12)
	first, _ := s2.PoolRange(1)
	other := k2.Spend(first, []testutil.In{k2.Out(txs2[2], 0, "o0xx")}, testutil.P("z", 100))
	bad := *txs2[3]
	bad.Witnesses = append(bad.Witnesses[:0:0], other.Witnesses...)
	be, _, err := entry.Build(s2, es2[2].Hash(s2), []*tx.Tx{&bad}, 0)
	if err != nil {
		t.Fatal(err)
	}
	es2[3] = be
	e2.PreverifyPage(key2, es2[:4])
	if got := n2.Load(); got > 4 {
		t.Fatalf("%d satisfy calls for a run with a bad fourth witness, want at most 4", got)
	}
	for i := 0; i < 3; i++ {
		if _, err := e2.HandleEntry(key2, es2[i], engine.Requested); err != nil {
			t.Fatalf("entry %d: %v", i, err)
		}
	}
	if _, err := e2.HandleEntry(key2, es2[3], engine.Requested); !errors.Is(err, ledger.ErrInvalidEntry) {
		t.Fatalf("bad witness: %v", err)
	}

	// A run on a parent the node does not hold: nothing verified.
	e3, s3, k3, n3 := countingEngine(t)
	es3, _ := chainOf(t, s3, k3, 4)
	orphan, _, _ := entry.Build(s3, hash.Hash{7}, nil, 0)
	e3.PreverifyPage(engine.PoolKey{Series: s3.ID(), Index: 1}, append([]*entry.Entry{orphan}, es3...))
	if got := n3.Load(); got != 0 {
		t.Fatalf("orphan run: %d satisfy calls", got)
	}
}
