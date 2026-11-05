package node_test

import (
	"os"
	"strings"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/node"
)

// RED-087: a release build names itself by the version and commit it was
// given by -X, so two releases never share snapshots, and the id carries
// the hand-bumped snapshot semantics.
func TestBuildIDNamesTheRelease(t *testing.T) {
	exe := func() string { return "exe" }
	a := node.MakeBuildID("0.1.0", "abc123", exe)
	b := node.MakeBuildID("0.2.0", "abc123", exe)
	c := node.MakeBuildID("0.1.0", "def456", exe)
	if a == b || a == c || b == c {
		t.Fatalf("ids shared: %q %q %q", a, b, c)
	}
	if !strings.HasPrefix(a, "poo snap") || !strings.Contains(a, "0.1.0") || !strings.Contains(a, "abc123") {
		t.Fatalf("id %q", a)
	}
}

// RED-088: a snapshot whose restored pool is not the pool it says it was
// written from (whole file, right checksum) is not used: the start
// replays the log whole and says why.
func TestSnapshotRestoreIsChecked(t *testing.T) {
	f := newFixture(t)
	n := f.open()
	f.history(n)
	want := f.full(n)
	if err := node.WriteSnapshotWithPrint(n, f.key, hash.Hash{9}); err != nil {
		t.Fatal(err)
	}
	bad, err := os.ReadFile(f.snapPath())
	if err != nil {
		t.Fatal(err)
	}
	n.Close() // writes a good snapshot over it
	if err := os.WriteFile(f.snapPath(), bad, 0o600); err != nil {
		t.Fatal(err)
	}
	n = f.open()
	defer n.Close()
	rs := n.Replayed()
	if rs.Snapshots != 0 || len(rs.Unused) != 1 || !strings.Contains(rs.Unused[0], "restored pool differs") {
		t.Fatalf("stats %+v", rs)
	}
	if got := f.full(n); got != want {
		t.Fatalf("got %+v\nwant %+v", got, want)
	}
}

// RED-088: -reverify with a snapshot behind the log says it compared
// nothing for that pool, instead of nothing at all.
func TestReverifyBehindTheLogIsReported(t *testing.T) {
	f := newFixture(t)
	n := f.open()
	f.history(n)
	n.Close()
	old, err := os.ReadFile(f.snapPath())
	if err != nil {
		t.Fatal(err)
	}
	n = f.open()
	f.more(n, 6, "zed")
	n.Close()
	if err := os.WriteFile(f.snapPath(), old, 0o600); err != nil {
		t.Fatal(err)
	}
	n = f.reverify()
	defer n.Close()
	rs := n.Replayed()
	if rs.Reverified != 0 || len(rs.NotCompared) != 1 || !strings.Contains(rs.NotCompared[0], "covers") {
		t.Fatalf("stats %+v", rs)
	}
}

// RED-089: a temporary snapshot a crash left is removed when its log is
// opened.
func TestSnapshotTmpRemoved(t *testing.T) {
	f := newFixture(t)
	n := f.open()
	f.history(n)
	n.Close()
	tmp := f.snapPath() + ".tmp"
	if err := os.WriteFile(tmp, []byte("left by a crash"), 0o600); err != nil {
		t.Fatal(err)
	}
	n = f.open()
	defer n.Close()
	if _, err := os.Stat(tmp); !os.IsNotExist(err) {
		t.Fatalf("temporary file still there: %v", err)
	}
	if rs := n.Replayed(); rs.TmpRemoved != 1 {
		t.Fatalf("stats %+v", rs)
	}
}
