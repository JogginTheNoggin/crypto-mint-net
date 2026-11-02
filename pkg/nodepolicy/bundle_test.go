package nodepolicy_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/nodepolicy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/testutil"
)

// twoSeries returns two unsigned series and their ids.
func twoSeries() (a, b nodepolicy.SeriesInfo, ida, idb hash.Hash) {
	pa := testutil.ExampleParams()
	pb := testutil.ExampleParams()
	pb.Mints = 12
	return nodepolicy.SeriesInfo{Params: pa}, nodepolicy.SeriesInfo{Params: pb},
		testutil.MustSeries(pa).ID(), testutil.MustSeries(pb).ID()
}

func fields(cs []nodepolicy.Change) string {
	var out []string
	for _, c := range cs {
		out = append(out, c.Field)
	}
	return strings.Join(out, " ")
}

func TestExportStripsMachineLocal(t *testing.T) {
	sa, _, ida, _ := twoSeries()
	p := nodepolicy.Default()
	p.Series = []nodepolicy.SeriesInfo{sa}
	p.Follow = []nodepolicy.Follow{{Series: &ida}}
	p.RPC = nodepolicy.RPC{Listen: "0.0.0.0:9000", AllowRemote: true}
	p.NoSync = true
	p.P2P = nodepolicy.P2P{Listen: ":1", Advertise: "example.org:1", Peers: []string{"a.example:1"}, DepthLimit: 5, MaxPeers: 9}
	p.Grind = &nodepolicy.Grind{Device: "gpu:0", Threads: 7, LayerFloor: 3, Jobs: []nodepolicy.GrindJob{{Target: "tip", Device: "gpu:1", Threads: 2, Hours: "22-06"}}}

	s := nodepolicy.ExportPolicy(p, false)
	if s.RPC != (nodepolicy.RPC{}) || s.NoSync || s.P2P.Listen != "" || s.P2P.Advertise != "" || s.P2P.DepthLimit != 0 {
		t.Fatalf("machine-local kept: %+v", s)
	}
	if s.Grind.Device != "" || s.Grind.Threads != 0 || s.Grind.Jobs[0].Device != "" || s.Grind.Jobs[0].Threads != 0 {
		t.Fatalf("grind machine-local kept: %+v", s.Grind)
	}
	if s.P2P.MaxPeers != 9 || len(s.P2P.Peers) != 1 || s.Grind.LayerFloor != 3 || s.Grind.Jobs[0].Hours != "22-06" || len(s.Follow) != 1 {
		t.Fatalf("shared settings lost: %+v", s)
	}
	if p.RPC.Listen == "" || p.Grind.Device == "" {
		t.Fatal("export changed its input")
	}
	full := nodepolicy.ExportPolicy(p, true)
	if full.RPC != p.RPC || full.Grind.Device != "gpu:0" || full.P2P.DepthLimit != 0 {
		t.Fatalf("full export: %+v", full)
	}
}

