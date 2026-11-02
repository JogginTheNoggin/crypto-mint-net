package ledger_test

import (
	"fmt"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/entry"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/layer"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/ledger"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/series"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/testutil"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tx"
)

// chain builds n entries on parent, each holding one signed spend of the
// last output of mint serial: the first from alice (prev's output 0) to
// to, then from to to itself.
func chain(b *testing.B, s *series.Series, k *testutil.Keys, parent hash.Hash, prev *tx.Tx, serial uint64, to string, n int) []*entry.Entry {
	b.Helper()
	out := make([]*entry.Entry, 0, n)
	owner := "alice"
	for i := 0; i < n; i++ {
		t := k.Spend(serial, []testutil.In{k.Out(prev, 0, owner)}, P(to, 100))
		owner = to
		e, _, err := entry.Build(s, parent, []*tx.Tx{t}, 0)
		if err != nil {
			b.Fatal(err)
		}
		out = append(out, e)
		parent, prev = e.Hash(s), t
	}
	return out
}

// base returns a pool with a claim of mint 4 selected.
func base(b *testing.B, s *series.Series, k *testutil.Keys) (*ledger.Pool, *entry.Entry, *tx.Tx) {
	b.Helper()
	p := ledger.New(s, 1, ledger.Config{K: ledger.DefaultK})
	claim := k.Claim(4, P("alice", 100))
	e, _, err := entry.Build(s, p.ID(), []*tx.Tx{claim}, 0)
	if err != nil {
		b.Fatal(err)
	}
	mustAdd(b, p, e)
	return p, e, claim
}

func mustAdd(b *testing.B, p *ledger.Pool, e *entry.Entry) ledger.AddResult {
	res, err := p.AddEntry(e)
	if err != nil {
		b.Fatal(err)
	}
	return res
}

func mustLayer(b *testing.B, p *ledger.Pool, l layer.Layer) ledger.AddResult {
	res, err := p.AddLayer(l)
	if err != nil {
		b.Fatal(err)
	}
	return res
}

// BenchmarkExtend measures the common case: an entry with one signed spend
// on the tip, then a layer that selects it.
func BenchmarkExtend(b *testing.B) {
	s := testutil.ExampleSeries()
	k := testutil.NewKeys(s)
	p, e, claim := base(b, s, k)
	es := chain(b, s, k, e.Hash(s), claim, 4, "alice", b.N)
	b.ResetTimer()
	for i, e := range es {
		mustAdd(b, p, e)
		mustLayer(b, p, layer.Layer{Entry: e.Hash(s), Nonce: uint64(i)})
	}
}

// BenchmarkSwitch measures a switch to a rival branch of the given depth:
// every entry of the old branch is reverted and every one of the new
// applied.
func BenchmarkSwitch(b *testing.B) {
	for _, depth := range []int{1, 10, 100} {
		b.Run(fmt.Sprintf("depth=%d", depth), func(b *testing.B) {
			s := testutil.ExampleSeries()
			k := testutil.NewKeys(s)
			_, e0, claim := base(b, s, k)
			honest := chain(b, s, k, e0.Hash(s), claim, 4, "alice", depth)
			rival := chain(b, s, k, e0.Hash(s), claim, 4, "mallory", depth)
			// The honest tip gets a layer of credit 1 (strength 0); the
			// rival one of credit 2 or more, so it is strictly heavier.
			weak := layerOf1(s, honest[depth-1].Hash(s))
			strong, _ := layer.GrindTo(s.Hash(), rival[depth-1].Hash(s), 0, 1, 1<<20)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				p := ledger.New(s, 1, ledger.Config{K: ledger.DefaultK})
				mustAdd(b, p, e0)
				for _, e := range honest {
					mustAdd(b, p, e)
				}
				mustLayer(b, p, weak)
				for _, e := range rival {
					mustAdd(b, p, e)
				}
				b.StartTimer()
				res := mustLayer(b, p, strong)
				if res.Switch == nil || len(res.Switch.Reverted) != depth {
					b.Fatalf("no switch of depth %d: %+v", depth, res.Switch)
				}
			}
		})
	}
}

// layerOf1 finds a layer of strength exactly 0 on h.
func layerOf1(s *series.Series, h hash.Hash) layer.Layer {
	for n := uint64(0); ; n++ {
		if l := (layer.Layer{Entry: h, Nonce: n}); l.Strength(s.Hash()) == 0 {
			return l
		}
	}
}

// BenchmarkLayerOnOldEntry measures a layer on the first entry of a long
// selected ledger: the weight structure makes it O(log n).
func BenchmarkLayerOnOldEntry(b *testing.B) {
	s := testutil.ExampleSeries()
	k := testutil.NewKeys(s)
	p, e0, claim := base(b, s, k)
	for _, e := range chain(b, s, k, e0.Hash(s), claim, 4, "alice", 2000) {
		mustAdd(b, p, e)
	}
	h := e0.Hash(s)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		mustLayer(b, p, layer.Layer{Entry: h, Nonce: uint64(i) + 1<<40})
	}
}

// BenchmarkExtendLong measures BenchmarkExtend's step (an entry on the tip,
// then a layer that selects it) on a selected ledger that is already n
// entries long. Its cost should not grow with n.
func BenchmarkExtendLong(b *testing.B) {
	for _, n := range []int{100, 1000, 10000} {
		b.Run(fmt.Sprintf("len=%d", n), func(b *testing.B) {
			s := testutil.ExampleSeries()
			k := testutil.NewKeys(s)
			p, e0, claim := base(b, s, k)
			es := chain(b, s, k, e0.Hash(s), claim, 4, "alice", n+b.N)
			for _, e := range es[:n] {
				mustAdd(b, p, e)
			}
			b.ResetTimer()
			for i, e := range es[n:] {
				mustAdd(b, p, e)
				mustLayer(b, p, layer.Layer{Entry: e.Hash(s), Nonce: uint64(i)})
			}
		})
	}
}

// BenchmarkOffChainBranch takes in a branch of b.N empty entries with no
// layers, parent first: the time per entry must not grow with b.N
// (RED-012).
func BenchmarkOffChainBranch(b *testing.B) {
	s := testutil.ExampleSeries()
	p := ledger.New(s, 1, ledger.Config{K: ledger.DefaultK})
	parent := p.ID()
	es := make([]*entry.Entry, b.N)
	for i := range es {
		e, _, err := entry.Build(s, parent, nil, 0)
		if err != nil {
			b.Fatal(err)
		}
		es[i], parent = e, e.Hash(s)
	}
	b.ResetTimer()
	for _, e := range es {
		mustAdd(b, p, e)
	}
}
