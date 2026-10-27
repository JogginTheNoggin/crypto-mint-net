package p2p_test

import (
	"bufio"
	"crypto/rand"
	"net"
	"testing"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/encoding"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/entry"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/ledger"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/p2p"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tracking"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tx"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/weight"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/wire"
)

// rawPeer is a hand-driven connection to a node, for checking exactly what
// the node sends.
type rawPeer struct {
	t        *testing.T
	c        net.Conn
	r        *bufio.Reader
	n        uint64
	coverage []wire.Section
	silent   bool // do not answer the node's follow requests and pings
}

func poolSection() wire.Section {
	return wire.PoolSection(hash.Zero, exSeries.ID(), exSeries.PoolID(exKey.Index))
}

// dialRaw connects to tn and exchanges hellos. The raw peer answers the
// node's follow requests and pings with a yes for anything its coverage
// includes, as it reads.
func dialRaw(t *testing.T, tn *tnode, coverage []wire.Section) *rawPeer {
	t.Helper()
	c, err := net.Dial("tcp", tn.h.Addr())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	var id hash.Hash
	rand.Read(id[:])
	hello := wire.Hello{Protocol: 1, NodeID: id, Agent: "raw"}
	r := &rawPeer{t: t, c: c, r: bufio.NewReader(c), coverage: coverage}
	r.write(wire.Message{Header: wire.Header{Type: wire.TypeHello}, Body: hello.Encode()})
	m, _, err := r.read()
	if err != nil || m.Type != wire.TypeHello {
		t.Fatalf("no hello back: %v %v", m.Type, err)
	}
	return r
}

func (r *rawPeer) write(m wire.Message) {
	r.t.Helper()
	if err := wire.WriteFrame(r.c, m); err != nil {
		r.t.Fatal(err)
	}
}

func (r *rawPeer) read() (wire.Message, int, error) {
	return r.readUntil(time.Now().Add(5 * time.Second))
}

// readUntil reads the next message the test should see, answering the
// node's follow requests and pings on the way.
func (r *rawPeer) readUntil(deadline time.Time) (wire.Message, int, error) {
	for {
		r.c.SetReadDeadline(deadline)
		m, n, err := wire.ReadFrame(r.r, wire.MaxFrame)
		if err != nil || r.silent || (m.Type != wire.TypeFollowReq && m.Type != wire.TypePing) {
			return m, n, err
		}
		for _, c := range r.coverage {
			if c.Contains(m.Section) {
				t := wire.TypeFollowYes
				body := wire.FollowReply{}.Encode()
				if m.Type == wire.TypePing {
					t, body = wire.TypePong, wire.PongReply{}.Encode()
				}
				wire.WriteFrame(r.c, wire.Message{Header: wire.Header{Type: t, Section: m.Section, RequestID: m.RequestID, Chunk: 1, Flags: wire.FlagLast}, Body: body})
				break
			}
		}
	}
}

func (r *rawPeer) common(l wire.Limits) wire.RequestCommon {
	r.n++
	return wire.RequestCommon{Nonce: r.n, Time: uint64(time.Now().Unix()), Limits: l}
}

// ask sends a request and reads its whole reply, skipping pushed items.
func (r *rawPeer) ask(s wire.Section, q wire.Request) []wire.Message {
	r.t.Helper()
	body := q.Encode()
	id := wire.RequestID(q.Type(), s, body)
	r.write(wire.Message{Header: wire.Header{Type: q.Type(), Section: s, RequestID: id}, Body: body})
	var out []wire.Message
	for {
		m, _, err := r.read()
		if err != nil {
			r.t.Fatalf("reading reply to %s: %v", q.Type(), err)
		}
		if m.RequestID != id {
			continue
		}
		if m.Chunk != uint32(len(out)+1) {
			r.t.Fatalf("chunk %d, want %d", m.Chunk, len(out)+1)
		}
		out = append(out, m)
		if m.Last() {
			return out
		}
	}
}

// expectClosed reads until the node closes the connection.
func (r *rawPeer) expectClosed() {
	r.t.Helper()
	for {
		if _, _, err := r.read(); err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				r.t.Fatal("connection still open")
			}
			return
		}
	}
}

