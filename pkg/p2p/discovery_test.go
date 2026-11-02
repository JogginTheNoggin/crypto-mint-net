package p2p_test

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/p2p"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/peers"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/registry"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/wire"
)

// peerAddrs lists the advertised addresses of tn's connected peers.
func peerAddrs(tn *tnode) map[string]bool {
	out := map[string]bool{}
	for _, p := range tn.h.Peers() {
		out[p.Listen] = true
	}
	return out
}

func (tn *tnode) addr() string { return tn.h.Addr() }

func (tn *tnode) bookState(addr string) (peers.State, bool) {
	for _, r := range tn.h.Book().Records() {
		if r.Addr == addr {
			for _, p := range r.Policies {
				if p.Section == poolSection() {
					return p.State, true
				}
			}
		}
	}
	return 0, false
}

// A broadcast follow request travels along a line of nodes; the replies
// come back along it, and the asker then connects to the repliers itself.
func TestRelayedDiscovery(t *testing.T) {
	a, b, c := startNode(t, "a", nil), startNode(t, "b", nil), startNode(t, "c", nil)
	a.connect(b)
	b.connect(c)
	d := startNode(t, "d", nil)
	d.connect(a)
	if n := d.h.Broadcast(poolSection()); n != 3 {
		t.Fatalf("broadcast added %d candidates, want a, b and c", n)
	}
	for _, o := range []*tnode{b, c} {
		if st, ok := d.bookState(o.addr()); !ok || st != peers.Candidate {
			t.Fatalf("%s: %v %v", o.name, st, ok)
		}
	}
	d.h.Manage()
	eventually(t, "d to peer with b and c", func() bool {
		ps := peerAddrs(d)
		return len(ps) == 3 && ps[b.addr()] && ps[c.addr()]
	})
	if st, _ := d.bookState(c.addr()); !st.Listed() {
		t.Fatalf("c is %v in d's book", st)
	}
}

// A node that does not relay, or a hop limit, stops a broadcast.
func TestRelayLimits(t *testing.T) {
	for _, tc := range []struct {
		name string
		b    func(*p2p.Config)
	}{
		{"no relay", func(c *p2p.Config) { c.NoRelay = true }},
		{"hop limit", func(c *p2p.Config) { c.HopLimit = 1 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, b, c := startNode(t, "a", nil), startNode(t, "b", tc.b), startNode(t, "c", nil)
			a.connect(b)
			b.connect(c)
			d := startNode(t, "d", nil)
			d.connect(a)
			d.h.Broadcast(poolSection())
			if _, ok := d.bookState(b.addr()); !ok {
				t.Fatal("b not found")
			}
			if _, ok := d.bookState(c.addr()); ok {
				t.Fatal("c found past the relay limit")
			}
		})
	}
}

// A peer list from a peer gives candidates; the connection manager asks
// for one when it runs short and then dials what it learned.
func TestPeerListAndManager(t *testing.T) {
	a := startNode(t, "a", func(c *p2p.Config) { c.NoRelay = true })
	b, c := startNode(t, "b", nil), startNode(t, "c", nil)
	a.connect(b)
	a.connect(c)
	d := startNode(t, "d", func(c *p2p.Config) { c.Slots = peers.Slots{Priority: 2, Backup: 1} })
	d.connect(a)
	d.h.Manage() // asks a for its peers
	eventually(t, "candidates from a's peer list", func() bool {
		_, okb := d.bookState(b.addr())
		_, okc := d.bookState(c.addr())
		return okb && okc
	})
	d.h.Manage() // dials them
	eventually(t, "d to peer with b and c", func() bool { return len(d.h.Peers()) == 3 })
}

// A node with no peers starts from an address registry.
func TestRegistryBootstrap(t *testing.T) {
	a := startNode(t, "a", nil)
	grow(t, a, nil, "alice", 2, 32)
	doc, err := registry.Encode([]registry.Entry{{Addr: a.addr(), Follows: []wire.Section{wire.SeriesSection(hash.Zero, exSeries.ID())}}})
	must(t, err)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(doc) }))
	defer srv.Close()
	d := startNode(t, "d", func(c *p2p.Config) { c.Registries = []string{srv.URL} })
	if n := d.h.LoadRegistries(context.Background()); n != 1 {
		t.Fatalf("registry gave %d candidates", n)
	}
	d.h.Manage()
	converged(t, a, d)
}

// The manager loads the registries itself when it has no candidates.
func TestManagerLoadsRegistry(t *testing.T) {
	a := startNode(t, "a", nil)
	doc, _ := registry.Encode([]registry.Entry{{Addr: a.addr(), Follows: []wire.Section{poolSection()}}})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(doc) }))
	defer srv.Close()
	d := startNode(t, "d", func(c *p2p.Config) {
		c.Registries = []string{srv.URL}
		c.ManageEvery = 50 * time.Millisecond
	})
	eventually(t, "d to find a through the registry", func() bool { return len(d.h.Peers()) == 1 })
}

