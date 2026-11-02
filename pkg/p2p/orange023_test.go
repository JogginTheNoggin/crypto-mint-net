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

// ORANGE-023: a fetch with no room and no measure yet probes: it asks for
// the part at its offset and throws the reply away (D140). A real
// sender's reply holds many parts, each following the last. Under D140
// only the first was thrown away; the second did not start at the
// fetch's offset, so an honest sender was shown false and its entry was
// never fetched. Now the whole reply is thrown away and the fetch, once
// measured, takes room and reaches its end.
func TestProbeKeepsAnHonestSender(t *testing.T) {
	if testing.Short() {
		t.Skip("long: run without -short")
	}
	const part = 64 << 10
	const total = 100 << 20
	const holderParts, freshParts = 96, 32
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
		// Above the reserve, room for the holder's entry and no more.
		free := uint64(total/50 + holderParts*part)
		if used >= free {
			return 0, total, nil
		}
		return free - used, total, nil
	})
	t.Cleanup(restore)
	dir := t.TempDir()
	a := startNode(t, "a", func(c *p2p.Config) {
		c.Chase.Backoff = 500 * time.Millisecond
		c.ChaseSlice = 2 * time.Second
		c.Chase.YieldWait = 50 * time.Millisecond
		c.PartsDir = dir
		c.PartsReserve = 1
	})
	var done atomic.Bool
	var wg sync.WaitGroup
	serve := func(s *largeSender, reached *atomic.Bool) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.serve(t, time.Now().Add(time.Hour), func() bool {
				if n := len(s.offsets); n > 0 && s.offsets[n-1]+uint64(max(s.chunks, 1))*part >= s.size {
					reached.Store(true)
				}
				return done.Load()
			})
		}()
	}
	// The holder fills most of the room quickly, then slows to a part a
	// second, so the fresh fetch finds no room.
	holder := startLarge(t, a)
	rand.Read(holder.eh[:])
	holder.size = holderParts * part
	holder.pause = func(off uint64) time.Duration {
		if off >= (holderParts-8)*part {
			return time.Second
		}
		return 0
	}
	var holderEnd, freshEnd atomic.Bool
	serve(holder, &holderEnd)
	t.Cleanup(func() { done.Store(true); wg.Wait() })
	until := time.Now().Add(20 * time.Second)
	for partsBytes(t, dir) < (holderParts-8)*part {
		if time.Now().After(until) {
			t.Fatal("the holder never filled the room")
		}
		time.Sleep(20 * time.Millisecond)
	}
	// The fresh sender is honest and fast, and its replies hold four parts.
	fresh := startLarge(t, a)
	rand.Read(fresh.eh[:])
	fresh.size = freshParts * part
	fresh.chunks = 4
	serve(fresh, &freshEnd)
	until = time.Now().Add(30 * time.Second)
	for !freshEnd.Load() {
		if time.Now().After(until) {
			t.Fatalf("the fresh fetch never reached its end: parts asked from %d offsets", len(fresh.offsets))
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func partsBytes(t *testing.T, dir string) uint64 {
	t.Helper()
	es, err := os.ReadDir(dir)
	must(t, err)
	var n uint64
	for _, e := range es {
		if fi, err := e.Info(); err == nil {
			n += uint64(fi.Size())
		}
	}
	return n
}
