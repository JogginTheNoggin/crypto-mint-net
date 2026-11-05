package engine_test

import (
	"errors"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/engine"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/entry"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/ledger"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/testutil"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tx"
)

// panicky is a body source that panics, standing in for any fault in this
// node's own code while a pool is being changed.
type panicky struct{}

func (panicky) Has(hash.Hash) bool                   { panic("injected fault") }
func (panicky) Load(hash.Hash) (*entry.Entry, error) { panic("injected fault") }

// A panic while a pool is being changed fails the engine: it takes nothing
// more, so nothing builds on a half-changed pool, and Failed is closed so
// the node stops and rebuilds from its stores (Black's Round 28 condition,
// D174). Reads still answer.
func TestPanicMidChangeFailsTheEngine(t *testing.T) {
	s := testutil.ExampleSeries()
	k := testutil.NewKeys(s)
	e := engine.New(engine.Config{
		Ledger: ledger.Config{K: ledger.Unlimited},
		Source: func(engine.PoolKey) ledger.BodySource { return panicky{} },
	})
	e.AddSeries(s)
	if err := e.FollowSeries(s.ID()); err != nil {
		t.Fatal(err)
	}
	key := engine.PoolKey{Series: s.ID(), Index: 1}
	first, _, _ := entry.Build(s, s.PoolID(1), []*tx.Tx{k.Claim(4, P("alice", 100))}, 0)
	if _, err := e.HandleEntry(key, first, engine.Pushed); err != nil {
		t.Fatal(err)
	}
	select {
	case <-e.Failed():
		t.Fatal("failed before any fault")
	default:
	}
	// The next entry makes the pool ask the source, which panics part way
	// through the change.
	second, _, _ := entry.Build(s, s.PoolID(1), []*tx.Tx{k.Claim(5, P("bob", 100))}, 0)
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("the panic did not reach the caller")
			}
		}()
		e.HandleEntry(key, second, engine.Pushed)
	}()
	select {
	case <-e.Failed():
	default:
		t.Fatal("Failed not closed after a panic mid-change")
	}
	if err := e.Err(); !errors.Is(err, engine.ErrFailed) {
		t.Fatalf("Err: %v", err)
	}
	third, _, _ := entry.Build(s, s.PoolID(1), []*tx.Tx{k.Claim(6, P("carol", 100))}, 0)
	if _, err := e.HandleEntry(key, third, engine.Pushed); !errors.Is(err, engine.ErrFailed) {
		t.Fatalf("entry after the failure: %v", err)
	}
	if _, err := e.HandleTx(k.Claim(7, P("dan", 100)), engine.Pushed); !errors.Is(err, engine.ErrFailed) {
		t.Fatalf("transaction after the failure: %v", err)
	}
	if _, err := e.Status(key); err != nil {
		t.Fatalf("a read after the failure: %v", err)
	}
}
