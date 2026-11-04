package node_test

import (
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/node"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/nodepolicy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/testutil"
)

// Several peers that pass the same release on at once all get past the
// host's known-part check before any of them is taken. Exactly one call
// takes it, so only that one is credited and passed on again
// (ORANGE-036); the rest add nothing.
func TestSameReleaseAtOnceTakenOnce(t *testing.T) {
	s := testutil.MustSeries(testutil.VariableParams(8, 4, 4))
	id := s.ID()
	p := nodepolicy.Default()
	p.Series = []nodepolicy.SeriesInfo{{Params: s.Params(), IssuerWitness: nodepolicy.EncodeWitness(testutil.IssuerWitness(s))}}
	p.Follow = []nodepolicy.Follow{{Series: &id}}
	p.RPC.Listen = "off"
	p.NoSync = true
	dir := t.TempDir()
	if err := nodepolicy.Save(filepath.Join(dir, nodepolicy.FileName), p); err != nil {
		t.Fatal(err)
	}
	n, err := node.Open(dir, node.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()

	for k := uint64(2); k <= 5; k++ {
		r := testutil.Release(s, k)
		var taken, added atomic.Int32
		var wg sync.WaitGroup
		for i := 0; i < 8; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				res, err := n.AddRelease(r)
				if err != nil {
					t.Error(err)
					return
				}
				if res.Taken {
					taken.Add(1)
				}
				if res.Added {
					added.Add(1)
				}
			}()
		}
		wg.Wait()
		if taken.Load() != 1 || added.Load() != 1 {
			t.Fatalf("part %d from 8 peers at once: taken %d times, saved %d times", k, taken.Load(), added.Load())
		}
		// The command path still answers a known part, and takes nothing.
		res, err := n.AddRelease(r)
		if err != nil || res.Taken || res.Added || res.Released != k {
			t.Fatalf("part %d again: %+v %v", k, res, err)
		}
	}
}
