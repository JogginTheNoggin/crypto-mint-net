package engine_test

import (
	"errors"
	"sync/atomic"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/engine"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/entry"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/ledger"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/policy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/series"
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

// counting wraps a policy type and counts calls of its satisfaction
// function, the expensive step RED-009 is about.
type counting struct {
	policy.Type
	n *atomic.Int64
}

func (c counting) Satisfy(r *policy.Registry, params, data, msg []byte) error {
	c.n.Add(1)
	return c.Type.Satisfy(r, params, data, msg)
}

// countingEngine follows the example series under a registry whose
// policy types count their satisfaction calls.
func countingEngine(t *testing.T) (*engine.Engine, *series.Series, *testutil.Keys, *atomic.Int64) {
	t.Helper()
	n := new(atomic.Int64)
	reg := policy.NewRegistry()
	for _, a := range policy.Default().Algs() {
		reg.RegisterAlg(a)
	}
	for _, ty := range policy.Default().Types() {
		reg.RegisterType(counting{ty, n})
	}
	s, err := series.NewWithRegistry(testutil.ExampleParams(), reg)
	if err != nil {
		t.Fatal(err)
	}
	e := engine.New(engine.DefaultConfig())
	e.AddSeries(s)
	if err := e.FollowSeries(s.ID()); err != nil {
		t.Fatal(err)
	}
	return e, s, testutil.NewKeys(s), n
}

// RED-009: preverifying costs signature checks only for what HandleTx
// and HandleEntry would verify, and stops at the first bad witness.
func TestPreverifyOnlyWhatHandleWouldVerify(t *testing.T) {
	e, s, k, n := countingEngine(t)
	key := engine.PoolKey{Series: s.ID(), Index: 1}
	first, _ := s.PoolRange(1)

	// Inputs that do not exist: no signature verified, and none remembered
	// although every witness is valid for the tid.
	ins := make([]testutil.In, 200)
	for i := range ins {
		ins[i] = testutil.In{Input: tx.Input{TID: hash.Hash{9}, Index: uint32(i)}, Owner: "alice"}
	}
	ghost := k.Spend(first, ins, testutil.P("bob", 1))
	e.PreverifyTx(ghost)
	if got := n.Load(); got != 0 || s.Remembered() != 0 {
		t.Fatalf("spend of nothing: %d satisfy calls, %d remembered", got, s.Remembered())
	}
	if _, err := e.HandleTx(ghost, engine.Pushed); err == nil || n.Load() != 0 {
		t.Fatalf("handle: %v, %d calls", err, n.Load())
	}

	// A real claim, selected.
	claim := k.Claim(first, testutil.P("alice", 50), testutil.P("alice2", 50))
	if _, err := e.HandleTx(claim, engine.Local); err != nil {
		t.Fatal(err)
	}
	en, _, err := e.Build(key, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.Work(key, en.Hash(s), 0, 50); err != nil {
		t.Fatal(err)
	}

	// A spend of both portions whose first witness signs another tid: one
	// satisfy call ahead, then one under the lock, and the error names a
	// bad witness.
	spend := k.Spend(first, []testutil.In{k.Out(claim, 0, "alice"), k.Out(claim, 1, "alice2")}, testutil.P("bob", 100))
	other := k.Spend(first, []testutil.In{k.Out(claim, 0, "alice"), k.Out(claim, 1, "alice2")}, testutil.P("carol", 100))
	spend.Witnesses[0] = other.Witnesses[0]
	n.Store(0)
	e.PreverifyTx(spend)
	if got := n.Load(); got != 1 {
		t.Fatalf("bad first witness: %d satisfy calls ahead, want 1", got)
	}
	if _, err := e.HandleTx(spend, engine.Pushed); !errors.Is(err, tx.ErrWitness) {
		t.Fatalf("bad witness not reported as one: %v", err)
	}
	if got := n.Load(); got != 2 {
		t.Fatalf("%d satisfy calls in all, want 2", got)
	}

	// An entry on a parent the node does not hold: no signature verified,
	// ahead or in HandleEntry.
	good := k.Spend(first, []testutil.In{k.Out(claim, 0, "alice"), k.Out(claim, 1, "alice2")}, testutil.P("dave", 100))
	orphan, _, err := entry.Build(s, hash.Hash{7}, []*tx.Tx{good}, 0)
	if err != nil {
		t.Fatal(err)
	}
	n.Store(0)
	e.PreverifyEntry(key, orphan, engine.Pushed)
	if _, err := e.HandleEntry(key, orphan, engine.Pushed); !errors.Is(err, ledger.ErrUnknownParent) {
		t.Fatalf("orphan: %v", err)
	}
	if got := n.Load(); got != 0 {
		t.Fatalf("orphan entry: %d satisfy calls", got)
	}

	// A valid spend is verified once ahead and remembered, so HandleTx
	// verifies nothing under the lock.
	n.Store(0)
	e.PreverifyTx(good)
	ahead := n.Load()
	if _, err := e.HandleTx(good, engine.Pushed); err != nil {
		t.Fatal(err)
	}
	if ahead != 2 || n.Load() != 2 {
		t.Fatalf("valid spend: %d ahead, %d in all, want 2 and 2", ahead, n.Load())
	}
}

// RED-011: an entry on a parent deeper than ledger.MaxPreviewCost below
// the selected tip is not preverified, so the revert to that parent is paid
// once (in HandleEntry), and EntryCost reports what it costs.
func TestPreverifySkipsDeepParent(t *testing.T) {
	e, s, k, n := countingEngine(t)
	key := engine.PoolKey{Series: s.ID(), Index: 1}
	var id hash.Hash
	if err := e.Ledger(key, func(p *ledger.Pool) { id = p.ID() }); err != nil {
		t.Fatal(err)
	}
	depth := ledger.MaxPreviewCost + 2
	parent := id
	for i := 0; i < depth; i++ {
		en, _, err := entry.Build(s, parent, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := e.HandleEntry(key, en, engine.Local); err != nil {
			t.Fatal(err)
		}
		parent = en.Hash(s)
		if _, _, err := e.Work(key, parent, 0, 50); err != nil {
			t.Fatal(err)
		}
	}
	first, _ := s.PoolRange(1)
	claim := k.Claim(first, testutil.P("alice", 100))
	for _, c := range []struct {
		parent hash.Hash
		cost   int
		ahead  int64
	}{{parent, 0, 1}, {id, depth, 0}} {
		en, _, err := entry.Build(s, c.parent, []*tx.Tx{claim}, 0)
		if err != nil {
			t.Fatal(err)
		}
		if got := e.EntryCost(key, en); got != c.cost {
			t.Errorf("parent %s: cost %d, want %d", c.parent.Short(), got, c.cost)
		}
		n.Store(0)
		e.PreverifyEntry(key, en, engine.Pushed)
		if got := n.Load(); got != c.ahead {
			t.Errorf("parent %s: %d satisfy calls ahead, want %d", c.parent.Short(), got, c.ahead)
		}
	}
}
