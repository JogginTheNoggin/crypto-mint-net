package p2p_test

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/engine"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/entry"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/layer"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/ledger"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/node"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/nodepolicy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/p2p"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/series"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/testutil"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tx"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/wire"
)

var P = testutil.P

var (
	exSeries = testutil.ExampleSeries()
	exKeys   = testutil.NewKeys(exSeries)
	exKey    = engine.PoolKey{Series: exSeries.ID(), Index: 1}
)

// tnode is a node and its host in a test.
type tnode struct {
	t      *testing.T
	name   string
	n      *node.Node
	h      *p2p.Host
	cancel context.CancelFunc
	done   chan struct{}
	log    *syncBuf

	mu     sync.Mutex
	events []node.Event
}

type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuf) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// startNode opens a node that follows pool 1 of the example series and
// runs a host for it on a loopback port.
func startNode(t *testing.T, name string, edit func(*p2p.Config)) *tnode {
	t.Helper()
	id := exSeries.ID()
	pol := nodepolicy.Default()
	pol.Series = []nodepolicy.SeriesInfo{{Params: exSeries.Params()}}
	pol.Follow = []nodepolicy.Follow{{Series: &id, Pools: []uint64{1}}}
	pol.RPC.Listen = "off"
	pol.NoSync = true
	dir := t.TempDir()
	if err := nodepolicy.Save(filepath.Join(dir, nodepolicy.FileName), pol); err != nil {
		t.Fatal(err)
	}
	n, err := node.Open(dir, node.Options{})
	if err != nil {
		t.Fatal(err)
	}
	tn := &tnode{t: t, name: name, n: n, done: make(chan struct{}), log: &syncBuf{}}
	n.Observe(func(ev node.Event) {
		tn.mu.Lock()
		tn.events = append(tn.events, ev)
		tn.mu.Unlock()
	})
	addr := freeAddr(t)
	// Every test node is on loopback, so peers must be allowed to name it.
	cfg := p2p.Config{Listen: addr, Advertise: addr, AllowLocalPeers: true, CheckEvery: -1, ManageEvery: -1, PingEvery: -1,
		RequestTimeout: 5 * time.Second, FollowTimeout: 500 * time.Millisecond, Log: tn.log}
	if edit != nil {
		edit(&cfg)
	}
	h, err := p2p.New(n, cfg)
	if err != nil {
		t.Fatal(err)
	}
	tn.h = h
	ctx, cancel := context.WithCancel(context.Background())
	tn.cancel = cancel
	ready := make(chan struct{})
	go func() {
		defer close(tn.done)
		if err := h.Run(ctx, func() { close(ready) }); err != nil {
			t.Error(err)
			close(ready)
		}
	}()
	<-ready
	t.Cleanup(func() {
		tn.stop()
		n.Close()
		if t.Failed() {
			t.Logf("%s log:\n%s", name, tn.log)
		}
	})
	return tn
}

// freeAddr returns a loopback address with a port free a moment ago, so a
// node can advertise the address it listens on.
func freeAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().String()
}

func (tn *tnode) stop() {
	tn.cancel()
	<-tn.done
}

func (tn *tnode) connect(o *tnode) {
	tn.t.Helper()
	if err := tn.h.Connect(o.h.Addr()); err != nil {
		tn.t.Fatalf("%s → %s: %v", tn.name, o.name, err)
	}
}

func (tn *tnode) status() engine.PoolStatus {
	st, err := tn.n.Engine().Status(exKey)
	if err != nil {
		tn.t.Fatal(err)
	}
	return st
}

// state is what two converged nodes agree on.
func (tn *tnode) state() string {
	st := tn.status()
	return fmt.Sprintf("tip %s height %d weight %s", st.Tip.Short(), st.Height, st.Weight)
}

