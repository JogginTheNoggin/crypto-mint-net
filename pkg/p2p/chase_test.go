package p2p_test

import (
	"crypto/rand"
	"testing"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/engine"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/entry"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/p2p"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tracking"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/weight"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/wire"
)

// verified reads from r, answering the node's follow requests, until tn
// has placed r in a category.
func verified(t *testing.T, tn *tnode, r *rawPeer) {
	t.Helper()
	eventually(t, "the raw peer to be verified", func() bool {
		r.readUntil(time.Now().Add(20 * time.Millisecond))
		for _, p := range tn.h.Peers() {
			if len(p.Categories) > 0 {
				return true
			}
		}
		return false
	})
}

// reply sends a one-chunk reply to m.
func (r *rawPeer) reply(m wire.Message, t wire.Type, body []byte, stated weight.Weight) {
	r.write(wire.Message{Header: wire.Header{Type: t, Section: m.Section, RequestID: m.RequestID,
		Chunk: 1, Flags: wire.FlagLast, Stated: stated}, Body: body})
}

// A huge stated weight backed by a made-up branch is chased from cheap to
// costly: the node pages the sender's hashes, asks for the layers on them,
// finds no work behind them, and stops there. It never asks for an entry,
// and the sender is backed off.
func TestEmptyBranchIsCheckedBeforeEntries(t *testing.T) {
	a := startNode(t, "a", nil)
	grow(t, a, nil, "alice", 2, 32)
	sec := poolSection()
	r := dialRaw(t, a, []wire.Section{sec})
	verified(t, a, r)

	fake := make([]hash.Hash, 3)
	for i := range fake {
		rand.Read(fake[i][:])
	}
	huge := weight.FromUint64(1 << 62)
	r.write(wire.Message{Header: wire.Header{Type: wire.TypeNotice, Section: sec, Stated: huge},
		Body: wire.Notice{Tip: fake[2]}.Encode()})

	var askedChain, askedLayers bool
	deadline := time.Now().Add(10 * time.Second)
	for !askedLayers || len(a.h.ChaseQueue().Groups("", time.Now())) > 0 {
		if time.Now().After(deadline) {
			t.Fatalf("chain %v layers %v groups %+v", askedChain, askedLayers, a.h.ChaseQueue().Groups("", time.Now()))
		}
		m, _, err := r.readUntil(time.Now().Add(50 * time.Millisecond))
		if err != nil {
			continue
		}
		switch m.Type {
		case wire.TypeHashChainReq:
			askedChain = true
			r.reply(m, wire.TypeHashChain, wire.HashChainChunk{FromStart: true, Hashes: fake}.Encode(), huge)
		case wire.TypeLayersReq:
			askedLayers = true
			req, err := wire.DecodeRequest(m.Type, m.Body)
			must(t, err)
			q := req.(wire.LayersReq)
			var items []wire.HeldLayers
			for _, h := range q.Hashes {
				items = append(items, wire.HeldLayers{Entry: h, Held: true})
			}
			r.reply(m, wire.TypeLayers, wire.LayersChunk{Items: items}.Encode(), huge)
		case wire.TypeWeightReq:
			r.reply(m, wire.TypeWeight, nil, huge)
		case wire.TypeExtensionReq, wire.TypeFullLedgerReq, wire.TypeEntriesReq:
			t.Fatalf("asked for entries (%s) before checking the work", m.Type)
		}
	}
	if !askedChain {
		t.Fatal("no hash chain asked for")
	}
	var id string
	for _, p := range a.h.Peers() {
		id = p.ID
	}
	if _, off := a.h.ChaseQueue().BackedOff(id, time.Now()); !off {
		t.Fatal("the sender was not backed off")
	}
	if a.status().Height != 2 {
		t.Fatalf("height %d", a.status().Height)
	}
	// While it is backed off, the same claim is not chased again.
	r.write(wire.Message{Header: wire.Header{Type: wire.TypeNotice, Section: sec, Stated: huge},
		Body: wire.Notice{Tip: fake[2]}.Encode()})
	end := time.Now().Add(300 * time.Millisecond)
	for time.Now().Before(end) {
		m, _, err := r.readUntil(end)
		if err == nil && m.Type == wire.TypeHashChainReq {
			t.Fatal("a backed-off sender was chased")
		}
	}
}

// A branch that contradicts itself is provably false: the sender is
// flagged and backed off. The repeated hash itself does not go on the
// known-bad list: a repeat shows the sender lied, not that the entry is
// invalid, and an honest peer may later send the same entry in a heavier
// ledger (audit item G4, D116).
func TestFalseBranchIsFlagged(t *testing.T) {
	a := startNode(t, "a", nil)
	grow(t, a, nil, "alice", 1, 32)
	sec := poolSection()
	r := dialRaw(t, a, []wire.Section{sec})
	verified(t, a, r)
	var x hash.Hash
	rand.Read(x[:])
	huge := weight.FromUint64(1 << 62)
	r.write(wire.Message{Header: wire.Header{Type: wire.TypeNotice, Section: sec, Stated: huge},
		Body: wire.Notice{Tip: x}.Encode()})
	key := ""
	for _, p := range a.h.Peers() {
		key = p.Key
	}
	deadline := time.Now().Add(10 * time.Second)
	for a.h.Tracker().Status(key, time.Now()).FlagTotal == 0 {
		if time.Now().After(deadline) {
			t.Fatal("the false branch was not flagged")
		}
		m, _, err := r.readUntil(time.Now().Add(50 * time.Millisecond))
		if err != nil {
			continue
		}
		switch m.Type {
		case wire.TypeHashChainReq:
			r.reply(m, wire.TypeHashChain, wire.HashChainChunk{FromStart: true, Hashes: []hash.Hash{x, x}}.Encode(), huge)
		case wire.TypeWeightReq:
			r.reply(m, wire.TypeWeight, nil, huge)
		}
	}
	if st := a.h.Tracker().Status(key, time.Now()); st.Priority {
		t.Fatalf("a flagged sender is a priority peer: %+v", st)
	}
	if a.h.ChaseQueue().IsBad(x, time.Now()) {
		t.Fatal("a hash not shown invalid went on the known-bad list")
	}
}

