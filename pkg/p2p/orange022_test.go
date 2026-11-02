package p2p_test

import (
	"crypto/rand"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/p2p"
)

// ORANGE-022: two honest fetches of different large entries, both slow
// (each chase takes in one part, then a request is lost), with room above
// the reserve for about one of them. Under D137 each turn deleted the
// other's parts file, so neither ever reached its end. The fetch that
// would finish first now keeps the room, and both reach their end.
func TestTwoSlowFetchesBothFinish(t *testing.T) {
	if testing.Short() || raceOn {
		t.Skip("long: run without -short and -race")
	}
	const total = 100 << 20
	const room = 1536 << 10 // above the reserve: one entry and a half
	restore := p2p.SetDiskSpace(func(dir string) (uint64, uint64, error) {
		es, err := os.ReadDir(dir)
		if err != nil {
			return 0, 0, err
		}
		var used uint64
		for _, e := range es {
			if fi, err := e.Info(); err == nil {
				used += uint64(fi.Size())
			}
		}
		free := uint64(total/50 + room)
		if used >= free {
			return 0, total, nil
		}
		return free - used, total, nil
	})
	t.Cleanup(restore)
	dir := t.TempDir()
	a := startNode(t, "a", func(c *p2p.Config) {
		c.RequestTimeout = 300 * time.Millisecond
		c.Chase.Backoff = 500 * time.Millisecond
		c.Chase.YieldWait = 50 * time.Millisecond
		c.PartsDir = dir
		c.PartsReserve = 1
	})
	const parts = 16
	var done atomic.Bool
	var reached [2]atomic.Bool
	var wg sync.WaitGroup
	ss := make([]*largeSender, 2)
	for i := range ss {
		s := startLarge(t, a)
		rand.Read(s.eh[:])
		s.size = parts * 64 << 10
		s.skip = func(n int) bool { return n%2 == 1 }
		ss[i] = s
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.serve(t, time.Now().Add(time.Hour), func() bool {
				if n := len(s.offsets); n > 0 && s.offsets[n-1] == s.size-64<<10 {
					reached[i].Store(true)
				}
				return done.Load()
			})
		}()
		time.Sleep(200 * time.Millisecond)
	}
	t.Cleanup(func() { done.Store(true); wg.Wait() })
	until := time.Now().Add(2 * time.Minute)
	for !reached[0].Load() || !reached[1].Load() {
		if time.Now().After(until) {
			t.Fatalf("a fetch never reached its end: parts asked %d and %d (restarts from 0: %d and %d)",
				len(ss[0].offsets), len(ss[1].offsets), restarts(ss[0].offsets), restarts(ss[1].offsets))
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func restarts(offsets []uint64) int {
	n := 0
	for i, o := range offsets {
		if o == 0 && i > 0 {
			n++
		}
	}
	return n
}
