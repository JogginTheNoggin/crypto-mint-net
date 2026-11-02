package p2p_test

import (
	"crypto/rand"
	"strings"
	"testing"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/layer"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/p2p"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tracking"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/weight"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/wire"
)

// manyLayersSender drives a raw sender that lists one entry, eh, with
// the layers in pool, served as a layers-from reply gives them, per
// nonces a page. serve may replace a page; froms records each page asked
// for. It returns when stop says so or the time is up.
func manyLayersSender(t *testing.T, r *rawPeer, eh hash.Hash, pool []uint64, per int, serve func(from uint32) (wire.LayersFrom, bool), stop func(froms []uint32) bool) []uint32 {
	t.Helper()
	sec := poolSection()
	huge := weight.FromUint64(1 << 62)
	var tip hash.Hash
	rand.Read(tip[:])
	r.write(wire.Message{Header: wire.Header{Type: wire.TypeNotice, Section: sec, Stated: huge},
		Body: wire.Notice{Tip: tip}.Encode()})
	var froms []uint32
	until := time.Now().Add(30 * time.Second)
	for !stop(froms) {
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
			r.reply(m, wire.TypeManyLayers, wire.ManyLayers{Entry: eh, Count: uint32(len(pool))}.Encode(), huge)
		case wire.TypeLayersFromReq:
			req, err := wire.DecodeRequest(m.Type, m.Body)
			must(t, err)
			q := req.(wire.LayersFromReq)
			froms = append(froms, q.From)
			lf, more := wire.LayersFrom{Total: uint32(len(pool)), From: q.From}, false
			if serve != nil {
				lf, more = serve(q.From)
			} else if int(q.From) < len(pool) {
				end := min(int(q.From)+per, len(pool))
				lf.Nonces, more = pool[q.From:end], end < len(pool)
			}
			flags := wire.FlagLast
			if more {
				flags |= wire.FlagMore
			}
			r.write(wire.Message{Header: wire.Header{Type: wire.TypeLayersFrom, Section: m.Section, RequestID: m.RequestID,
				Chunk: 1, Flags: flags, Stated: huge}, Body: lf.Encode()})
		case wire.TypeWeightReq:
			r.reply(m, wire.TypeWeight, nil, huge)
		}
	}
	return froms
}

// strongestPool returns n layer nonces of eh in the strongest-first
// order.
func strongestPool(eh hash.Hash, n int) []uint64 {
	pool := make([]uint64, n)
	for i := range pool {
		pool[i] = uint64(i) + 1
	}
	p2p.StrongestNonces(exSeries.Hash(), eh, pool)
	return pool
}

// Black's B3 (D124): paging one entry's many layers checks the time slice
// before each page. A chase past its slice yields, and the next goes on
// from the offset it reached, not from the first layer, so a sender
// cannot hold the chase in the paging.
func TestManyLayersPagingYieldsAndGoesOn(t *testing.T) {
	a := startNode(t, "a", func(c *p2p.Config) {
		c.ChaseSlice = time.Nanosecond // every page after a chase's first yields
		c.Chase.YieldWait = 10 * time.Millisecond
	})
	// a holds real work, so no single page of weak layers beats it.
	tip := grow2(t, a)
	_, _, err := a.n.Work(exKey, tip, 0, 1<<14)
	must(t, err)
	r := dialRaw(t, a, []wire.Section{poolSection()})
	verified(t, a, r)
	var eh hash.Hash
	rand.Read(eh[:])
	pool := strongestPool(eh, 4000)
	froms := manyLayersSender(t, r, eh, pool[len(pool)-400:], 10, nil, func(fs []uint32) bool {
		return len(fs) >= 8 && strings.Contains(a.log.String(), "paging an entry's layers")
	})
	for i := 1; i < len(froms); i++ {
		if froms[i] <= froms[i-1] {
			t.Fatalf("paging started again: layers asked from %v", froms)
		}
	}
}

// grow2 gives tn a short ledger and returns its tip.
func grow2(t *testing.T, tn *tnode) hash.Hash {
	t.Helper()
	grow(t, tn, nil, "alice", 3, 32)
	return tn.status().Tip
}

// Black's B3 (D124): a sender that pages the same layers again with no
// new layers to account for it is false, so it cannot keep a chase
// paging by repeating itself.
func TestManyLayersRepeatedAreFalse(t *testing.T) {
	a := startNode(t, "a", nil)
	tip := grow2(t, a)
	_, _, err := a.n.Work(exKey, tip, 0, 1<<14)
	must(t, err)
	r := dialRaw(t, a, []wire.Section{poolSection()})
	verified(t, a, r)
	var eh hash.Hash
	rand.Read(eh[:])
	pool := strongestPool(eh, 4000)[3990:] // weak layers only
	const total = 1 << 20
	manyLayersSender(t, r, eh, pool, 0, func(from uint32) (wire.LayersFrom, bool) {
		// Always the same page, at whatever offset is asked for.
		return wire.LayersFrom{Total: total, From: from, Nonces: pool}, true
	}, func([]uint32) bool { return strings.Contains(a.log.String(), "result=false") })
	if !strings.Contains(a.log.String(), "read again") {
		t.Fatalf("not flagged for repeating:\n%s", a.log)
	}
}

