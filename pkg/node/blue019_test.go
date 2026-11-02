package node_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/node"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/nodepolicy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/series"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/testutil"
)

// RED-046, D152: a node following a series of 2^40 pools whole starts at
// once: it reads the stores on disk, not every pool it follows (4M pools
// took 10 s before). A file in the pools directory that is not a store of
// a followed pool is left alone.
func TestHugeSeriesFollowedWholeStartsAtOnce(t *testing.T) {
	p := testutil.ExampleParams()
	p.PoolSize, p.Mints = 1, 1<<40
	s, err := series.New(p)
	if err != nil {
		t.Fatal(err)
	}
	id := s.ID()
	pol := nodepolicy.Default()
	pol.Series = []nodepolicy.SeriesInfo{{Params: p}}
	pol.Follow = []nodepolicy.Follow{{Series: &id}}
	pol.RPC.Listen = "off"
	dir := t.TempDir()
	if err := nodepolicy.Save(filepath.Join(dir, nodepolicy.FileName), pol); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(dir, "pools", "not-a-series")
	if err := os.MkdirAll(other, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pools", id.String()+"x"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	n, err := node.Open(dir, node.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	if d := time.Since(start); d > 5*time.Second {
		t.Fatalf("start took %v", d)
	}
	if got := n.Engine().FollowedCount(); got != 1<<40 {
		t.Fatalf("follows %d pools", got)
	}
}
