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

// Black's B4 (D132), in the P0 tests: a sender that shows the same work
// as an honest one (it copied the heavier ledger) claims a huge large
// entry and keeps trickling its bytes, so its parts file holds the disk
// at its reserve. An honest sender of the same work whose entry needs
// room must still get it: the claim with more still to come ranks below,
// so its parts give way. Before D132 the file holding more bytes kept
// the room, and the honest fetch waited for as long as the trickle went
// on: the heavier ledger was never taken.
func TestTricklerCannotHoldTheReserve(t *testing.T) {
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
		c.ChaseSlice = 200 * time.Millisecond
		c.Chase.YieldWait = 50 * time.Millisecond
		c.PartsDir = dir
	})
	// The trickler goes first, and never stops sending.
	tr := startLarge(t, a)
	rand.Read(tr.eh[:])
	var done atomic.Bool
	var sent atomic.Int64
	var wg sync.WaitGroup
	started := make(chan struct{})
	wg.Add(1)
	go func() {
		defer wg.Done()
		tr.serve(t, time.Now().Add(time.Hour), func() bool {
			sent.Store(int64(len(tr.offsets)))
			if len(tr.offsets) == 4 {
				select {
				case <-started:
				default:
					close(started)
				}
			}
			return done.Load()
		})
	}()
	t.Cleanup(func() { done.Store(true); wg.Wait() })
	select {
	case <-started:
	case <-time.After(20 * time.Second):
		t.Fatal("the trickler's parts were never asked for")
	}
	// The honest sender shows the same work and an entry of 16 parts.
	const parts = 16
	hs := startLarge(t, a)
	hs.eh, hs.size = tr.eh, parts*64<<10
	hs.serve(t, time.Now().Add(20*time.Second), func() bool {
		return len(hs.offsets) > 0 && hs.offsets[len(hs.offsets)-1] == hs.size-64<<10
	})
	if sent.Load() <= 4 {
		t.Fatal("setup: the trickler stopped sending")
	}
}
