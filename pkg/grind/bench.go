package grind

import (
	"context"
	"crypto/rand"
	"sync/atomic"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/layer"
)

// Bench is what a benchmark measured for one plan.
type Bench struct {
	Plan    Plan    `json:"plan"`
	Tries   uint64  `json:"tries"`
	Seconds float64 `json:"seconds"`
	Rate    float64 `json:"rate"` // tries a second
}

// RunBench grinds layers on a random entry with plan p for d, or until ctx
// ends, and reports the rate. A layer try and an entry try cost the same
// once the entry's prefix is saved (ORANGE-006), so the rate holds for
// both.
func RunBench(ctx context.Context, p Plan, d time.Duration) (Bench, error) {
	var entry hash.Hash
	_, _ = rand.Read(entry[:])
	ctx, cancel := context.WithTimeout(ctx, d)
	defer cancel()
	var tried atomic.Uint64
	t0 := time.Now()
	var err error
	if p.search != nil {
		_, _, err = layer.GrindSearch(ctx, p.search, entry, 0, 0, p.Threads, &tried)
	} else {
		_, _, err = layer.GrindWith(ctx, p.newHasher, entry, 0, 0, p.Threads, &tried)
	}
	secs := time.Since(t0).Seconds()
	if err != nil && ctx.Err() == nil {
		return Bench{}, err
	}
	n := tried.Load()
	return Bench{Plan: p, Tries: n, Seconds: secs, Rate: float64(n) / max(secs, 1e-9)}, nil
}
