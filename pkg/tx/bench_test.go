package tx_test

import (
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/series"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/testutil"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tx"
)

func BenchmarkTID(b *testing.B) {
	s := testutil.ExampleSeries()
	t := testutil.NewKeys(s).Claim(4, P("alice", 60), P("bob", 40))
	h := s.Hash()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		t.TID(h)
	}
}

// BenchmarkCheck is the full check of a one-input spend: structure,
// witness signature, and the input against the state. "first" checks it on
// a series that has not seen it, so the signature is verified; "again"
// checks it once more, as a node does when a pending transaction arrives
// in an entry, and the series remembers the signature (ORANGE-002).
func BenchmarkCheck(b *testing.B) {
	k, st, claim := setup()
	spend := k.Spend(4, []testutil.In{k.Out(claim, 0, "alice")}, P("bob", 30), P("alice", 70))
	b.Run("first", func(b *testing.B) {
		ss := make([]*series.Series, b.N)
		for i := range ss {
			ss[i] = testutil.ExampleSeries()
		}
		tid := spend.TID(ss[0].Hash())
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if err := tx.Check(ss[i], st, spend, tid); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("again", func(b *testing.B) {
		s := testutil.ExampleSeries()
		tid := spend.TID(s.Hash())
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if err := tx.Check(s, st, spend, tid); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkDecode(b *testing.B) {
	k, _, claim := setup()
	enc := k.Spend(4, []testutil.In{k.Out(claim, 0, "alice")}, P("bob", 30), P("alice", 70)).Encode()
	b.SetBytes(int64(len(enc)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := tx.Decode(enc); err != nil {
			b.Fatal(err)
		}
	}
}
