package entry_test

import (
	"context"
	"errors"
	"math/rand"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/entry"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/series"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/state"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/testutil"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tx"
)

var P = testutil.P

func TestCanonicalOrderAnyArrival(t *testing.T) {
	s := testutil.ExampleSeries()
	k := testutil.NewKeys(s)
	pool := s.PoolID(1)
	txs := []*tx.Tx{
		k.Claim(6, P("carol", 100)),
		k.Claim(4, P("alice", 100)),
		k.Claim(5, P("bob", 100)),
		k.Claim(7, P("hank", 60), P("gina", 40)),
	}
	_, want, err := entry.Build(s, pool, txs, 0)
	if err != nil {
		t.Fatal(err)
	}
	rng := rand.New(rand.NewSource(1))
	for i := 0; i < 20; i++ {
		rng.Shuffle(len(txs), func(a, b int) { txs[a], txs[b] = txs[b], txs[a] })
		e, h, err := entry.Build(s, pool, txs, 0)
		if err != nil {
			t.Fatal(err)
		}
		if h != want || e.Hash(s) != want {
			t.Fatal("same set gave a different entry hash")
		}
		if _, _, err := entry.CheckStructure(s, 1, e); err != nil {
			t.Fatalf("built entry refused: %v", err)
		}
	}
}

func TestStructureRules(t *testing.T) {
	s := testutil.ExampleSeries()
	k := testutil.NewKeys(s)
	a, b := k.Claim(4, P("alice", 100)), k.Claim(5, P("bob", 100))
	e, _, _ := entry.Build(s, s.PoolID(1), []*tx.Tx{a, b}, 0)

	swapped := &entry.Entry{Parent: e.Parent, Txs: []*tx.Tx{e.Txs[1], e.Txs[0]}}
	if _, _, err := entry.CheckStructure(s, 1, swapped); !errors.Is(err, entry.ErrOrder) {
		t.Errorf("want ErrOrder, got %v", err)
	}
	dup := &entry.Entry{Parent: e.Parent, Txs: []*tx.Tx{a, a}}
	if _, _, err := entry.CheckStructure(s, 1, dup); !errors.Is(err, entry.ErrOrder) {
		t.Errorf("duplicate tx: want ErrOrder, got %v", err)
	}
	if _, _, err := entry.CheckStructure(s, 0, e); !errors.Is(err, entry.ErrWrongPool) {
		t.Errorf("want ErrWrongPool, got %v", err)
	}
	withNonce := &entry.Entry{Parent: e.Parent, Txs: e.Txs, Nonce: 1}
	if _, _, err := entry.CheckStructure(s, 1, withNonce); !errors.Is(err, entry.ErrNonce) {
		t.Errorf("want ErrNonce, got %v", err)
	}
	bad := *a
	bad.Outputs = []tx.Output{{Address: a.Outputs[0].Address, Amount: 99}}
	if _, _, err := entry.CheckStructure(s, 1, &entry.Entry{Parent: e.Parent, Txs: []*tx.Tx{&bad}}); !errors.Is(err, entry.ErrInvalidTx) {
		t.Errorf("want ErrInvalidTx, got %v", err)
	}
}

