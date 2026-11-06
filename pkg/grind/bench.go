package grind

import (
	"context"
	"crypto/rand"
	"math"
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
	// MinuteStrength is the strength this plan finds in about a minute on
	// average: log2(rate × 60), rounded down. Each bit more doubles the
	// time; it is where to start bounty.max_strength (GREEN-064).
	MinuteStrength int `json:"minute_strength,omitempty"`
}

// minuteStrength is log2(rate × 60), rounded down, or 0 for no rate.
func minuteStrength(rate float64) int {
	if rate*60 < 2 {
		return 0
	}
	return int(math.Floor(math.Log2(rate * 60)))
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
		_, _, err = layer.GrindSearchContext(ctx, p.search, entry, 0, 0, p.Threads, &tried, nil, p.chunk)
	} else {
		_, _, err = layer.GrindWith(ctx, p.newHasher, entry, 0, 0, p.Threads, &tried)
	}
	secs := time.Since(t0).Seconds()
	if err != nil && ctx.Err() == nil {
		return Bench{}, err
	}
	n := tried.Load()
	rate := float64(n) / max(secs, 1e-9)
	return Bench{Plan: p, Tries: n, Seconds: secs, Rate: rate, MinuteStrength: minuteStrength(rate)}, nil
}
