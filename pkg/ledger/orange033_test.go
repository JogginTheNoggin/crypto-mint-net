package ledger_test

import (
	"errors"
	"strconv"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/entry"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/layer"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/ledger"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/series"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/testutil"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tx"
)

// spread returns a pool whose selected ledger holds one claim of mint 4
// paying n outputs of 1 to n owners, and that claim.
func spread(tb testing.TB, n int) (*series.Series, *testutil.Keys, *ledger.Pool, *tx.Tx) {
	tb.Helper()
	prm := testutil.ExampleParams()
	prm.Supply = uint64(n)
	s := testutil.MustSeries(prm)
	k := testutil.NewKeys(s)
	p := ledger.New(s, 1, ledger.Config{K: ledger.DefaultK})
	outs := make([]testutil.Pay, n)
	for i := range outs {
		outs[i] = P("a"+strconv.Itoa(i), 1)
	}
	claim := k.Claim(4, outs...)
	e, _, err := entry.Build(s, p.ID(), []*tx.Tx{claim}, 0)
	if err != nil {
		tb.Fatal(err)
	}
	if _, err := p.AddEntry(e); err != nil {
		tb.Fatal(err)
	}
	if _, err := p.AddLayer(layer.Layer{Entry: e.Hash(s), Nonce: 1}); err != nil {
		tb.Fatal(err)
	}
	return s, k, p, claim
}

// CheckOn keeps one of each set of transactions that claim the same mint
// or spend the same input, refuses the rest as conflicts, and keeps every
// transaction that conflicts with none (ORANGE-033: the conflict check is
// a set lookup, with the same result as the pairwise one).
func TestCheckOnConflicts(t *testing.T) {
	s, k, p, claim := spread(t, 3)
	in := func(i int) testutil.In { return k.Out(claim, uint32(i), "a"+strconv.Itoa(i)) }
	txs := []*tx.Tx{
		k.Spend(4, []testutil.In{in(0)}, P("bob", 1)),
		k.Spend(4, []testutil.In{in(0)}, P("carol", 1)), // a0 again
		k.Spend(4, []testutil.In{in(1)}, P("bob", 1)),
		k.Spend(4, []testutil.In{in(1), in(2)}, P("carol", 2)), // a1 again
		k.Claim(5, P("bob", 3)),
		k.Claim(5, P("carol", 3)), // mint 5 again
		k.Claim(6, P("bob", 3)),
	}
	tip, _, _ := p.Tip()
	ok, refused, err := p.CheckOn(tip, txs)
	if err != nil {
		t.Fatal(err)
	}
	if len(ok) != 4 || len(refused) != 3 {
		t.Fatalf("kept %d, refused %d; want 4 and 3: %+v", len(ok), len(refused), refused)
	}
	for _, r := range refused {
		if !errors.Is(r.Err, entry.ErrConflict) {
			t.Fatalf("refused for %v, not a conflict", r.Err)
		}
	}
	claims := map[uint64]bool{}
	spent := map[tx.Input]bool{}
	for _, x := range ok {
		if x.IsClaim() {
			if claims[x.Mint.Serial] {
				t.Fatalf("mint %d claimed twice", x.Mint.Serial)
			}
			claims[x.Mint.Serial] = true
		}
		for _, i := range x.Inputs {
			if spent[i] {
				t.Fatalf("input %v spent twice", i)
			}
			spent[i] = true
		}
	}
	// What CheckOn keeps is itself one valid entry on the tip.
	if _, _, err := entry.Build(s, tip, ok, 0); err != nil {
		t.Fatal(err)
	}
}

// BenchmarkCheckOnMany measures CheckOn on many independent pending
// spends, as a build on a chosen parent with no -tx list takes them all
// (ORANGE-033). Before: quadratic in their number.
func BenchmarkCheckOnMany(b *testing.B) {
	const n = 4096
	s, k, p, claim := spread(b, n)
	ctid := claim.TID(s.Hash())
	txs := make([]*tx.Tx, n)
	for i := range txs {
		txs[i] = k.Spend(4, []testutil.In{{Input: tx.Input{TID: ctid, Index: uint32(i)}, Owner: "a" + strconv.Itoa(i)}}, P("bob", 1))
	}
	tip, _, _ := p.Tip()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if ok, _, err := p.CheckOn(tip, txs); err != nil || len(ok) != n {
			b.Fatal(len(ok), err)
		}
	}
}
