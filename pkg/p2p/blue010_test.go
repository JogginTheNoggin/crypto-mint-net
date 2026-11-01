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

// startLarge dials a raw sender of a large entry to a and starts its
// chase with a notice.
func startLarge(t *testing.T, a *tnode) *largeSender {
	t.Helper()
	sec := poolSection()
	r := dialRaw(t, a, []wire.Section{sec})
	verified(t, a, r)
	poolID := exSeries.PoolID(exKey.Index)
	ls := &largeSender{r: r, size: 1 << 40, first: poolID[:]}
	var tip hash.Hash
	rand.Read(tip[:])
	r.write(wire.Message{Header: wire.Header{Type: wire.TypeNotice, Section: sec, Stated: weight.FromUint64(1 << 62)},
		Body: wire.Notice{Tip: tip}.Encode()})
	return ls
}

func partsFiles(t *testing.T, dir string) int {
	t.Helper()
	es, err := os.ReadDir(dir)
	must(t, err)
	return len(es)
}

// RED-024: a parts request that times out stalls the chase, and the next
// chase goes on from the offset the fetch reached, not from 0.
func TestStalledFetchKeepsItsParts(t *testing.T) {
	dir := t.TempDir()
	a := startNode(t, "a", func(c *p2p.Config) {
		c.RequestTimeout = 300 * time.Millisecond
		c.Chase.Backoff = 50 * time.Millisecond
		c.Chase.YieldWait = 50 * time.Millisecond
		c.PartsDir = dir
	})
	ls := startLarge(t, a)
	ls.skip = func(n int) bool { return n == 9 }
	ls.serve(t, time.Now().Add(20*time.Second), func() bool { return len(ls.offsets) >= 13 })
	if ls.offsets[9] != 9*64<<10 || ls.offsets[10] != 9*64<<10 {
		t.Fatalf("parts asked from %v: after the stall the fetch did not go on from its offset", ls.offsets)
	}
	for i := 11; i < len(ls.offsets); i++ {
		if ls.offsets[i] != ls.offsets[i-1]+64<<10 {
			t.Fatalf("parts asked from %v", ls.offsets)
		}
	}
	if n := partsFiles(t, dir); n != 1 {
		t.Fatalf("%d parts files, want 1", n)
	}
}

// RED-023: a sender's parts file goes once the sender has been gone for
// the grace; the rest of its progress, which costs little memory, is kept
// to the time to live (Black's C-6, D129).
func TestDepartedSenderLeavesNothing(t *testing.T) {
	dir := t.TempDir()
	a := startNode(t, "a", func(c *p2p.Config) {
		c.ChaseSlice = 200 * time.Millisecond
		c.Chase.YieldWait = time.Hour // the chase yields and is not taken up again
		c.CheckEvery = 50 * time.Millisecond
		c.ProgressGrace = 200 * time.Millisecond
		c.PartsDir = dir
	})
	ls := startLarge(t, a)
	ls.serve(t, time.Now().Add(20*time.Second), func() bool {
		_, held := a.h.ProgressHeld()
		return held > 0 || a.h.ProgressCount() > 0
	})
	if partsFiles(t, dir) != 1 {
		t.Fatalf("setup: %d parts files", partsFiles(t, dir))
	}
	ls.r.c.Close()
	eventuallyFor(t, 10*time.Second, "the departed sender's parts to go", func() bool {
		return partsFiles(t, dir) == 0
	}, a)
	if a.h.ProgressCount() != 1 {
		t.Fatalf("%d progress kept, want the departed sender's, to its time to live", a.h.ProgressCount())
	}
}

// RED-023: a fetch that would wait for disk space first takes the room
// that the parts of departed senders hold, however long their grace.
func TestDiskWaitTakesRoomFromDepartedSenders(t *testing.T) {
	const disk = 1 << 50
	// The disk is full while two parts files are in it.
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
		c.ChaseSlice = 200 * time.Millisecond
		c.Chase.YieldWait = time.Hour
		c.PartsDir = dir
	})
	gone := startLarge(t, a)
	gone.serve(t, time.Now().Add(20*time.Second), func() bool { return a.h.ProgressCount() > 0 })
	gone.r.c.Close()
	eventuallyFor(t, 5*time.Second, "the first sender to be gone", func() bool { return len(a.h.Peers()) == 0 }, a)

	ls := startLarge(t, a)
	ls.serve(t, time.Now().Add(20*time.Second), func() bool { return len(ls.offsets) >= 10 })
	if n := partsFiles(t, dir); n != 1 {
		t.Fatalf("%d parts files, want the sending one's only", n)
	}
}
