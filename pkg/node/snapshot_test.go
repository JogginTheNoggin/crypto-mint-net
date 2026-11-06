package node_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/engine"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/ledger"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/node"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/nodepolicy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/testutil"
)

func (f *fixture) snapPath() string {
	return filepath.Join(f.dir, "pools", f.key.Series.String(), strconv.FormatUint(f.key.Index, 10)+".log.snap")
}

func (f *fixture) reverify() *node.Node {
	f.t.Helper()
	n, err := node.Open(f.dir, node.Options{Reverify: true})
	if err != nil {
		f.t.Fatal(err)
	}
	return n
}

func (f *fixture) print(n *node.Node) hash.Hash {
	f.t.Helper()
	fp, err := n.Engine().Fingerprint(f.key)
	if err != nil {
		f.t.Fatal(err)
	}
	return fp
}

// state is everything the tests compare: the status and holdings, the
// fingerprint, and the pending transactions.
type fullState struct {
	snap    snapshot
	print   hash.Hash
	pending string
}

func (f *fixture) full(n *node.Node) fullState {
	f.t.Helper()
	ps, err := n.Engine().Pending(f.key)
	if err != nil {
		f.t.Fatal(err)
	}
	var b strings.Builder
	for _, it := range ps {
		b.WriteString(it.TID.String())
	}
	return fullState{f.snap(n), f.print(n), b.String()}
}

// more runs entries on top of the fixture's history: a claim, an entry
// and a layer each time, so the log grows after a snapshot.
func (f *fixture) more(n *node.Node, serial uint64, who string) {
	f.t.Helper()
	if _, err := n.HandleTx(f.k.Claim(serial, P(who, 100)), engine.Local); err != nil {
		f.t.Fatal(err)
	}
	e, _, err := n.Build(f.key, 0)
	if err != nil {
		f.t.Fatal(err)
	}
	if _, _, err := n.Work(f.key, e.Hash(f.s), 0, 300); err != nil {
		f.t.Fatal(err)
	}
}

// TestSnapshotStart is D178's node-level differential: a start from the
// snapshot a clean stop wrote, and a start from an older snapshot with
// the records after it replayed, each build exactly the pool a full
// replay (-reverify) builds, pending transactions included.
func TestSnapshotStart(t *testing.T) {
	f := newFixture(t)
	n := f.open()
	f.history(n)
	want := f.full(n)
	if err := n.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(f.snapPath()); err != nil {
		t.Fatalf("a clean stop wrote no snapshot: %v", err)
	}

	n = f.open()
	rs := n.Replayed()
	if rs.Snapshots != 1 || rs.Records != 0 || rs.Covered == 0 || len(rs.Unused) != 0 {
		t.Fatalf("start from the snapshot: %+v", rs)
	}
	if got := f.full(n); got != want {
		t.Fatalf("from the snapshot\n got %+v\nwant %+v", got, want)
	}
	if si := n.Snapshots(); len(si) != 1 || si[0].Written.IsZero() || si[0].Since != 0 {
		t.Fatalf("snapshot info %+v", si)
	}
	// Keep this snapshot, then take in more.
	old, err := os.ReadFile(f.snapPath())
	if err != nil {
		t.Fatal(err)
	}
	f.more(n, 6, "gina")
	f.more(n, 7, "hal")
	want = f.full(n)
	if err := n.Close(); err != nil {
		t.Fatal(err)
	}

	// The older snapshot, as after a crash: the records after it are
	// replayed on top of it.
	if err := os.WriteFile(f.snapPath(), old, 0o600); err != nil {
		t.Fatal(err)
	}
	n = f.open()
	rs = n.Replayed()
	if rs.Snapshots != 1 || rs.Records == 0 || len(rs.Unused) != 0 {
		t.Fatalf("start from an older snapshot: %+v", rs)
	}
	if got := f.full(n); got != want {
		t.Fatalf("older snapshot and the records after it\n got %+v\nwant %+v", got, want)
	}
	n.Close()

	// -reverify replays everything and finds the snapshot Close wrote
	// matches.
	n = f.reverify()
	rs = n.Replayed()
	if rs.Snapshots != 0 || rs.Reverified != 1 || rs.Differed != 0 {
		t.Fatalf("reverify: %+v", rs)
	}
	if got := f.full(n); got != want {
		t.Fatalf("full replay\n got %+v\nwant %+v", got, want)
	}
	n.Close()
}

