package p2p

import (
	"testing"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/entry"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/layer"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/testutil"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/wire"
)

// RED-017: a fetch in many parts requests encodes its item once, and the
// cache lets go of items past its size or unused for a while.
func TestPartsCacheEncodesOnce(t *testing.T) {
	var c partsCache
	it := wire.EntryItem{Entry: &entry.Entry{Nonce: 1}, HasLayers: true, Nonces: []uint64{1, 2, 3}}
	k := partsKey{entry: hash.Hash{1}, variant: wire.EntriesWithLayers}
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
	c.get(partsKey{entry: hash.Hash{2}, variant: wire.EntriesWithLayers}, it)
	if _, ok := c.items[k]; ok {
		t.Fatal("a stale item was kept")
	}
}

// RED-020: the layers-from order is strongest first, the lower nonce
// first among equals, and is cached by the layer count it was made for,
// so a fetch in many pages sorts once and a new layer makes a new order.
func TestLayerOrderIsCachedByCount(t *testing.T) {
	fn := testutil.ExampleSeries().Hash()
	eh := hash.Hash{7}
	ns := make([]uint64, 500)
	for i := range ns {
		ns[i] = uint64(len(ns) - i)
	}
	strongestNonces(fn, eh, ns)
	for i := 1; i < len(ns); i++ {
		a := layer.Layer{Entry: eh, Nonce: ns[i-1]}.Strength(fn)
		b := layer.Layer{Entry: eh, Nonce: ns[i]}.Strength(fn)
		if a < b || (a == b && ns[i-1] > ns[i]) {
			t.Fatalf("nonces %d and %d out of order (strength %d, %d)", ns[i-1], ns[i], a, b)
		}
	}
	var c partsCache
	k := partsKey{entry: eh, order: true, layers: len(ns)}
	c.put(k, nil, ns)
	if _, got, ok := c.peek(k); !ok || &got[0] != &ns[0] {
		t.Fatal("the order was not cached")
	}
	if _, _, ok := c.peek(partsKey{entry: eh, order: true, layers: len(ns) + 1}); ok {
		t.Fatal("an order made for other layers was found")
	}
	if _, _, ok := c.peek(partsKey{entry: eh, variant: wire.EntriesWithLayers}); ok {
		t.Fatal("an order was found as an encoded item")
	}
}