// A node that holds nothing takes in a whole ledger however long its run
// of entries without layers: they add no weight, but the ledger is
// heavier, and no limit may stop a node reaching it (audit item G2,
// D116; this replaces RED-012's bare-run limit).
func TestLongBareRunIsTaken(t *testing.T) {
	a := startNode(t, "a", nil)
	parent := exSeries.PoolID(exKey.Index)
	const bare = 40
	for i := 0; i <= bare; i++ {
		e, _, err := entry.Build(exSeries, parent, nil, 0)
		must(t, err)
		_, err = a.n.HandleEntry(exKey, e, engine.Local)
		must(t, err)
		parent = e.Hash(exSeries)
	}
	_, _, err := a.n.Work(exKey, parent, 0, 32) // a layer on the last only
	must(t, err)
	d := startNode(t, "d", func(c *p2p.Config) { c.Limits.EntryLimit = 4 })
	d.connect(a)
	converged(t, a, d)
	if h := d.status().Height; h != bare+1 {
		t.Fatalf("d's height %d", h)
	}
}

// A split deeper than the old limit of 16 hash chain pages is still
// found, by halving over the node's own chain, and the node switches
// (audit item G3, D116).
func TestDeepSplitIsFound(t *testing.T) {
	small := func(c *p2p.Config) { c.Limits.HashLimit = 2; c.Limits.EntryLimit = 4 }
	a := startNode(t, "a", small)
	b := startNode(t, "b", small)
	last, owner := grow(t, a, nil, "alice", 3, 32)
	copyLedger(t, a, b)
	growTagged(t, b, last, owner, "b", 40, 8) // 20 pages of 2 past the split
	growTagged(t, a, last, owner, "a", 44, 32)
	b.connect(a)
	converged(t, a, b)
}

// A fork switch deeper than the sender's revert budget is not refused: a
// chase whose work was checked first is not charged, and an entry over
// the budget waits instead of being dropped (audit item G1, D116).
func TestForkPastRevertBudget(t *testing.T) {
	small := func(c *p2p.Config) { c.Limits.HashLimit = 4; c.Limits.EntryLimit = 3 }
	tr, err := tracking.New(tracking.Config{LoopbackAsOne: true, Budgets: map[string]int{tracking.KindRevert: 1}}, "")
	must(t, err)
	a := startNode(t, "a", small)
	b := startNode(t, "b", func(c *p2p.Config) { small(c); c.Tracker = tr })
	last, owner := grow(t, a, nil, "alice", 5, 32)
	copyLedger(t, a, b)
	growTagged(t, b, last, owner, "b", 8, 32)
	growTagged(t, a, last, owner, "a", 20, 32)
	b.connect(a)
	converged(t, a, b)
}

// A chase that runs past its time slice yields and goes on later with
// what it took in kept, until the node reaches the heavier ledger (audit
// items P2 to P4, D116).
func TestChaseYieldsAndGoesOn(t *testing.T) {
	small := func(c *p2p.Config) {
		c.Limits.HashLimit = 2
		c.Limits.EntryLimit = 2
		c.ChaseSlice = time.Millisecond
		c.Chase.YieldWait = 10 * time.Millisecond
	}
	a := startNode(t, "a", small)
	b := startNode(t, "b", small)
	last, owner := grow(t, a, nil, "alice", 3, 32)
	copyLedger(t, a, b)
	growTagged(t, b, last, owner, "b", 6, 8)
	growTagged(t, a, last, owner, "a", 24, 32)
	b.connect(a)
	converged(t, a, b)
}

// RED-012: an honest fork switch taken in order is never flagged, even
// with a revert budget set.
func TestChaseInOrderCostsLittleRevert(t *testing.T) {
	small := func(c *p2p.Config) { c.Limits.HashLimit = 4; c.Limits.EntryLimit = 3 }
	tr, err := tracking.New(tracking.Config{LoopbackAsOne: true, Budgets: map[string]int{tracking.KindRevert: 20}}, "")
	must(t, err)
	a := startNode(t, "a", small)
	b := startNode(t, "b", func(c *p2p.Config) { small(c); c.Tracker = tr })
	last, owner := grow(t, a, nil, "alice", 5, 32)
	copyLedger(t, a, b)
	growTagged(t, b, last, owner, "b", 3, 32)
	growTagged(t, a, last, owner, "a", 14, 32) // heavier: b reverts 3 and takes 14
	b.connect(a)
	converged(t, a, b)
	if st := tr.Status("127.0.0.1:1", time.Now()); st.FlagTotal != 0 {
		t.Fatalf("an honest switch was flagged: %+v", st)
	}
}
