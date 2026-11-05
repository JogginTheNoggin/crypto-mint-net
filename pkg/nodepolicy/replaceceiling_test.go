package nodepolicy_test

import (
	"strings"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/nodepolicy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/testutil"
)

// GUI-030: a replace import honours the node's own series_ceiling, as a
// merge does, and keeps each series once.
func TestReplaceHonoursSeriesCeiling(t *testing.T) {
	sa, sb, _, idb := twoSeries()
	// sa fits a ceiling of 8192 bytes; sb sets none.
	sa.Params.MaxWitness = 4096
	ida := testutil.MustSeries(sa.Params).ID()
	for _, settings := range []bool{false, true} {
		local := nodepolicy.Default()
		local.SeriesCeiling = 8192
		file := nodepolicy.Default()
		file.Series = []nodepolicy.SeriesInfo{sa, sb, sa}
		file.Follow = []nodepolicy.Follow{{Series: &ida}, {Series: &idb}}
		res, plan, err := nodepolicy.Merge(local, file, nodepolicy.MergeOptions{Replace: true, Settings: settings})
		if err != nil {
			t.Fatal(err)
		}
		ids := map[string]int{}
		for _, si := range res.Series {
			s, _, err := si.Build()
			if err != nil {
				t.Fatal(err)
			}
			ids[s.ID().String()]++
		}
		declinedB := len(plan.Declined) == 1 && strings.Contains(plan.Declined[0], idb.Short())
		if !declinedB {
			t.Fatalf("settings=%v: declined %v", settings, plan.Declined)
		}
		if ids[idb.String()] != 0 {
			t.Fatalf("settings=%v: the declined series was kept", settings)
		}
		for id, n := range ids {
			if n != 1 {
				t.Fatalf("settings=%v: series %s kept %d times", settings, id, n)
			}
		}
		for _, f := range res.Follow {
			if f.Series != nil && *f.Series == idb {
				t.Fatalf("settings=%v: the declined series' follow rule was kept", settings)
			}
		}
	}
}
