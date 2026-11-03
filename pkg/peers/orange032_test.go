package peers

import (
	"fmt"
	"math/rand/v2"
	"path/filepath"
	"testing"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/wire"
)

// checkCounts compares the book's kept counts with a scan of the book.
func checkCounts(t *testing.T, b *Book) {
	t.Helper()
	b.mu.Lock()
	defer b.mu.Unlock()
	per := map[string]int{}
	imp := 0
	for a, r := range b.recs {
		per[Range(a)]++
		if r.imported() {
			imp++
			if b.imp[a] != r {
				t.Fatalf("%s is imported but not counted", a)
			}
		}
	}
	if len(per) != len(b.perRange) {
		t.Fatalf("%d ranges counted, %d in the book", len(b.perRange), len(per))
	}
	for rng, n := range per {
		if len(b.perRange[rng]) != n {
			t.Fatalf("range %s: counted %d, holds %d", rng, len(b.perRange[rng]), n)
		}
	}
	if imp != len(b.imp) {
		t.Fatalf("%d imported counted, %d in the book", len(b.imp), imp)
	}
}

// ORANGE-032: the per-range counts and the imported records the book
// keeps match a scan of the book through every kind of change, and after
// a save and a load.
func TestBookCountsKept(t *testing.T) {
	s1 := wire.SeriesSection(hash.Zero, hash.Hash{1})
	s2 := wire.SeriesSection(hash.Zero, hash.Hash{2})
	now := time.Unix(1_800_000_000, 0)
	path := filepath.Join(t.TempDir(), "peers.json")
	b, err := Open(path, Limits{MaxRecords: 300, PerRange: 6, VerifiedPerRange: 2, MaxUnlisted: 80})
	if err != nil {
		t.Fatal(err)
	}
	rnd := rand.New(rand.NewPCG(1, 2))
	addr := func() string { return fmt.Sprintf("%d.%d.1.%d:7000", 11+rnd.IntN(3), rnd.IntN(30), rnd.IntN(20)) }
	secs := func() []wire.Section {
		if rnd.IntN(2) == 0 {
			return []wire.Section{s1}
		}
		return []wire.Section{s1, s2}
	}
	for i := range 5000 {
		now = now.Add(time.Second)
		a := addr()
		switch rnd.IntN(10) {
		case 0, 1, 2:
			b.Add(a, secs(), FromPeerList, now)
		case 3, 4, 5:
			b.AddUnlisted(a, secs(), now)
		case 6:
			b.Delete(a)
		case 7:
			b.Verify(a, s1, Priority, now)
		case 8:
			b.Archive(a)
		default:
			switch rnd.IntN(4) {
			case 0:
				b.PromoteImported(s2, 3)
			case 1:
				b.Unarchive(s1)
			case 2:
				b.Unverify(a)
			default:
				if rnd.IntN(20) == 0 {
					b.ForgetImported()
				}
			}
		}
		if i%250 == 0 {
			checkCounts(t, b)
		}
	}
	checkCounts(t, b)
	b.dirty = true
	if err := b.Save(); err != nil {
		t.Fatal(err)
	}
	c, err := Open(path, b.Limits())
	if err != nil {
		t.Fatal(err)
	}
	checkCounts(t, c)
	if len(c.recs) != len(b.recs) || len(c.imp) != len(b.imp) {
		t.Fatalf("loaded %d records, %d imported; saved %d, %d", len(c.recs), len(c.imp), len(b.recs), len(b.imp))
	}
}
