package node

import (
	"fmt"
	"testing"
)

// A status reply carries at most MaxReasons per-pool reasons and the whole
// count, so after an upgrade it does not grow with every pool (GUI
// ORANGE-023, D184).
func TestReplayedCapsReasons(t *testing.T) {
	n := &Node{}
	for i := 0; i < 8192; i++ {
		n.replay.Unused = append(n.replay.Unused, fmt.Sprintf("p%d: written by \"poo older\"", i))
	}
	n.replay.NotCompared = []string{"p1: covers 1 of 2"}
	r := n.Replayed()
	if len(r.Unused) != MaxReasons || r.UnusedCount != 8192 || r.Unused[0] != `p0: written by "poo older"` {
		t.Fatalf("unused %d of %d", len(r.Unused), r.UnusedCount)
	}
	if len(r.NotCompared) != 1 || r.NotComparedCount != 1 {
		t.Fatalf("not compared %v %d", r.NotCompared, r.NotComparedCount)
	}
	if len(n.replay.Unused) != 8192 {
		t.Fatal("Replayed cut the node's own list")
	}
	r.Unused[0] = "changed"
	if n.replay.Unused[0] == "changed" {
		t.Fatal("Replayed shares the node's list")
	}
}

// The release status shows is the one SetRelease named (W-10, D184).
func TestRunningRelease(t *testing.T) {
	old := release
	t.Cleanup(func() { release = old })
	SetRelease("1.2.3", "abc123")
	if r := RunningRelease(); r.Version != "1.2.3" || r.Commit != "abc123" {
		t.Fatalf("%+v", r)
	}
}
