package watch_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/engine"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/testutil"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/watch"
)

// GREEN-014: an address whose history spans two pools, read a page at a
// time by following Next, must yield every item exactly once.
func TestAddressPagingAcrossPools(t *testing.T) {
	f := newFixture(t)
	n, s, k := f.n, f.s, f.k
	const perPool = 3
	for _, serial := range []uint64{0, 4} { // pools 0 and 1
		key := engine.PoolKey{Series: s.ID(), Index: s.PoolOf(serial)}
		prev := k.Claim(serial, testutil.P("alice", 100))
		for i := 0; i < perPool; i++ {
			if i > 0 {
				prev = k.Spend(serial, []testutil.In{k.Out(prev, 0, "alice")}, testutil.P("alice", 100))
			}
			_, err := n.HandleTx(prev, engine.Local)
			must(t, err)
			e, _, err := n.Build(key, 0)
			must(t, err)
			_, _, err = n.Work(key, e.Hash(s), 0, 20)
			must(t, err)
		}
	}
	eng := n.Engine()
	seen := map[string]int{}
	add := func(a watch.Address) {
		for _, h := range a.History {
			seen[fmt.Sprintf("%d/%d/%s", h.Pool, h.Height, h.Tx.TID)]++
		}
	}
	first, err := watch.AddressPage(context.Background(), eng, s.ID(), k.Addr("alice"), parse(t), 0, 2)
	must(t, err)
	add(first)
	for _, nx := range first.Next {
		below := nx.Below
		for below > 0 {
			a, err := watch.AddressPage(context.Background(), eng, s.ID(), k.Addr("alice"), parse(t, "pool", fmt.Sprint(nx.Pool)), below, 2)
			must(t, err)
			add(a)
			below = 0
			for _, m := range a.Next {
				below = m.Below
			}
		}
	}
	if len(seen) != 2*perPool {
		t.Errorf("following Next reached %d of %d history items: %v", len(seen), 2*perPool, seen)
	}
	for it, c := range seen {
		if c != 1 {
			t.Errorf("%s seen %d times", it, c)
		}
	}
}
