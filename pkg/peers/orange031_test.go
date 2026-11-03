package peers

import (
	"fmt"
	"testing"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/wire"
)

// ORANGE-031: the range cache counts ranges as Range does, through
// deletes and a book filled to its bound.
func TestRangeCacheCounts(t *testing.T) {
	sec := []wire.Section{wire.SeriesSection(hash.Zero, hash.Hash{1})}
	now := time.Unix(1_800_000_000, 0)
	b, err := Open("", DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	lim := b.Limits()
	addr := func(r, i int) string { return fmt.Sprintf("%d.%d.%d.1:7000", 11+r/256, r%256, i) }
	for r := range lim.MaxRecords / lim.PerRange {
		for i := range lim.PerRange {
			if err := b.Add(addr(r, i), sec, FromImport, now); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := b.Add("9.9.9.9:1", sec, FromImport, now); err != ErrFull {
		t.Fatalf("a full book: %v", err)
	}
	b.Delete(addr(0, 0))
	b.Delete(addr(1, 0))
	if err := b.Add(addr(0, lim.PerRange), sec, FromImport, now); err != nil {
		t.Fatalf("room in range 0 after a delete: %v", err)
	}
	if err := b.Add(addr(0, lim.PerRange+1), sec, FromImport, now); err != ErrRangeFull {
		t.Fatalf("range 0 full again: %v", err)
	}
	b.mu.Lock()
	for a, r := range b.rng {
		if r != Range(a) {
			t.Errorf("cache: %s → %s, Range %s", a, r, Range(a))
		}
	}
	b.mu.Unlock()
}
