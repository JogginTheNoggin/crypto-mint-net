package p2p_test

import (
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