// A node that finds its own address among the candidates archives it.
func TestOwnAddressIsArchived(t *testing.T) {
	a := startNode(t, "a", nil)
	other := freeAddr(t)
	a.h.Book().Add(other, []wire.Section{poolSection()}, peers.FromRegistry, time.Now())
	// Pretend a also listens at other: a second listener that forwards to a.
	stop := forwardPort(t, other, a.addr())
	defer stop()
	a.h.Manage()
	eventually(t, "the self address to be archived", func() bool {
		st, _ := a.bookState(other)
		return st == peers.Archived
	})
	if len(a.h.Peers()) != 0 {
		t.Fatal("connected to self")
	}
}

// forwardPort accepts on from and pipes each connection to to.
func forwardPort(t *testing.T, from, to string) func() {
	ln, err := netListen(from)
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go pipe(c, to)
		}
	}()
	return func() { ln.Close() }
}

// A peer that stops answering pings stops being a peer.
func TestSilentPeerIsDropped(t *testing.T) {
	a := startNode(t, "a", func(c *p2p.Config) {
		c.PingEvery = 50 * time.Millisecond
		c.FollowTimeout = 100 * time.Millisecond
		c.MaxMisses = 2
	})
	r := dialRaw(t, a, []wire.Section{poolSection()})
	r.replies(hash.Zero, 200*time.Millisecond) // answers a's follow request
	eventually(t, "the raw peer to be verified", func() bool { return len(a.h.Peers()) == 1 && len(a.h.Peers()[0].Coverage) == 1 })
	r.silent = true
	r.expectClosed()
}

// A dropped policy update ends the peering for that policy.
func TestDroppedPolicy(t *testing.T) {
	a := startNode(t, "a", nil)
	r := dialRaw(t, a, []wire.Section{poolSection()})
	r.replies(hash.Zero, 200*time.Millisecond)
	eventually(t, "the raw peer to be verified", func() bool { return len(a.h.Peers()) == 1 && len(a.h.Peers()[0].Coverage) == 1 })
	r.write(wire.Message{Header: wire.Header{Type: wire.TypeDroppedPolicy, Section: poolSection()}})
	r.expectClosed()
}

// rawAsk sends a discovery request and returns its reply chunks, or none
// after a short wait.
func (r *rawPeer) rawAsk(t wire.Type, s wire.Section, body []byte) []wire.Message {
	r.t.Helper()
	id := wire.RequestID(t, s, body)
	r.write(wire.Message{Header: wire.Header{Type: t, Section: s, RequestID: id}, Body: body})
	return r.replies(id, 300*time.Millisecond)
}

func (r *rawPeer) replies(id hash.Hash, wait time.Duration) []wire.Message {
	var out []wire.Message
	deadline := time.Now().Add(wait)
	for time.Now().Before(deadline) {
		m, _, err := r.readUntil(deadline)
		if err != nil {
			break
		}
		if m.RequestID == id {
			out = append(out, m)
			if m.Last() {
				break
			}
		}
	}
	return out
}

func now() uint64 { return uint64(time.Now().Unix()) }

func TestFollowAndPingReplies(t *testing.T) {
	a := startNode(t, "a", nil)
	r := dialRaw(t, a, []wire.Section{poolSection()})
	series := wire.SeriesSection(hash.Zero, exSeries.ID())
	// a follows pool 1 only: a partial yes for the series, a yes for the
	// pool and for a mint inside it, silence for another pool.
	ms := r.rawAsk(wire.TypeFollowReq, series, wire.FollowReq{Nonce: 1, Time: now()}.Encode())
	if len(ms) != 1 || ms[0].Type != wire.TypePartialYes {
		t.Fatalf("series: %v", ms)
	}
	rep, _ := wire.DecodeFollowReply(ms[0].Body)
	if len(rep.Policies) != 1 || rep.Policies[0] != poolSection() || rep.Contact.Addr != a.addr() {
		t.Fatalf("partial yes %+v", rep)
	}
	ms = r.rawAsk(wire.TypeFollowReq, wire.MintSection(hash.Zero, exSeries.ID(), exSeries.PoolID(1), 5), wire.FollowReq{Nonce: 2, Time: now()}.Encode())
	if len(ms) != 1 || ms[0].Type != wire.TypeFollowYes {
		t.Fatalf("mint: %v", ms)
	}
	other := wire.PoolSection(hash.Zero, exSeries.ID(), exSeries.PoolID(0))
	if ms := r.rawAsk(wire.TypeFollowReq, other, wire.FollowReq{Nonce: 3, Time: now()}.Encode()); len(ms) != 0 {
		t.Fatalf("unfollowed pool answered: %v", ms)
	}
	// A repeated or stale request is dropped.
	body := wire.FollowReq{Nonce: 4, Time: now()}.Encode()
	r.rawAsk(wire.TypeFollowReq, poolSection(), body)
	if ms := r.rawAsk(wire.TypeFollowReq, poolSection(), body); len(ms) != 0 {
		t.Fatal("repeated request answered")
	}
	if ms := r.rawAsk(wire.TypeFollowReq, poolSection(), wire.FollowReq{Nonce: 5, Time: now() - 7200}.Encode()); len(ms) != 0 {
		t.Fatal("stale request answered")
	}
	ms = r.rawAsk(wire.TypePing, series, wire.Ping{Nonce: 6, Time: now()}.Encode())
	if len(ms) != 1 || ms[0].Type != wire.TypePartialPong {
		t.Fatalf("ping for the series: %v", ms)
	}
	ms = r.rawAsk(wire.TypePing, poolSection(), wire.Ping{Nonce: 7, Time: now()}.Encode())
	if len(ms) != 1 || ms[0].Type != wire.TypePong {
		t.Fatalf("ping for the pool: %v", ms)
	}
}

