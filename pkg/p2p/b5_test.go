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

// Black R11 (B5): an honest sender whose one parts request timed out is
// marked slow and, while waiting for room, never asks again, so the flag
// never clears. A trickler (also slow) with less still to come then holds
// the reserve against it for as long as it trickles.
func TestBlackSlowHonestCannotReclaimFromTrickler(t *testing.T) {
	const disk = 1 << 50
	restore := p2p.SetDiskSpace(func(dir string) (uint64, uint64, error) {
		es, err := os.ReadDir(dir)
		if err != nil {
			return 0, 0, err
		}
		if len(es) >= 2 {
			return disk / 100, disk, nil
		}
		return disk, disk, nil
	})
	t.Cleanup(restore)
	dir := t.TempDir()
	a := startNode(t, "a", func(c *p2p.Config) {
		c.RequestTimeout = 300 * time.Millisecond
		c.Chase.Backoff = 2 * time.Second
		c.Chase.YieldWait = 50 * time.Millisecond
		c.PartsDir = dir
	})
	const parts = 64
	hs := startLarge(t, a)
	rand.Read(hs.eh[:])
	hs.size = parts * 64 << 10
	hs.skip = func(n int) bool { return n == 0 } // one lost request
	var done, reached atomic.Bool
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		hs.serve(t, time.Now().Add(time.Hour), func() bool {
			if len(hs.offsets) > 0 && hs.offsets[len(hs.offsets)-1] == hs.size-64<<10 {
				reached.Store(true)
			}
			return done.Load() || reached.Load()
		})
	}()
	t.Cleanup(func() { done.Store(true); wg.Wait() })
	time.Sleep(700 * time.Millisecond) // the honest request times out, the chase stalls
	tr := startLarge(t, a)
	tr.eh, tr.size = hs.eh, hs.size
	tr.skip = func(n int) bool { return n%2 == 1 } // one part a chase, then a time-out
	wg.Add(1)
	go func() {
		defer wg.Done()
		tr.serve(t, time.Now().Add(time.Hour), func() bool { return done.Load() })
	}()
	until := time.Now().Add(25 * time.Second)
	for !reached.Load() {
		if time.Now().After(until) {
			t.Fatalf("honest fetch never reached its end: honest asked %d parts, trickler %d", len(hs.offsets), len(tr.offsets))
		}
		time.Sleep(50 * time.Millisecond)
	}
}
