package engine_test

import (
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/engine"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/series"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/testutil"
)

// RED-046, D152: a series of very many pools followed whole is listed by
// the pools in use, while every pool stays followed; a small one is listed
// whole, and pools followed by index are always listed.
func TestListedPoolsOfAHugeSeries(t *testing.T) {
	p := testutil.ExampleParams()
	p.PoolSize, p.Mints = 1, 1<<40
	huge, err := series.New(p)
	if err != nil {
		t.Fatal(err)
	}
	small := testutil.ExampleSeries()
	q := testutil.ExampleParams()
	q.Mints = 1 << 20
	part, err := series.New(q)
	if err != nil {
		t.Fatal(err)
	}
	e := engine.New(engine.DefaultConfig())
	for _, s := range []*series.Series{huge, small, part} {
		e.AddSeries(s)
	}
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(e.FollowSeries(huge.ID()))
	must(e.FollowSeries(small.ID()))
	must(e.FollowPool(part.ID(), 7))

	if n := e.FollowedCount(); n != huge.PoolCount()+small.PoolCount()+1 {
		t.Fatalf("followed count %d", n)
	}
	if ids := e.FollowedSeries(); len(ids) != 3 {
		t.Fatalf("followed series %v", ids)
	}
	count := func(id engine.PoolKey) (n int, has bool) {
		for _, k := range e.ListedPools() {
			if k.Series == id.Series {
				n++
				has = has || k == id
			}
		}
		return
	}
	if n, _ := count(engine.PoolKey{Series: huge.ID()}); n != 0 {
		t.Fatalf("huge series listed %d pools before any was used", n)
	}
	if n, _ := count(engine.PoolKey{Series: small.ID()}); uint64(n) != small.PoolCount() {
		t.Fatalf("small series listed %d of %d pools", n, small.PoolCount())
	}
	if n, has := count(engine.PoolKey{Series: part.ID(), Index: 7}); n != 1 || !has {
		t.Fatalf("pool 7 of part listed: %d %v", n, has)
	}
	k := engine.PoolKey{Series: huge.ID(), Index: 1 << 39}
	if !e.Follows(k) {
		t.Fatal("a pool of the huge series is not followed")
	}
	if _, err := e.Status(k); err != nil {
		t.Fatal(err)
	}
	if n, has := count(k); n != 1 || !has {
		t.Fatalf("used pool listed: %d %v", n, has)
	}
}