// TestSnapshotAfterCompaction: a compaction replaces the snapshot, and a
// start from the new one builds the pool a full replay of the compacted
// log builds, first-sighting order included (Pool.Renumber).
func TestSnapshotAfterCompaction(t *testing.T) {
	f := newFixture(t)
	n := f.open()
	f.history(n)
	f.more(n, 6, "gina")
	old := filepath.Join(t.TempDir(), "old.snap")
	n.Close()
	copyFile(t, f.snapPath(), old)

	n = f.open()
	if err := n.Compact(f.key); err != nil {
		t.Fatal(err)
	}
	want := f.full(n)
	n.Close()
	n = f.open()
	if rs := n.Replayed(); rs.Snapshots != 1 || rs.Records != 0 {
		t.Fatalf("after compaction: %+v", rs)
	}
	if got := f.full(n); got != want {
		t.Fatalf("snapshot after compaction\n got %+v\nwant %+v", got, want)
	}
	n.Close()
	n = f.reverify()
	if rs := n.Replayed(); rs.Reverified != 1 || rs.Differed != 0 {
		t.Fatalf("reverify after compaction: %+v", rs)
	}
	if got := f.full(n); got != want {
		t.Fatalf("full replay of the compacted log\n got %+v\nwant %+v", got, want)
	}
	n.Close()

	// The snapshot from before the compaction no longer fits the log.
	copyFile(t, old, f.snapPath())
	n = f.open()
	defer n.Close()
	rs := n.Replayed()
	if rs.Snapshots != 0 || len(rs.Unused) != 1 || !strings.Contains(rs.Unused[0], "no longer holds") {
		t.Fatalf("stale snapshot: %+v", rs)
	}
	if got := f.full(n); got != want {
		t.Fatalf("after a stale snapshot\n got %+v\nwant %+v", got, want)
	}
}

func copyFile(t *testing.T, from, to string) {
	t.Helper()
	b, err := os.ReadFile(from)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(to, b, 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestSnapshotFaults: a snapshot that is torn, damaged, swapped in from
// another node's pool, or not a snapshot at all is never used; the pool
// is replayed whole and ends as it would have.
func TestSnapshotFaults(t *testing.T) {
	other := newFixture(t)
	on := other.open()
	other.history(on)
	other.more(on, 6, "zed")
	on.Close()

	for _, c := range []struct {
		name, reason string
		edit         func(t *testing.T, b []byte) []byte
	}{
		{"torn", "cut short", func(t *testing.T, b []byte) []byte { return b[:len(b)-40] }},
		{"torn in the header", "cut short", func(t *testing.T, b []byte) []byte { return b[:12] }},
		{"a byte flipped", "checksum", func(t *testing.T, b []byte) []byte {
			b[len(b)/2] ^= 1
			return b
		}},
		{"bytes after the end", "after the end", func(t *testing.T, b []byte) []byte { return append(b, 0) }},
		{"another node's", "no longer holds", func(t *testing.T, b []byte) []byte {
			o, err := os.ReadFile(other.snapPath())
			if err != nil {
				t.Fatal(err)
			}
			return o
		}},
		{"not a snapshot", "not a snapshot", func(t *testing.T, b []byte) []byte { return []byte("hello") }},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := newFixture(t)
			n := f.open()
			f.history(n)
			want := f.full(n)
			n.Close()
			b, err := os.ReadFile(f.snapPath())
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(f.snapPath(), c.edit(t, b), 0o600); err != nil {
				t.Fatal(err)
			}
			n = f.open()
			defer n.Close()
			rs := n.Replayed()
			if rs.Snapshots != 0 || len(rs.Unused) != 1 || !strings.Contains(rs.Unused[0], c.reason) {
				t.Fatalf("stats %+v, want one unused snapshot (%s)", rs, c.reason)
			}
			if got := f.full(n); got != want {
				t.Fatalf("after replaying whole\n got %+v\nwant %+v", got, want)
			}
		})
	}
}

