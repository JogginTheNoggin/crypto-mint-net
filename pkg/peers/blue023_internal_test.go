package peers

import (
	"fmt"
	"testing"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/wire"
)

// D160: the queues of records that give way stay within a small multiple
// of the book however records come, change and go, and agree with a scan.
func TestGiveWayQueuesStayBounded(t *testing.T) {
	b, err := Open("", Limits{MaxRecords: 64, PerRange: 8, MaxUnlisted: 16, CandidateAge: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	s := []wire.Section{wire.SeriesSection(hash.Zero, hash.Hash{1})}
	t0 := time.Unix(1_800_000_000, 0)
	for i := 0; i < 20000; i++ {
		now := t0.Add(time.Duration(i) * time.Minute)
		a := fmt.Sprintf("%d.%d.0.1:1", 20+i%50, i%200)
		switch i % 5 {
		case 0:
			_ = b.AddUnlisted(a, s, now)
		case 1:
			_ = b.Add(a, s, FromHand, now)
		default:
			_ = b.Add(a, s, FromPeerList, now)
		}
		b.Tried(a, i%3 == 0, now)
		if i%7 == 0 {
			b.Delete(fmt.Sprintf("%d.%d.0.1:1", 20+(i/2)%50, (i/2)%200))
		}
		if i%11 == 0 {
			_, _ = b.SetLabel(a, "x")
			_, _ = b.SetLabel(a, "")
		}
		b.mu.Lock()
		for name, q := range map[string]*queue{"imported": &b.impQ, "learned": &b.learnedQ, "failed": &b.failedQ} {
			if len(q.h) > 2*len(b.recs)+65 {
				b.mu.Unlock()
				t.Fatalf("step %d: the %s queue holds %d for a book of %d", i, name, len(q.h), len(b.recs))
			}
		}
		for _, r := range b.recs {
			for _, c := range []struct {
				q  *queue
				in bool
			}{{&b.impQ, r.imported()}, {&b.learnedQ, r.learned()}, {&b.failedQ, r.learned() && r.Failures > 0}} {
				if c.in && !contains23(c.q.h, r) {
					b.mu.Unlock()
					t.Fatalf("step %d: %s is not on its queue", i, r.Addr)
				}
			}
		}
		b.mu.Unlock()
		if i%997 == 0 {
			checkCounts(t, b)
		}
	}
}

func contains23(h []*Record, r *Record) bool {
	for _, x := range h {
		if x == r {
			return true
		}
	}
	return false
}
