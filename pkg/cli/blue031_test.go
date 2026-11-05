package cli

import (
	"strings"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/node"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/rpc"
)

// RED-088: a -reverify that compared nothing for a pool is a warning, not
// ok. RED-089: a removed temporary snapshot is named.
func TestSnapshotsCheckNotComparedAndTmp(t *testing.T) {
	c := snapshotsCheck(rpc.StatusReply{
		Replay:    node.ReplayStats{NotCompared: []string{"p1: the snapshot covers 9 of the log's 12 records"}},
		Snapshots: node.SnapshotSummary{Pools: 1, Every: 50000},
	})
	if c.Result != checkWarn || !strings.Contains(c.Detail, "compared nothing for 1 pool") || !strings.Contains(c.Detail, "9 of the log's 12") {
		t.Fatalf("not compared: %+v", c)
	}
	c = snapshotsCheck(rpc.StatusReply{
		Replay:    node.ReplayStats{TmpRemoved: 2},
		Snapshots: node.SnapshotSummary{Pools: 2, Every: 50000},
	})
	if c.Result != checkOK || !strings.Contains(c.Detail, "2 temporary snapshot files left by a crash were removed") {
		t.Fatalf("tmp: %+v", c)
	}
}

// doctor counts every unused snapshot when status carries only the first
// reasons, and points at the start log for the rest (D184).
func TestSnapshotsCheckCountsCutReasons(t *testing.T) {
	c := snapshotsCheck(rpc.StatusReply{Replay: node.ReplayStats{Unused: []string{"p1: x", "p2: x"}, UnusedCount: 900}})
	if c.Result != checkWarn || !strings.Contains(c.Detail, "900 pool snapshots were") || !strings.Contains(c.Detail, "and 898 more, each named in the node's start log") {
		t.Fatalf("%+v", c)
	}
}
