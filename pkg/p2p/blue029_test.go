package p2p_test

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/ledger"
)

// damageLog flips one byte in every 50 of a pool log, in place, so the
// node's open file reads the damage back.
func damageLog(t *testing.T, path string) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	for i := 40; i < len(b); i += 50 {
		if _, err := f.WriteAt([]byte{b[i] ^ 0xff}, int64(i)); err != nil {
			t.Fatal(err)
		}
	}
}

// RED-084: a node whose log is damaged under it, with its bodies read
// back from disk, serves a syncing peer without crashing: an entry it
// cannot read back is not held for that peer, and the node runs on.
func TestDamagedLogDoesNotCrashServing(t *testing.T) {
	a := startNode(t, "a", nil)
	grow(t, a, nil, "alice", 6, 32)
	a.n.BodyCache().SetMax(0)
	a.n.Engine().Ledger(a.key, func(p *ledger.Pool) { p.ReleaseBodies() })
	if st := a.n.Bodies(); st.InMemory != 0 {
		t.Fatalf("bodies still in memory: %+v", st)
	}
	damageLog(t, filepath.Join(a.n.Dir(), "pools", a.key.Series.String(), strconv.FormatUint(a.key.Index, 10)+".log"))

	b := startNode(t, "b", nil)
	b.connect(a)
	eventually(t, "a failed read", func() bool { return a.n.Bodies().Errors > 0 })
	time.Sleep(200 * time.Millisecond)
	// a is still running and answering.
	if st := a.status(); st.Height != 6 {
		t.Fatalf("a: height %d", st.Height)
	}
	if b.status().Height >= 6 {
		t.Fatal("b synced entries a cannot read")
	}
}
