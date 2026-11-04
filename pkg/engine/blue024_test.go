package engine_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/engine"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/entry"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/testutil"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tx"
)

// RED-069, D162: a transaction on a mint of another pool is refused by the
// check before the build, so the grind never starts.
func TestBuildOnRefusesAnotherPool(t *testing.T) {
	s := testutil.ExampleSeries()
	k := testutil.NewKeys(s)
	net := newNetwork(t, s, 1)
	e := net.nodes[0]
	id, err := e.PoolID(net.key)
	if err != nil {
		t.Fatal(err)
	}
	var tried atomic.Uint64
	g := engine.Grind{Tried: &tried}
	_, refused, err := e.BuildEntryOn(context.Background(), net.key, 0, g, engine.BuildOn{Parent: &id, Txs: []*tx.Tx{k.Claim(0, P("a", 100))}})
	if !errors.Is(err, engine.ErrNothingPending) || len(refused) != 1 || !errors.Is(refused[0].Err, entry.ErrWrongPool) {
		t.Fatalf("a claim of another pool's mint: %v, refused %+v", err, refused)
	}
	if tried.Load() != 0 {
		t.Fatal("the grind started")
	}
	// With a transaction of the pool beside it, only the stranger is left out.
	en, refused, err := e.BuildEntryOn(context.Background(), net.key, 0, engine.Grind{}, engine.BuildOn{Parent: &id, Txs: []*tx.Tx{k.Claim(0, P("a", 100)), k.Claim(4, P("a", 100))}})
	if err != nil || len(refused) != 1 || len(en.Txs) != 1 {
		t.Fatalf("mixed: %v, refused %+v", err, refused)
	}
}