// Black's C-5 (D124): step 4 stops counting once the work beats the
// node's, and moves on only past the entries it counted. When the node's
// own ledger grows meanwhile, the chase goes back to step 4 and counts
// the rest of the branch, which used to be skipped: the chase then ended
// as "heavier than the whole branch" with the branch's heavier rest never
// counted.
func TestEarlyStopCountsTheRestLater(t *testing.T) {
	a := startNode(t, "a", nil)
	tip := grow2(t, a)
	r := dialRaw(t, a, []wire.Section{poolSection()})
	verified(t, a, r)
	fn := exSeries.Hash()
	hs := make([]hash.Hash, 5)
	for i := range hs {
		rand.Read(hs[i][:])
	}
	// Each listed entry carries one layer stronger than a's whole weight.
	w := a.status().Weight
	strong := func(eh hash.Hash) uint64 {
		for n := uint64(1); ; n++ {
			if weight.Credit(layer.Layer{Entry: eh, Nonce: n}.Strength(fn)).Greater(w) {
				return n
			}
		}
	}
	items := make([]wire.HeldLayers, len(hs))
	for i, eh := range hs {
		items[i] = wire.HeldLayers{Entry: eh, Held: true, Nonces: []uint64{strong(eh)}}
	}
	sec := poolSection()
	huge := weight.FromUint64(1 << 62)
	r.write(wire.Message{Header: wire.Header{Type: wire.TypeNotice, Section: sec, Stated: huge},
		Body: wire.Notice{Tip: hs[len(hs)-1]}.Encode()})
	var asked [][]hash.Hash
	until := time.Now().Add(30 * time.Second)
	for len(asked) < 2 {
		if time.Now().After(until) {
			t.Fatalf("timed out: layers asked %d times\n%s", len(asked), a.log)
		}
		if strings.Contains(a.log.String(), "heavier than the whole branch") {
			t.Fatalf("the rest of the branch was skipped:\n%s", a.log)
		}
		m, _, err := r.readUntil(time.Now().Add(20 * time.Millisecond))
		if err != nil {
			continue
		}
		switch m.Type {
		case wire.TypeHashChainReq:
			r.reply(m, wire.TypeHashChain, wire.HashChainChunk{FromStart: true, Hashes: hs}.Encode(), huge)
		case wire.TypeLayersReq:
			req, err := wire.DecodeRequest(m.Type, m.Body)
			must(t, err)
			q := req.(wire.LayersReq)
			asked = append(asked, q.Hashes)
			if len(asked) == 1 {
				// a's own ledger grows past the first entry's layer while
				// step 4 runs.
				for !a.status().Weight.Greater(w.Add(weight.Credit(layer.Layer{Entry: hs[0], Nonce: items[0].Nonces[0]}.Strength(fn)))) {
					_, _, err := a.n.Work(exKey, tip, uint64(time.Now().UnixNano()), 1<<16)
					must(t, err)
				}
			}
			var reply []wire.HeldLayers
			for _, h := range q.Hashes {
				for _, it := range items {
					if it.Entry == h {
						reply = append(reply, it)
					}
				}
			}
			r.reply(m, wire.TypeLayers, wire.LayersChunk{Items: reply}.Encode(), huge)
		case wire.TypeExtensionReq:
			r.reply(m, wire.TypeNothingHeavier, nil, huge)
		case wire.TypeWeightReq:
			r.reply(m, wire.TypeWeight, nil, huge)
		}
	}
	if len(asked[1]) == 0 || asked[1][0] != hs[1] {
		t.Fatalf("step 4 went on from %v, want the second listed entry %s", asked[1], hs[1].Short())
	}
}

// Black's B2 (D124): a sender whose request budget is below this node's
// pace drops some of the chase's requests. The chase stalls on the
// timeout, keeps what it took in, and goes on after the backoff, so it
// still reaches the heavier ledger, only later.
func TestChaseBelowSenderBudgetIsReached(t *testing.T) {
	tc := tracking.DefaultConfig()
	tc.Budgets[tracking.KindRequest] = 30
	tc.Windows.Budget = 3 * time.Second
	tr, err := tracking.New(tc, "")
	must(t, err)
	a := startNode(t, "a", func(c *p2p.Config) { c.Tracker = tr })
	b := startNode(t, "b", func(c *p2p.Config) {
		c.Limits.HashLimit = 16
		c.RequestTimeout = 500 * time.Millisecond
		c.Chase.YieldWait = 100 * time.Millisecond
		c.Chase.Backoff, c.Chase.MaxBackoff = 200*time.Millisecond, time.Second
		c.RequestPace = 60 // twice what a serves in a window
		c.RequestPaceWindow = 3 * time.Second
	})
	grow(t, a, nil, "alice", 3, 32)
	copyLedger(t, a, b)
	bareRun(t, a, 600)
	b.connect(a)
	eventuallyFor(t, time.Minute, "b to reach a's ledger", func() bool { return a.state() == b.state() }, a, b)
	if !strings.Contains(b.log.String(), "result=stalled") && !strings.Contains(b.log.String(), "result=slowed") {
		t.Fatalf("no request was dropped, so the stall was not tested:\n%s", b.log)
	}
}

// Black's B2 (D124), at the size Orange measured (ORANGE-018): with the
// default budgets and pace, a heavier ledger whose newest 100,000 entries
// carry no layers is reached. It takes minutes, so it is skipped with
// -short and under the race detector; CI runs it on its own.
func TestHeavierLedgerLongBareRun(t *testing.T) {
	if testing.Short() || raceOn {
		t.Skip("long: run without -short and -race")
	}
	a := startNode(t, "a", nil)
	b := startNode(t, "b", nil)
	grow(t, a, nil, "alice", 3, 32)
	copyLedger(t, a, b)
	bareRun(t, a, 100000)
	b.connect(a)
	eventuallyFor(t, 10*time.Minute, "b to reach a's ledger", func() bool { return a.state() == b.state() }, a, b)
}
