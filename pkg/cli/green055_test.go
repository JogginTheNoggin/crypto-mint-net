package cli

import (
	"strings"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/node"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/rpc"
)

// Doctor names a -reverify that matched every snapshot, and lists every
// reason a snapshot was not used, with the counts in agreement (GREEN-055).
func TestSnapshotsCheckReverifyAndReasons(t *testing.T) {
	c := snapshotsCheck(rpc.StatusReply{
		Replay:    node.ReplayStats{Reverified: 1},
		Snapshots: node.SnapshotSummary{Pools: 1, Every: 50000},
	})
	if c.Result != checkOK || !strings.Contains(c.Detail, "-reverify replayed 1 pool whole") {
		t.Fatalf("reverified: %+v", c)
	}
	c = snapshotsCheck(rpc.StatusReply{
		Replay:    node.ReplayStats{Unused: []string{"p1: checksum differs", "p2: other build"}},
		Snapshots: node.SnapshotSummary{Pools: 2, Every: 50000},
	})
	if c.Result != checkWarn || !strings.Contains(c.Detail, "2 pool snapshots were") ||
		!strings.Contains(c.Detail, "checksum differs") || !strings.Contains(c.Detail, "other build") {
		t.Fatalf("unused: %+v", c)
	}
}
