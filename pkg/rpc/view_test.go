package rpc_test

import (
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/engine"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/filter"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/rpc"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/testutil"
)

func TestViews(t *testing.T) {
	f := newFixture(t)
	s, k := f.s, f.k
	c4 := k.Claim(4, P("alice", 100))
	_, err := f.c.SubmitTx(c4)
	must(t, err)
	_, err = f.c.Build(f.pool, 0)
	must(t, err)
	st, _ := f.n.Engine().Status(engine.PoolKey{Series: s.ID(), Index: 1})
	_, err = f.c.Work(f.pool, st.Branches[0].Tip, 0, 100)
	must(t, err)
	spend := k.Spend(4, []testutil.In{k.Out(c4, 0, "alice")}, P("bob", 40), P("alice2", 60))
	_, err = f.c.SubmitTx(spend)
	must(t, err)

	pools, err := f.c.Pools(filter.Filter{})
	must(t, err)
	if len(pools) != 1 || pools[0].Height != 1 || pools[0].Pending != 1 {
		t.Fatalf("pools %+v", pools)
	}
	lp, err := f.c.Ledger(nil, 1, 0, 0, filter.Filter{})
	must(t, err)
	if len(lp.Entries) != 1 || lp.Entries[0].Txs[0].TID != c4.TID(s.Hash()) {
		t.Fatalf("ledger %+v", lp)
	}
	var b filter.Builder
	must(t, b.Add("address", k.Addr("bob").String()))
	pend, err := f.c.Pending(b.Filter())
	must(t, err)
	if len(pend) != 1 || pend[0].TID != spend.TID(s.Hash()) {
		t.Fatalf("pending %+v", pend)
	}
	ts, err := f.c.Tx(nil, c4.TID(s.Hash()), nil)
	must(t, err)
	if !ts.Selected || ts.Height != 1 || ts.Tx.Raw == "" {
		t.Fatalf("tx %+v", ts)
	}
	a, err := f.c.Address(nil, k.Addr("alice"), 0, filter.Filter{})
	must(t, err)
	if a.Balance != 100 || len(a.History) != 1 || len(a.Pending) != 1 {
		t.Fatalf("address %+v", a)
	}
	// RED-008: below pages the history; below the only entry there is none.
	if a, err := f.c.AddressPage(nil, k.Addr("alice"), 1, 1, filter.Filter{}); err != nil || len(a.History) != 1 || len(a.Next) != 0 {
		t.Fatalf("address from height 1: %+v %v", a, err)
	}
	un, err := f.c.Unspent(filter.Filter{})
	must(t, err)
	if len(un) != 1 || un[0].Amount != 100 {
		t.Fatalf("unspent %+v", un)
	}

	// Errors.
	if _, err := f.c.Tx(nil, hash.Hash{1}, nil); code(err) != rpc.CodeNotFound {
		t.Fatalf("unknown tx: %v", err)
	}
	if _, err := f.c.Ledger(nil, 0, 0, 0, filter.Filter{}); code(err) != rpc.CodeNotFound {
		t.Fatalf("unfollowed pool: %v", err)
	}
	other := hash.Hash{2}
	if _, err := f.c.Ledger(&other, 1, 0, 0, filter.Filter{}); code(err) != rpc.CodeNotFound {
		t.Fatalf("unknown series: %v", err)
	}
	tok, _ := rpc.LoadOrCreateToken(f.dir)
	if raw(t, f, "GET", "/v1/pending?kind=nope", tok, "") != 400 {
		t.Fatal("bad filter accepted")
	}
	// Without a hub or a reload hook these say so.
	if err := f.c.Reload(); code(err) != rpc.CodeNotFound {
		t.Fatalf("reload: %v", err)
	}
	if err := f.c.Watch(t.Context(), filter.Filter{}, nil); code(err) != rpc.CodeNotFound {
		t.Fatalf("watch: %v", err)
	}
	id := s.ID()
	fr, err := f.c.Follow(rpc.FollowRequest{Series: &id, NoReload: true})
	must(t, err)
	if fr.Reloading || len(fr.Follow) != 1 || len(fr.Follow[0].Pools) != 0 {
		t.Fatalf("follow without reload %+v", fr)
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
