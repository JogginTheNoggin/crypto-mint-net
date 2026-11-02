package ledger_test

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/entry"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/ledger"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/state"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/testutil"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tx"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/weight"
)

// model is a brute-force reference: it stores every entry and its layers and
// recomputes everything from scratch.
type model struct {
	parent  map[hash.Hash]hash.Hash
	entries map[hash.Hash]*entry.Entry
	own     map[hash.Hash]weight.Weight
	seq     map[hash.Hash]int
	n       int
	current hash.Hash // selected newest entry, pool id when empty
	poolID  hash.Hash
}

func (m *model) path(h hash.Hash) []hash.Hash {
	var out []hash.Hash
	for h != m.poolID {
		out = append([]hash.Hash{h}, out...)
		h = m.parent[h]
	}
	return out
}

func (m *model) weight(h hash.Hash) weight.Weight {
	w := weight.Zero
	for _, e := range m.path(h) {
		w = w.Add(m.own[e])
	}
	return w
}

// reselect applies the rule: switch only to a strictly heavier ledger,
// choosing the heaviest and, among equals, the one seen first.
func (m *model) reselect() {
	cur := m.weight(m.current)
	best := m.current
	bestW := cur
	for h := range m.entries {
		w := m.weight(h)
		switch {
		case w.Greater(bestW):
			best, bestW = h, w
		case w.Cmp(bestW) == 0 && bestW.Greater(cur) && m.seq[h] < m.seq[best]:
			best = h
		}
	}
	m.current = best
}

func replay(f *fixture, path []hash.Hash, m *model) *state.State {
	st := state.New()
	for _, h := range path {
		st.Apply(itemsOf(f, m.entries[h]))
	}
	return st
}

func itemsOf(f *fixture, e *entry.Entry) []state.Item {
	var out []state.Item
	for _, it := range e.Items(f.s.Hash()) {
		out = append(out, state.Item{TID: it.TID, Tx: it.Tx})
	}
	return out
}

func snapshotOf(st *state.State) string {
	m := map[tx.Input]tx.Portion{}
	st.ForEach(func(in tx.Input, p tx.Portion) { m[in] = p })
	return fmt.Sprint(m)
}

func TestRandomAgainstModel(t *testing.T) {
	total := 0
	for seed := int64(1); seed <= 12; seed++ {
		t.Run(fmt.Sprint(seed), func(t *testing.T) { total += runRandom(t, seed, 150, ledger.Unlimited) })
	}
	// Guard against the test silently getting weaker.
	if total < 12 {
		t.Fatalf("only %d reorganizations across all seeds", total)
	}
	t.Logf("%d reorganizations checked", total)
}

// TestRandomPruned runs the random steps with pruning on. Pruned entries
// leave the model behind, so it checks only the pool's own bookkeeping.
func TestRandomPruned(t *testing.T) {
	for seed := int64(1); seed <= 12; seed++ {
		t.Run(fmt.Sprint(seed), func(t *testing.T) { runRandom(t, seed, 150, 1) })
	}
}