func TestRequestRateLimit(t *testing.T) {
	a := startNode(t, "a", func(c *p2p.Config) { c.RequestRate = 5 })
	r := dialRaw(t, a, []wire.Section{poolSection()})
	answered := 0
	for i := 0; i < 10; i++ {
		if ms := r.rawAsk(wire.TypePing, poolSection(), wire.Ping{Nonce: uint64(100 + i), Time: now()}.Encode()); len(ms) > 0 {
			answered++
		}
	}
	if answered > 5 {
		t.Fatalf("%d of 10 pings answered with a rate of 5 a minute", answered)
	}
}

// A peer list reply comes in chunks; one lost chunk can be asked for again.
func TestPeerListChunksAndMissing(t *testing.T) {
	a := startNode(t, "a", nil)
	b, c := startNode(t, "b", nil), startNode(t, "c", nil)
	a.connect(b)
	a.connect(c)
	r := dialRaw(t, a, []wire.Section{poolSection()})
	q := wire.PeerListReq{Nonce: 1, Time: now(), Variant: wire.AllPeers, Limits: wire.DiscoveryLimits{ChunkSize: 1, PeerListLimit: 10}}
	body := q.Encode()
	ms := r.rawAsk(wire.TypePeerListReq, poolSection(), body)
	if len(ms) != 2 || !ms[1].Last() {
		t.Fatalf("%d chunks", len(ms))
	}
	got := map[string]bool{}
	for _, m := range ms {
		pc, err := wire.DecodePeerListChunk(m.Body)
		if err != nil || len(pc.Peers) != 1 || len(pc.Peers[0].Policies) != 1 {
			t.Fatalf("chunk %+v %v", pc, err)
		}
		got[pc.Peers[0].Contact.Addr] = true
	}
	if !got[b.addr()] || !got[c.addr()] {
		t.Fatalf("listed %v", got)
	}
	id := wire.RequestID(wire.TypePeerListReq, poolSection(), body)
	r.write(wire.Message{Header: wire.Header{Type: wire.TypeMissingChunksReq, Section: poolSection(), RequestID: id},
		Body: wire.MissingChunksReq{Positions: []uint32{2}}.Encode()})
	again := r.replies(id, 300*time.Millisecond)
	if len(again) != 1 || again[0].Chunk != 2 || string(again[0].Body) != string(ms[1].Body) {
		t.Fatalf("missing chunk: %v", again)
	}
	// Full peers only lists peers that follow all of the section.
	series := wire.SeriesSection(hash.Zero, exSeries.ID())
	q2 := wire.PeerListReq{Nonce: 2, Time: now(), Variant: wire.FullPeers, Limits: wire.DiscoveryLimits{PeerListLimit: 10}}
	ms = r.rawAsk(wire.TypePeerListReq, series, q2.Encode())
	pc, _ := wire.DecodePeerListChunk(ms[0].Body)
	if len(pc.Peers) != 2 {
		t.Fatalf("full peers for the part a follows: %d", len(pc.Peers))
	}
}

// A direct follow request with peers carries them in the reply.
func TestFollowWithPeers(t *testing.T) {
	a := startNode(t, "a", nil)
	b := startNode(t, "b", nil)
	a.connect(b)
	r := dialRaw(t, a, []wire.Section{poolSection()})
	q := wire.FollowReq{Nonce: 1, Time: now(), Variant: wire.AllPeers, Limits: wire.DiscoveryLimits{PeerListLimit: 5}}
	ms := r.rawAsk(wire.TypeFollowReq, poolSection(), q.Encode())
	rep, err := wire.DecodeFollowReply(ms[0].Body)
	if err != nil || len(rep.Peers) != 1 || rep.Peers[0].Contact.Addr != b.addr() {
		t.Fatalf("%+v %v", rep, err)
	}
	q.Nonce, q.Limits.PeerListLimit = 2, 0
	ms = r.rawAsk(wire.TypeFollowReq, poolSection(), q.Encode())
	if rep, _ := wire.DecodeFollowReply(ms[0].Body); len(rep.Peers) != 0 {
		t.Fatal("peers sent with a peer list limit of 0")
	}
}

func netListen(addr string) (net.Listener, error) { return net.Listen("tcp", addr) }

func pipe(c net.Conn, to string) {
	defer c.Close()
	d, err := net.Dial("tcp", to)
	if err != nil {
		return
	}
	defer d.Close()
	go io.Copy(d, c)
	io.Copy(c, d)
}
