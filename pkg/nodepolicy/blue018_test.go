package nodepolicy_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/nodepolicy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/testutil"
)

// hostile is a file that, besides a series to follow, sets node settings
// that turn off defences, reach outside or start work (RED-043).
func hostile() nodepolicy.Policy {
	sa, _, ida, _ := twoSeries()
	one, zero := 1, 0
	f := nodepolicy.Default()
	f.Series = []nodepolicy.SeriesInfo{sa}
	f.Follow = []nodepolicy.Follow{{Series: &ida}}
	f.K = &zero
	f.P2P.AllowLocalPeers, f.P2P.NoConfirm, f.P2P.StrictReturn, f.P2P.NoNotices, f.P2P.NoRelay = true, true, true, true, true
	f.P2P.RequestRate, f.P2P.UnlistedConns, f.P2P.MaxPeers, f.P2P.EntryLimit = 1<<30, -1, one, 1
	f.P2P.Registries = []string{"http://169.254.169.254/latest/meta-data/", "/etc/passwd"}
	f.P2P.Peers = []string{"198.51.100.7:9000"}
	f.Tracking.NoTimeoutMisses, f.Tracking.StartingPoints = true, 1e9
	f.Grind = &nodepolicy.Grind{Jobs: []nodepolicy.GrindJob{{Target: "tip"}}}
	return f
}

// RED-043, D150: a default import takes only what to follow; every node
// setting of the file is listed as skipped and left alone.
func TestDefaultImportTakesOnlyWhatToFollow(t *testing.T) {
	local := nodepolicy.Default()
	for _, opt := range []nodepolicy.MergeOptions{{}, {PreferFile: true}, {Replace: true}, {Replace: true, PreferFile: true}} {
		res, plan, err := nodepolicy.Merge(local, hostile(), opt)
		if err != nil {
			t.Fatalf("%+v: %v", opt, err)
		}
		want := local
		want.Series, want.Follow = res.Series, res.Follow
		if !samePolicy(t, res, want) {
			t.Fatalf("%+v took node settings: %+v", opt, res)
		}
		if len(res.Series) != 1 || len(res.Follow) != 1 {
			t.Fatalf("%+v did not take the series: %+v", opt, res)
		}
		got := strings.Join(plan.SkippedSettings, " ")
		for _, f := range []string{"p2p.allow_local_peers", "p2p.no_confirm", "p2p.registries", "p2p.peers", "tracking.no_timeout_misses", "grind.jobs", "k"} {
			if !strings.Contains(got, f) {
				t.Errorf("%+v: %s not listed as skipped: %s", opt, f, got)
			}
		}
		if len(plan.Defences) != 0 {
			t.Errorf("%+v: defences %v without -settings", opt, plan.Defences)
		}
	}
}

func samePolicy(t *testing.T, a, b nodepolicy.Policy) bool {
	t.Helper()
	var x, y bytes.Buffer
	bx := nodepolicy.Bundle{Policies: []nodepolicy.BundlePolicy{{Policy: a}}}
	by := nodepolicy.Bundle{Policies: []nodepolicy.BundlePolicy{{Policy: b}}}
	if bx.Write(&x) != nil || by.Write(&y) != nil {
		t.Fatal("encode")
	}
	return x.String() == y.String()
}

