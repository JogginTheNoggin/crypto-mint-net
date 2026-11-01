package p2p_test

import (
	"crypto/rand"
	"os"
	"testing"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/p2p"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/weight"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/wire"
)

// RED-026: a chase that counted many layers and stalled keeps nothing for
// each layer, only the entry they sit on; once its sender has been gone
// for the grace, its progress is cut down to the common entry, and is
// still there for the sender to go on from (Black's C-6).
func TestKeptProgressHoldsNothingPerLayer(t *testing.T) {
	a := startNode(t, "a", func(c *p2p.Config) {
		c.LayersWhole = 1
		c.RequestTimeout = 300 * time.Millisecond
		c.CheckEvery = 50 * time.Millisecond
		c.ProgressGrace = 200 * time.Millisecond
	})
	grow(t, a, nil, "alice", 2, 32)
	sec := poolSection()
	r := dialRaw(t, a, []wire.Section{sec})
	verified(t, a, r)
	huge := weight.FromUint64(1 << 62)
	var eh, tip hash.Hash
	rand.Read(eh[:])
	rand.Read(tip[:])
	r.write(wire.Message{Header: wire.Header{Type: wire.TypeNotice, Section: sec, Stated: huge},
		Body: wire.Notice{Tip: tip}.Encode()})
	pages := 0
	pool := strongestPool(eh, 2000)
	until := time.Now().Add(20 * time.Second)
	for a.h.ProgressCount() == 0 {
		if time.Now().After(until) {
			t.Fatalf("no progress kept after %d pages", pages)
		}
		m, _, err := r.readUntil(time.Now().Add(20 * time.Millisecond))
		if err != nil {
			continue
		}
		switch m.Type {
		case wire.TypeHashChainReq:
			r.reply(m, wire.TypeHashChain, wire.HashChainChunk{FromStart: true, Hashes: []hash.Hash{eh}}.Encode(), huge)
		case wire.TypeLayersReq:
			r.reply(m, wire.TypeManyLayers, wire.ManyLayers{Entry: eh, Count: 2000}.Encode(), huge)
		case wire.TypeWeightReq:
			r.reply(m, wire.TypeWeight, nil, huge)
		case wire.TypeLayersFromReq:
			pages++
			if pages > 2 {
				continue // a later request goes unanswered and the chase stalls
			}
			req, err := wire.DecodeRequest(m.Type, m.Body)
			must(t, err)
			q := req.(wire.LayersFromReq)
			ns := pool[q.From : q.From+1000] // strongest first, as the node checks
			r.write(wire.Message{Header: wire.Header{Type: wire.TypeLayersFrom, Section: m.Section, RequestID: m.RequestID,
				Chunk: 1, Flags: wire.FlagLast | wire.FlagMore, Stated: huge},
				Body: wire.LayersFrom{Total: 2000, From: q.From, Nonces: ns}.Encode()})
		}
	}
	entries, total := a.h.ProgressCounted()
	if total.IsZero() || entries != 1 {
		t.Fatalf("kept progress counts %s over %d entries, want work over 1", total, entries)
	}
	r.c.Close()
	eventuallyFor(t, 10*time.Second, "the departed sender's progress to be cut down", func() bool {
		entries, total := a.h.ProgressCounted()
		_, held := a.h.ProgressHeld()
		return entries == 0 && total.IsZero() && held == 0
	}, a)
	if a.h.ProgressCount() != 1 {
		t.Fatal("the progress itself went before its time to live")
	}
}

// RED-027: a sender that stays connected but stops sending parts does not
// hold the disk reserve: a fetch from a sender that is sending takes the
// idle one's room, and goes on.
func TestIdleConnectedSenderGivesUpItsParts(t *testing.T) {
	const disk = 1 << 50
	restore := p2p.SetDiskSpace(func(dir string) (uint64, uint64, error) {
		es, err := os.ReadDir(dir)
		if err != nil {
			return 0, 0, err
		}
		if len(es) >= 2 {
			return disk / 100, disk, nil // full while two parts files exist
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
	hog := startLarge(t, a)
	hog.skip = func(n int) bool { return n >= 10 }
	// Ten parts, then two chases that ask and get nothing.
	hog.serve(t, time.Now().Add(20*time.Second), func() bool { return len(hog.offsets) >= 12 })
	settle := time.Now().Add(500 * time.Millisecond)
	hog.serve(t, time.Now().Add(5*time.Second), func() bool { return time.Now().After(settle) })
	if partsFiles(t, dir) != 1 {
		t.Fatalf("setup: %d parts files", partsFiles(t, dir))
	}

	ls := startLarge(t, a)
	ls.serve(t, time.Now().Add(20*time.Second), func() bool { return len(ls.offsets) >= 10 })
	if n := partsFiles(t, dir); n != 1 {
		t.Fatalf("%d parts files, want the sending one's only", n)
	}
	if len(a.h.Peers()) != 2 {
		t.Fatal("setup: the idle sender is not connected")
	}
}