func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func converged(t *testing.T, nodes ...*tnode) {
	t.Helper()
	eventually(t, "nodes to agree", func() bool {
		for _, o := range nodes[1:] {
			if o.state() != nodes[0].state() {
				return false
			}
		}
		return true
	})
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

// grow adds entries to tn's selected ledger: each moves the portion of
// serial 4 from one owner to the next and carries one layer. It returns
// the last transaction, so the next call can continue from it.
func grow(t *testing.T, tn *tnode, prev *tx.Tx, owner string, entries int, tries uint64) (*tx.Tx, string) {
	return growTagged(t, tn, prev, owner, "", entries, tries)
}

// growTagged is grow with new owners' labels marked by tag, so two nodes
// growing from the same point build different entries.
func growTagged(t *testing.T, tn *tnode, prev *tx.Tx, owner, tag string, entries int, tries uint64) (*tx.Tx, string) {
	t.Helper()
	for i := 0; i < entries; i++ {
		var next *tx.Tx
		nextOwner := fmt.Sprintf("%s-%s%d", owner, tag, i)
		if prev == nil {
			nextOwner = owner
			next = exKeys.Claim(4, P(nextOwner, 100))
		} else {
			next = exKeys.Spend(4, []testutil.In{exKeys.Out(prev, 0, owner)}, P(nextOwner, 100))
		}
		_, err := tn.n.HandleTx(next, engine.Local)
		must(t, err)
		e, _, err := tn.n.Build(exKey, 0)
		must(t, err)
		_, _, err = tn.n.Work(exKey, e.Hash(exSeries), uint64(i)*tries, tries)
		must(t, err)
		prev, owner = next, nextOwner
	}
	return prev, owner
}

// copyLedger gives dst every entry and layer of src's selected ledger, as
// if they had been forwarded before the two met.
func copyLedger(t *testing.T, src, dst *tnode) {
	t.Helper()
	var hs []hash.Hash
	src.n.Engine().Ledger(exKey, func(l *ledger.Pool) { hs = l.Chain(1, l.Height()) })
	for _, h := range hs {
		src.n.Engine().Ledger(exKey, func(l *ledger.Pool) {
			e, _ := l.Entry(h)
			_, err := dst.n.HandleEntry(exKey, e, engine.Pushed)
			must(t, err)
			for _, ly := range l.Layers(h) {
				_, err := dst.n.HandleLayer(exKey, ly, engine.Pushed)
				must(t, err)
			}
		})
	}
}

// A payment made at one end of a line of nodes reaches the other end
// through forwarding.
func TestForwardingSpreads(t *testing.T) {
	a, b, c := startNode(t, "a", nil), startNode(t, "b", nil), startNode(t, "c", nil)
	a.connect(b)
	c.connect(b)
	// Let the catch-ups every new connection starts find nothing, so all
	// that follows arrives by forwarding.
	time.Sleep(200 * time.Millisecond)
	grow(t, a, nil, "alice", 3, 64)
	converged(t, a, b, c)
	if c.status().Height != 3 {
		t.Fatalf("c height %d", c.status().Height)
	}
	// c got everything pushed: nothing needed asking for.
	c.mu.Lock()
	for _, ev := range c.events {
		if ev.Origin != engine.Pushed || ev.From == "" {
			t.Errorf("c kept an item with origin %v from %q", ev.Origin, ev.From)
		}
	}
	c.mu.Unlock()
	// A pending transaction spreads too.
	_, err := a.n.HandleTx(exKeys.Claim(5, P("bob", 100)), engine.Local)
	must(t, err)
	eventually(t, "the claim to reach c", func() bool { return c.status().Pending == 1 })
}

// A node that joins late fetches the whole ledger with a full ledger
// request, and that data is not forwarded.
func TestNewNodeCatchesUp(t *testing.T) {
	a := startNode(t, "a", nil)
	grow(t, a, nil, "alice", 12, 32)
	d := startNode(t, "d", func(c *p2p.Config) { c.Limits.EntryLimit = 5 }) // three replies
	d.connect(a)
	converged(t, a, d)
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, ev := range d.events {
		if ev.Origin != engine.Requested {
			t.Fatalf("d kept an item with origin %v", ev.Origin)
		}
		if ev.Forward {
			t.Fatal("requested data marked for forwarding")
		}
	}
}

// Two nodes that grew apart from a shared prefix converge on the heavier
// ledger. The split is deep, so finding it pages back through hash
// chains, and the extension comes in several replies.
func TestForkConverges(t *testing.T) {
	small := func(c *p2p.Config) { c.Limits.HashLimit = 4; c.Limits.EntryLimit = 3 }
	a, b := startNode(t, "a", small), startNode(t, "b", small)
	last, owner := grow(t, a, nil, "alice", 5, 32)
	copyLedger(t, a, b)
	if a.state() != b.state() {
		t.Fatal("prefix not shared")
	}
	growTagged(t, a, last, owner, "a", 3, 32)
	growTagged(t, b, last, owner, "b", 14, 32)
	wa, wb := a.status().Weight, b.status().Weight
	heavier := b
	if wa.Greater(wb) {
		heavier = a
	}
	want := heavier.state()
	a.connect(b)
	converged(t, a, b)
	if got := a.state(); got != want {
		t.Fatalf("converged on %s, want the heavier %s", got, want)
	}
}

// After a node switches to data it asked for, its other peers get a
// notice, not the data, and catch up by asking themselves.
func TestRequestedDataIsNotForwarded(t *testing.T) {
	a, b, c := startNode(t, "a", nil), startNode(t, "b", nil), startNode(t, "c", nil)
	grow(t, a, nil, "alice", 6, 32)
	c.connect(b)
	b.connect(a)
	converged(t, a, b, c)
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.events) == 0 {
		t.Fatal("c kept nothing")
	}
	for _, ev := range c.events {
		if ev.Origin != engine.Requested {
			t.Fatalf("c was forwarded requested data (origin %v)", ev.Origin)
		}
	}
}

