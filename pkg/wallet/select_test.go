package wallet

import (
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/testutil"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tx"
)

func sp(serial uint64, amounts ...uint64) []Spendable {
	var out []Spendable
	for i, a := range amounts {
		out = append(out, Spendable{Serial: serial, Portion: Portion{Input: tx.Input{TID: hash.Hash{byte(serial), byte(i)}}, Amount: a}})
	}
	return out
}

func cat(xs ...[]Spendable) []Spendable {
	var out []Spendable
	for _, x := range xs {
		out = append(out, x...)
	}
	return out
}

func TestChoose(t *testing.T) {
	s := testutil.ExampleSeries() // pools of 4 mints
	// The smallest single portion that covers the amount.
	ps, ok := choose(s, cat(sp(4, 10, 60, 40), sp(5, 35)), 35)
	if !ok || len(ps) != 1 || ps[0].serial != 5 || len(ps[0].inputs) != 1 || ps[0].total != 35 {
		t.Fatalf("single portion: %+v", ps)
	}
	// Merge and pay in one mint, largest first.
	ps, ok = choose(s, cat(sp(4, 10, 30, 20), sp(5, 15)), 45)
	if !ok || len(ps) != 1 || ps[0].serial != 4 || len(ps[0].inputs) != 2 || ps[0].total != 50 {
		t.Fatalf("merge: %+v", ps)
	}
	// Fewer inputs win over less change.
	ps, ok = choose(s, cat(sp(4, 20, 20), sp(5, 90)), 40)
	if !ok || ps[0].serial != 5 {
		t.Fatalf("fewest inputs: %+v", ps)
	}
	// No mint covers it: one pool (mints 4 and 5) before two pools, even
	// though mint 0 is largest.
	ps, ok = choose(s, cat(sp(0, 70), sp(4, 50), sp(5, 40)), 80)
	if !ok || len(ps) != 2 || s.PoolOf(ps[0].serial) != 1 || s.PoolOf(ps[1].serial) != 1 || ps[0].pay+ps[1].pay != 80 {
		t.Fatalf("one pool: %+v", ps)
	}
	// Only across pools.
	ps, ok = choose(s, cat(sp(0, 70), sp(4, 50)), 100)
	if !ok || len(ps) != 2 || ps[0].serial != 0 || ps[0].pay != 70 || ps[1].pay != 30 {
		t.Fatalf("across pools: %+v", ps)
	}
	if _, ok := choose(s, sp(4, 10), 11); ok {
		t.Fatal("paid more than held")
	}
}
