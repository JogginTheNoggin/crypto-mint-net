package entry_test

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/entry"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/testutil"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tx"
)

// BenchmarkBuildTry is the cost of one nonce tried while building an
// entry of txs transactions with workers goroutines, for each known hash: the search for an entry
// that reaches the series' minimum entry strength. The minimum is set out
// of reach, so every one of b.N tries fails.
func BenchmarkBuildTry(b *testing.B) {
	for _, id := range hash.IDs() {
		for _, c := range []struct{ txs, workers int }{{1, 1}, {16, 1}, {256, 1}, {256, runtime.GOMAXPROCS(0)}} {
			n := c.txs
			b.Run(fmt.Sprintf("%s/txs=%d/workers=%d", id.Name(), n, c.workers), func(b *testing.B) {
				p := testutil.ExampleParams()
				p.HashID, p.Mints, p.PoolSize, p.MinEntryStrength = id, 1024, 1024, 255
				s := testutil.MustSeries(p)
				txs := make([]*tx.Tx, n)
				for i := range txs {
					// Only the tid counts here, so the claims are unsigned.
					txs[i] = &tx.Tx{Mint: s.Mint(uint64(i)), Outputs: []tx.Output{{Address: hash.Hash{1}, Amount: p.Supply}}}
				}
				b.ReportAllocs()
				b.ResetTimer()
				_, _, err := entry.BuildWorkers(context.Background(), s, s.PoolID(0), txs, uint64(b.N), c.workers)
				if !errors.Is(err, entry.ErrNotFound) {
					b.Fatal(err)
				}
			})
		}
	}
}