func TestSelfConnectionIsRefused(t *testing.T) {
	a := startNode(t, "a", nil)
	if err := a.h.Connect(a.h.Addr()); err == nil {
		t.Fatal("connected to self")
	}
	if len(a.h.Peers()) != 0 {
		t.Fatal("self listed as a peer")
	}
}

func TestPeersListed(t *testing.T) {
	a, b := startNode(t, "a", nil), startNode(t, "b", nil)
	a.connect(b)
	eventually(t, "both sides to list the peer", func() bool { return len(a.h.Peers()) == 1 && len(b.h.Peers()) == 1 })
	pa := a.h.Peers()[0]
	if pa.ID != b.h.ID().String() || !pa.Outbound || len(pa.Coverage) != 1 {
		t.Fatalf("peer info %+v", pa)
	}
	if b.h.Peers()[0].Outbound {
		t.Fatal("inbound peer listed as outbound")
	}
	// A second connection to the same node is refused or replaces the
	// first; either way one remains.
	b.h.Connect(a.h.Addr())
	time.Sleep(50 * time.Millisecond)
	eventually(t, "one connection", func() bool { return len(a.h.Peers()) == 1 && len(b.h.Peers()) == 1 })
}

// A node with no pool in common is not a peer. It may stay connected as an
// unlisted node, as a wallet would, and is served but sent nothing; with no
// room for unlisted nodes it is disconnected.
func TestUnlistedConnections(t *testing.T) {
	other := testutil.MustSeries(func() series.Params { p := testutil.ExampleParams(); p.Mints = 16; return p }())
	cov := []wire.Section{wire.SeriesSection(hash.Zero, other.ID())}

	a := startNode(t, "a", nil)
	r := dialRaw(t, a, cov)
	eventually(t, "the follow request to time out", func() bool {
		ps := a.h.Peers()
		return len(ps) == 1 && len(ps[0].Categories) == 0
	})
	grow(t, a, nil, "alice", 1, 32)
	if _, _, err := r.readUntil(time.Now().Add(300 * time.Millisecond)); err == nil {
		t.Fatal("an unlisted node was forwarded to")
	}
	ms := r.ask(poolSection(), wire.WeightReq{RequestCommon: r.common(wire.Limits{})})
	if ms[0].Type != wire.TypeWeight {
		t.Fatalf("unlisted node not served: %s", ms[0].Type)
	}

	b := startNode(t, "b", func(c *p2p.Config) { c.UnlistedConns = -1 })
	dialRaw(t, b, cov).expectClosed()
}

// When a peer holds the same entries with more layers on them, the node
// asks for the layers.
func TestMoreLayersAreFetched(t *testing.T) {
	a, b := startNode(t, "a", nil), startNode(t, "b", nil)
	grow(t, a, nil, "alice", 4, 16)
	copyLedger(t, a, b)
	tip := a.status().Tip
	for i := 0; i < 3; i++ {
		_, _, err := a.n.Work(exKey, tip, uint64(1000+i*500), 500)
		must(t, err)
	}
	if !a.status().Weight.Greater(b.status().Weight) {
		t.Fatal("extra layers added no weight")
	}
	b.connect(a)
	converged(t, a, b)
}

// Without notices, a node's peers learn of a ledger it fetched from their
// regular checks.
func TestRegularCheckWithoutNotices(t *testing.T) {
	a := startNode(t, "a", nil)
	b := startNode(t, "b", func(c *p2p.Config) { c.NoNotices = true })
	c := startNode(t, "c", func(c *p2p.Config) { c.CheckEvery = 50 * time.Millisecond })
	grow(t, a, nil, "alice", 3, 32)
	c.connect(b)
	time.Sleep(100 * time.Millisecond)
	b.connect(a)
	converged(t, a, b, c)
}

