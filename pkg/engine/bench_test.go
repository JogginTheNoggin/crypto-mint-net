package engine_test

import (
	"sync/atomic"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/engine"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/testutil"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tx"
)

// BenchmarkHandleTxParallel takes in distinct, valid claims from as many
// goroutines as -cpu says, the way a node takes them from its peers. Each
// claim's signature is new, so it is verified once. With -cpu 1,4 it
// shows how far checking scales across cores.
func BenchmarkHandleTxParallel(b *testing.B) {
	p := testutil.ExampleParams()
	p.Mints = 1 << 20
	s := testutil.MustSeries(p)
	k := testutil.NewKeys(s)
	claims := make([]*tx.Tx, b.N)
	for i := range claims {
		claims[i] = k.Claim(uint64(i), testutil.P("alice", p.Supply))
	}
	e := engine.New(engine.DefaultConfig())
	e.AddSeries(s)
	if err := e.FollowSeries(s.ID()); err != nil {
		b.Fatal(err)
	}
	var next atomic.Int64
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			t := claims[next.Add(1)-1]
			// A node preverifies before its own lock (ORANGE-004).
			e.PreverifyTx(t)
			if _, err := e.HandleTx(t, engine.Pushed); err != nil {
				b.Error(err)
				return
			}
		}
	})
}
