package node

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/entry"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/store"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/testutil"
)

// A location that holds another entry, or a record of another kind, is
// never given as the body asked for (D172).
func TestPoolBodiesChecks(t *testing.T) {
	s := testutil.ExampleSeries()
	l, _, err := store.Open(filepath.Join(t.TempDir(), "1.log"), store.Options{NoSync: true})
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	e, _, err := entry.Build(s, hash.Hash{9}, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, eloc, err := l.AppendLaterAt(store.Record{Kind: store.KindEntry, Data: e.Encode()})
	if err != nil {
		t.Fatal(err)
	}
	_, tloc, err := l.AppendLaterAt(store.Record{Kind: store.KindTx, Data: []byte{1, 2, 3}})
	if err != nil {
		t.Fatal(err)
	}
	b := &poolBodies{s: s, log: l, locs: map[hash.Hash]store.Loc{}}
	h := e.Hash(s)
	b.add(h, eloc)
	if got, err := b.load(h); err != nil || got.Hash(s) != h {
		t.Fatalf("load: %v", err)
	}
	other := hash.Hash{1}
	b.add(other, eloc)
	if _, err := b.load(other); !errors.Is(err, store.ErrBadRecord) {
		t.Fatalf("another entry's record: %v", err)
	}
	b.add(other, tloc)
	if _, err := b.load(other); !errors.Is(err, store.ErrBadRecord) {
		t.Fatalf("a transaction's record: %v", err)
	}
	if _, err := b.load(hash.Hash{2}); !errors.Is(err, errNoBody) {
		t.Fatalf("an entry with no record: %v", err)
	}
}
