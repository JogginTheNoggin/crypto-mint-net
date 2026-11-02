package engine_test

import (
	"errors"
	"runtime"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/engine"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/entry"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/ledger"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/testutil"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tx"
)

// ORANGE-021: WarmStored checks stored transactions' witnesses ahead,
// each once, without their owners, so replaying the entries in order
// verifies nothing more; a witness that fails is not remembered, and the
// replay refuses its entry as before.
func TestWarmStored(t *testing.T) {
	if runtime.GOMAXPROCS(0) < 2 {
		t.Skip("on one core, replaying checks the witnesses itself")
	}
	e, s, k, n := countingEngine(t)
	key := engine.PoolKey{Series: s.ID(), Index: 1}
	es, txs := chainOf(t, s, k, 12)
	// A stored transaction and the entry holding it list it twice.
	e.WarmStored(append(append([]*tx.Tx(nil), txs...), txs...))
	if got := n.Load(); got != 12 {
		t.Fatalf("%d satisfy calls ahead, want 12", got)
	}
	n.Store(0)
	for i, en := range es {
		if _, err := e.HandleEntry(key, en, engine.Stored); err != nil {
			t.Fatalf("entry %d: %v", i, err)
		}
	}
	if got := n.Load(); got != 0 {
		t.Fatalf("%d satisfy calls replaying, want 0", got)
	}

	// A forged witness: warming does not remember it, and the replay
	// refuses the entry.
	e2, s2, k2, _ := countingEngine(t)
	key2 := engine.PoolKey{Series: s2.ID(), Index: 1}
	es2, txs2 := chainOf(t, s2, k2, 4)
	first, _ := s2.PoolRange(1)
	other := k2.Spend(first, []testutil.In{k2.Out(txs2[2], 0, "o0xx")}, testutil.P("z", 100))
	bad := *txs2[3]
	bad.Witnesses = append(bad.Witnesses[:0:0], other.Witnesses...)
	be, _, err := entry.Build(s2, es2[2].Hash(s2), []*tx.Tx{&bad}, 0)
	if err != nil {
		t.Fatal(err)
	}
	before := s2.Remembered()
	e2.WarmStored([]*tx.Tx{txs2[0], txs2[1], txs2[2], &bad})
	if got := s2.Remembered() - before; got != 3 {
		t.Fatalf("%d witnesses remembered, want 3", got)
	}
	for i := 0; i < 3; i++ {
		if _, err := e2.HandleEntry(key2, es2[i], engine.Stored); err != nil {
			t.Fatalf("entry %d: %v", i, err)
		}
	}
	if _, err := e2.HandleEntry(key2, be, engine.Stored); !errors.Is(err, ledger.ErrInvalidEntry) {
		t.Fatalf("forged witness: %v", err)
	}
}
