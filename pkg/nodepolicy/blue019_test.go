package nodepolicy_test

import (
	"strings"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/nodepolicy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/testutil"
)

// RED-048, D152: with Settings, every node setting the import changes is
// named first, except the cosmetic ones: the settings Red found unnamed,
// and any added later.
func TestSettingsNameEveryNodeSetting(t *testing.T) {
	f := nodepolicy.Default()
	ties, pending, seen := 3, 17, 1025
	f.Ties, f.MaxPending, f.SeenSize = ties, pending, seen
	f.P2P.CheckSeconds, f.P2P.PingSeconds, f.P2P.ReviewSeconds = -1, -1, -1
	f.P2P.Margin, f.P2P.PrioritySlots, f.P2P.BackupSlots, f.P2P.HopLimit = 0.5, 1, 1, 1
	f.P2P.ChunkSize, f.P2P.HashLimit, f.P2P.LayerLimit = 1024, 1, 1
	f.Log = &nodepolicy.Log{Level: "error", Format: "json"}
	f.Grind = &nodepolicy.Grind{LayerFloor: 1, LayerRate: 1}
	_, plan, err := nodepolicy.Merge(nodepolicy.Default(), f, nodepolicy.MergeOptions{Settings: true})
	if err != nil {
		t.Fatal(err)
	}
	named := " " + strings.Join(plan.Defences, " ") + " "
	for _, s := range []string{"ties", "max_pending", "seen_size", "p2p.check_seconds", "p2p.ping_seconds",
		"p2p.review_seconds", "p2p.margin", "p2p.priority_slots", "p2p.backup_slots", "p2p.hop_limit",
		"p2p.chunk_size", "p2p.hash_limit", "p2p.layer_limit", "log.level", "grind.layer_floor", "grind.layer_rate"} {
		if !strings.Contains(named, " "+s+" ") {
			t.Errorf("%s is not named first: %s", s, named)
		}
	}
	if strings.Contains(named, "log.format") {
		t.Errorf("a cosmetic setting is named: %s", named)
	}
}

// RED-046, D152: the plan says how many pools each series it touches has
// and how many it follows, and names first a whole-series follow of
// LargeFollow pools or more, even in a default import.
func TestPlanStatesPoolCounts(t *testing.T) {
	big := testutil.ExampleParams()
	big.PoolSize, big.Mints = 1, nodepolicy.LargeFollow
	small := testutil.ExampleParams()
	bid, sid := testutil.MustSeries(big).ID(), testutil.MustSeries(small).ID()
	f := nodepolicy.Default()
	f.Series = []nodepolicy.SeriesInfo{{Params: big}, {Params: small}}
	f.Follow = []nodepolicy.Follow{{Series: &bid}, {Series: &sid, Pools: []uint64{1}}}
	_, plan, err := nodepolicy.Merge(nodepolicy.Default(), f, nodepolicy.MergeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]nodepolicy.FollowCount{}
	for _, c := range plan.Follows {
		got[c.Series] = c
	}
	if c := got[bid.Short()]; !c.Whole || c.Pools != nodepolicy.LargeFollow || c.Followed != c.Pools {
		t.Fatalf("big: %+v", c)
	}
	if c := got[sid.Short()]; c.Whole || c.Followed != 1 || c.Pools != testutil.MustSeries(small).PoolCount() {
		t.Fatalf("small: %+v", c)
	}
	if len(plan.Defences) != 1 || !strings.Contains(plan.Defences[0], bid.Short()) || !strings.Contains(plan.Defences[0], "65536 pools") {
		t.Fatalf("defences: %v", plan.Defences)
	}
	f.Follow = []nodepolicy.Follow{{Series: &bid, Pools: []uint64{0}}}
	if _, plan, _ = nodepolicy.Merge(nodepolicy.Default(), f, nodepolicy.MergeOptions{}); len(plan.Defences) != 0 {
		t.Fatalf("one pool of the big series named: %v", plan.Defences)
	}
}
