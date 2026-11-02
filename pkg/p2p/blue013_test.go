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

// RED-031 (D139): a sender that claims a size just under the honest
// entry's and takes in 1 MiB a chase, then lets a request time out, was
// "moving" under D137 and ranked above every honest fetch for good. Room
// now goes to whoever would finish first at the rate it delivers, so a
// faster honest fetch takes it, a part at a time as its parts arrive.
func TestMovingTricklerJustUnderCannotHoldTheReserve(t *testing.T) {
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
		c.RequestPace = 1 << 20 // the test's speed is not paced
	})
	const honest = 2001 // parts of 64 KiB
	tr := startLarge(t, a)
	rand.Read(tr.eh[:])
	tr.size = (honest - 1) * 64 << 10
	tr.skip = func(n int) bool { return n%25 == 24 } // 1.5 MiB, then a time-out
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
	time.Sleep(1500 * time.Millisecond)
	for asked.Load() < 40 {
		if time.Now().After(until) {
			t.Fatal("setup: the trickler was not asked for parts")
		}
		time.Sleep(10 * time.Millisecond)
	}
	hs := startLarge(t, a)
	hs.eh, hs.size = tr.eh, honest*64<<10
	hs.serve(t, time.Now().Add(15*time.Second), func() bool { return len(hs.offsets) >= 64 })
}
