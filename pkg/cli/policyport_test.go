package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/nodepolicy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/testutil"
)

func runOut(t *testing.T, args ...string) string {
	t.Helper()
	var out bytes.Buffer
	if err := run(args, &out); err != nil {
		t.Fatalf("poo %s: %v\n%s", strings.Join(args, " "), err, out.String())
	}
	return out.String()
}

// D149: a node policy exported on one machine imports on another, the
// plan comes first, and importing it twice changes nothing.
func TestPolicyExportImport(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	cli(t, nil, "init", "-example", "-root", src, "-listen", "127.0.0.1:7111", "-p2p", "off", "-peer", "a.example:1")
	cli(t, nil, "init", "-root", dst, "-p2p", "off")
	bundle := filepath.Join(t.TempDir(), "share.json")
	runOut(t, "policy", "export", "-root", src, "-out", bundle)

	b, err := os.ReadFile(bundle)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "7111") || !strings.Contains(string(b), nodepolicy.BundleKind) {
		t.Fatalf("export kept the RPC address or is not a bundle:\n%s", b)
	}

	// The plan only.
	dstPath := filepath.Join(dst, "policies", "default", nodepolicy.FileName)
	before, _ := os.ReadFile(dstPath)
	out := runOut(t, "policy", "import", bundle, "-root", dst)
	id := testutil.ExampleSeries().ID().Short()
	if !strings.Contains(out, "+ series: "+id) || !strings.Contains(out, "dry run") {
		t.Fatalf("plan:\n%s", out)
	}
	if after, _ := os.ReadFile(dstPath); !bytes.Equal(before, after) {
		t.Fatal("a dry run changed node.json")
	}

	// Applied, offline.
	out = runOut(t, "policy", "import", bundle, "-apply", "-root", dst)
	if !strings.Contains(out, "saved") {
		t.Fatalf("apply:\n%s", out)
	}
	p, err := nodepolicy.Load(dstPath)
	if err != nil {
		t.Fatal(err)
	}
	// What to follow only: the peer is a node setting (RED-043, D150).
	if len(p.Series) != 1 || len(p.Follow) != 1 || len(p.P2P.Peers) != 0 || p.P2P.Listen != "off" {
		t.Fatalf("imported: %+v", p)
	}
	if !strings.Contains(out, "skipped, node settings (-settings takes them): p2p.peers: a.example:1") {
		t.Fatalf("apply did not say what it skipped:\n%s", out)
	}
	out = runOut(t, "policy", "import", bundle, "-apply", "-settings", "-root", dst)
	if !strings.Contains(out, "read first") || !strings.Contains(out, "many pools: p2p.peers: a.example:1") {
		t.Fatalf("-settings did not name the peer first:\n%s", out)
	}
	if p, _ = nodepolicy.Load(dstPath); len(p.P2P.Peers) != 1 || p.P2P.Listen != "off" {
		t.Fatalf("imported with -settings: %+v", p)
	}
	out = runOut(t, "policy", "import", bundle, "-apply", "-settings", "-root", dst)
	if !strings.Contains(out, "nothing to change") {
		t.Fatalf("second import:\n%s", out)
	}

	// A series bundle, and JSON output.
	sb := filepath.Join(t.TempDir(), "series.json")
	runOut(t, "policy", "export", "-root", src, "-series", testutil.ExampleSeries().ID().String(), "-out", sb)
	var res []importResult
	cli(t, &res, "policy", "import", sb, "-json", "-root", dst)
	if len(res) != 1 || !res[0].Plan.Empty() || res[0].Plan.Same == 0 {
		t.Fatalf("series import: %+v", res)
	}
}

// D149: a whole node's policies go into one file and come back under
// their names, the current one made current where none is.
func TestPolicyExportAllImportsEachByName(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	cli(t, nil, "init", "-example", "-root", src, "-p2p", "off")
	cli(t, nil, "init", "-root", src, "-policy", "spare", "-p2p", "off")
	runOut(t, "policy", "use", "-root", src, "default")
	bundle := filepath.Join(t.TempDir(), "all.json")
	runOut(t, "policy", "export", "-all", "-root", src, "-out", bundle)

	if err := run([]string{"policy", "import", bundle, "-root", dst, "-policy", "x"}, &bytes.Buffer{}); err == nil {
		t.Fatal("a node bundle took -policy")
	}
	out := runOut(t, "policy", "import", bundle, "-apply", "-root", dst)
	if !strings.Contains(out, `node policy "default"`) || !strings.Contains(out, `node policy "spare"`) || !strings.Contains(out, "made current") {
		t.Fatalf("import -all:\n%s", out)
	}
	m := nodepolicy.Manager{Root: dst}
	if cur, _ := m.Current(); cur != "default" {
		t.Fatalf("current %q", cur)
	}
	if p, err := m.Load("default"); err != nil || len(p.Series) != 1 {
		t.Fatalf("default: %+v %v", p, err)
	}

	// An archived policy of the same name stops the import before any
	// change.
	runOut(t, "policy", "archive", "-root", dst, "spare")
	if err := run([]string{"policy", "import", bundle, "-apply", "-root", dst}, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "archived") {
		t.Fatalf("archived: %v", err)
	}
}

