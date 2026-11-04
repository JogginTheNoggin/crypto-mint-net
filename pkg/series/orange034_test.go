package series_test

import (
	"math/rand"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/series"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/testutil"
)

func variableSeries(t *testing.T) *series.Series {
	t.Helper()
	p := testutil.ExampleParams()
	p.SupplyType = series.Variable
	p.PartSize = 4
	iss := hash.Hash{1}
	p.Issuer = &iss
	s, err := series.New(p)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// PartID keeps one id in every stride and the stride last asked for
// (ORANGE-034). Whatever order parts are asked for in, each id is the one
// the chain part_id(k) = H("PoO-part" ∥ series_id ∥ k ∥ part_id(k − 1))
// gives, worked out here from scratch with Release.ID.
func TestPartIDsAnyOrder(t *testing.T) {
	const n = 5000
	ref := variableSeries(t)
	want := make([]hash.Hash, n+1)
	prev := ref.ID()
	for k := uint64(1); k <= n; k++ {
		prev = series.Release{Series: ref.ID(), Part: k, Previous: prev}.ID(ref.Hash())
		want[k] = prev
	}
	check := func(s *series.Series, k uint64) {
		t.Helper()
		got, err := s.PartID(k)
		if err != nil || got != want[k] {
			t.Fatalf("part %d: %s, %v; want %s", k, got.Short(), err, want[k].Short())
		}
	}
	// In order, as poo series parts lists them.
	s := variableSeries(t)
	for k := uint64(1); k <= n; k++ {
		check(s, k)
	}
	// Far first, then back across stride edges, then at random.
	s = variableSeries(t)
	for _, k := range []uint64{4000, 1, 1024, 1025, 1026, 2048, 2049, 3072, 4000, 3999, 4001, 5000, 2} {
		check(s, k)
	}
	rng := rand.New(rand.NewSource(34))
	for i := 0; i < 3000; i++ {
		check(s, 1+uint64(rng.Intn(n)))
	}
	// Grown a little at a time, as releases arrive one by one.
	s = variableSeries(t)
	for k := uint64(1); k <= n; k += 1 + uint64(rng.Intn(700)) {
		check(s, k)
		check(s, 1+uint64(rng.Intn(int(k))))
	}
}
