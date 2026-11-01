package p2p

import (
	"testing"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/entry"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/wire"
)

// RED-017: a fetch in many parts requests encodes its item once, and the
// cache lets go of items past its size or unused for a while.
func TestPartsCacheEncodesOnce(t *testing.T) {
	var c partsCache
	it := wire.EntryItem{Entry: &entry.Entry{Nonce: 1}, HasLayers: true, Nonces: []uint64{1, 2, 3}}
	k := partsKey{hash.Hash{1}, wire.EntriesWithLayers}
	a := c.get(k, it)
	b := c.get(k, wire.EntryItem{Entry: &entry.Entry{Nonce: 2}})
	if &a[0] != &b[0] {
		t.Fatal("the item was encoded twice")
	}
	if string(a) != string(wire.EncodeItem(it)) {
		t.Fatal("the cached bytes are not the item's")
	}
	// An item unused past partsCacheFor goes when another comes.
	c.items[k].used = time.Now().Add(-2 * partsCacheFor)
	c.get(partsKey{hash.Hash{2}, wire.EntriesWithLayers}, it)
	if _, ok := c.items[k]; ok {
		t.Fatal("a stale item was kept")
	}
}