// D149: a running node takes an import through its RPC, under its own
// lock, and applies it at once.
func TestPolicyImportIntoRunningNode(t *testing.T) {
	src, root := t.TempDir(), t.TempDir()
	cli(t, nil, "init", "-example", "-root", src, "-p2p", "off")
	cli(t, nil, "init", "-root", root, "-listen", "127.0.0.1:0", "-p2p", "off")
	bundle := filepath.Join(t.TempDir(), "share.json")
	runOut(t, "policy", "export", "-root", src, "-out", bundle)
	dir := filepath.Join(root, "policies", "default")
	stop := startNode(t, dir)
	defer stop()

	out := runOut(t, "policy", "import", bundle, "-root", root)
	if !strings.Contains(out, "running") || !strings.Contains(out, "dry run") {
		t.Fatalf("plan:\n%s", out)
	}
	out = runOut(t, "policy", "import", bundle, "-apply", "-root", root)
	if !strings.Contains(out, "applied: the node reloaded") {
		t.Fatalf("apply:\n%s", out)
	}
	waitFor(t, func() bool {
		var b bytes.Buffer
		if run([]string{"follow", "-root", root}, &b) != nil {
			return false
		}
		return strings.Contains(b.String(), testutil.ExampleSeries().ID().String())
	})
}

// D149: a running node's interface never takes an rpc section from an
// import, even with -full: that changes only with the node stopped.
func TestPolicyImportKeepsRunningRPC(t *testing.T) {
	root := t.TempDir()
	cli(t, nil, "init", "-root", root, "-listen", "127.0.0.1:0", "-p2p", "off")
	dir := filepath.Join(root, "policies", "default")
	stop := startNode(t, dir)
	defer stop()
	p := nodepolicy.Default()
	p.RPC = nodepolicy.RPC{Listen: "0.0.0.0:9999", AllowRemote: true}
	b := nodepolicy.NewBundle(nodepolicy.ScopePolicy, true, time.Now())
	b.Policies = []nodepolicy.BundlePolicy{{Policy: p}}
	file := filepath.Join(t.TempDir(), "full.json")
	var buf bytes.Buffer
	if err := b.Write(&buf); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(file, buf.Bytes(), 0o600)
	out := runOut(t, "policy", "import", file, "-full", "-apply", "-root", root)
	if !strings.Contains(out, "changes only with the node stopped") {
		t.Fatalf("import:\n%s", out)
	}
	got, err := nodepolicy.Load(filepath.Join(dir, nodepolicy.FileName))
	if err != nil || got.RPC.AllowRemote || got.RPC.Listen != "127.0.0.1:0" {
		t.Fatalf("rpc section changed: %+v %v", got.RPC, err)
	}
}

// D149: "-" reads the bundle from standard input, as poo submit -f -
// does, so the GUI can hand over a file picked in the browser.
func TestPolicyImportFromStdin(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	cli(t, nil, "init", "-example", "-root", src, "-p2p", "off")
	cli(t, nil, "init", "-root", dst, "-p2p", "off")
	bundle := runOut(t, "policy", "export", "-root", src)
	var out bytes.Buffer
	if err := Run(context.Background(), []string{"policy", "import", "-", "-apply", "-root", dst}, Stdio{In: strings.NewReader(bundle), Out: &out, Err: &out}); err != nil {
		t.Fatalf("import -: %v\n%s", err, out.String())
	}
	p, err := nodepolicy.Load(filepath.Join(dst, "policies", "default", nodepolicy.FileName))
	if err != nil || len(p.Series) != 1 {
		t.Fatalf("imported from stdin: %+v %v\n%s", p, err, out.String())
	}
	// Cut off: nothing is read or changed.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := Run(ctx, []string{"policy", "import", "-", "-apply", "-root", t.TempDir()}, Stdio{In: strings.NewReader(bundle), Out: &out, Err: &out}); err == nil {
		t.Fatal("a cut-off standard input was imported")
	}
}

