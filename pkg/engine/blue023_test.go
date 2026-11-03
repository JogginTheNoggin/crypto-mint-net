package engine_test

import (
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/engine"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
)

// RED-063, D160: the seen list grows with the items it holds, so a large
// size costs nothing at start, and still drops its oldest item when full.
func TestSeenGrowsWithItems(t *testing.T) {
	s := engine.NewSeen(1 << 22)
	for i := 0; i < 10; i++ {
		s.Add(hash.Hash{byte(i)})
	}
	if s.Len() != 10 || !s.Has(hash.Hash{9}) {
		t.Fatalf("len %d", s.Len())
	}
	r := engine.NewSeen(3)
	for i := 0; i < 7; i++ {
		r.Add(hash.Hash{byte(i)})
	}
	if r.Len() != 3 || r.Has(hash.Hash{3}) || !r.Has(hash.Hash{4}) || !r.Has(hash.Hash{6}) {
		t.Fatalf("a full list of 3 after 7 adds: len %d", r.Len())
	}
	r.Add(hash.Hash{6}) // the same item twice holds two slots
	r.Add(hash.Hash{7})
	if !r.Has(hash.Hash{6}) || r.Has(hash.Hash{4}) || r.Has(hash.Hash{5}) {
		t.Fatal("a repeated item was dropped with its older copy")
	}
}
