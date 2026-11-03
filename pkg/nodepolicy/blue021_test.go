package nodepolicy

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/testutil"
)

// RED-052, D156: a plain export leaves out registry files, registry URLs
// with a user, password, query or fragment, and grind jobs on one payment
// or entry; it says so, and how many addresses the file holds. -full
// keeps them all.
func TestExportLeavesOutPersonal(t *testing.T) {
	p := Default()
	p.P2P.Peers = []string{"192.168.1.20:8646"}
	p.Tracking.Peers = map[string]string{"192.168.1.20:8646": "detailed"}
	p.P2P.Registries = []string{"/home/alice/regs/mine.json", "https://bob:hunter2@reg.example/list.json", "https://reg.example/list.json?key=SECRET", "https://reg.example/l#k", "https://reg.example/ok.json"}
	p.Grind = &Grind{Jobs: []GrindJob{{Target: "tip"}, {Target: "tx:00ff"}, {Target: "pending"}}}
	e := ExportPolicy(p, false)
	if !slices.Equal(e.P2P.Registries, []string{"https://reg.example/ok.json"}) {
		t.Fatalf("registries %v", e.P2P.Registries)
	}
	if e.Grind == nil || len(e.Grind.Jobs) != 2 || e.Grind.Jobs[0].Target != "tip" || e.Grind.Jobs[1].Target != "pending" {
		t.Fatalf("jobs %+v", e.Grind)
	}
	s := ExportSettings(p, false)
	for _, bad := range []string{"alice", "hunter2", "SECRET", "tx:00ff"} {
		if strings.Contains(strings.Join(s.P2P.Registries, " "), bad) || (s.Grind != nil && slices.ContainsFunc(s.Grind.Jobs, func(j GrindJob) bool { return j.Target == bad })) {
			t.Fatalf("settings export holds %q", bad)
		}
	}
	notes := strings.Join(ExportNotes(p, false), "\n")
	for _, want := range []string{"left out 4 registries", "left out 1 grind job", "the file holds 1 peer address (p2p.peers), 1 tracked peer (tracking.peers), 1 registry (p2p.registries)"} {
		if !strings.Contains(notes, want) {
			t.Fatalf("notes miss %q:\n%s", want, notes)
		}
	}
	if f := ExportPolicy(p, true); len(f.P2P.Registries) != 5 || len(f.Grind.Jobs) != 3 {
		t.Fatalf("-full left something out: %+v", f)
	}
	if n := ExportNotes(p, true); len(n) != 1 || !strings.Contains(n[0], "5 registries") {
		t.Fatalf("-full notes %v", n)
	}
	if len(p.P2P.Registries) != 5 {
		t.Fatal("ExportPolicy changed its argument")
	}
}

// RED-053, D156: text with a control character is refused wherever it is
// in a node policy, and the error shows it escaped.
func TestControlCharactersRefused(t *testing.T) {
	for _, edit := range []func(*Policy){
		func(p *Policy) { p.P2P.Registries = []string{"https://reg.example/x\x1b[8A\x1b[J"} },
		func(p *Policy) { p.P2P.Peers = []string{"a.example:1\nnote: fake line"} },
		func(p *Policy) { p.Tracking.Peers = map[string]string{"a.example:1\r": "detailed"} },
		func(p *Policy) { p.P2P.Registries = []string{"https://reg.example/\u202ex"} },
		func(p *Policy) { p.P2P.Registries = []string{"https://reg.example/\xff"} },
	} {
		p := Default()
		edit(&p)
		err := p.Validate()
		if !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), "control or invisible character") {
			t.Fatalf("took it: %v", err)
		}
		if strings.ContainsAny(err.Error(), "\x1b\n\r\u202e") {
			t.Fatalf("the error holds the raw character: %q", err)
		}
		if _, _, err := Merge(Default(), p, MergeOptions{SettingsOnly: true}); err == nil && utf8.ValidString(strings.Join(p.P2P.Registries, "")) {
			// (Bytes that are not UTF-8 become U+FFFD on the way through
			// JSON, as they do when a bundle is read.)
			t.Fatalf("Merge took %+v", p.P2P)
		}
	}
}

// RED-053, D156: the preview does not open a network or device path.
func TestFitDoesNotOpenNetworkPaths(t *testing.T) {
	opened := []string{}
	m := Machine{Exists: func(path string) bool { opened = append(opened, path); return false }}
	after := Default()
	after.P2P.Registries = []string{`\\host\share\x`, "//host/share/x", `\\?\C:\x`, "/here/x"}
	lines := Fit(Default(), after, m)
	if !slices.Equal(opened, []string{"/here/x"}) {
		t.Fatalf("opened %v", opened)
	}
	if len(lines) != 4 || !strings.Contains(lines[0], "the preview does not open it") || !strings.Contains(lines[3], "no such file here") {
		t.Fatalf("lines %v", lines)
	}
}