// RED-043, D150: with Settings the node settings come in, the lines that
// touch a defence are named, and a registry that is a local file still
// needs Full.
func TestSettingsImportNamesDefences(t *testing.T) {
	res, plan, err := nodepolicy.Merge(nodepolicy.Default(), hostile(), nodepolicy.MergeOptions{Settings: true})
	if err != nil {
		t.Fatal(err)
	}
	if !res.P2P.AllowLocalPeers || len(res.P2P.Peers) != 1 || res.Grind == nil || len(res.Grind.Jobs) != 1 {
		t.Fatalf("settings not taken: %+v", res)
	}
	if len(res.P2P.Registries) != 1 || res.P2P.Registries[0] != "http://169.254.169.254/latest/meta-data/" {
		t.Fatalf("registries: %v", res.P2P.Registries)
	}
	if !strings.Contains(strings.Join(plan.SkippedSettings, " "), "p2p.registries[/etc/passwd] (a local file") {
		t.Fatalf("file registry not listed as skipped: %v", plan.SkippedSettings)
	}
	d := strings.Join(plan.Defences, " ")
	for _, f := range []string{"p2p.allow_local_peers", "p2p.no_confirm", "p2p.registries.[http", "p2p.peers.[198.51.100.7:9000]", "tracking.starting_points", "grind.jobs.[", "k"} {
		if !strings.Contains(d, f) {
			t.Errorf("%s not named as a defence: %s", f, d)
		}
	}
	if strings.Contains(d, "series") || strings.Contains(d, "follow") {
		t.Errorf("a follow line named as a defence: %s", d)
	}
	res, _, err = nodepolicy.Merge(nodepolicy.Default(), hostile(), nodepolicy.MergeOptions{Full: true})
	if err != nil || len(res.P2P.Registries) != 2 {
		t.Fatalf("full: %v %v", res.P2P.Registries, err)
	}
}

// RED-044, D150: merging a bundle of many followed series takes time
// linear in the series: four times the series take about four times as
// long, not sixteen (4000 took about 10 s before).
func TestManySeriesMergeIsLinear(t *testing.T) {
	if testing.Short() {
		t.Skip("builds 5000 series")
	}
	merge := func(n int) time.Duration {
		file := nodepolicy.Default()
		for i := 0; i < n; i++ {
			p := testutil.ExampleParams()
			p.Mints = uint64(8 + 4*i)
			id := testutil.MustSeries(p).ID()
			file.Series = append(file.Series, nodepolicy.SeriesInfo{Params: p})
			file.Follow = append(file.Follow, nodepolicy.Follow{Series: &id, Pools: []uint64{1}})
		}
		best := time.Duration(1 << 62)
		for try := 0; try < 2; try++ {
			start := time.Now()
			res, plan, err := nodepolicy.Merge(nodepolicy.Default(), file, nodepolicy.MergeOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if len(res.Series) != n || len(res.Follow) != n || len(plan.Added) != 2*n {
				t.Fatalf("merged %d series, %d follows, %d lines", len(res.Series), len(res.Follow), len(plan.Added))
			}
			best = min(best, time.Since(start))
		}
		return best
	}
	small, big := merge(1000), merge(4000)
	t.Logf("1000 series: %v, 4000 series: %v", small, big)
	if big > 9*small {
		t.Fatalf("4000 series took %v, 1000 took %v: it grows faster than the series", big, small)
	}
}

// RED-045, D150: a bundle cannot name a node policy Windows would take
// for a device or rename.
func TestBundleRefusesWindowsNames(t *testing.T) {
	for _, n := range []string{"con", "NUL", "com1", "lpt9.x", "aux.json", "name."} {
		in := `{"kind":"poo-node-policies","format":1,"scope":"node","policies":[{"name":"` + n + `","policy":{"format":1}}]}`
		if _, err := nodepolicy.ReadBundle(strings.NewReader(in)); !errors.Is(err, nodepolicy.ErrBundle) {
			t.Errorf("%q: %v", n, err)
		}
		m := nodepolicy.Manager{Root: t.TempDir()}
		if err := m.Save(n, nodepolicy.Default()); !errors.Is(err, nodepolicy.ErrName) {
			t.Errorf("save %q: %v", n, err)
		}
	}
	for _, n := range []string{"console", "com", "comx", "default", "a.b"} {
		m := nodepolicy.Manager{Root: t.TempDir()}
		if err := m.Save(n, nodepolicy.Default()); err != nil {
			t.Errorf("save %q: %v", n, err)
		}
	}
}
