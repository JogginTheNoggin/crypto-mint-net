package p2p_test

import (
	"crypto/rand"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/p2p"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/weight"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/wire"
)

// Black R12 (B6): two colluding senders fill the parts disk once, leave
// one byte above the reserve, and then each sends one part a chase and
// lets the next request time out. With a byte of room each may ask, and
// its part, arriving at once, finishes "first" against the other's file,
// so each cuts the other in turn and neither ever completes. Both keep a
// fresh measure, and each claims only a little more than it holds, so
// both are expected to finish before the honest fetch and it may cut
// neither. The honest fetch, delivering many times faster, never holds a
// byte.
func TestBlackColludingTricklersHoldTheReserve(t *testing.T) {
	const part = 64 << 10
	const hold, extra = 512, 32 // parts each trickler holds, and claims beyond
	const capParts = 2 * hold   // the parts disk above the reserve
	const disk = 1 << 50
	reserve := uint64(disk / 50)
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
		room := uint64(capParts*part + 1)
		if used >= room {
			return reserve, disk, nil
		}
		return reserve + room - used, disk, nil
	})
	t.Cleanup(restore)
	dir := t.TempDir()
	a := startNode(t, "a", func(c *p2p.Config) {
		c.RequestTimeout = 300 * time.Millisecond
		c.Chase.Backoff = 500 * time.Millisecond
		c.Chase.YieldWait = 50 * time.Millisecond
		c.PartsDir = dir
		c.RequestPace = 1 << 20
	})
	var done atomic.Bool
	var wg sync.WaitGroup
	t.Cleanup(func() { done.Store(true); wg.Wait() })
	trickler := func() *atomic.Int64 {
		tr := startLarge(t, a)
		rand.Read(tr.eh[:])
		tr.size = (hold + extra) * part
		// Fill hold parts, time out, then four parts at once and a time-out each chase.
		tr.skip = func(n int) bool { return n == hold || n > hold && (n-hold)%5 == 0 }
		var asked atomic.Int64
		wg.Add(1)
		go func() {
			defer wg.Done()
			tr.serve(t, time.Now().Add(time.Hour), func() bool {
				asked.Store(int64(len(tr.offsets)))
				return done.Load()
			})
		}()
		return &asked
	}
	wait := func(n *atomic.Int64, want int64) {
		until := time.Now().Add(30 * time.Second)
		for n.Load() < want {
			if time.Now().After(until) {
				t.Fatal("setup: a trickler did not fill")
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	x := trickler()
	wait(x, hold+1)
	y := trickler()
	wait(y, hold+1)
	time.Sleep(2 * time.Second) // a few rounds of cutting each other

	hs := startLarge(t, a)
	rand.Read(hs.eh[:])
	hs.size = capParts * part // fits the parts disk exactly
	var most atomic.Uint64
	wg.Add(1)
	go func() {
		defer wg.Done()
		serveDelayed(t, hs, 5*time.Millisecond, func() bool {
			for _, o := range hs.offsets {
				if o > most.Load() {
					most.Store(o)
				}
			}
			return done.Load()
		})
	}()
	time.Sleep(20 * time.Second)
	// Grey (D140): the raw senders' connections close in cleanups that run
	// before the one above, so the serving goroutines stop first; only
	// this teardown is changed from Black's appendix.
	done.Store(true)
	wg.Wait()
	if most.Load() < 64*part {
		t.Fatalf("the honest fetch held %d parts after 20 s (the tricklers were asked %d and %d times)", most.Load()/part, x.Load(), y.Load())
	}
}

// serveDelayed is largeSender.serve with each part sent delay after its
// request, as over a link of 64 KiB a delay.
func serveDelayed(t *testing.T, ls *largeSender, delay time.Duration, stop func() bool) {
	huge := weight.FromUint64(1 << 62)
	eh := ls.eh
	for !stop() {
		m, _, err := ls.r.readUntil(time.Now().Add(20 * time.Millisecond))
		if err != nil {
			continue
		}
		switch m.Type {
		case wire.TypeHashChainReq:
			ls.r.reply(m, wire.TypeHashChain, wire.HashChainChunk{FromStart: true, Hashes: []hash.Hash{eh}}.Encode(), huge)
		case wire.TypeLayersReq:
			ls.r.reply(m, wire.TypeLayers, wire.LayersChunk{Items: []wire.HeldLayers{{Entry: eh, Held: true, Nonces: []uint64{1}}}}.Encode(), huge)
		case wire.TypeWeightReq:
			ls.r.reply(m, wire.TypeWeight, nil, huge)
		case wire.TypeFullLedgerReq, wire.TypeExtensionReq:
			ls.r.reply(m, wire.TypeLarge, wire.Large{Entry: eh, Size: ls.size}.Encode(), huge)
		case wire.TypePartsReq:
			req, err := wire.DecodeRequest(m.Type, m.Body)
			must(t, err)
			q := req.(wire.PartsReq)
			ls.offsets = append(ls.offsets, q.Offset)
			data := make([]byte, 64<<10)
			if q.Offset == 0 {
				copy(data, ls.first)
			}
			time.Sleep(delay)
			ls.r.write(wire.Message{Header: wire.Header{Type: wire.TypePart, Section: m.Section, RequestID: m.RequestID,
				Chunk: 1, Flags: wire.FlagLast | wire.FlagMore, Stated: huge}, Body: wire.Part{Total: ls.size, Offset: q.Offset, Data: data}.Encode()})
		}
	}
}
