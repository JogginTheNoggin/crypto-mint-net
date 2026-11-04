package p2p_test

import (
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/engine"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/node"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/nodepolicy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/p2p"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/policy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/series"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/testutil"
)

// variableInfo returns a variable series of 8 mints in part 1, then parts
// of 4, in pools of 4, as node.json lists it with its issuer witness.
func variableInfo(t *testing.T) (nodepolicy.SeriesInfo, *series.Series) {
	t.Helper()
	s := testutil.MustSeries(testutil.VariableParams(8, 4, 4))
	return nodepolicy.SeriesInfo{Params: s.Params(), IssuerWitness: nodepolicy.EncodeWitness(testutil.IssuerWitness(s))}, s
}

// keptRelease returns the release node.json in tn's directory keeps.
func keptRelease(t *testing.T, tn *tnode) *series.Release {
	t.Helper()
	p, err := nodepolicy.Load(filepath.Join(tn.n.Dir(), nodepolicy.FileName))
	if err != nil {
		t.Fatal(err)
	}
	return p.Series[0].Release
}

func released(tn *tnode, s *series.Series) uint64 {
	rs, _ := tn.n.Engine().Series(s.ID())
	return rs.Released()
}

// Variable supply phase 3 (D168): a node that has a release passes it to
// a peer when they connect, and to its peers when it takes a new one. The
// peer checks it in full, keeps it in node.json, follows the part's pools
// in place and passes it on, so a claim in the new part travels as any
// other. A node that follows the series by a list of pools reloads
// instead. A release that does not check is an offence and changes
// nothing.
func TestReleasesPassBetweenPeers(t *testing.T) {
	info, s := variableInfo(t)
	id := s.ID()
	whole := nodepolicy.Follow{Series: &id}
	a := startNodeInfo(t, "a", info, whole, nil)
	b := startNodeInfo(t, "b", info, whole, nil)
	var reloads atomic.Int32
	c := startNodeInfo(t, "c", info, nodepolicy.Follow{Series: &id, Pools: []uint64{0, 1}}, func(cfg *p2p.Config) {
		cfg.Reload = func() { reloads.Add(1) }
	})
	if a.h.IndexedPools() != 2 {
		t.Fatalf("part 1 only: %d pools indexed", a.h.IndexedPools())
	}

	// a takes part 2 as poo series add-release gives it.
	r2 := testutil.Release(s, 2)
	res, err := a.n.AddRelease(r2)
	if err != nil || !res.Added || res.Released != 2 || res.Mints != 12 {
		t.Fatalf("a: %+v %v", res, err)
	}
	if !a.h.Released(r2, "") || a.h.IndexedPools() != 3 || keptRelease(t, a) == nil {
		t.Fatalf("a in place: %d pools indexed, kept %v", a.h.IndexedPools(), keptRelease(t, a))
	}

	// b hears of it when it connects, and takes it in place.
	b.connect(a)
	eventually(t, "b to take part 2", func() bool { return released(b, s) == 2 })
	eventually(t, "b to index part 2's pool", func() bool { return b.h.IndexedPools() == 3 })
	if k := keptRelease(t, b); k == nil || k.Part != 2 {
		t.Fatalf("b's node.json keeps %+v", k)
	}

	// A claim in part 2 now travels from a to b.
	keys := testutil.NewKeys(s)
	claim := keys.Claim(9, testutil.P("alice", s.Params().Supply))
	k9 := engine.PoolKey{Series: id, Index: s.PoolOf(9)}
	if _, err := a.n.HandleTx(claim, engine.Pushed); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the part 2 claim at b", func() bool {
		st, err := b.n.Engine().Status(k9)
		return err == nil && st.Pending == 1
	})

	// b passes part 3 on to c when c connects. c follows part 1's pools by
	// a list, which covered the whole series and no longer does, so it
	// takes the release and asks for a reload to work out its coverage.
	r3 := testutil.Release(s, 3)
	if _, err := b.n.AddRelease(r3); err != nil {
		t.Fatal(err)
	}
	b.h.Released(r3, "")
	eventually(t, "a to take part 3 from b", func() bool { return released(a, s) == 3 })
	c.connect(b)
	eventually(t, "c to take part 3", func() bool { return released(c, s) == 3 })
	eventually(t, "c to ask for a reload", func() bool { return reloads.Load() > 0 })
	if k := keptRelease(t, c); k == nil || k.Part != 3 {
		t.Fatalf("c's node.json keeps %+v", k)
	}

	// A release that does not check, passed on by a, is refused at b: b
	// keeps part 3 and says why.
	bad := testutil.Release(s, 4)
	bad.Witness = policy.SingleKeyWitness(testutil.NewKey(s, "mallory").Signer, s.ReleaseMessage(bad.ID(s.Hash())))
	a.h.SendRelease(bad)
	eventually(t, "b to refuse the bad release", func() bool {
		return strings.Contains(b.log.String(), "release that does not check")
	})
	if released(b, s) != 3 || keptRelease(t, b).Part != 3 {
		t.Fatalf("b after a bad release: part %d", released(b, s))
	}
}

// A node that took a release in place holds what a node opened on the
// same node.json holds: the released parts, the followed and listed pools,
// and the host's index and coverage (ORANGE-035's in-place rule, D168).
func TestReleaseInPlaceMatchesReopen(t *testing.T) {
	info, s := variableInfo(t)
	id := s.ID()
	a := startNodeInfo(t, "a", info, nodepolicy.Follow{Series: &id}, nil)
	for k := uint64(2); k <= 4; k++ {
		r := testutil.Release(s, k)
		if _, err := a.n.AddRelease(r); err != nil {
			t.Fatal(err)
		}
		if !a.h.Released(r, "") {
			t.Fatalf("part %d not taken in place", k)
		}
	}
	dir := t.TempDir()
	data, err := os.ReadFile(filepath.Join(a.n.Dir(), nodepolicy.FileName))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, nodepolicy.FileName), data, 0o600); err != nil {
		t.Fatal(err)
	}
	n, err := node.Open(dir, node.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	h, err := p2p.New(n, p2p.Config{CheckEvery: -1})
	if err != nil {
		t.Fatal(err)
	}
	rs, _ := n.Engine().Series(id)
	if rs.Released() != released(a, s) || rs.Mints() != 20 {
		t.Fatalf("reopened: part %d, %d mints; in place: part %d", rs.Released(), rs.Mints(), released(a, s))
	}
	if x, y := len(n.Engine().FollowedPools()), len(a.n.Engine().FollowedPools()); x != y {
		t.Fatalf("followed pools: %d reopened, %d in place", x, y)
	}
	if x, y := len(n.Engine().ListedPools()), len(a.n.Engine().ListedPools()); x != y {
		t.Fatalf("listed pools: %d reopened, %d in place", x, y)
	}
	if h.IndexedPools() != a.h.IndexedPools() || len(h.Coverage()) != len(a.h.Coverage()) || h.Coverage()[0] != a.h.Coverage()[0] {
		t.Fatalf("index: %d reopened, %d in place; coverage %v and %v", h.IndexedPools(), a.h.IndexedPools(), h.Coverage(), a.h.Coverage())
	}
}
