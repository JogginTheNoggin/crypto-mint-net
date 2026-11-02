package p2p_test

import (
	"crypto/rand"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/p2p"
)

// RED-035 (D142, with ORANGE-023): a fetch at the disk reserve that
// probes throws away the rest of the reply it is in, so the chunks after
// the one that found no room are never taken for parts out of order. An
// honest sender whose replies hold several parts is never flagged,
// whichever chunk of a reply the probe begins on: the first (no room at
// all), a middle one or the last.
func TestProbeNeverFlagsAMultiChunkSender(t *testing.T) {
	if testing.Short() {
		t.Skip("long: run without -short")
	}
	const part = 64 << 10
	const total = 100 << 20
	const holderParts, chunks = 64, 4
	for _, room := range []int{0, 1, chunks - 1} {
		t.Run(fmt.Sprintf("room for %d parts", room), func(t *testing.T) {
			var free atomic.Uint64
			free.Store(1 << 40)
			restore := p2p.SetDiskSpace(func(dir string) (uint64, uint64, error) {
				used := partsBytes(t, dir)
				if f := free.Load(); used < f {
					return total/50 + f - used, total, nil
				}
				return total / 50, total, nil
			})
			t.Cleanup(restore)
			dir := t.TempDir()
			a := startNode(t, "a", func(c *p2p.Config) {
				c.RequestTimeout = 300 * time.Millisecond
				c.Chase.Backoff = 20 * time.Second
				c.ChaseSlice = 2 * time.Second
				c.Chase.YieldWait = 50 * time.Millisecond
				c.PartsDir = dir
				c.PartsReserve = 1
			})
			// The holder sends all but its last part, then nothing: its
			// file is kept, measured and fresh.
			holder := startLarge(t, a)
			rand.Read(holder.eh[:])
			holder.size = (holderParts + 1) * part
			holder.skip = func(n int) bool { return n >= holderParts }
			var done atomic.Bool
			var wg sync.WaitGroup
			wg.Add(1)
			go func() {
				defer wg.Done()
				holder.serve(t, time.Now().Add(time.Hour), done.Load)
			}()
			t.Cleanup(func() { done.Store(true); wg.Wait() })
			until := time.Now().Add(20 * time.Second)
			for partsBytes(t, dir) < holderParts*part {
				if time.Now().After(until) {
					t.Fatal("the holder never filled its file")
				}
				time.Sleep(20 * time.Millisecond)
			}
			free.Store(uint64(holderParts+room) * part)
			keys := map[string]bool{}
			for _, p := range a.h.Peers() {
				keys[p.Key] = true
			}
			honest := startLarge(t, a)
			honest.eh = holder.eh
			honest.size = 1000 * part
			honest.chunks = chunks
			key := ""
			for _, p := range a.h.Peers() {
				if !keys[p.Key] {
					key = p.Key
				}
			}
			if key == "" {
				t.Fatal("setup: the honest sender is not a peer")
			}
			stop := time.Now().Add(4 * time.Second)
			wg.Add(1)
			go func() {
				defer wg.Done()
				honest.serve(t, time.Now().Add(time.Hour), func() bool { return done.Load() || time.Now().After(stop) })
			}()
			for time.Now().Before(stop) {
				if st := a.h.Tracker().Status(key, time.Now()); st.FlagTotal > 0 {
					t.Fatalf("the honest sender was flagged %v after parts asked from %v", st.FlagTotal, honest.offsets)
				}
				time.Sleep(20 * time.Millisecond)
			}
			if len(honest.offsets) == 0 {
				t.Fatal("setup: the honest sender was never asked for parts")
			}
		})
	}
}
