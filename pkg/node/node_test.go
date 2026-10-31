package node_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/engine"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/entry"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/layer"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/ledger"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/node"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/nodepolicy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/series"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/store"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/testutil"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tx"
)

var P = testutil.P

type fixture struct {
	t   *testing.T
	dir string
	s   *series.Series
	k   *testutil.Keys
	key engine.PoolKey
}

// newFixture writes a node policy that knows the example series and
// follows pool 1 (serials 4 to 7).
func newFixture(t *testing.T, edit ...func(*nodepolicy.Policy)) *fixture {
	t.Helper()
	s := testutil.ExampleSeries()
	id := s.ID()
	p := nodepolicy.Default()
	p.Series = []nodepolicy.SeriesInfo{{Params: s.Params()}}
	p.Follow = []nodepolicy.Follow{{Series: &id, Pools: []uint64{1}}}
	p.RPC.Listen = "off"
	for _, f := range edit {
		f(&p)
	}
	dir := t.TempDir()
	if err := nodepolicy.Save(filepath.Join(dir, nodepolicy.FileName), p); err != nil {
		t.Fatal(err)
	}
	return &fixture{t: t, dir: dir, s: s, k: testutil.NewKeys(s), key: engine.PoolKey{Series: id, Index: 1}}
}

func (f *fixture) open() *node.Node {
	f.t.Helper()
	n, err := node.Open(f.dir, node.Options{})
	if err != nil {
		f.t.Fatal(err)
	}
	return n
}

type snapshot struct {
	tip      hash.Hash
	height   int
	weight   string
	entries  int
	pending  int
	holdings string
}

func (f *fixture) snap(n *node.Node) snapshot {
	f.t.Helper()
	st, err := n.Engine().Status(f.key)
	if err != nil {
		f.t.Fatal(err)
	}
	h := ""
	for serial := uint64(4); serial < 8; serial++ {
		hs, _ := n.Engine().Holdings(f.s.ID(), serial)
		h += fmt.Sprint(hs)
	}
	return snapshot{st.Tip, st.Height, st.Weight.String(), st.Entries, st.Pending, h}
}

// history runs a small history: two claims, a spend, entries and layers, a
// rival branch, and a transaction left pending.
func (f *fixture) history(n *node.Node) {
	f.t.Helper()
	must := func(err error) {
		f.t.Helper()
		if err != nil {
			f.t.Fatal(err)
		}
	}
	c4 := f.k.Claim(4, P("alice", 100))
	c5 := f.k.Claim(5, P("bob", 60), P("carol", 40))
	_, err := n.HandleTx(c4, engine.Local)
	must(err)
	_, err = n.HandleTx(c5, engine.Pushed)
	must(err)
	e1, _, err := n.Build(f.key, 0)
	must(err)
	_, _, err = n.Work(f.key, e1.Hash(f.s), 0, 500)
	must(err)
	spend := f.k.Spend(4, []testutil.In{f.k.Out(c4, 0, "alice")}, P("dave", 30), P("alice2", 70))
	_, err = n.HandleTx(spend, engine.Local)
	must(err)
	e2, _, err := n.Build(f.key, 0)
	must(err)
	_, _, err = n.Work(f.key, e2.Hash(f.s), 0, 300)
	must(err)
	// A rival double spend on e1, lighter than e2.
	rival := f.k.Spend(4, []testutil.In{f.k.Out(c4, 0, "alice")}, P("eve", 100))
	r, _, err := entry.Build(f.s, e1.Hash(f.s), []*tx.Tx{rival}, 0)
	must(err)
	_, err = n.HandleEntry(f.key, r, engine.Pushed)
	must(err)
	// Left pending.
	_, err = n.HandleTx(f.k.Spend(5, []testutil.In{f.k.Out(c5, 0, "bob")}, P("frank", 60)), engine.Pushed)
	must(err)
}

func TestRestartRestoresState(t *testing.T) {
	f := newFixture(t)
	n := f.open()
	f.history(n)
	before := f.snap(n)
	if before.height != 2 || before.pending != 1 || before.entries != 3 {
		t.Fatalf("unexpected history: %+v", before)
	}
	if err := n.Close(); err != nil {
		t.Fatal(err)
	}
	n = f.open()
	defer n.Close()
	if after := f.snap(n); after != before {
		t.Fatalf("after restart\n got %+v\nwant %+v", after, before)
	}
	rs := n.Replayed()
	if rs.Pools != 1 || rs.Kept != rs.Records || rs.Cut != 0 {
		t.Fatalf("replay stats %+v", rs)
	}
}

