package entry_test

import (
	"context"
	"errors"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/entry"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/testutil"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tx"
)

// TestBuildWorkersMatchesSearch checks that BuildWorkers finds the lowest
// nonce a plain search with HashOf finds, for every known hash, any number
// of workers, and limits that stop the search before, at and after it.
func TestBuildWorkersMatchesSearch(t *testing.T) {
	for _, id := range hash.IDs() {
		p := testutil.ExampleParams()
		p.HashID, p.Mints, p.PoolSize, p.MinEntryStrength = id, 64, 64, 13
		s := testutil.MustSeries(p)
		var txs []*tx.Tx
		var tids []hash.Hash
		for i := 0; i < 5; i++ {
			t := &tx.Tx{Mint: s.Mint(uint64(i)), Outputs: []tx.Output{{Address: hash.Hash{1}, Amount: p.Supply}}}
			txs = append(txs, t)
		}
		e0, _, err := entry.Build(s, s.PoolID(0), txs, 0)
		if err != nil {
			t.Fatal(err)
		}
		tids = e0.TIDs(s.Hash())
		var want uint64
		for !(entry.HashOf(s, s.PoolID(0), tids, want).Strength() >= 13) {
			want++
		}
		if e0.Nonce != want {
			t.Fatalf("%s: Build found %d, want %d", id.Name(), e0.Nonce, want)
		}
		for _, workers := range []int{1, 2, 3, 8} {
			for _, max := range []uint64{0, want, want + 1, want + 5000} {
				e, h, err := entry.BuildWorkers(context.Background(), s, s.PoolID(0), txs, max, workers)
				if max == want {
					if !errors.Is(err, entry.ErrNotFound) {
						t.Fatalf("%s workers %d max %d: %v, want ErrNotFound", id.Name(), workers, max, err)
					}
					continue
				}
				if err != nil {
					t.Fatal(err)
				}
				if e.Nonce != want || h != e.Hash(s) {
					t.Fatalf("%s workers %d max %d: nonce %d, want %d", id.Name(), workers, max, e.Nonce, want)
				}
			}
		}
	}
}

// TestBuildWorkersCancel checks that a search with no end stops when its
// context is done.
func TestBuildWorkersCancel(t *testing.T) {
	p := testutil.ExampleParams()
	p.MinEntryStrength = 255
	s := testutil.MustSeries(p)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	txs := []*tx.Tx{{Mint: s.Mint(4), Outputs: []tx.Output{{Address: hash.Hash{1}, Amount: p.Supply}}}}
	for _, w := range []int{1, 4} {
		if _, _, err := entry.BuildWorkers(ctx, s, s.PoolID(1), txs, 0, w); !errors.Is(err, context.Canceled) {
			t.Fatalf("workers %d: %v", w, err)
		}
	}
}
