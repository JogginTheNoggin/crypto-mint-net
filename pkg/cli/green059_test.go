package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"testing"
)

// A dev build reports its VCS revision as the release commit, marked when
// its tree was modified; -X Commit wins (GREEN-059).
func TestVCSCommit(t *testing.T) {
	set := func(kv ...string) []debug.BuildSetting {
		var out []debug.BuildSetting
		for i := 0; i < len(kv); i += 2 {
			out = append(out, debug.BuildSetting{Key: kv[i], Value: kv[i+1]})
		}
		return out
	}
	if got := vcsCommit(set("vcs.revision", "abc", "vcs.modified", "false")); got != "abc" {
		t.Fatalf("clean: %q", got)
	}
	if got := vcsCommit(set("vcs.revision", "abc", "vcs.modified", "true")); got != "abc+modified" {
		t.Fatalf("modified: %q", got)
	}
	if got := vcsCommit(set("vcs.modified", "true")); got != "" {
		t.Fatalf("no revision: %q", got)
	}
	old := Commit
	defer func() { Commit = old }()
	Commit = "release-commit"
	if got := releaseCommit(); got != "release-commit" {
		t.Fatalf("-X Commit: %q", got)
	}
}

// poo ledger import -dry-run says whether node.json follows the file's
// pool, and what a real import needs when it does not (GREEN-059).
func TestDryRunSaysPoolFollowed(t *testing.T) {
	root := t.TempDir()
	cli(t, nil, "init", "-example", "-root", root, "-listen", "127.0.0.1:0", "-p2p", "off")
	var pol struct {
		Follow []struct{ Series string }
	}
	b, _ := os.ReadFile(filepath.Join(root, "policies", "default", "node.json"))
	if json.Unmarshal(b, &pol) != nil || len(pol.Follow) == 0 {
		t.Fatalf("node.json: %s", b)
	}
	ser := []struct{ ID string }{{pol.Follow[0].Series}}
	file := func(pool int) string {
		b, _ := json.Marshal(map[string]any{"kind": LedgerFileKind, "format": LedgerFileFormat, "series": ser[0].ID, "pool": pool, "from": 1, "to": 0, "entries": []any{}})
		p := filepath.Join(t.TempDir(), "l.json")
		os.WriteFile(p, b, 0o600)
		return p
	}
	var rep LedgerDryRun
	cli(t, &rep, "ledger", "import", "-dry-run", file(0), "-root", root)
	if !rep.SeriesKnown || !rep.PoolFollowed || rep.Note != "" {
		t.Fatalf("pool 0: %+v", rep)
	}
	cli(t, nil, "unfollow", "-series", ser[0].ID, "-pool", "0", "-no-reload", "-root", root)
	rep = LedgerDryRun{}
	cli(t, &rep, "ledger", "import", "-dry-run", file(0), "-root", root)
	if rep.PoolFollowed || !strings.Contains(rep.Note, "poo follow -series "+ser[0].ID+" -pool 0") {
		t.Fatalf("pool 0 unfollowed: %+v", rep)
	}
	rep = LedgerDryRun{}
	cli(t, &rep, "ledger", "import", "-dry-run", file(1), "-root", root)
	if !rep.PoolFollowed {
		t.Fatalf("pool 1: %+v", rep)
	}
}
