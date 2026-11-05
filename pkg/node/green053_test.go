package node_test

import (
	"os"
	"strings"
	"testing"
)

// A live node drops each entry's body from memory as soon as the entry is
// written, so the bodies outside the bounded cache stay at none, not one
// per pool until its next entry (GREEN-053). The bodies still read back.
func TestEntryBodyReleasedOnWrite(t *testing.T) {
	f := newFixture(t)
	n := f.open()
	defer n.Close()
	f.history(n)
	st := n.Bodies()
	if st.Held == 0 || st.InMemory != 0 || st.Located < st.Held {
		t.Fatalf("after taking entries in: %+v, want none in memory", st)
	}
	if got := f.bodies(n); len(got) != st.Held {
		t.Fatalf("%d bodies read back, want %d", len(got), st.Held)
	}
}

// -reverify reports a snapshot it cannot read, as any start does, rather
// than leaving it out of both reverified and snapshots_unused (GREEN-055).
func TestReverifyReportsADamagedSnapshot(t *testing.T) {
	f := newFixture(t)
	n := f.open()
	f.history(n)
	n.Close()
	b, err := os.ReadFile(f.snapPath())
	if err != nil {
		t.Fatal(err)
	}
	b[len(b)/2] ^= 1
	if err := os.WriteFile(f.snapPath(), b, 0o600); err != nil {
		t.Fatal(err)
	}
	n = f.reverify()
	defer n.Close()
	rs := n.Replayed()
	if rs.Reverified != 0 || len(rs.Unused) != 1 || !strings.Contains(rs.Unused[0], "checksum") {
		t.Fatalf("-reverify with a damaged snapshot: %+v", rs)
	}
}