// runRandom makes random entries and layers. With k Unlimited it checks
// the pool against a model after every step; otherwise it checks only the
// pool's own bookkeeping, since pruned entries leave the model behind.
func runRandom(t *testing.T, seed int64, steps int, k int) int {
	rng := rand.New(rand.NewSource(seed))
	f := newFixture(t, ledger.Config{K: k, Ties: 2})
	owners := map[hash.Hash]string{}
	for i := 0; i < 6; i++ {
		l := fmt.Sprintf("o%d", i)
		owners[f.k.Addr(l)] = l
	}
	labels := func() string { return fmt.Sprintf("o%d", rng.Intn(6)) }
	m := &model{
		parent: map[hash.Hash]hash.Hash{}, entries: map[hash.Hash]*entry.Entry{},
		own: map[hash.Hash]weight.Weight{}, seq: map[hash.Hash]int{},
		poolID: f.pool.ID(), current: f.pool.ID(),
	}
	var held []hash.Hash
	reorgs := 0
	checkSwitch := func(step int, sw *ledger.Switch) {
		if sw == nil || sw.Extension() {
			return
		}
		reorgs++
		n := 0
		for _, r := range sw.Reverted {
			n += len(m.entries[r].Txs)
		}
		if got := len(sw.Unchanged) + len(sw.Carried) + len(sw.Void); got != n {
			t.Fatalf("step %d: %d displaced, want %d", step, got, n)
		}
	}

	for step := 0; step < steps; step++ {
		if len(held) > 0 && rng.Intn(3) == 0 {
			// Add a layer to a random held entry.
			h := held[rng.Intn(len(held))]
			l := layerOf(t, f.s, h, rng.Intn(6), uint64(rng.Int63()))
			res, err := f.pool.AddLayer(l)
			if err != nil {
				continue // a repeated nonce is fine to skip
			}
			checkSwitch(step, res.Switch)
			m.own[h] = m.own[h].Add(weight.Credit(l.Strength(f.s.Hash())))
		} else {
			// Build a random valid entry on a random parent.
			parent := f.pool.ID()
			if len(held) > 0 && rng.Intn(5) != 0 {
				parent = held[rng.Intn(len(held))]
			}
			st := replay(f, m.path(parent), m)
			var txs []*tx.Tx
			used := map[tx.Input]bool{}
			claimed := map[uint64]bool{}
			for i := 0; i < 1+rng.Intn(3); i++ {
				serial := uint64(4 + rng.Intn(4))
				if !st.Claimed(serial) {
					if claimed[serial] {
						continue
					}
					claimed[serial] = true
					txs = append(txs, f.k.Claim(serial, P(labels(), 100)))
					continue
				}
				ps := st.Portions(serial)
				p := ps[rng.Intn(len(ps))]
				if used[p.Input] {
					continue
				}
				used[p.Input] = true
				owner := owners[p.Portion.Address]
				in := []testutil.In{{Input: p.Input, Owner: owner}}
				to1 := labels()
				if p.Portion.Amount > 1 && rng.Intn(2) == 0 {
					to2 := labels()
					for to2 == to1 {
						to2 = labels()
					}
					a := 1 + uint64(rng.Int63n(int64(p.Portion.Amount-1)))
					txs = append(txs, f.k.Spend(serial, in, P(to1, a), P(to2, p.Portion.Amount-a)))
				} else {
					txs = append(txs, f.k.Spend(serial, in, P(to1, p.Portion.Amount)))
				}
			}
			e := f.build(parent, txs...)
			h := e.Hash(f.s)
			res, err := f.pool.AddEntry(e)
			if err != nil {
				continue // same entry twice
			}
			checkSwitch(step, res.Switch)
			m.n++
			m.parent[h], m.entries[h], m.seq[h] = parent, e, m.n
			held = append(held, h)
		}
		if err := ledger.CheckOff(f.pool); err != nil {
			t.Fatalf("step %d: %v", step, err)
		}
		if k != ledger.Unlimited {
			continue
		}
		m.reselect()

		tip, height, w := f.pool.Tip()
		if tip != m.current {
			t.Fatalf("step %d: selected %s, model %s", step, tip.Short(), m.current.Short())
		}
		if w.Cmp(m.weight(m.current)) != 0 || height != len(m.path(m.current)) {
			t.Fatalf("step %d: weight %s, model %s", step, w, m.weight(m.current))
		}
		if got, want := snapshotOf(f.pool.State()), snapshotOf(replay(f, m.path(m.current), m)); got != want {
			t.Fatalf("step %d: tip state differs from a replay of the selected ledger", step)
		}
		for serial, total := range f.pool.State().Totals() {
			if total != 100 {
				t.Fatalf("step %d: mint %d totals %d", step, serial, total)
			}
		}
		for _, h := range held {
			info, _ := f.pool.Info(h)
			if info.Ledger.Cmp(m.weight(h)) != 0 {
				t.Fatalf("step %d: ledger weight at %s is %s, model %s", step, h.Short(), info.Ledger, m.weight(h))
			}
		}
	}
	if k == ledger.Unlimited {
		checkReplayOrder(t, f)
	}
	return reorgs
}

// checkReplayOrder rebuilds the pool from ReplayOrder and checks that it
// selects the same ledger with the same weight and holds the same entries.
func checkReplayOrder(t *testing.T, f *fixture) {
	t.Helper()
	g := ledger.New(f.s, 1, ledger.Config{K: ledger.Unlimited})
	order := f.pool.ReplayOrder()
	if len(order) != f.pool.Len() {
		t.Fatalf("replay order has %d entries, pool holds %d", len(order), f.pool.Len())
	}
	for _, h := range order {
		e, _ := f.pool.Entry(h)
		if _, err := g.AddEntry(e); err != nil {
			t.Fatalf("replay entry %s: %v", h.Short(), err)
		}
		for _, l := range f.pool.Layers(h) {
			if _, err := g.AddLayer(l); err != nil {
				t.Fatalf("replay layer on %s: %v", h.Short(), err)
			}
		}
	}
	t1, h1, w1 := f.pool.Tip()
	t2, h2, w2 := g.Tip()
	if t1 != t2 || h1 != h2 || w1.Cmp(w2) != 0 || g.Len() != f.pool.Len() {
		t.Fatalf("replayed pool selects %s (%d, %s), original %s (%d, %s)", t2.Short(), h2, w2, t1.Short(), h1, w1)
	}
}
