package node_test

import (
	"fmt"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/engine"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/node"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/nodepolicy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/testutil"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tx"
)

// BenchmarkHandleTxParallel takes in distinct, valid claims of one pool
// from as many goroutines as -cpu says, the way a node takes them from its
// peers: each is checked, stored and announced. With sync=true every kept
// item waits for the store's fsync, the default; peers=16x runs 16
// goroutines per -cpu, as a node with many peers does.
func BenchmarkHandleTxParallel(b *testing.B) {
	for _, c := range []struct {
		sync bool
		par  int // goroutines per -cpu, standing for peers
	}{{false, 1}, {true, 1}, {true, 16}} {
		sync := c.sync
		b.Run(fmt.Sprintf("sync=%v/peers=%dx", sync, c.par), func(b *testing.B) {
			p := testutil.ExampleParams()
			p.Mints, p.PoolSize = 1<<20, 1<<20
			s := testutil.MustSeries(p)
			k := testutil.NewKeys(s)
			claims := make([]*tx.Tx, b.N)
			for i := range claims {
				claims[i] = k.Claim(uint64(i), testutil.P("alice", p.Supply))
			}
			id := s.ID()
			pol := nodepolicy.Default()
			pol.Series = []nodepolicy.SeriesInfo{{Params: p}}
			pol.Follow = []nodepolicy.Follow{{Series: &id}}
			pol.RPC.Listen = "off"
			pol.NoSync = !sync
			pol.MaxPending = max(b.N+1, 16) // the floor is 16
			dir := b.TempDir()
			if err := nodepolicy.Save(filepath.Join(dir, nodepolicy.FileName), pol); err != nil {
				b.Fatal(err)
			}
			n, err := node.Open(dir, node.Options{})
			if err != nil {
				b.Fatal(err)
			}
			defer n.Close()
			var next atomic.Int64
			b.SetParallelism(c.par)
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					if _, err := n.HandleTx(claims[next.Add(1)-1], engine.Pushed); err != nil {
						b.Error(err)
						return
					}
				}
			})
		})
	}
}