// RED-054, D156: a conflict where the file's value was taken is a change,
// even when it is the only one, so -apply saves it.
func TestPreferFileConflictAloneIsAChange(t *testing.T) {
	local, file := Default(), Default()
	local.Issuers.Any = false
	res, plan, err := Merge(local, file, MergeOptions{SettingsOnly: true, PreferFile: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Conflicts) != 1 || plan.Conflicts[0].Took != "file" || !res.Issuers.Any {
		t.Fatalf("plan %+v, any %v", plan.Conflicts, res.Issuers.Any)
	}
	if plan.Empty() {
		t.Fatal("a plan that takes the file's value is empty")
	}
	if _, plan, _ := Merge(local, file, MergeOptions{SettingsOnly: true}); !plan.Empty() {
		t.Fatalf("a conflict that kept local's value is a change: %+v", plan)
	}
}

// RED-055, D156: -map-path joins the rest of a path with one separator,
// To's, and matches a Windows From in any case.
func TestMapPathJoins(t *testing.T) {
	for _, c := range []struct{ from, to, loc, want string }{
		{"/home/alice/", "/srv/bob", "/home/alice/reg/a.json", "/srv/bob/reg/a.json"},
		{"/", "/srv", "/home/alice/reg/a.json", "/srv/home/alice/reg/a.json"},
		{"/home/alice", "/srv/bob/", "/home/alice/reg/a.json", "/srv/bob/reg/a.json"},
		{"/home/alice", "/", "/home/alice/a.json", "/a.json"},
		{`C:\Users\alice`, "/srv", `C:\Users\alice\r.json`, "/srv/r.json"},
		{`c:\users\alice`, "/srv", `C:\Users\alice\sub\r.json`, "/srv/sub/r.json"},
		{"/home/alice", `D:\data`, "/home/alice/reg/a.json", `D:\data\reg\a.json`},
		{"/home/alice", "/srv", "/home/alice", "/srv"},
		{"/data", "/srv", "/database/x", "/database/x"},
		{"/Home/alice", "/srv", "/home/alice/x", "/home/alice/x"},
	} {
		got, _ := PathMap{From: c.from, To: c.to}.apply(c.loc)
		if got != c.want {
			t.Errorf("%s=%s on %s: %s, want %s", c.from, c.to, c.loc, got, c.want)
		}
	}
}

// RED-055, D156: a registry file -map-path points at this machine is taken
// without -full; one it does not map is still skipped, and so are the
// machine-local settings.
func TestMapPathWithoutFull(t *testing.T) {
	file := Default()
	file.P2P.Registries = []string{"/old/reg.txt", "/elsewhere/reg.txt"}
	file.RPC.Listen = "0.0.0.0:7000"
	res, plan, err := Merge(Default(), file, MergeOptions{SettingsOnly: true, MapPaths: []PathMap{{From: "/old", To: "/new"}}})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(res.P2P.Registries, []string{"/new/reg.txt"}) || res.RPC.Listen != "" {
		t.Fatalf("took %v, rpc %q", res.P2P.Registries, res.RPC.Listen)
	}
	if !slices.ContainsFunc(plan.SkippedSettings, func(s string) bool { return strings.Contains(s, "/elsewhere/reg.txt") }) {
		t.Fatalf("skipped %v", plan.SkippedSettings)
	}
}

// RED-056, D156: a settings import that turns on a series the node lists
// but does not use names it, under the lines to read first.
func TestSettingsImportNamesSeriesNowUsed(t *testing.T) {
	params := testutil.ExampleParams()
	id := testutil.MustSeries(params).ID()
	local := Default()
	local.Issuers.Any = false
	local.Series = []SeriesInfo{{Params: params}}
	local.Follow = []Follow{{Series: &id}}
	file := Default() // made on a default node: issuers.any is true
	res, plan, err := Merge(local, file, MergeOptions{SettingsOnly: true, Replace: true})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Issuers.Any || !slices.Equal(plan.NowUsed, []string{id.Short()}) || !slices.Contains(plan.Defences, "now used: series "+id.Short()) {
		t.Fatalf("now used %v, read first %v", plan.NowUsed, plan.Defences)
	}
	// The default import keeps local's value, and uses nothing new.
	if _, plan, _ := Merge(local, file, MergeOptions{SettingsOnly: true}); len(plan.NowUsed) != 0 {
		t.Fatalf("default import: now used %v", plan.NowUsed)
	}
}
