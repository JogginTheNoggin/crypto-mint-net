package p2p_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/engine"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/node"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/nodepolicy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/p2p"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/testutil"
)

func hostFor(t *testing.T, pol nodepolicy.Policy) (*p2p.Host, error) {
	t.Helper()
	pol.RPC.Listen = "off"
	dir := t.TempDir()
	if err := nodepolicy.Save(filepath.Join(dir, nodepolicy.FileName), pol); err != nil {
		t.Fatal(err)
	}
	n, err := node.Open(dir, node.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { n.Close() })
	return p2p.New(n, p2p.Config{Listen: "127.0.0.1:0", CheckEvery: -1, ManageEvery: -1, PingEvery: -1})
}

// RED-049, D154: the host of a node following a series of 2^40 pools
// whole starts at once and announces the series; it indexes the pools as
// they come into use.
func TestHostOfAHugeWholeSeriesStartsAtOnce(t *testing.T) {
	prm := testutil.ExampleParams()
	prm.PoolSize, prm.Mints = 1, 1<<40
	id := testutil.MustSeries(prm).ID()
	pol := nodepolicy.Default()
	pol.Series = []nodepolicy.SeriesInfo{{Params: prm}}
	pol.Follow = []nodepolicy.Follow{{Series: &id}}
	start := time.Now()
	h, err := hostFor(t, pol)
	if err != nil {
		t.Fatal(err)
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Fatalf("p2p.New took %v", d)
	}
	if c := h.Coverage(); len(c) != 1 || c[0].Depth != 2 || c[0].Series != id {
		t.Fatalf("coverage %v", c)
	}
	if n := h.IndexedPools(); n != 0 {
		t.Fatalf("%d pools indexed before any is in use", n)
	}
}

// RED-050, D154: all but one pool of a series of 5000 pools is one entry,
// and the node starts with it, announcing the series.
func TestAllButOnePoolStarts(t *testing.T) {
	prm := testutil.ExampleParams()
	prm.PoolSize, prm.Mints = 1, 5000
	id := testutil.MustSeries(prm).ID()
	pol := nodepolicy.Default()
	pol.Series = []nodepolicy.SeriesInfo{{Params: prm}}
	pol.Follow = []nodepolicy.Follow{{Series: &id}}
	if err := pol.RemoveFollow(nodepolicy.Follow{Series: &id, Pools: []uint64{7}}); err != nil {
		t.Fatal(err)
	}
	h, err := hostFor(t, pol)
	if err != nil {
		t.Fatalf("p2p.New: %v", err)
	}
	if c := h.Coverage(); len(c) != 1 || c[0].Depth != 2 {
		t.Fatalf("coverage %v", c)
	}
}

// RED-049, D154: two nodes following a series of 2^40 pools whole pass a
// transaction on: the sender indexes the pool once it keeps the claim,
// and the receiver finds the pool by the claim's mint.
func TestHugeWholeSeriesForwardsATransaction(t *testing.T) {
	prm := testutil.ExampleParams()
	prm.PoolSize, prm.Mints = 1, 1<<40
	s := testutil.MustSeries(prm)
	id := s.ID()
	a := startNodeWith(t, "a", s, nodepolicy.Follow{Series: &id}, nil)
	b := startNodeWith(t, "b", s, nodepolicy.Follow{Series: &id}, nil)
	a.connect(b)
	eventually(t, "a and b to peer", func() bool { return len(a.h.Peers()) == 1 && len(b.h.Peers()) == 1 })
	keys := testutil.NewKeys(s)
	serial := uint64(1) << 39
	claim := keys.Claim(serial, testutil.P("alice", prm.Supply))
	if _, err := a.n.HandleTx(claim, engine.Local); err != nil {
		t.Fatal(err)
	}
	k := engine.PoolKey{Series: id, Index: serial}
	eventually(t, "b to take the claim", func() bool {
		st, err := b.n.Engine().Status(k)
		return err == nil && st.Pending == 1
	})
	if a.h.IndexedPools() != 1 || b.h.IndexedPools() != 1 {
		t.Fatalf("indexed: a %d, b %d", a.h.IndexedPools(), b.h.IndexedPools())
	}
}