func chainOf(t *testing.T, tn *tnode) []hash.Hash {
	var hs []hash.Hash
	tn.n.Engine().Ledger(exKey, func(l *ledger.Pool) { hs = l.Chain(1, l.Height()) })
	return hs
}

func hashChain(t *testing.T, ms []wire.Message) ([]hash.Hash, bool) {
	t.Helper()
	var hs []hash.Hash
	from := false
	for i, m := range ms {
		if m.Type != wire.TypeHashChain {
			t.Fatalf("reply %s", m.Type)
		}
		c, err := wire.DecodeHashChainChunk(m.Body)
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			from = c.FromStart
		}
		hs = append(hs, c.Hashes...)
	}
	return hs, from
}

func sameHashes(a, b []hash.Hash) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestServe(t *testing.T) {
	a := startNode(t, "a", nil)
	grow(t, a, nil, "alice", 10, 16)
	chain := chainOf(t, a)
	st := a.status()
	sec := poolSection()
	r := dialRaw(t, a, []wire.Section{sec})
	none := wire.Limits{}

	t.Run("weight", func(t *testing.T) {
		ms := r.ask(sec, wire.WeightReq{RequestCommon: r.common(none)})
		if len(ms) != 1 || ms[0].Type != wire.TypeWeight || ms[0].Stated.Cmp(st.Weight) != 0 {
			t.Fatalf("got %s stated %s", ms[0].Type, ms[0].Stated)
		}
	})
	t.Run("hash chain", func(t *testing.T) {
		hs, from := hashChain(t, r.ask(sec, wire.HashChainReq{RequestCommon: r.common(none), Count: 3}))
		if !sameHashes(hs, chain[7:]) || from {
			t.Fatalf("latest 3: %d hashes, from start %v", len(hs), from)
		}
		ms := r.ask(sec, wire.HashChainReq{RequestCommon: r.common(wire.Limits{HashLimit: 4})})
		hs, from = hashChain(t, ms)
		if !sameHashes(hs, chain[:4]) || !from || !ms[len(ms)-1].More() {
			t.Fatalf("whole chain cut at 4: %d hashes, from start %v", len(hs), from)
		}
		hs, from = hashChain(t, r.ask(sec, wire.HashChainReq{RequestCommon: r.common(none), HasAnchor: true, Anchor: chain[4], Dir: wire.Before, Count: 10}))
		if !sameHashes(hs, chain[:4]) || !from {
			t.Fatalf("before: %d hashes", len(hs))
		}
		hs, _ = hashChain(t, r.ask(sec, wire.HashChainReq{RequestCommon: r.common(none), HasAnchor: true, Anchor: chain[7], Dir: wire.After}))
		if !sameHashes(hs, chain[8:]) {
			t.Fatalf("after: %d hashes", len(hs))
		}
		ms = r.ask(sec, wire.HashChainReq{RequestCommon: r.common(none), HasAnchor: true, Anchor: hash.Hash{1}, Dir: wire.After})
		if ms[0].Type != wire.TypeAnchorNotHeld {
			t.Fatalf("unknown anchor: %s", ms[0].Type)
		}
		// One hash per chunk when the chunk size is tiny.
		ms = r.ask(sec, wire.HashChainReq{RequestCommon: r.common(wire.Limits{ChunkSize: 1}), Count: 3})
		if len(ms) != 3 {
			t.Fatalf("%d chunks", len(ms))
		}
	})
	t.Run("extension", func(t *testing.T) {
		ms := r.ask(sec, wire.ExtensionReq{RequestCommon: r.common(none), From: chain[9], Beat: st.Weight})
		if ms[0].Type != wire.TypeNothingHeavier || ms[0].Stated.Cmp(st.Weight) != 0 {
			t.Fatalf("beat own weight: %s", ms[0].Type)
		}
		ms = r.ask(sec, wire.ExtensionReq{RequestCommon: r.common(none), From: hash.Hash{1}, Beat: weight.Zero})
		if ms[0].Type != wire.TypeNotFromHere {
			t.Fatalf("unknown entry: %s", ms[0].Type)
		}
		ms = r.ask(sec, wire.ExtensionReq{RequestCommon: r.common(none), From: chain[6], Beat: weight.Zero, Variant: wire.EntriesWithLayers})
		c, err := wire.DecodeEntriesChunk(ms[0].Body)
		if err != nil || ms[0].Type != wire.TypeExtension || len(c.Items) != 3 || ms[0].More() {
			t.Fatalf("extension: %s %d items %v", ms[0].Type, len(c.Items), err)
		}
		if c.Items[0].Entry.Hash(exSeries) != chain[7] || !c.Items[0].HasLayers || len(c.Items[0].Nonces) != 1 {
			t.Fatal("extension items wrong")
		}
	})
	t.Run("full ledger", func(t *testing.T) {
		ms := r.ask(sec, wire.FullLedgerReq{RequestCommon: r.common(wire.Limits{ChunkSize: 1, EntryLimit: 4})})
		if len(ms) != 4 || !ms[3].More() {
			t.Fatalf("%d chunks, more %v", len(ms), ms[len(ms)-1].More())
		}
		for i, m := range ms {
			c, err := wire.DecodeEntriesChunk(m.Body)
			if err != nil || len(c.Items) != 1 || c.Items[0].Entry.Hash(exSeries) != chain[i] || c.Items[0].HasLayers {
				t.Fatalf("chunk %d wrong (%v)", i, err)
			}
		}
	})
	t.Run("entries and layers by hash", func(t *testing.T) {
		ms := r.ask(sec, wire.EntriesReq{RequestCommon: r.common(none), Hashes: []hash.Hash{chain[2], {1}}})
		c, err := wire.DecodeHeldEntriesChunk(ms[0].Body)
		if err != nil || len(c.Items) != 2 || !c.Items[0].Held || c.Items[1].Held || c.Items[0].Item.Entry.Hash(exSeries) != chain[2] {
			t.Fatalf("entries by hash wrong (%v)", err)
		}
		ms = r.ask(sec, wire.LayersReq{RequestCommon: r.common(none), Hashes: []hash.Hash{chain[2], {1}}})
		lc, err := wire.DecodeLayersChunk(ms[0].Body)
		if err != nil || len(lc.Items) != 2 || !lc.Items[0].Held || len(lc.Items[0].Nonces) != 1 || lc.Items[1].Held {
			t.Fatalf("layers wrong (%v)", err)
		}
	})
	t.Run("refused and not held", func(t *testing.T) {
		q := wire.WeightReq{RequestCommon: r.common(none)}
		r.ask(sec, q)
		if ms := r.ask(sec, q); ms[0].Type != wire.TypeRefused {
			t.Fatalf("repeated request: %s", ms[0].Type)
		}
		old := wire.WeightReq{RequestCommon: r.common(none)}
		old.Time -= 3600
		if ms := r.ask(sec, old); ms[0].Type != wire.TypeRefused {
			t.Fatalf("old request: %s", ms[0].Type)
		}
		other := wire.PoolSection(hash.Zero, exSeries.ID(), exSeries.PoolID(0))
		if ms := r.ask(other, wire.WeightReq{RequestCommon: r.common(none)}); ms[0].Type != wire.TypeNotHeld {
			t.Fatalf("unfollowed pool: %s", ms[0].Type)
		}
	})
}

