package node_test

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/filter"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/ledger"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/testutil"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/watch"
)

// RED-084: with the pool log damaged under the node and every body read
// back from it, queries say what they could not read instead of
// answering as if complete, and a payment's rivals are still found.
func TestDamagedLogQueriesSayIncomplete(t *testing.T) {
	f := newFixture(t)
	n := f.open()
	defer n.Close()
	f.history(n)
	c4 := f.k.Claim(4, P("alice", 100))
	spend := f.k.Spend(4, []testutil.In{f.k.Out(c4, 0, "alice")}, P("dave", 30), P("alice2", 70))
	tid := spend.TID(f.s.Hash())

	p, err := n.Engine().Payment(f.key, tid)
	if err != nil || !p.Selected || p.Tx == nil || len(p.Rivals) != 1 || p.Unreadable {
		t.Fatalf("before: %+v, %v", p, err)
	}

	n.BodyCache().SetMax(0)
	n.Engine().Ledger(f.key, func(p *ledger.Pool) { p.ReleaseBodies() })
	path := filepath.Join(f.dir, "pools", f.key.Series.String(), strconv.FormatUint(f.key.Index, 10)+".log")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for i := 40; i < len(b); i += 50 {
		b[i] ^= 0xff
	}
	fh, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fh.WriteAt(b, 0); err != nil {
		t.Fatal(err)
	}
	fh.Close()

	p, err = n.Engine().Payment(f.key, tid)
	if err != nil || !p.Selected || p.Tx != nil || !p.Unreadable {
		t.Fatalf("after: %+v, %v", p, err)
	}
	if len(p.Rivals) != 1 {
		t.Fatalf("rivals %d, want 1 from the skeleton", len(p.Rivals))
	}

	page, err := watch.LedgerPage(n.Engine(), f.key, filter.Filter{}, 0, 50)
	if err != nil {
		t.Fatal(err)
	}
	if page.Unreadable != 2 || len(page.Entries) != 2 || !page.Entries[0].Unreadable {
		t.Fatalf("ledger page: unreadable %d, entries %+v", page.Unreadable, page.Entries)
	}
	addr, err := watch.AddressPage(context.Background(), n.Engine(), f.s.ID(), f.k.Addr("dave"), filter.Filter{}, 0, 50)
	if err != nil {
		t.Fatal(err)
	}
	if addr.Unreadable == 0 || len(addr.History) != 0 {
		t.Fatalf("address page: unreadable %d, history %d", addr.Unreadable, len(addr.History))
	}
	if st := n.Bodies(); st.Errors == 0 {
		t.Fatalf("no read errors counted: %+v", st)
	}
}
