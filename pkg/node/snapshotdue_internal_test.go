package node

import (
	"path/filepath"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/engine"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/store"
)

// ORANGE-040 proposal 2: a running snapshot is due every snapshot_every
// records, or every tenth of the log once that is more.
func TestSnapshotDue(t *testing.T) {
	l, _, err := store.Open(filepath.Join(t.TempDir(), "1.log"), store.Options{NoSync: true})
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	k := engine.PoolKey{Index: 1}
	n := &Node{logs: map[engine.PoolKey]*store.Log{k: l}}
	n.run.SnapshotEvery = 1000
	if got := n.snapshotDue(k); got != 1000 {
		t.Fatalf("empty log: due every %d", got)
	}
	for range 25000 {
		if _, err := l.AppendLater(store.Record{Kind: store.KindTx, Data: []byte{1}}); err != nil {
			t.Fatal(err)
		}
	}
	if got := n.snapshotDue(k); got != 2500 {
		t.Fatalf("25,000 records: due every %d, want 2500", got)
	}
}