func TestMalformedMessageDisconnects(t *testing.T) {
	a := startNode(t, "a", nil)
	sec := poolSection()
	r := dialRaw(t, a, []wire.Section{sec})
	r.write(wire.Message{Header: wire.Header{Type: wire.TypeEntry, Section: sec}, Body: []byte{1, 2, 3}})
	r.expectClosed()

	r = dialRaw(t, a, []wire.Section{sec})
	body := wire.WeightReq{RequestCommon: r.common(wire.Limits{})}.Encode()
	r.write(wire.Message{Header: wire.Header{Type: wire.TypeWeightReq, Section: sec, RequestID: hash.Hash{9}}, Body: body})
	r.expectClosed()

	r = dialRaw(t, a, []wire.Section{sec})
	r.write(wire.Message{Header: wire.Header{Type: wire.TypeHello}, Body: wire.Hello{Protocol: 1}.Encode()})
	r.expectClosed()

	// Raw bytes that are not a message at all.
	r = dialRaw(t, a, []wire.Section{sec})
	w := encoding.NewWriter(8)
	w.U32(3).Raw([]byte{9, 9, 9})
	r.c.Write(w.Bytes())
	r.expectClosed()
}

// An invalid entry is an offence: a flag that bars priority. A second one
// passes the timeout limit, and the address is timed out: disconnected and
// refused until the timeout ends.
func TestInvalidEntriesTimeOut(t *testing.T) {
	tr, err := tracking.New(tracking.Config{LoopbackAsOne: true}, "")
	must(t, err)
	a := startNode(t, "a", func(c *p2p.Config) { c.Tracker = tr })
	sec := poolSection()
	r := dialRaw(t, a, []wire.Section{sec})
	bad := func(serial uint64) wire.Message {
		claim := exKeys.Claim(serial, P("alice", 100))
		claim.Outputs[0].Amount = 99 // breaks the claim: the supply is 100
		e, _, err := entry.Build(exSeries, exSeries.PoolID(exKey.Index), []*tx.Tx{claim}, 0)
		must(t, err)
		return wire.Message{Header: wire.Header{Type: wire.TypeEntry, Section: sec}, Body: e.Encode()}
	}
	r.write(bad(4))
	eventually(t, "a flag", func() bool { return tr.Status("127.0.0.1:1", time.Now()).FlagTotal == 5 })
	if st := tr.Status("127.0.0.1:1", time.Now()); st.Priority || !st.Allowed() {
		t.Fatalf("after one invalid entry: %+v", st)
	}
	r.write(bad(5))
	r.expectClosed()
	if a.status().Height != 0 {
		t.Fatal("invalid entry kept")
	}
	if st := tr.Status("127.0.0.1:1", time.Now()); !st.TimedOut {
		t.Fatalf("not timed out: %+v", st)
	}
	// A new connection from the address is refused while it lasts.
	c, err := net.Dial("tcp", a.h.Addr())
	must(t, err)
	defer c.Close()
	c.SetReadDeadline(time.Now().Add(5 * time.Second))
	if _, err := c.Read(make([]byte, 1)); err == nil {
		t.Fatal("a timed out address was answered")
	}
}

