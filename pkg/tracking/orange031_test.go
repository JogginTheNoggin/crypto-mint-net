package tracking

import (
	"fmt"
	"testing"
	"time"
)

// ORANGE-031: when blocked records fill the bound, a new address does
// not scan every record again; the next trim waits for room to be made.
// Once the blocks end, the records go back under the bound.
func TestTrimWhenBlocksFillTheBound(t *testing.T) {
	const max = 256
	tr, err := New(Config{Retain: Retention{MaxAddresses: max}}, "")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_800_000_000, 0)
	addr := func(i int) string { return fmt.Sprintf("10.%d.%d.%d", i>>16&255, i>>8&255, i&255) }
	const blocked = 4 * max
	for i := range blocked {
		tr.BlockFor(addr(i), time.Hour, now)
	}
	tr.mu.Lock()
	n, at := len(tr.recs), tr.trimAt
	tr.mu.Unlock()
	if n != blocked {
		t.Fatalf("%d records kept, want every block (%d)", n, blocked)
	}
	if at <= n {
		t.Fatalf("trimAt %d: the next new address would scan all %d records", at, n)
	}
	// The blocks end; new addresses bring the records back under the bound.
	later := now.Add(2 * time.Hour)
	for i := range max {
		tr.Heard(addr(blocked+i), "", later)
	}
	tr.mu.Lock()
	n = len(tr.recs)
	tr.mu.Unlock()
	if n > max {
		t.Fatalf("%d records after the blocks ended, bound %d", n, max)
	}
	for i := range blocked {
		if tr.Status(addr(i), later).Blocked {
			t.Fatalf("%s still blocked", addr(i))
		}
	}
}
