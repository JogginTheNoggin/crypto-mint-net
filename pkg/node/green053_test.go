package node_test

import (
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
