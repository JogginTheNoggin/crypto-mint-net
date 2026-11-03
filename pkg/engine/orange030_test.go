package engine

import (
	"errors"
	"reflect"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/series"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/testutil"
)

// ORANGE-030: Status and Tip of a followed pool with no state answer what
// a fresh state reads, make no state, and refuse what peek refuses.
func TestUnstatedReadsAsFresh(t *testing.T) {
	p := testutil.ExampleParams()
	p.PoolSize, p.Mints = 1, 1000
	s, err := series.New(p)
	if err != nil {
		t.Fatal(err)
	}
	e := New(DefaultConfig())
	e.AddSeries(s)
	if err := e.FollowSeriesExcept(s.ID(), []uint64{7}); err != nil {
		t.Fatal(err)
	}
	for _, i := range []uint64{0, 1, 500, 999} {
		k := PoolKey{Series: s.ID(), Index: i}
		got, err := e.Status(k)
		if err != nil {
			t.Fatal(err)
		}
		e.mu.Lock()
		fresh, created, err := e.poolFor(k)
		e.mu.Unlock()
		if err != nil || !created {
			t.Fatalf("pool %d: fresh state: %v, created %v", i, err, created)
		}
		tip, h, w := fresh.ledger.Tip()
		want := PoolStatus{Key: k, Tip: tip, Height: h, Weight: w, Entries: fresh.ledger.Len(),
			Branches: fresh.ledger.Branches(), Pending: fresh.pending.Len(), Waiting: fresh.pending.Waiting()}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("pool %d: status %+v, a fresh state reads %+v", i, got, want)
		}
		gt, gh, gw, err := e.Tip(k)
		if err != nil || gt != tip || gh != h || gw != w {
			t.Fatalf("pool %d: tip %v %d %v %v, want %v %d %v", i, gt, gh, gw, err, tip, h, w)
		}
	}
	if n := len(e.pools); n != 0 {
		t.Fatalf("%d states made by reads", n)
	}
	if _, err := e.Status(PoolKey{Series: s.ID(), Index: 7}); !errors.Is(err, ErrNotFollowed) {
		t.Fatalf("a pool left out: %v", err)
	}
	if _, _, _, err := e.Tip(PoolKey{Series: hash.Hash{1}, Index: 0}); !errors.Is(err, ErrUnknownSeries) {
		t.Fatalf("an unknown series: %v", err)
	}
}
