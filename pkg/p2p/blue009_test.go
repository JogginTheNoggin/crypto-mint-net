package p2p_test

import (
	"crypto/rand"
	"sync/atomic"
	"testing"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/p2p"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/weight"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/wire"
)

// RED-021: a sender that claims more layers on one entry than any node
// could hold is paged only until the layers counted beat the node's own
// work; the node then goes on to the ledger. Nothing caps the layers: an
// entry whose work needs every page gets every page (D121, D123).
func TestManyLayersArePagedOnlyUntilEnough(t *testing.T) {
	a := startNode(t, "a", func(c *p2p.Config) { c.LayersWhole = 1 })
	sec := poolSection()
	r := dialRaw(t, a, []wire.Section{sec})
	verified(t, a, r)
	huge := weight.FromUint64(1 << 62)
	var eh, tip hash.Hash
	rand.Read(eh[:])
	rand.Read(tip[:])
	r.write(wire.Message{Header: wire.Header{Type: wire.TypeNotice, Section: sec, Stated: huge},
		Body: wire.Notice{Tip: tip}.Encode()})
	const claimed = 1<<32 - 1
	var froms []uint32
	ledger := false
	until := time.Now().Add(20 * time.Second)
	for !ledger {
		if time.Now().After(until) {
			t.Fatalf("timed out: layers asked from %v", froms)
		}
		m, _, err := r.readUntil(time.Now().Add(20 * time.Millisecond))
		if err != nil {
			continue
		}
		switch m.Type {
		case wire.TypeHashChainReq:
			r.reply(m, wire.TypeHashChain, wire.HashChainChunk{FromStart: true, Hashes: []hash.Hash{eh}}.Encode(), huge)
		case wire.TypeLayersReq:
			r.reply(m, wire.TypeManyLayers, wire.ManyLayers{Entry: eh, Count: claimed}.Encode(), huge)
		case wire.TypeLayersFromReq:
			req, err := wire.DecodeRequest(m.Type, m.Body)
			must(t, err)
			q := req.(wire.LayersFromReq)
			froms = append(froms, q.From)
			ns := make([]uint64, 1000)
			for i := range ns {
				ns[i] = uint64(q.From) + uint64(i) + 1
			}
			// In the strongest-first order the node checks (Black's B3).
			p2p.StrongestNonces(exSeries.Hash(), eh, ns)
			r.write(wire.Message{Header: wire.Header{Type: wire.TypeLayersFrom, Section: m.Section, RequestID: m.RequestID,
				Chunk: 1, Flags: wire.FlagLast | wire.FlagMore, Stated: huge},
				Body: wire.LayersFrom{Total: claimed, From: q.From, Nonces: ns}.Encode()})
		case wire.TypeWeightReq:
			r.reply(m, wire.TypeWeight, nil, huge)
		case wire.TypeFullLedgerReq, wire.TypeExtensionReq:
			ledger = true
		}
	}
	// The node holds nothing, so the first page's work already beats it.
	if len(froms) != 1 || froms[0] != 0 {
		t.Fatalf("layers asked from %v, want one page from 0", froms)
	}
}

// RED-022: when the disk of the parts directory is down to its reserve,
// the fetch of a large entry waits rather than writing on; it goes on
// from the offset it reached once there is room again. It is never
// refused.
func TestLargeEntryWaitsForDiskSpace(t *testing.T) {
	var full atomic.Bool
	const disk = 1 << 50
	restore := p2p.SetDiskSpace(func(string) (uint64, uint64, error) {
		if full.Load() {
			return disk / 100, disk, nil // under the 2% reserve
		}
		return disk, disk, nil
	})
	t.Cleanup(restore) // runs after the node below has stopped
	dir := t.TempDir()
	a := startNode(t, "a", func(c *p2p.Config) {
		c.ChaseSlice = 300 * time.Millisecond
		c.Chase.YieldWait = 50 * time.Millisecond
		c.PartsDir = dir
	})
	sec := poolSection()
	r := dialRaw(t, a, []wire.Section{sec})
	verified(t, a, r)
	poolID := exSeries.PoolID(exKey.Index)
	ls := &largeSender{r: r, size: 1 << 40, first: poolID[:]}
	var tip hash.Hash
	rand.Read(tip[:])
	r.write(wire.Message{Header: wire.Header{Type: wire.TypeNotice, Section: sec, Stated: weight.FromUint64(1 << 62)},
		Body: wire.Notice{Tip: tip}.Encode()})

	// Some parts come, then the disk fills.
	ls.serve(t, time.Now().Add(20*time.Second), func() bool { return len(ls.offsets) >= 10 })
	full.Store(true)
	at := len(ls.offsets)
	wait := time.Now().Add(time.Second)
	ls.serve(t, time.Now().Add(5*time.Second), func() bool { return time.Now().After(wait) })
	if got := len(ls.offsets) - at; got > 1 {
		t.Fatalf("%d parts asked while the disk was at its reserve", got)
	}
	held := len(ls.offsets)

	// Room is made: the fetch goes on from its offset.
	full.Store(false)
	ls.serve(t, time.Now().Add(20*time.Second), func() bool { return len(ls.offsets) >= held+10 })
	for i := 1; i < len(ls.offsets); i++ {
		if ls.offsets[i] != ls.offsets[i-1]+64<<10 {
			t.Fatalf("parts asked from %v: the fetch did not go on from its offset", ls.offsets)
		}
	}
}
