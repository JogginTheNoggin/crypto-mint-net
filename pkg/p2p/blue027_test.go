package p2p_test

import (
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/engine"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/policy"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/nodepolicy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/p2p"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/series"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/testutil"
)

// RED-078, D169: a peer that replays genuine releases oldest first makes
// a node take only the first and then the highest of each window: a few
// saves, relays and reloads, not one each, and the node ends at the
// newest part.
func TestReleaseReplayIsCoalesced(t *testing.T) {
	defer p2p.SetReleaseWindow(300 * time.Millisecond)()
	info, s := variableInfo(t)
	id := s.ID()
	a := startNodeInfo(t, "a", info, nodepolicy.Follow{Series: &id}, nil)
	var reloads atomic.Int32
	b := startNodeInfo(t, "b", info, nodepolicy.Follow{Series: &id, Pools: []uint64{0, 1}}, func(cfg *p2p.Config) {
		cfg.Reload = func() { reloads.Add(1) }
	})
	b.connect(a)
	const top = 200
	rels := make([]series.Release, 0, top)
	for k := uint64(2); k <= top; k++ {
		rels = append(rels, testutil.Release(s, k))
	}
	for _, r := range rels {
		a.h.SendRelease(r)
	}
	eventually(t, "b to take the newest part", func() bool { return released(b, s) == top })
	time.Sleep(400 * time.Millisecond)
	kept := strings.Count(b.log.String(), "release kept")
	if kept > 4 {
		t.Fatalf("b took %d releases of the replay", kept)
	}
	if n := reloads.Load(); n > 4 {
		t.Fatalf("b asked for %d reloads", n)
	}
	if k := keptRelease(t, b); k == nil || k.Part != top {
		t.Fatalf("b's node.json keeps %+v", k)
	}
	if strings.Contains(b.log.String(), "peer offence") {
		t.Fatalf("an honest replay was an offence:\n%s", b.log.String())
	}
}

// RED-082, D171: a release of a far part, signed by the issuer with a
// wrong previous id, is checked in a worker, not in the peer's read loop:
// the peer's other messages still arrive, and stopping the host ends the
// check at once.
func TestFarReleaseDoesNotHoldTheNode(t *testing.T) {
	info, s := variableInfo(t)
	id := s.ID()
	a := startNodeInfo(t, "a", info, nodepolicy.Follow{Series: &id}, nil)
	b := startNodeInfo(t, "b", info, nodepolicy.Follow{Series: &id}, nil)
	b.connect(a)
	far := series.Release{Series: id, Part: 1 << 36, Previous: hash.Hash{7}}
	far.Witness = policy.SingleKeyWitness(testutil.NewKey(s, "issuer").Signer, s.ReleaseMessage(far.ID(s.Hash())))
	if err := s.CheckReleaseSigned(far); err != nil {
		t.Fatal(err)
	}
	a.h.SendRelease(far)
	eventually(t, "b to start checking", func() bool {
		return strings.Contains(b.log.String(), "checking a release of a far part")
	})

	// A claim from a still reaches b.
	keys := testutil.NewKeys(s)
	claim := keys.Claim(1, testutil.P("alice", s.Params().Supply))
	k := engine.PoolKey{Series: id, Index: s.PoolOf(1)}
	if _, err := a.n.HandleTx(claim, engine.Pushed); err != nil {
		t.Fatal(err)
	}
	eventually(t, "the claim at b", func() bool {
		st, err := b.n.Engine().Status(k)
		return err == nil && st.Pending == 1
	})

	t0 := time.Now()
	b.stop()
	if took := time.Since(t0); took > 3*time.Second {
		t.Fatalf("b took %v to stop", took)
	}
	if released(b, s) != 1 {
		t.Fatalf("b took the far release")
	}
}
