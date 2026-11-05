package mempool_test

import (
	"math/rand"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tx"
)

// TestOrderKeptWithoutSorting checks that Ready and Revalidate still see
// the pool oldest first, as inserted, through removals and re-insertions
// (ORANGE-039: the pool keeps its order instead of sorting each call).
func TestOrderKeptWithoutSorting(t *testing.T) {
	k, lp, mp, _ := setup(t)
	v := view{lp}
	var claims []*tx.Tx
	for s := uint64(0); s < 8; s++ {
		if s != 4 { // setup claimed mint 4
			claims = append(claims, k.Claim(s, P("a", 100)))
		}
	}
	r := rand.New(rand.NewSource(39))
	var model []hash.Hash // what the pool holds, oldest first
	held := map[hash.Hash]bool{}
	for step := 0; step < 2000; step++ {
		c := claims[r.Intn(len(claims))]
		id := tid(c)
		if held[id] {
			mp.Remove(id)
			delete(held, id)
			for i, m := range model {
				if m == id {
					model = append(model[:i], model[i+1:]...)
					break
				}
			}
		} else {
			if err := mp.Add(v, c, id); err != nil {
				t.Fatal(err)
			}
			held[id] = true
			model = append(model, id)
		}
		if step%7 == 0 {
			if d := mp.Revalidate(v); len(d) != 0 {
				t.Fatalf("step %d: dropped %v", step, d)
			}
		}
		got := mp.Ready()
		if len(got) != len(model) || mp.Len() != len(model) {
			t.Fatalf("step %d: ready %d, len %d, want %d", step, len(got), mp.Len(), len(model))
		}
		for i := range got {
			if got[i].TID != model[i] {
				t.Fatalf("step %d: position %d out of order", step, i)
			}
		}
	}
}
