package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/nodepolicy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/peers"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/rpc"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tracking"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/wire"
)

// seedBook writes a peer book and a block into a stopped node's
// directory.
func seedBook(t *testing.T, dir string) {
	t.Helper()
	sec := wire.SeriesSection(hash.Zero, hash.Hash{1})
	b, err := peers.Open(filepath.Join(dir, peers.FileName), peers.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range []string{"8.8.1.1:1", "8.9.1.1:1"} {
		if err := b.Add(a, []wire.Section{sec}, peers.FromRegistry, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	b.Archive("8.9.1.1:1")
	b.SetLabel("8.8.1.1:1", "fast one")
	if err := b.Save(); err != nil {
		t.Fatal(err)
	}
	p, err := nodepolicy.Load(filepath.Join(dir, nodepolicy.FileName))
	if err != nil {
		t.Fatal(err)
	}
	tr, err := tracking.New(p.TrackingConfig(), filepath.Join(dir, tracking.FileName))
	if err != nil {
		t.Fatal(err)
	}
	tr.BlockFor("9.9.9.9", -1, time.Now())
	if err := tr.Save(); err != nil {
		t.Fatal(err)
	}
}

// D157: a peer book goes to another node in one file, whole or by
// category; every address arrives unlisted, blocks only with -blocks, and
// the plan comes first.
func TestPeersExportImport(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	cli(t, nil, "init", "-root", src, "-p2p", "off")
	cli(t, nil, "init", "-root", dst, "-p2p", "off")
	srcDir, dstDir := filepath.Join(src, "policies", "default"), filepath.Join(dst, "policies", "default")
	seedBook(t, srcDir)

	file := filepath.Join(t.TempDir(), "peers.json")
	if out := runOut(t, "peers", "export", "-root", src, "-out", file); !strings.Contains(out, "exported 2 addresses and 1 block") {
		t.Fatalf("export: %s", out)
	}
	some := filepath.Join(t.TempDir(), "archived.json")
	runOut(t, "peers", "export", "-root", src, "-category", "archived", "-out", some)
	if b, _ := os.ReadFile(some); strings.Contains(string(b), "8.8.1.1") || !strings.Contains(string(b), "8.9.1.1") {
		t.Fatalf("archived only:\n%s", b)
	}

	out := runOut(t, "peers", "import", file, "-root", dst)
	if !strings.Contains(out, "+ 8.8.1.1:1 (unlisted, 1 mint policy)") || !strings.Contains(out, "label 8.8.1.1:1: fast one") ||
		!strings.Contains(out, "skipped: 1 block (-blocks takes them)") || !strings.Contains(out, "dry run") {
		t.Fatalf("plan:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(dstDir, peers.FileName)); err == nil {
		t.Fatal("a dry run wrote the peer book")
	}
	dg := regexp.MustCompile(`import digest ([0-9a-f]{64})`).FindStringSubmatch(out)
	if dg == nil {
		t.Fatalf("no digest:\n%s", out)
	}
	var o bytes.Buffer
	if err := run([]string{"peers", "import", file, "-blocks", "-apply", "-if-digest", dg[1], "-root", dst}, &o); err == nil || !strings.Contains(err.Error(), "not the one previewed") {
		t.Fatalf("a digest applied with other options: %v", err)
	}
	runOut(t, "peers", "import", file, "-apply", "-if-digest", dg[1], "-root", dst)
	b, err := peers.Open(filepath.Join(dstDir, peers.FileName), peers.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range b.Records() {
		if !r.Has(peers.Unlisted) || r.Has(peers.Candidate) || r.Has(peers.Archived) {
			t.Fatalf("%s came in as %+v", r.Addr, r.Policies)
		}
	}
	if r, _ := b.Record("8.8.1.1:1"); r.Label != "fast one" {
		t.Fatalf("label: %+v", r)
	}
	if _, err := os.Stat(filepath.Join(dstDir, tracking.FileName)); err == nil {
		t.Fatal("a block was taken without -blocks")
	}
	out = runOut(t, "peers", "import", file, "-blocks", "-apply", "-root", dst)
	if !strings.Contains(out, "read first") || !strings.Contains(out, "! block 9.9.9.9 until unblocked") {
		t.Fatalf("-blocks:\n%s", out)
	}
	p, _ := nodepolicy.Load(filepath.Join(dstDir, nodepolicy.FileName))
	tr, err := tracking.New(p.TrackingConfig(), filepath.Join(dstDir, tracking.FileName))
	if err != nil || !tr.Status("9.9.9.9", time.Now()).Blocked {
		t.Fatalf("the block: %v", err)
	}
	if out := runOut(t, "peers", "import", file, "-blocks", "-root", dst); !strings.Contains(out, "nothing to change") {
		t.Fatalf("again:\n%s", out)
	}
	if err := run([]string{"peers", "import", file, "-ranges", "-root", dst}, &o); err == nil {
		t.Fatal("-ranges without -blocks")
	}
}

// D157: with the node stopped, add -keep lists an address in node.json,
// label labels it in the book, and remove takes it out of both.
func TestPeerActionsOffline(t *testing.T) {
	root := t.TempDir()
	cli(t, nil, "init", "-root", root, "-p2p", "off")
	dir := filepath.Join(root, "policies", "default")
	seedBook(t, dir)
	runOut(t, "peer", "add", "8.8.8.8:8646", "-keep", "-root", root)
	p, _ := nodepolicy.Load(filepath.Join(dir, nodepolicy.FileName))
	if !slices.Contains(p.P2P.Peers, "8.8.8.8:8646") {
		t.Fatalf("add -keep: %v", p.P2P.Peers)
	}
	var o bytes.Buffer
	if err := run([]string{"peer", "add", "8.8.8.8:1", "-root", root}, &o); err == nil {
		t.Fatal("add without a node or -keep")
	}
	runOut(t, "peer", "label", "8.9.1.1:1", "-label", "old one", "-root", root)
	runOut(t, "peer", "remove", "8.8.8.8:8646", "-root", root)
	runOut(t, "peer", "remove", "8.8.1.1:1", "-root", root)
	p, _ = nodepolicy.Load(filepath.Join(dir, nodepolicy.FileName))
	b, _ := peers.Open(filepath.Join(dir, peers.FileName), peers.Limits{})
	r, _ := b.Record("8.9.1.1:1")
	if slices.Contains(p.P2P.Peers, "8.8.8.8:8646") || r.Label != "old one" {
		t.Fatalf("after: %v %+v", p.P2P.Peers, r)
	}
	if _, ok := b.Record("8.8.1.1:1"); ok {
		t.Fatal("remove left the address in the book")
	}
}

// D157: on a running node, add, label, the filters, the blocked view and
// connect -imported.
func TestPeersRunning(t *testing.T) {
	root := t.TempDir()
	cli(t, nil, "init", "-root", root, "-listen", "127.0.0.1:0", "-p2p", "127.0.0.1:0")
	stop := startNode(t, filepath.Join(root, "policies", "default"))
	defer stop()
	runOut(t, "peer", "add", "8.8.4.4:1", "-label", "Friend", "-root", root)
	runOut(t, "peer", "block", "9.9.9.9:1", "-seconds", "-1", "-root", root)
	var r rpc.PeersReply
	cli(t, &r, "peers", "-label", "friend", "-root", root)
	if len(r.Book) != 1 || r.Book[0].Label != "Friend" || r.Book[0].Source != peers.FromHand {
		t.Fatalf("by label: %+v", r.Book)
	}
	cli(t, &r, "peers", "-category", "priority", "-root", root)
	if len(r.Book) != 0 {
		t.Fatalf("by category: %+v", r.Book)
	}
	var o bytes.Buffer
	if err := run([]string{"peers", "-category", "friends", "-root", root}, &o); err == nil {
		t.Fatal("an unknown category was taken")
	}
	var tr rpc.TrackingReply
	cli(t, &tr, "tracking", "-blocked", "-root", root)
	if len(tr.Tracked) != 1 || !tr.Tracked["9.9.9.9"].Blocked {
		t.Fatalf("blocked: %+v", tr)
	}
	var pr rpc.PeerReply
	cli(t, &pr, "peer", "connect", "-imported", "-root", root)
	if pr.Promoted != 0 {
		t.Fatalf("connect -imported: %+v", pr)
	}
	file := filepath.Join(t.TempDir(), "p.json")
	doc := `{"kind":"poo-peers","format":1,"peers":[{"addr":"8.8.5.5:1","policies":[{"section":"*/*/*/*","state":"priority"}]}]}`
	if err := os.WriteFile(file, []byte(doc), 0o600); err != nil {
		t.Fatal(err)
	}
	if out := runOut(t, "peers", "import", file, "-apply", "-root", root); !strings.Contains(out, "peer book (running)") || !strings.Contains(out, "applied") {
		t.Fatalf("running import:\n%s", out)
	}
	cli(t, &r, "peers", "-category", "unlisted", "-root", root)
	if len(r.Book) != 1 || r.Book[0].Addr != "8.8.5.5:1" {
		t.Fatalf("a file's priority peer came in as: %+v", r.Book)
	}
	cli(t, &pr, "peer", "remove", "8.8.4.4:1", "-root", root)
	if !pr.Held {
		t.Fatalf("remove: %+v", pr)
	}
}