func TestBundleRoundTrip(t *testing.T) {
	sa, _, ida, _ := twoSeries()
	p := nodepolicy.Default()
	p.Series = []nodepolicy.SeriesInfo{sa}
	p.Follow = []nodepolicy.Follow{{Series: &ida, Pools: []uint64{1}}}
	b := nodepolicy.NewBundle(nodepolicy.ScopeNode, false, time.Unix(0, 0))
	b.Policies = []nodepolicy.BundlePolicy{{Name: "main", Current: true, Policy: p}, {Name: "spare", Policy: nodepolicy.Default()}}
	var buf bytes.Buffer
	if err := b.Write(&buf); err != nil {
		t.Fatal(err)
	}
	got, err := nodepolicy.ReadBundle(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if got.Scope != nodepolicy.ScopeNode || len(got.Policies) != 2 || !got.Policies[0].Current || got.Policies[0].Policy.Follow[0].Pools[0] != 1 {
		t.Fatalf("round trip: %+v", got)
	}

	// A plain node.json reads as a one-policy bundle.
	plain := `{"format":1,"series":[],"issuers":{"any":true},"follow":[],"rpc":{},"p2p":{}}`
	got, err = nodepolicy.ReadBundle(strings.NewReader(plain))
	if err != nil || got.Scope != nodepolicy.ScopePolicy || len(got.Policies) != 1 {
		t.Fatalf("node.json: %+v %v", got, err)
	}
}

func TestReadBundleRefuses(t *testing.T) {
	for _, c := range []struct{ name, in string }{
		{"not json", `{`},
		{"other kind", `{"kind":"x","format":1}`},
		{"newer format", `{"kind":"poo-node-policies","format":2,"scope":"policy","policies":[]}`},
		{"unknown field", `{"kind":"poo-node-policies","format":1,"scope":"policy","policies":[{"policy":{"format":1,"issuers":{"any":true},"rpc":{},"p2p":{},"surprise":1}}]}`},
		{"unknown scope", `{"kind":"poo-node-policies","format":1,"scope":"all","policies":[{"policy":{"format":1,"issuers":{"any":true}}}]}`},
		{"empty", `{"kind":"poo-node-policies","format":1,"scope":"policy","policies":[]}`},
		{"two in a policy bundle", `{"kind":"poo-node-policies","format":1,"scope":"policy","policies":[{"policy":{"format":1}},{"policy":{"format":1}}]}`},
		{"node bundle needs names", `{"kind":"poo-node-policies","format":1,"scope":"node","policies":[{"policy":{"format":1}}]}`},
		{"same name twice", `{"kind":"poo-node-policies","format":1,"scope":"node","policies":[{"name":"a","policy":{"format":1}},{"name":"a","policy":{"format":1}}]}`},
		{"bad name", `{"kind":"poo-node-policies","format":1,"scope":"node","policies":[{"name":"../x","policy":{"format":1}}]}`},
		{"invalid policy", `{"kind":"poo-node-policies","format":1,"scope":"policy","policies":[{"policy":{"format":1,"k":-5}}]}`},
		{"remote rpc", `{"kind":"poo-node-policies","format":1,"scope":"policy","policies":[{"policy":{"format":1,"rpc":{"listen":"0.0.0.0:1"}}}]}`},
		{"series bundle without a series", `{"kind":"poo-node-policies","format":1,"scope":"series","policies":[{"policy":{"format":1}}]}`},
	} {
		if _, err := nodepolicy.ReadBundle(strings.NewReader(c.in)); !errors.Is(err, nodepolicy.ErrBundle) {
			t.Errorf("%s: %v, want ErrBundle", c.name, err)
		}
	}
	big := strings.NewReader(`{"kind":"` + strings.Repeat("x", nodepolicy.MaxBundleSize) + `"}`)
	if _, err := nodepolicy.ReadBundle(big); !errors.Is(err, nodepolicy.ErrBundle) {
		t.Errorf("oversized: %v", err)
	}
}

func TestMergeAddsAndKeepsLocal(t *testing.T) {
	sa, sb, ida, idb := twoSeries()
	k4, k8 := 4, 8
	local := nodepolicy.Default()
	local.Series = []nodepolicy.SeriesInfo{sa}
	local.Follow = []nodepolicy.Follow{{Series: &ida, Pools: []uint64{0}}}
	local.K = &k4
	local.P2P.Peers = []string{"a.example:1"}
	local.RPC.Listen = "127.0.0.1:7000"

	file := nodepolicy.Default()
	file.Series = []nodepolicy.SeriesInfo{sa, sb}
	file.Follow = []nodepolicy.Follow{{Series: &ida, Pools: []uint64{1}}, {Series: &idb}}
	file.K = &k8
	file.MaxPending = 50
	file.P2P.Peers = []string{"b.example:1", "a.example:1"}
	file.RPC.Listen = "127.0.0.1:9999"

	res, plan, err := nodepolicy.Merge(local, file, nodepolicy.MergeOptions{Settings: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Series) != 2 || *res.K != 4 || res.MaxPending != 50 || len(res.P2P.Peers) != 2 || res.RPC.Listen != "127.0.0.1:7000" {
		t.Fatalf("merged: %+v", res)
	}
	var pools []uint64
	whole := false
	for _, f := range res.Follow {
		if f.Series != nil && *f.Series == ida {
			pools = f.Pools
		}
		if f.Series != nil && *f.Series == idb && len(f.Pools) == 0 {
			whole = true
		}
	}
	if len(pools) != 2 || !whole {
		t.Fatalf("follow: %+v", res.Follow)
	}
	if fields(plan.Conflicts) != "k" || plan.Conflicts[0].Took != "local" {
		t.Fatalf("conflicts: %+v", plan.Conflicts)
	}
	if len(plan.Skipped) != 1 || plan.Skipped[0] != "rpc.listen" {
		t.Fatalf("skipped: %v", plan.Skipped)
	}
	added := fields(plan.Added)
	for _, want := range []string{"max_pending", "series.[" + idb.Short() + "]", "follow.[series " + idb.Short() + "]", "p2p.peers.[b.example:1]"} {
		if !strings.Contains(added, want) {
			t.Errorf("added %q lacks %q", added, want)
		}
	}
	if fields(plan.Changed) != "follow.[series "+ida.Short()+"]" {
		t.Errorf("changed: %+v", plan.Changed)
	}
	if plan.Same == 0 || plan.Empty() {
		t.Errorf("plan: %+v", plan)
	}
	if *local.K != 4 || len(local.Series) != 1 || len(local.Follow[0].Pools) != 1 {
		t.Fatal("merge changed its input")
	}

	// Taking the file's on a conflict, and the machine-local settings.
	res, plan, err = nodepolicy.Merge(local, file, nodepolicy.MergeOptions{PreferFile: true, Full: true})
	if err != nil || *res.K != 8 || res.RPC.Listen != "127.0.0.1:9999" || plan.Conflicts[0].Took != "file" {
		t.Fatalf("prefer file: %+v %+v %v", res, plan, err)
	}

	// Importing the result again changes nothing.
	again, plan, err := nodepolicy.Merge(res, file, nodepolicy.MergeOptions{Full: true})
	if err != nil || !plan.Empty() || len(plan.Conflicts) != 0 || !again.SameApplied(res) {
		t.Fatalf("second import: %+v %v", plan, err)
	}
}

func TestMergeReplace(t *testing.T) {
	sa, sb, _, idb := twoSeries()
	local := nodepolicy.Default()
	local.Series = []nodepolicy.SeriesInfo{sa}
	local.RPC.Listen = "127.0.0.1:7000"
	local.Grind = &nodepolicy.Grind{Device: "cpu"}
	file := nodepolicy.Default()
	file.Series = []nodepolicy.SeriesInfo{sb}
	file.Follow = []nodepolicy.Follow{{Series: &idb}}
	res, plan, err := nodepolicy.Merge(local, file, nodepolicy.MergeOptions{Replace: true, Settings: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Series) != 1 || res.RPC.Listen != "127.0.0.1:7000" || res.Grind == nil || res.Grind.Device != "cpu" {
		t.Fatalf("replace: %+v", res)
	}
	if len(plan.Removed) != 1 || !strings.HasPrefix(plan.Removed[0].Field, "series.") {
		t.Fatalf("removed: %+v", plan.Removed)
	}
}

func TestMergeSeriesOnly(t *testing.T) {
	sa, sb, ida, idb := twoSeries()
	local := nodepolicy.Default()
	local.Series = []nodepolicy.SeriesInfo{sa}
	local.Follow = []nodepolicy.Follow{{Series: &ida}}
	src := nodepolicy.Default()
	src.Series = []nodepolicy.SeriesInfo{sa, sb}
	src.Follow = []nodepolicy.Follow{{Series: &ida}, {Series: &idb, Pools: []uint64{2}}}
	src.Issuers.Any = false
	src.MaxPending = 9
	share, err := nodepolicy.ExportSeries(src, idb)
	if err != nil {
		t.Fatal(err)
	}
	if len(share.Series) != 1 || len(share.Follow) != 1 || share.MaxPending != 0 {
		t.Fatalf("share: %+v", share)
	}
	res, plan, err := nodepolicy.Merge(local, src, nodepolicy.MergeOptions{SeriesOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Issuers.Any || res.MaxPending != 0 || len(res.Series) != 2 || len(plan.Conflicts) != 0 {
		t.Fatalf("series only touched more: %+v %+v", res, plan)
	}
	if _, err := nodepolicy.ExportSeries(src, hash.Hash{}); !errors.Is(err, nodepolicy.ErrInvalid) {
		t.Fatalf("unknown series: %v", err)
	}
}

func TestMergeWitnessConflictAndRefused(t *testing.T) {
	a, b := newIssuer("a"), newIssuer("b")
	sa := a.signed(12)
	other := sa
	other.IssuerWitness = b.signed(12).IssuerWitness // the wrong signer
	local := nodepolicy.Default()
	local.Issuers.Any = false
	local.Series = []nodepolicy.SeriesInfo{{Params: sa.Params}}
	file := nodepolicy.Default()
	file.Issuers.Any = false
	file.Series = []nodepolicy.SeriesInfo{sa}

	// A witness local lacks is added; then a different one conflicts.
	res, plan, err := nodepolicy.Merge(local, file, nodepolicy.MergeOptions{})
	if err != nil || res.Series[0].IssuerWitness != sa.IssuerWitness || len(plan.Conflicts) != 0 {
		t.Fatalf("witness added: %+v %v", plan, err)
	}
	// The issuer is not accepted: the plan says so.
	if len(plan.Refused) != 1 || !strings.Contains(plan.Refused[0], "issuer not accepted") {
		t.Fatalf("refused: %v", plan.Refused)
	}
	file.Series = []nodepolicy.SeriesInfo{other}
	_, plan, err = nodepolicy.Merge(res, file, nodepolicy.MergeOptions{})
	if err != nil || len(plan.Conflicts) != 1 || !strings.HasSuffix(plan.Conflicts[0].Field, "issuer_witness") {
		t.Fatalf("witness conflict: %+v %v", plan, err)
	}
}

func TestMergeRefusesWhatWouldNotValidate(t *testing.T) {
	sa, _, _, _ := twoSeries()
	local := nodepolicy.Default()
	local.Grind = &nodepolicy.Grind{}
	for i := 0; i < nodepolicy.MaxGrindJobs; i++ {
		local.Grind.Jobs = append(local.Grind.Jobs, nodepolicy.GrindJob{Target: "tip", Hours: []string{"0-1", "1-2", "2-3", "3-4", "4-5", "5-6", "6-7", "7-8", "8-9", "9-10", "10-11", "11-12", "12-13", "13-14", "14-15", "15-16"}[i]})
	}
	file := nodepolicy.Default()
	file.Series = []nodepolicy.SeriesInfo{sa}
	file.Grind = &nodepolicy.Grind{Jobs: []nodepolicy.GrindJob{{Target: "pending"}}}
	if _, _, err := nodepolicy.Merge(local, file, nodepolicy.MergeOptions{Settings: true}); !errors.Is(err, nodepolicy.ErrInvalid) {
		t.Fatalf("too many jobs: %v", err)
	}
	// A follow of a series neither lists.
	missing := hash.Hash{1}
	file = nodepolicy.Default()
	file.Follow = []nodepolicy.Follow{{Series: &missing}}
	if _, _, err := nodepolicy.Merge(nodepolicy.Default(), file, nodepolicy.MergeOptions{}); !errors.Is(err, nodepolicy.ErrInvalid) {
		t.Fatalf("follow of an unlisted series: %v", err)
	}
}