func TestCompactKeepsState(t *testing.T) {
	f := newFixture(t)
	n := f.open()
	f.history(n)
	before := f.snap(n)
	recordsBefore := n.Stores()[0].Records
	if err := n.Compact(f.key); err != nil {
		t.Fatal(err)
	}
	after := n.Stores()[0].Records
	// Three transactions are now in entries, so their pending records go.
	if after >= recordsBefore {
		t.Fatalf("compaction kept %d of %d records", after, recordsBefore)
	}
	n.Close()
	n = f.open()
	defer n.Close()
	if got := f.snap(n); got != before {
		t.Fatalf("after compaction\n got %+v\nwant %+v", got, before)
	}
	if rs := n.Replayed(); rs.Kept != rs.Records {
		t.Fatalf("compacted log has dead records: %+v", rs)
	}
}

func TestTornWriteIsCut(t *testing.T) {
	f := newFixture(t)
	n := f.open()
	f.history(n)
	before := f.snap(n)
	path := n.Stores()[0].Path
	n.Close()
	fh, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	fh.Write([]byte{1, 0, 0, 1, 0, 'x'}) // a record cut short
	fh.Close()
	var log bytes.Buffer
	n, err = node.Open(f.dir, node.Options{Log: &log})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	if got := f.snap(n); got != before {
		t.Fatalf("got %+v\nwant %+v", got, before)
	}
	if n.Replayed().Cut != 6 || !bytes.Contains(log.Bytes(), []byte("unfinished write")) {
		t.Fatalf("cut %d, log %q", n.Replayed().Cut, log.String())
	}
}

func TestTamperedStoreIsCheckedAgain(t *testing.T) {
	f := newFixture(t)
	n := f.open()
	c := f.k.Claim(4, P("alice", 100))
	if _, err := n.HandleTx(c, engine.Local); err != nil {
		t.Fatal(err)
	}
	path := n.Stores()[0].Path
	n.Close()
	// Rewrite the claim to pay someone else, keeping the checksum valid:
	// replay must refuse it because the witness no longer matches.
	forged := f.k.Claim(4, P("mallory", 100))
	forged.Witnesses = c.Witnesses
	l, _, err := store.Open(path, store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err := l.Rewrite([]store.Record{{Kind: store.KindTx, Data: forged.Encode()}}); err != nil {
		t.Fatal(err)
	}
	l.Close()
	n = f.open()
	defer n.Close()
	if rs := n.Replayed(); rs.Records != 1 || rs.Kept != 0 {
		t.Fatalf("forged record kept: %+v", rs)
	}
	if st, _ := n.Engine().Status(f.key); st.Pending != 0 {
		t.Fatalf("pending = %d", st.Pending)
	}
}

func TestSecondNodeOnSameDirIsRefused(t *testing.T) {
	f := newFixture(t)
	n := f.open()
	defer n.Close()
	if _, err := node.Open(f.dir, node.Options{}); !errors.Is(err, nodepolicy.ErrLocked) {
		t.Fatalf("err = %v", err)
	}
}

func TestClosedNode(t *testing.T) {
	f := newFixture(t)
	n := f.open()
	n.Close()
	if err := n.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := n.HandleTx(f.k.Claim(4, P("a", 100)), engine.Local); !errors.Is(err, node.ErrClosed) {
		t.Fatalf("err = %v", err)
	}
	if _, _, err := n.Build(f.key, 0); !errors.Is(err, node.ErrClosed) {
		t.Fatalf("err = %v", err)
	}
	// The lock is released: the directory opens again.
	f.open().Close()
}

func TestRefusedItemsAreNotStored(t *testing.T) {
	f := newFixture(t)
	n := f.open()
	defer n.Close()
	c := f.k.Claim(4, P("alice", 100))
	n.HandleTx(c, engine.Local)
	if _, err := n.HandleTx(c, engine.Pushed); err == nil {
		t.Fatal("second copy kept")
	}
	// Serial 0 is in pool 0, which is not followed.
	if _, err := n.HandleTx(f.k.Claim(0, P("alice", 100)), engine.Pushed); !errors.Is(err, engine.ErrNotFollowed) {
		t.Fatalf("err = %v", err)
	}
	if got := n.Stores()[0].Records; got != 1 {
		t.Fatalf("%d records stored", got)
	}
}

func TestUnfollowedPoolStoreIsLeftAlone(t *testing.T) {
	f := newFixture(t)
	n := f.open()
	f.history(n)
	path := n.Stores()[0].Path
	n.Close()
	info, _ := os.Stat(path)
	// Follow pool 0 instead.
	p, _ := nodepolicy.Load(filepath.Join(f.dir, nodepolicy.FileName))
	p.Follow[0].Pools = []uint64{0}
	nodepolicy.Save(filepath.Join(f.dir, nodepolicy.FileName), p)
	n = f.open()
	if rs := n.Replayed(); rs.Pools != 0 {
		t.Fatalf("replayed %+v", rs)
	}
	n.Close()
	after, _ := os.Stat(path)
	if after.Size() != info.Size() {
		t.Fatal("store of an unfollowed pool changed")
	}
}

func TestFollowRules(t *testing.T) {
	cases := []struct {
		name  string
		edit  func(p *nodepolicy.Policy)
		pools int
	}{
		{"whole series", func(p *nodepolicy.Policy) { p.Follow[0].Pools = nil }, 2},
		{"one mint", func(p *nodepolicy.Policy) { p.Follow[0].Pools, p.Follow[0].Mints = nil, []uint64{2} }, 1},
		{"unknown series", func(p *nodepolicy.Policy) { h := hash.Hash{1}; p.Follow[0].Series = &h }, 0},
		{"series not accepted", func(p *nodepolicy.Policy) { p.Issuers.Any = false }, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newFixture(t, c.edit)
			var log bytes.Buffer
			n, err := node.Open(f.dir, node.Options{Log: &log})
			if err != nil {
				t.Fatal(err)
			}
			defer n.Close()
			if got := len(n.Engine().FollowedPools()); got != c.pools {
				t.Fatalf("%d pools followed, want %d (log %q)", got, c.pools, log.String())
			}
		})
	}
}

