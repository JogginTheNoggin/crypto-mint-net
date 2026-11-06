package cli

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/nodepolicy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/peers"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/wire"
)

// A peer import works within the book limits node.json sets (D159):
// p2p.book_per_range 1 takes one address of a network range.
func TestPeerImportUsesPolicyLimits(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	cli(t, nil, "init", "-root", src, "-p2p", "off")
	cli(t, nil, "init", "-root", dst, "-p2p", "off")
	srcDir, dstDir := filepath.Join(src, "policies", "default"), filepath.Join(dst, "policies", "default")
	b, err := peers.Open(filepath.Join(srcDir, peers.FileName), peers.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	sec := wire.SeriesSection(hash.Zero, hash.Hash{1})
	for _, a := range []string{"8.8.1.1:1", "8.8.2.2:1"} {
		if err := b.Add(a, []wire.Section{sec}, peers.FromRegistry, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	if err := b.Save(); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "peers.json")
	runOut(t, "peers", "export", "-root", src, "-out", file)

	path := filepath.Join(dstDir, nodepolicy.FileName)
	p, err := nodepolicy.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	p.P2P.BookPerRange = 1
	if err := nodepolicy.Save(path, p); err != nil {
		t.Fatal(err)
	}
	out := runOut(t, "peers", "import", file, "-root", dst)
	if !strings.Contains(out, "+ 8.8.1.1:1") || !strings.Contains(out, "1 addresses from network range") {
		t.Fatalf("plan with book_per_range 1:\n%s", out)
	}

	p.P2P.MaxBook = 100 // under the floor
	if err := nodepolicy.Save(path, p); err == nil {
		t.Fatal("saved p2p.max_book 100")
	}
}

// poon policy limits shows each settable limit's range, default and value,
// reads only, and works in view-only mode (GREEN-047).
func TestPolicyLimitsCmd(t *testing.T) {
	root := t.TempDir()
	cli(t, nil, "init", "-root", root, "-p2p", "off")
	path := filepath.Join(root, "policies", "default", nodepolicy.FileName)
	p, err := nodepolicy.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	p.Tracking.LogSize = 32
	if err := nodepolicy.Save(path, p); err != nil {
		t.Fatal(err)
	}
	var got []nodepolicy.Limit
	cli(t, &got, "policy", "limits", "-root", root)
	if len(got) != len(nodepolicy.Bounds()) {
		t.Fatalf("limits: %+v", got)
	}
	for _, l := range got {
		if l.Name == "tracking.log_size" && (l.Set != 32 || l.Used != 32 || l.Min != 16) {
			t.Fatalf("log_size: %+v", l)
		}
	}
	var out bytes.Buffer
	if err := Run(context.Background(), []string{"-view-only", "policy", "limits", "-o", "table", "-root", root}, Stdio{Out: &out, Err: &out}); err != nil || !strings.Contains(out.String(), "p2p.max_book") {
		t.Fatalf("view-only: %v\n%s", err, out.String())
	}
}
