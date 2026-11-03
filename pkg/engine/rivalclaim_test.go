package engine_test

import (
	"context"
	"errors"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/engine"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/ledger"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/testutil"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tx"
)

// A mint the selected ledger has claimed can be claimed otherwise on a
// branch from before that claim (D161): the pending pool refuses the rival
// claim, BuildEntryOn builds it on the parent ClaimParent names, every
// node keeps it as a branch, and weight added to it makes it selected.
func TestRivalClaimOnBranch(t *testing.T) {
	s := testutil.ExampleSeries()
	k := testutil.NewKeys(s)
	net := newNetwork(t, s, 3)

	first := k.Claim(4, P("alice", 100))
	net.submit(0, first)
	e1 := net.build(0)
	net.work(0, e1.Hash(s), 500)
	// Something after the claim, so the claim's entry is not the tip.
	other := k.Claim(5, P("carol", 100))
	net.submit(0, other)
	e2 := net.build(0)
	net.work(0, e2.Hash(s), 500)

	rival := k.Claim(4, P("bob", 100))
	if _, err := net.nodes[2].HandleTx(rival, engine.Pushed); !errors.Is(err, tx.ErrAlreadyClaimed) {
		t.Fatalf("the pending pool took a second claim: %v", err)
	}
	b := net.nodes[2]
	parent, err := b.ClaimParent(net.key, 4)
	if err != nil {
		t.Fatal(err)
	}
	if want := e1.Parent; parent != want {
		t.Fatalf("claim parent %s, want %s", parent.Short(), want.Short())
	}
	// On the selected tip the rival claim does not fit.
	if _, refused, err := b.BuildEntryOn(context.Background(), net.key, 0, engine.Grind{}, engine.BuildOn{Txs: []*tx.Tx{rival}}); !errors.Is(err, engine.ErrNothingPending) || len(refused) != 1 {
		t.Fatalf("on the tip: %v %v", refused, err)
	}
	en, refused, err := b.BuildEntryOn(context.Background(), net.key, 0, engine.Grind{}, engine.BuildOn{Parent: &parent, Txs: []*tx.Tx{rival, other}})
	if err != nil {
		t.Fatal(err)
	}
	if len(en.Txs) != 2 || len(refused) != 0 {
		t.Fatalf("rival entry %d txs, refused %v", len(en.Txs), refused)
	}
	if _, err := b.HandleEntry(net.key, en, engine.Local); err != nil {
		t.Fatalf("the rival entry was not kept: %v", err)
	}
	net.flood(2, item{entry: en})
	if st := net.agree(); st.Tip != e2.Hash(s) {
		t.Fatal("a rival with no weight was selected")
	}
	for i := uint64(0); ; i++ {
		l, _, err := b.Work(net.key, en.Hash(s), (i+7)<<36, 100000)
		if err != nil {
			t.Fatal(err)
		}
		net.flood(2, item{layer: &l})
		if st := net.agree(); st.Tip == en.Hash(s) {
			break
		}
	}
	for i, node := range net.nodes {
		hs, err := node.Holdings(s.ID(), 4)
		if err != nil || len(hs) != 1 || hs[0].Portion.Address != k.Addr("bob") {
			t.Fatalf("node %d: mint 4 is held by %+v (%v), want bob", i, hs, err)
		}
	}
}

// A parent not held and a mint with no claim are refused before any work.
func TestBuildOnRefusals(t *testing.T) {
	s := testutil.ExampleSeries()
	k := testutil.NewKeys(s)
	net := newNetwork(t, s, 1)
	e := net.nodes[0]
	if _, err := e.ClaimParent(net.key, 4); !errors.Is(err, ledger.ErrNoClaim) {
		t.Fatalf("no claim: %v", err)
	}
	bad := k.Addr("nobody")
	if _, _, err := e.BuildEntryOn(context.Background(), net.key, 0, engine.Grind{}, engine.BuildOn{Parent: &bad, Txs: []*tx.Tx{k.Claim(4, P("a", 100))}}); !errors.Is(err, ledger.ErrNotHeld) {
		t.Fatalf("a parent not held: %v", err)
	}
	// The pool_id is a parent: a first entry built that way is the same
	// as one built on the empty tip.
	id, err := e.PoolID(net.key)
	if err != nil {
		t.Fatal(err)
	}
	en, _, err := e.BuildEntryOn(context.Background(), net.key, 0, engine.Grind{}, engine.BuildOn{Parent: &id, Txs: []*tx.Tx{k.Claim(4, P("a", 100))}})
	if err != nil || en.Parent != id {
		t.Fatalf("on the pool_id: %v", err)
	}
}
