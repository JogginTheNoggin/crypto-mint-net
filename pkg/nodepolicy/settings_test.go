package nodepolicy

import (
	"bytes"
	"net"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/testutil"
)

// settingsLocal is a node policy that follows a series and has settings
// of its own.
func settingsLocal(t *testing.T) Policy {
	t.Helper()
	params := testutil.ExampleParams()
	id := testutil.MustSeries(params).ID()
	p := Default()
	p.Series = []SeriesInfo{{Params: params}}
	p.Follow = []Follow{{Series: &id}}
	p.P2P.Peers = []string{"local.example:1"}
	p.P2P.MaxPeers = 8
	p.RPC.Listen = "127.0.0.1:7000"
	return p
}

// D155: a settings export holds no series, follows or accepted issuers,
// and machine-local settings only when full.
func TestExportSettings(t *testing.T) {
	p := settingsLocal(t)
	p.Issuers.Accept = []hash.Hash{{1}}
	s := ExportSettings(p, false)
	if !isSettingsOnly(s) {
		t.Fatalf("settings export kept what it follows: %+v", s)
	}
	if s.RPC.Listen != "" || len(s.P2P.Peers) != 1 || s.P2P.MaxPeers != 8 {
		t.Fatalf("settings export: %+v", s)
	}
	if f := ExportSettings(p, true); f.RPC.Listen != "127.0.0.1:7000" || !isSettingsOnly(f) {
		t.Fatalf("full settings export: %+v", f)
	}
	if len(p.Series) == 0 || len(p.Issuers.Accept) != 1 {
		t.Fatal("ExportSettings changed its argument")
	}

	// It reads back as a settings bundle, and one with a series is refused.
	b := NewBundle(ScopeSettings, false, time.Now())
	b.Policies = []BundlePolicy{{Policy: s}}
	var buf bytes.Buffer
	if err := b.Write(&buf); err != nil {
		t.Fatal(err)
	}
	rb, err := ReadBundle(&buf)
	if err != nil || rb.Scope != ScopeSettings {
		t.Fatalf("read back: %v %+v", err, rb)
	}
	b.Policies[0].Policy.Series = p.Series
	buf.Reset()
	b.Write(&buf)
	if _, err := ReadBundle(&buf); err == nil || !strings.Contains(err.Error(), "settings bundle holds no series") {
		t.Fatalf("a settings bundle with a series: %v", err)
	}
}

// D155: a settings import takes the settings and never what to follow,
// even from a whole node policy, and with Replace keeps what the node
// follows.
func TestMergeSettingsOnly(t *testing.T) {
	local := Default()
	local.P2P.Peers = []string{"mine.example:1"}
	file := settingsLocal(t)
	file.P2P.MaxPeers = 12
	res, plan, err := Merge(local, file, MergeOptions{SettingsOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Series) != 0 || len(res.Follow) != 0 {
		t.Fatalf("a settings import took what to follow: %+v", res)
	}
	if res.P2P.MaxPeers != 12 || !slices.Contains(res.P2P.Peers, "local.example:1") || !slices.Contains(res.P2P.Peers, "mine.example:1") {
		t.Fatalf("settings merged: %+v", res.P2P)
	}
	if res.RPC.Listen != "" || !slices.Contains(plan.Skipped, "rpc.listen") {
		t.Fatalf("machine-local taken without Full: %+v %v", res.RPC, plan.Skipped)
	}
	if len(plan.Defences) == 0 {
		t.Fatal("the settings were not named first")
	}

	// Replace: the file's settings, the node's follows.
	local = settingsLocal(t)
	file = Default()
	file.P2P.Peers = []string{"other.example:1"}
	res, _, err = Merge(local, file, MergeOptions{SettingsOnly: true, Replace: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Series) != len(local.Series) || len(res.Follow) != len(local.Follow) || !slices.Equal(res.P2P.Peers, []string{"other.example:1"}) || res.P2P.MaxPeers != 0 {
		t.Fatalf("replace: %+v", res)
	}
	if res.RPC.Listen != "127.0.0.1:7000" {
		t.Fatalf("replace dropped the machine-local rpc: %+v", res.RPC)
	}

	if _, _, err := Merge(local, file, MergeOptions{SettingsOnly: true, SeriesOnly: true}); err == nil {
		t.Fatal("series only and settings only together")
	}
}

// D155: -map-path rewrites registry files at a path boundary, never a
// URL, and before the plan.
func TestMapPaths(t *testing.T) {
	p := Default()
	p.P2P.Registries = []string{"/old/reg.txt", "/older/reg.txt", "/old", "https://old/reg"}
	m, err := ParsePathMap("/old=/new")
	if err != nil {
		t.Fatal(err)
	}
	mp, _ := mapPaths(p, []PathMap{m})
	got := mp.P2P.Registries
	want := []string{"/new/reg.txt", "/older/reg.txt", "/new", "https://old/reg"}
	if !slices.Equal(got, want) {
		t.Fatalf("mapped %v, want %v", got, want)
	}
	if p.P2P.Registries[0] != "/old/reg.txt" {
		t.Fatal("mapPaths changed its argument")
	}
	for _, bad := range []string{"", "a", "=b", "a="} {
		if _, err := ParsePathMap(bad); err == nil {
			t.Fatalf("ParsePathMap(%q) took it", bad)
		}
	}
	res, _, err := Merge(Default(), p, MergeOptions{SettingsOnly: true, Full: true, MapPaths: []PathMap{m}})
	if err != nil || !slices.Contains(res.P2P.Registries, "/new/reg.txt") {
		t.Fatalf("merge with a map: %v %v", err, res.P2P.Registries)
	}
}

// D155: Fit names what an import brings that this machine lacks, and not
// what the node already had.
func TestFit(t *testing.T) {
	m := Machine{CPUs: 4, Devices: []string{"cpu"}, Addrs: []net.IP{net.ParseIP("10.0.0.5")},
		Exists: func(p string) bool { return p == "/here/reg.txt" }}
	before := Default()
	before.P2P.Registries = []string{"/gone/old.txt"}
	after := clone(before)
	after.Grind = &Grind{Device: "gpu:1", Threads: 16, Jobs: []GrindJob{{Target: "tip", Series: &hash.Hash{9}, Device: "cpu", Threads: 2}}}
	after.P2P.Registries = append(after.P2P.Registries, "/here/reg.txt", "/there/reg.txt", "https://x/reg")
	after.P2P.Listen = "10.0.0.9:8646"
	after.RPC.Listen = "127.0.0.1:8645"
	after.P2P.Advertise = "old.example:8646"
	got := strings.Join(Fit(before, after, m), "\n")
	for _, want := range []string{
		`grind.device: no device "gpu:1"`,
		"grind.threads: 16, but this machine runs 4",
		"grind.jobs[0] (tip): series",
		"p2p.registries: /there/reg.txt: no such file",
		"p2p.listen: 10.0.0.9:8646 is not an address of this machine",
		"p2p.advertise: old.example:8646",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("fit lacks %q:\n%s", want, got)
		}
	}
	for _, not := range []string{"/gone/old.txt", "/here/reg.txt", "https://x", "rpc.listen", "jobs[0] (tip).device"} {
		if strings.Contains(got, not) {
			t.Errorf("fit names %q:\n%s", not, got)
		}
	}
	if f := Fit(after, after, m); len(f) != 0 {
		t.Fatalf("an import that changes nothing does not fit: %v", f)
	}
	if f := Fit(before, after, Machine{}); len(f) != 2 {
		t.Fatalf("an unknown machine checks only the job's series and the advertised address: %v", f)
	}
}