func TestBadFollowRule(t *testing.T) {
	f := newFixture(t, func(p *nodepolicy.Policy) { p.Follow[0].Pools = []uint64{9} })
	if _, err := node.Open(f.dir, node.Options{}); !errors.Is(err, nodepolicy.ErrInvalid) {
		t.Fatalf("err = %v", err)
	}
	// The failed open released the lock.
	p, _ := nodepolicy.Load(filepath.Join(f.dir, nodepolicy.FileName))
	p.Follow[0].Pools = []uint64{1}
	nodepolicy.Save(filepath.Join(f.dir, nodepolicy.FileName), p)
	f.open().Close()
}

func TestMissingPolicy(t *testing.T) {
	if _, err := node.Open(t.TempDir(), node.Options{}); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("err = %v", err)
	}
}

func TestAutomaticCompaction(t *testing.T) {
	f := newFixture(t)
	n := f.open()
	f.history(n)
	before := f.snap(n)
	n.Close()
	var log bytes.Buffer
	n, err := node.Open(f.dir, node.Options{Log: &log, CompactMin: 5})
	if err != nil {
		t.Fatal(err)
	}
	// The three transactions now in entries are dead, but that is not more
	// than half the log, so nothing happens yet.
	if bytes.Contains(log.Bytes(), []byte("compacted")) {
		t.Fatalf("compacted too early: %q", log.String())
	}
	// Twenty rival entries on the parent of the tip are lighter than the
	// selected ledger, so all but the top K = 8 and the newest (D79) are
	// pruned, which makes most of the log dead.
	st, _ := n.Engine().Status(f.key)
	var parent hash.Hash
	n.Engine().Ledger(f.key, func(p *ledger.Pool) {
		e, _ := p.Entry(st.Tip)
		parent = e.Parent
	})
	for i := 0; i < 20; i++ {
		rival := f.k.Claim(6, P(fmt.Sprintf("r%d", i), 100))
		e, _, err := entry.Build(f.s, parent, []*tx.Tx{rival}, 0)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := n.HandleEntry(f.key, e, engine.Pushed); err != nil {
			t.Fatal(err)
		}
	}
	n.Close()
	if !bytes.Contains(log.Bytes(), []byte("compacted")) {
		t.Fatalf("not compacted on close: %q", log.String())
	}
	n = f.open()
	defer n.Close()
	got := f.snap(n)
	before.entries += ledger.DefaultK // K branches and the newest kept; one was already held
	if got != before {
		t.Fatalf("got %+v\nwant %+v", got, before)
	}
}

