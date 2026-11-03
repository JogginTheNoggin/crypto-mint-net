package p2p_test

import (
	"crypto/rand"
	"fmt"
	"testing"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/entry"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/nodepolicy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/p2p"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/testutil"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tx"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/weight"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/wire"
)

// RED-057, D156: entries that build on nothing, each in another pool of a
// series of 2^40 pools followed whole, with a stated weight above the
// node's own, add nothing to the pool index. Their pools are held for the
// chase only, and dropped once the queue holds no chase for them.
func TestStatedWeightIndexesNoPool(t *testing.T) {
	prm := testutil.ExampleParams()
	prm.PoolSize, prm.Mints = 1, 1<<40
	s := testutil.MustSeries(prm)
	id := s.ID()
	a := startNodeWith(t, "a", s, nodepolicy.Follow{Series: &id}, func(c *p2p.Config) {
		c.CheckEvery = 200 * time.Millisecond
		c.RequestTimeout = 300 * time.Millisecond
	})
	r := dialRaw(t, a, []wire.Section{wire.SeriesSection(hash.Zero, id)})
	keys := testutil.NewKeys(s)
	huge := weight.FromUint64(1 << 62)
	const n = 20
	for i := 0; i < n; i++ {
		serial := uint64(i+1) << 30
		claim := keys.Claim(serial, testutil.P(fmt.Sprintf("mallory-%d", i), prm.Supply))
		var parent hash.Hash
		rand.Read(parent[:])
		e, _, err := entry.Build(s, parent, []*tx.Tx{claim}, 0)
		if err != nil {
			t.Fatal(err)
		}
		sec := wire.PoolSection(hash.Zero, id, s.PoolID(s.PoolOf(serial)))
		r.write(wire.Message{Header: wire.Header{Type: wire.TypeEntry, Section: sec, Stated: huge}, Body: e.Encode()})
	}
	eventually(t, "the pools to be held for a chase", func() bool {
		r.readUntil(time.Now().Add(50 * time.Millisecond))
		return a.h.Chasing() > 0
	})
	if got := a.h.IndexedPools(); got != 0 {
		t.Fatalf("%d pools indexed for a stated weight", got)
	}
	// The sender leaves: the queue drops its chases, and the records go.
	r.closing.Store(true)
	r.c.Close()
	eventually(t, "the chase records to go", func() bool { return a.h.Chasing() == 0 })
	if got := a.h.IndexedPools(); got != 0 {
		t.Fatalf("%d pools indexed after the chases", got)
	}
}
