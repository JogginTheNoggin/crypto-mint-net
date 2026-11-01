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

// RED-029 (D136): a sender that claims a size just under the honest
// entry's, so D132's tie-break ranks it above, and then sends one part a
// chase and lets the next request time out, is slow: its parts give way
// to the honest fetch at the reserve, whatever size it claims.
func TestTricklerJustUnderCannotHoldTheReserve(t *testing.T) {
	const disk = 1 << 50
	restore := p2p.SetDiskSpace(func(dir string) (uint64, uint64, error) {
		es, err := os.ReadDir(dir)
		if err != nil {
			return 0, 0, err
		}
		if len(es) >= 2 {
			return disk / 100, disk, nil // at the reserve while two parts files exist
		}
		return disk, disk, nil
	})
	t.Cleanup(restore)
	dir := t.TempDir()
	a := startNode(t, "a", func(c *p2p.Config) {
		c.RequestTimeout = 300 * time.Millisecond
		c.Chase.Backoff = 50 * time.Millisecond
		c.Chase.YieldWait = 50 * time.Millisecond
		c.PartsDir = dir
	})
	const honest = 2001 // parts of 64 KiB
	tr := startLarge(t, a)
	rand.Read(tr.eh[:])
	tr.size = (honest - 1) * 64 << 10
	tr.skip = func(n int) bool { return n%2 == 1 } // one part, then a time-out
	var done atomic.Bool
	var asked atomic.Int64
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		tr.serve(t, time.Now().Add(time.Hour), func() bool {
			asked.Store(int64(len(tr.offsets)))
			return done.Load()
		})
	}()
	t.Cleanup(func() { done.Store(true); wg.Wait() })
	until := time.Now().Add(20 * time.Second)
	for asked.Load() < 8 {
		if time.Now().After(until) {
			t.Fatal("setup: the trickler was not asked for parts")
		}
		time.Sleep(10 * time.Millisecond)
	}
	hs := startLarge(t, a)
	hs.eh, hs.size = tr.eh, honest*64<<10
	hs.serve(t, time.Now().Add(15*time.Second), func() bool { return len(hs.offsets) >= 16 })
}