// TestSnapshotEvery: a running node writes a snapshot once its log took
// snapshot_every records, without a stop, and no_snapshots writes none
// and uses none.
func TestSnapshotEvery(t *testing.T) {
	f := newFixture(t, func(p *nodepolicy.Policy) { p.SnapshotEvery = nodepolicy.BoundSnapshotEvery.Min })
	n := f.open()
	c := f.k.Claim(4, P("a0", 100))
	if _, err := n.HandleTx(c, engine.Local); err != nil {
		t.Fatal(err)
	}
	prev, owner := c, "a0"
	for i := 0; n.Snapshots()[0].Written.IsZero(); i++ {
		if i > 600 {
			t.Fatalf("no snapshot after %d records", n.Stores()[0].Records)
		}
		e, _, err := n.Build(f.key, 0)
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := n.Work(f.key, e.Hash(f.s), 0, 300); err != nil {
			t.Fatal(err)
		}
		nw := fmt.Sprintf("a%d", i+1)
		sp := f.k.Spend(4, []testutil.In{f.k.Out(prev, 0, owner)}, P(nw, 100))
		if _, err := n.HandleTx(sp, engine.Local); err != nil {
			t.Fatal(err)
		}
		prev, owner = sp, nw
	}
	si := n.Snapshots()[0]
	if si.Records < nodepolicy.BoundSnapshotEvery.Min || si.Bytes == 0 {
		t.Fatalf("snapshot info %+v", si)
	}
	want := f.full(n)
	n.Close()
	n = f.open()
	if got := f.full(n); got != want {
		t.Fatalf("after restart\n got %+v\nwant %+v", got, want)
	}
	n.Close()

	// no_snapshots: none used at start, none written.
	os.Remove(f.snapPath())
	p, err := nodepolicy.Load(filepath.Join(f.dir, nodepolicy.FileName))
	if err != nil {
		t.Fatal(err)
	}
	p.NoSnapshots = true
	if err := nodepolicy.Save(filepath.Join(f.dir, nodepolicy.FileName), p); err != nil {
		t.Fatal(err)
	}
	n = f.open()
	if rs := n.Replayed(); rs.Snapshots != 0 {
		t.Fatalf("no_snapshots used one: %+v", rs)
	}
	if got := f.full(n); got != want {
		t.Fatalf("no_snapshots\n got %+v\nwant %+v", got, want)
	}
	n.Close()
	if _, err := os.Stat(f.snapPath()); !os.IsNotExist(err) {
		t.Fatalf("no_snapshots wrote one: %v", err)
	}
}

// TestSnapshotOtherBuild: a snapshot written by another build is replayed
// over, so an upgrade never trusts a pool an older build computed, and
// the next stop writes one for this build.
func TestSnapshotOtherBuild(t *testing.T) {
	f := newFixture(t)
	undo := node.SetBuildID("poon older")
	n := f.open()
	f.history(n)
	want := f.full(n)
	n.Close()
	undo()
	n = f.open()
	rs := n.Replayed()
	if rs.Snapshots != 0 || len(rs.Unused) != 1 || !strings.Contains(rs.Unused[0], `written by "poon older"`) {
		t.Fatalf("stats %+v", rs)
	}
	if got := f.full(n); got != want {
		t.Fatalf("got %+v\nwant %+v", got, want)
	}
	n.Close()
	n = f.open()
	defer n.Close()
	if rs := n.Replayed(); rs.Snapshots != 1 {
		t.Fatalf("this build's snapshot not used: %+v", rs)
	}
}

// TestSnapshotDroppedOnDamage: once a body in a log the snapshot covers
// fails to read back, the snapshot is removed and none is written, so
// the next start replays the log whole and cuts it at the damage, as it
// did before snapshots (D173), instead of skipping the damage.
func TestSnapshotDroppedOnDamage(t *testing.T) {
	f := newFixture(t)
	n := f.open()
	f.history(n)
	bodies := f.bodies(n)
	n.Close()
	n = f.open()
	if rs := n.Replayed(); rs.Snapshots != 1 {
		t.Fatalf("not from the snapshot: %+v", rs)
	}
	logPath := strings.TrimSuffix(f.snapPath(), ".snap")
	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	damaged := false
	for _, b := range bodies {
		if i := bytes.Index(raw, b); i >= 0 {
			raw[i+len(b)/2] ^= 0xff
			damaged = true
			break
		}
	}
	if !damaged {
		t.Fatal("no entry record found in the log")
	}
	if err := os.WriteFile(logPath, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	n.BodyCache().SetMax(0)
	failed := 0
	n.Engine().Ledger(f.key, func(p *ledger.Pool) {
		for _, h := range p.ReplayOrder() {
			if _, err := p.Entry(h); err != nil {
				failed++
			}
		}
	})
	if failed != 1 {
		t.Fatalf("%d bodies failed to read back, want 1", failed)
	}
	n.Close()
	if _, err := os.Stat(f.snapPath()); !os.IsNotExist(err) {
		t.Fatalf("snapshot kept after a damaged read: %v", err)
	}
	n = f.open()
	defer n.Close()
	if rs := n.Replayed(); rs.Snapshots != 0 || rs.Cut == 0 {
		t.Fatalf("restart after damage: %+v", rs)
	}
}
