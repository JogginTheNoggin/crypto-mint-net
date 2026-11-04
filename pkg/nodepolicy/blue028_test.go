package nodepolicy_test

import (
	"strings"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/nodepolicy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/testutil"
)

// RED-083: an import that adds a series with no witness ceiling, or one
// above CeilingLimit, warns; one with a small ceiling, or one the node
// already lists and the file leaves alone, does not.
func TestImportWarnsOfUnboundedSeries(t *testing.T) {
	sa, sb, ida, idb := twoSeries() // both with no ceiling
	sc := nodepolicy.SeriesInfo{Params: testutil.ExampleParams()}
	sc.Params.Mints, sc.Params.MaxWitness = 16, 4096
	sd := nodepolicy.SeriesInfo{Params: testutil.ExampleParams()}
	sd.Params.Mints, sd.Params.MaxWitness = 20, nodepolicy.CeilingLimit+1
	idc, idd := testutil.MustSeries(sc.Params).ID(), testutil.MustSeries(sd.Params).ID()

	local := nodepolicy.Default()
	local.Series = []nodepolicy.SeriesInfo{sa}
	file := nodepolicy.Default()
	file.Series = []nodepolicy.SeriesInfo{sa, sb, sc, sd}
	_, plan, err := nodepolicy.Merge(local, file, nodepolicy.MergeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(plan.Unbounded, "\n")
	if len(plan.Unbounded) != 2 || !strings.Contains(got, idb.Short()) || !strings.Contains(got, idd.Short()) {
		t.Fatalf("unbounded %q", got)
	}
	if strings.Contains(got, ida.Short()) || strings.Contains(got, idc.Short()) {
		t.Fatalf("warned of a listed or bounded series: %q", got)
	}
	if !strings.Contains(got, "no witness ceiling") || !strings.Contains(got, "above") {
		t.Fatalf("reasons: %q", got)
	}
	if nodepolicy.CeilingWarning(sc.Params) != "" {
		t.Fatal("a 4096-byte ceiling warned")
	}
}

// RED-083: a declined series says how many follow rules went with it.
func TestDeclinedSeriesCountsItsFollows(t *testing.T) {
	_, sb, _, idb := twoSeries()
	local := nodepolicy.Default()
	local.SeriesCeiling = 8192
	file := nodepolicy.Default()
	file.Series = []nodepolicy.SeriesInfo{sb}
	file.Follow = []nodepolicy.Follow{{Series: &idb}}
	_, plan, err := nodepolicy.Merge(local, file, nodepolicy.MergeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Declined) != 1 || !strings.Contains(plan.Declined[0], "1 follow rule(s)") {
		t.Fatalf("declined %v", plan.Declined)
	}
}

// RED-083: series_ceiling is a bounded setting: 0 or 1024..1 GiB.
func TestSeriesCeilingIsBounded(t *testing.T) {
	for _, c := range []struct {
		v  int
		ok bool
	}{{0, true}, {1023, false}, {1024, true}, {1 << 30, true}, {1<<30 + 1, false}, {-1, false}} {
		if err := nodepolicy.BoundSeriesCeiling.Check(c.v); (err == nil) != c.ok {
			t.Errorf("%d: %v", c.v, err)
		}
	}
}
