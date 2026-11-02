package p2p_test

import (
	"crypto/rand"
	"testing"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
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
