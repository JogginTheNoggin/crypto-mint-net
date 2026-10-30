package p2p_test

import (
	"crypto/rand"
	"testing"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/engine"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/entry"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/ledger"
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
// flagged and the hash goes on the known-bad list, so the same claim from
// anyone is ignored.
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
	for !a.h.ChaseQueue().IsBad(x, time.Now()) {
		if time.Now().After(deadline) {
			t.Fatal("the false branch was not marked")
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
	if st := a.h.Tracker().Status(key, time.Now()); st.FlagTotal == 0 || st.Priority {
		t.Fatalf("sender not flagged: %+v", st)
	}
}

// RED-012: a node that holds nothing takes in a whole ledger only while
// its entries bring layers: a run of more than MaxBareRun entries without
// any ends the fetch.
func TestBareRunEndsFetch(t *testing.T) {
	for _, c := range []struct {
		bare, held int
		selected   bool
	}{{6, 7, true}, {10, 6, false}} {
		a := startNode(t, "a", nil)
		parent := exSeries.PoolID(exKey.Index)
		for i := 0; i <= c.bare; i++ {
			e, _, err := entry.Build(exSeries, parent, nil, 0)
			must(t, err)
			_, err = a.n.HandleEntry(exKey, e, engine.Local)
			must(t, err)
			parent = e.Hash(exSeries)
		}
		_, _, err := a.n.Work(exKey, parent, 0, 32) // a layer on the last only
		must(t, err)
		if a.status().Height != c.bare+1 {
			t.Fatalf("a's height %d", a.status().Height)
		}
		d := startNode(t, "d", func(c *p2p.Config) { c.Limits.EntryLimit = 4; c.MaxBareRun = 6 })
		d.connect(a)
		held := func() (n int) {
			d.n.Engine().Ledger(exKey, func(l *ledger.Pool) { n = l.Len() })
			return n
		}
		eventually(t, "d to take in entries", func() bool { return held() >= c.held })
		time.Sleep(300 * time.Millisecond)
		if got := held(); got != c.held {
			t.Fatalf("%d bare entries: d holds %d, want %d", c.bare, got, c.held)
		}
		if got := d.status().Height > 0; got != c.selected {
			t.Fatalf("%d bare entries: d selected %v", c.bare, got)
		}
	}
}

// RED-012: requested entries pay the revert budget too, but a catch-up
// and a fork switch taken in order cost next to nothing: each entry
// builds on the state the last one left. Here the fork costs 6 units (3
// entries of one transaction reverted), within a budget of 20, and a
// budget of 5 stops it.
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