// RED-006: the search for an entry's nonce holds neither the node's lock
// nor the engine's, so the node keeps taking items, and the search stops
// when its context is cancelled.
func TestBuildSearchHoldsNoLock(t *testing.T) {
	pr := testutil.ExampleParams()
	pr.MinEntryStrength = series.MaxStrength // never met: the search runs until cancelled
	s := testutil.MustSeries(pr)
	id := s.ID()
	p := nodepolicy.Default()
	p.Series = []nodepolicy.SeriesInfo{{Params: pr}}
	p.Follow = []nodepolicy.Follow{{Series: &id, Pools: []uint64{1}}}
	p.RPC.Listen = "off"
	dir := t.TempDir()
	if err := nodepolicy.Save(filepath.Join(dir, nodepolicy.FileName), p); err != nil {
		t.Fatal(err)
	}
	n, err := node.Open(dir, node.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	k := testutil.NewKeys(s)
	key := engine.PoolKey{Series: id, Index: 1}
	if _, err := n.HandleTx(k.Claim(4, P("alice", 100)), engine.Local); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, _, err := n.BuildContext(ctx, key, 0)
		done <- err
	}()
	time.Sleep(50 * time.Millisecond)
	start := time.Now()
	if _, err := n.HandleTx(k.Claim(5, P("bob", 100)), engine.Local); err != nil {
		t.Fatal(err)
	}
	if d := time.Since(start); d > time.Second {
		t.Fatalf("a transaction waited %v for the build", d)
	}
	n.Engine().Status(key)
	select {
	case err := <-done:
		t.Fatalf("the search ended by itself: %v", err)
	default:
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("want context.Canceled, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the search did not stop when cancelled")
	}
}

// A floor keeps the other layers a search found, so the same tries give
// the entry at least as much weight, and only ever more (ORANGE-010).
func TestWorkWithFloor(t *testing.T) {
	f := newFixture(t)
	n := f.open()
	defer n.Close()
	_, err := n.HandleTx(f.k.Claim(4, P("alice", 100)), engine.Local)
	if err != nil {
		t.Fatal(err)
	}
	e, _, err := n.Build(f.key, 0)
	if err != nil {
		t.Fatal(err)
	}
	eh := e.Hash(f.s)
	var kept atomic.Uint64
	if _, _, err := n.WorkWith(context.Background(), f.key, eh, 0, 1<<16, engine.Grind{Floor: 4, KeptCount: &kept}); err != nil {
		t.Fatal(err)
	}
	if kept.Load() == 0 {
		t.Fatal("no layer kept besides the strongest")
	}
	if got := len(layersOn(n, f, eh)); got != int(kept.Load())+1 {
		t.Fatalf("%d layers held, %d kept", got, kept.Load())
	}
}

func layersOn(n *node.Node, f *fixture, eh hash.Hash) (out []layer.Layer) {
	n.Engine().Ledger(f.key, func(l *ledger.Pool) { out = l.Layers(eh) })
	return out
}

// RED-018: a search with a low floor over many ranges sends at most
// MaxKeptLayers besides its strongest, the strongest first, and searches
// together no more than about KeptPerSecond a second.
func TestWorkWithFloorIsCapped(t *testing.T) {
	f := newFixture(t)
	n := f.open()
	defer n.Close()
	_, err := n.HandleTx(f.k.Claim(4, P("alice", 100)), engine.Local)
	if err != nil {
		t.Fatal(err)
	}
	e, _, err := n.Build(f.key, 0)
	if err != nil {
		t.Fatal(err)
	}
	eh := e.Hash(f.s)
	var kept atomic.Uint64
	// 256 ranges of 4,096 nonces, each reaching a floor of 1.
	if _, _, err := n.WorkWith(context.Background(), f.key, eh, 0, 1<<20, engine.Grind{Floor: 1, KeptCount: &kept}); err != nil {
		t.Fatal(err)
	}
	if kept.Load() != node.MaxKeptLayers {
		t.Fatalf("kept %d, want %d", kept.Load(), node.MaxKeptLayers)
	}
	ls := layersOn(n, f, eh)
	fn := f.s.Hash()
	weakest := 256
	for _, l := range ls {
		weakest = min(weakest, l.Hash(fn).Strength())
	}
	// The 17 strongest of 256 ranges are all well above the floor.
	if weakest < 8 {
		t.Fatalf("a layer of strength %d was sent", weakest)
	}
	// More searches at once: the bucket holds a second's worth.
	start := time.Now()
	for i := 1; i <= 6; i++ {
		if _, _, err := n.WorkWith(context.Background(), f.key, eh, uint64(i)<<20, 1<<20, engine.Grind{Floor: 1, KeptCount: &kept}); err != nil {
			t.Fatal(err)
		}
	}
	limit := uint64(node.KeptPerSecond) + uint64(time.Since(start).Seconds()*node.KeptPerSecond) + 1
	if got := kept.Load(); got > limit {
		t.Fatalf("kept %d in %v, want at most %d", got, time.Since(start), limit)
	}
}