// A plan line says what would be set, and an import with nothing to
// change does not tell the person to add -apply (GREEN-042).
func TestPlanLinesShowValues(t *testing.T) {
	short := func(s string) string { return s }
	for _, c := range []struct{ field, value, want string }{
		{"k", "3", "k: 3"},
		{`grind.jobs.[{"target":"pending"}]`, "true", `grind.jobs: {"target":"pending"}`},
		{"p2p.peers.[10.0.0.1:8646]", "true", "p2p.peers: 10.0.0.1:8646"},
		{"log", "", "log"},
	} {
		if got := planLine(c.field, c.value, short); got != c.want {
			t.Errorf("planLine(%q, %q) = %q, want %q", c.field, c.value, got, c.want)
		}
	}
	root := t.TempDir()
	cli(t, nil, "init", "-example", "-root", root, "-p2p", "off")
	bundle := filepath.Join(t.TempDir(), "same.json")
	runOut(t, "policy", "export", "-root", root, "-out", bundle)
	if out := runOut(t, "policy", "import", bundle, "-root", root); strings.Contains(out, "dry run") || !strings.Contains(out, "nothing to change") {
		t.Fatalf("importing a node's own policy:\n%s", out)
	}
}

// GUI-023, D151: -if-digest applies only the file that was previewed.
func TestPolicyImportIfDigest(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	cli(t, nil, "init", "-example", "-root", src, "-p2p", "off")
	cli(t, nil, "init", "-root", dst, "-p2p", "off")
	a := filepath.Join(t.TempDir(), "a.json")
	runOut(t, "policy", "export", "-root", src, "-out", a)
	var res []importResult
	cli(t, &res, "policy", "import", a, "-json", "-root", dst)
	if len(res) != 1 || len(res[0].Digest) != 64 {
		t.Fatalf("digest: %+v", res)
	}
	d := res[0].Digest
	// The file changes after the preview: refused, nothing changed.
	b, _ := os.ReadFile(a)
	os.WriteFile(a, append(b, ' '), 0o600)
	before, _ := os.ReadFile(filepath.Join(dst, "policies", "default", nodepolicy.FileName))
	if err := run([]string{"policy", "import", a, "-apply", "-if-digest", d, "-root", dst}, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "not the one previewed") {
		t.Fatalf("changed file: %v", err)
	}
	if after, _ := os.ReadFile(filepath.Join(dst, "policies", "default", nodepolicy.FileName)); !bytes.Equal(before, after) {
		t.Fatal("a refused import changed node.json")
	}
	os.WriteFile(a, b, 0o600)
	runOut(t, "policy", "import", a, "-apply", "-if-digest", d, "-root", dst)
	p, err := nodepolicy.Load(filepath.Join(dst, "policies", "default", nodepolicy.FileName))
	if err != nil || len(p.Series) != 1 {
		t.Fatalf("applied: %+v %v", p, err)
	}
}

// The plan's notes name fields as its lines do, the digest comes with a
// preview that has something to apply, and one setting "is" the same
// (GREEN-043).
func TestPlanNotesReadLikeItsLines(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	cli(t, nil, "init", "-example", "-root", src, "-p2p", "off", "-peer", "a.example:1")
	cli(t, nil, "init", "-root", dst, "-p2p", "off")
	bundle := filepath.Join(t.TempDir(), "share.json")
	runOut(t, "policy", "export", "-root", src, "-out", bundle)

	out := runOut(t, "policy", "import", bundle, "-settings", "-root", dst)
	if !strings.Contains(out, "many pools: p2p.peers: a.example:1") || strings.Contains(out, ".[") || !strings.Contains(out, "import digest") {
		t.Fatalf("preview:\n%s", out)
	}
	if !strings.Contains(out, "1 of the file's settings is already the same") {
		t.Fatalf("one same setting:\n%s", out)
	}
	out = runOut(t, "policy", "import", bundle, "-apply", "-root", dst)
	if strings.Contains(out, "import digest") || !strings.Contains(out, "saved") {
		t.Fatalf("apply:\n%s", out)
	}
	if out = runOut(t, "policy", "import", bundle, "-root", dst); strings.Contains(out, "import digest") || !strings.Contains(out, "nothing to change") {
		t.Fatalf("nothing to change:\n%s", out)
	}
}