func TestMinimumEntryStrength(t *testing.T) {
	p := testutil.ExampleParams()
	p.MinEntryStrength = 8
	s := testutil.MustSeries(p)
	k := testutil.NewKeys(s)
	e, h, err := entry.Build(s, s.PoolID(0), []*tx.Tx{k.Claim(0, P("a", 100))}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if h.Strength() < 8 {
		t.Fatalf("strength %d", h.Strength())
	}
	if _, _, err := entry.CheckStructure(s, 0, e); err != nil {
		t.Fatal(err)
	}
	// The nonce is inside the hash: changing it changes the hash and, almost
	// always, drops it below the minimum.
	e2 := *e
	e2.Nonce++
	if e2.Hash(s) == h {
		t.Fatal("nonce not covered by the hash")
	}
	// Try a few nonces to find one that is too weak.
	for n := uint64(0); ; n++ {
		e2.Nonce = n
		if e2.Hash(s).Strength() < 8 {
			break
		}
	}
	if _, _, err := entry.CheckStructure(s, 0, &e2); !errors.Is(err, entry.ErrWeak) {
		t.Fatalf("want ErrWeak, got %v", err)
	}
	if _, _, err := entry.Build(s, s.PoolID(0), nil, 1); err != nil && !errors.Is(err, entry.ErrNotFound) {
		t.Fatal(err)
	}
}

func TestCheckAgainst(t *testing.T) {
	s := testutil.ExampleSeries()
	k := testutil.NewKeys(s)
	claim := k.Claim(4, P("alice", 100))
	st := state.New()
	st.Apply([]state.Item{{TID: claim.TID(s.Hash()), Tx: claim}})

	pay1 := k.Spend(4, []testutil.In{k.Out(claim, 0, "alice")}, P("bob", 100))
	pay2 := k.Spend(4, []testutil.In{k.Out(claim, 0, "alice")}, P("eve", 100))
	e, _, _ := entry.Build(s, s.PoolID(1), []*tx.Tx{pay1, pay2}, 0)
	_, its, err := entry.CheckStructure(s, 1, e)
	if err != nil {
		t.Fatal(err)
	}
	if err := entry.CheckAgainst(s, st, its); !errors.Is(err, entry.ErrConflict) {
		t.Errorf("double spend in one entry: want ErrConflict, got %v", err)
	}

	// A claim and a spend of what it creates cannot share an entry.
	spend := k.Spend(4, []testutil.In{k.Out(claim, 0, "alice")}, P("bob", 100))
	e, _, _ = entry.Build(s, s.PoolID(1), []*tx.Tx{claim, spend}, 0)
	_, its, _ = entry.CheckStructure(s, 1, e)
	if err := entry.CheckAgainst(s, state.New(), its); !errors.Is(err, entry.ErrInvalidTx) {
		t.Errorf("dependency inside an entry: want ErrInvalidTx, got %v", err)
	}

	// Two claims of one mint conflict.
	c2 := k.Claim(4, P("bob", 100))
	e, _, _ = entry.Build(s, s.PoolID(1), []*tx.Tx{claim, c2}, 0)
	_, its, _ = entry.CheckStructure(s, 1, e)
	if err := entry.CheckAgainst(s, state.New(), its); !errors.Is(err, entry.ErrConflict) {
		t.Errorf("two claims: want ErrConflict, got %v", err)
	}
}

func TestEncodeDecode(t *testing.T) {
	s := testutil.ExampleSeries()
	k := testutil.NewKeys(s)
	e, h, _ := entry.Build(s, s.PoolID(1), []*tx.Tx{k.Claim(4, P("a", 100)), k.Claim(5, P("b", 100))}, 0)
	back, err := entry.Decode(e.Encode())
	if err != nil {
		t.Fatal(err)
	}
	if back.Hash(s) != h {
		t.Fatal("round trip changed the hash")
	}
	if _, err := entry.Decode(append(e.Encode(), 0)); err == nil {
		t.Fatal("trailing bytes must fail")
	}
	empty, eh, _ := entry.Build(s, s.PoolID(1), nil, 0)
	if back, err := entry.Decode(empty.Encode()); err != nil || back.Hash(s) != eh {
		t.Fatal("empty entry round trip failed")
	}
}

// RED-006: a nonce search with no limit stops once its context is done.
func TestBuildContextCancels(t *testing.T) {
	p := testutil.ExampleParams()
	p.MinEntryStrength = series.MaxStrength // never met
	s := testutil.MustSeries(p)
	k := testutil.NewKeys(s)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := entry.BuildContext(ctx, s, s.PoolID(0), []*tx.Tx{k.Claim(0, P("a", 100))}, 0); !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled, got %v", err)
	}
}
