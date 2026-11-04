package rpc_test

import (
	"fmt"
	"math/rand/v2"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/peers"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/rpc"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/wire"
)

// ORANGE-034: a counts-only peers reply counts the book in place
// (peers.Book.Counts). Through every kind of change its counts are the
// ones rpc.CountBook gives from a copy of the whole book.
func TestBookCountsInPlace(t *testing.T) {
	s1 := wire.SeriesSection(hash.Zero, hash.Hash{1})
	s2 := wire.SeriesSection(hash.Zero, hash.Hash{2})
	now := time.Unix(1_800_000_000, 0)
	b, err := peers.Open(filepath.Join(t.TempDir(), "peers.json"), peers.Limits{MaxRecords: 300, PerRange: 6, VerifiedPerRange: 2, MaxUnlisted: 80})
	if err != nil {
		t.Fatal(err)
	}
	rnd := rand.New(rand.NewPCG(3, 4))
	addr := func() string { return fmt.Sprintf("%d.%d.1.%d:7000", 11+rnd.IntN(3), rnd.IntN(30), rnd.IntN(20)) }
	check := func() {
		t.Helper()
		want := rpc.CountBook(b.Records(), 0)
		n, states := b.Counts()
		got := rpc.BookCounts{Addresses: n, States: map[string]int{}}
		for _, st := range peers.States {
			got.States[st.String()] = states[st]
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("in place %+v, from a copy %+v", got, want)
		}
	}
	for i := range 4000 {
		now = now.Add(time.Second)
		a := addr()
		switch rnd.IntN(9) {
		case 0, 1:
			b.Add(a, []wire.Section{s1, s2}, peers.FromPeerList, now)
		case 2, 3:
			b.AddUnlisted(a, []wire.Section{s1}, now)
		case 4:
			b.Delete(a)
		case 5:
			b.Verify(a, s1, peers.Priority, now)
		case 6:
			b.Verify(a, s2, peers.Backup, now)
		case 7:
			b.Archive(a)
		default:
			b.PromoteImported(s1, 2)
		}
		if i%100 == 0 {
			check()
		}
	}
	check()
}