func TestFromPolicy(t *testing.T) {
	pol := nodepolicy.Default()
	c := p2p.FromPolicy(pol)
	if c.Listen != nodepolicy.DefaultP2PListen || c.CheckEvery != 0 || c.Limits != p2p.DefaultConfig().Limits {
		t.Fatalf("defaults: %+v", c)
	}
	pol.P2P = nodepolicy.P2P{Listen: "off", Peers: []string{"x:1"}, HashLimit: 7, NoNotices: true, CheckSeconds: -1}
	c = p2p.FromPolicy(pol)
	if c.Listen != "" || len(c.Peers) != 1 || c.Limits.HashLimit != 7 || c.Limits.EntryLimit != 512 || !c.NoNotices || c.CheckEvery >= 0 {
		t.Fatalf("set: %+v", c)
	}
	pol.P2P.CheckSeconds = 5
	if c := p2p.FromPolicy(pol); c.CheckEvery != 5*time.Second {
		t.Fatal(c.CheckEvery)
	}
	pol.P2P.Registries, pol.P2P.NoRelay, pol.P2P.HopLimit, pol.P2P.PingSeconds = []string{"r"}, true, 3, -1
	if c := p2p.FromPolicy(pol); len(c.Registries) != 1 || !c.NoRelay || c.HopLimit != 3 || c.PingEvery >= 0 {
		t.Fatalf("%+v", c)
	}
}

// Inbound connections cannot take the slots kept for peers the node dials
// itself (decision D75).
func TestInboundCannotFillOutboundSlots(t *testing.T) {
	a := startNode(t, "a", func(c *p2p.Config) { c.MaxPeers = 3; c.MaxOutbound = 1 })
	b, c, d, e := startNode(t, "b", nil), startNode(t, "c", nil), startNode(t, "d", nil), startNode(t, "e", nil)
	b.connect(a)
	c.connect(a)
	eventually(t, "two inbound peers", func() bool { return len(a.h.Peers()) == 2 })
	d.h.Connect(a.h.Addr())
	time.Sleep(100 * time.Millisecond)
	if n := len(a.h.Peers()); n != 2 {
		t.Fatalf("a third inbound connection was kept: %d peers", n)
	}
	a.connect(e)
	eventually(t, "the outbound slot to be used", func() bool { return len(a.h.Peers()) == 3 })
}

// A node with K stale siblings, each heavier than a rival ledger's first
// entry, still takes the rival when it is heavier in all: it fetches the
// rival one entry at a time from the fork, and its first entry alone ranks
// last (D79).
func TestHeavierRivalBeyondKSiblings(t *testing.T) {
	a, b := startNode(t, "a", nil), startNode(t, "b", nil)
	last, owner := grow(t, a, nil, "alice", 2, 32)
	copyLedger(t, a, b)
	var fork hash.Hash
	a.n.Engine().Ledger(exKey, func(l *ledger.Pool) { fork, _, _ = l.Tip() })
	// K+1 siblings of distinct strengths: the strongest is selected and
	// the other K are inactive.
	for i := 0; i <= ledger.DefaultK; i++ {
		spend := exKeys.Spend(4, []testutil.In{exKeys.Out(last, 0, owner)}, P(fmt.Sprintf("sib%d", i), 100))
		e, _, err := entry.Build(exSeries, fork, []*tx.Tx{spend}, 0)
		must(t, err)
		_, err = a.n.HandleEntry(exKey, e, engine.Local)
		must(t, err)
		for n := uint64(0); ; n++ {
			if l := (layer.Layer{Entry: e.Hash(exSeries), Nonce: n}); l.Strength(exSeries.Hash()) == 8+i {
				_, err = a.n.HandleLayer(exKey, l, engine.Local)
				must(t, err)
				break
			}
		}
	}
	// The rival starts light, then grows heavy.
	blast, bowner := growTagged(t, b, last, owner, "b", 3, 2)
	growTagged(t, b, blast, bowner, "c", 30, 1024)
	if !b.status().Weight.Greater(a.status().Weight) {
		t.Fatalf("the rival should be heavier: %s against %s", b.status().Weight, a.status().Weight)
	}
	want := b.state()
	a.connect(b)
	converged(t, a, b)
	if got := a.state(); got != want {
		t.Fatalf("converged on %s, want the heavier %s", got, want)
	}
}