// What a node serves is counted against the asker, a whole ledger as a
// large request; asking the same request again is a repeat and refused.
func TestServedCostAndRepeats(t *testing.T) {
	tr, err := tracking.New(tracking.Config{LoopbackAsOne: true, RepeatLimit: 1, Default: tracking.StandardName}, "")
	must(t, err)
	a := startNode(t, "a", func(c *p2p.Config) { c.Tracker = tr })
	grow(t, a, nil, "alice", 2, 32)
	sec := poolSection()
	r := dialRaw(t, a, []wire.Section{sec})
	q := wire.FullLedgerReq{RequestCommon: r.common(wire.Limits{})}
	if ms := r.ask(sec, q); ms[0].Type != wire.TypeExtension {
		t.Fatalf("full ledger answered %s", ms[0].Type)
	}
	rep, _ := tr.Report("127.0.0.1", time.Now())
	pr := rep.Policies[sec.Text()]
	if pr.Served == 0 || pr.Large != 1 {
		t.Fatalf("cost not counted: %+v in %+v", pr, rep)
	}
	if ms := r.ask(sec, q); ms[0].Type != wire.TypeRefused {
		t.Fatalf("a repeated request answered %s", ms[0].Type)
	}
	if ms := r.ask(sec, q); ms[0].Type != wire.TypeRefused {
		t.Fatalf("a repeated request answered %s", ms[0].Type)
	}
	if st := tr.Status("127.0.0.1", time.Now()); st.FlagTotal == 0 {
		t.Fatalf("repeats past the limit not flagged: %+v", st)
	}
}
