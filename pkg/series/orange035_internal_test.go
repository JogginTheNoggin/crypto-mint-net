package series

import (
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/policy"
)

// ORANGE-035: every Series built from the same params in a process shares
// one part chain, so building a series again (each reload, Accepted, an
// added release) does not hash a far release's chain again. Another
// series has its own, a fixed series none, and the ids are the chain's.
func TestPartChainShared(t *testing.T) {
	iss := hash.Hash{35}
	p := Params{HashID: 1, Mints: 8, Supply: 100, PoolSize: 4, PolicyTypes: []policy.TypeID{1}, KeyAlgs: []policy.AlgID{1},
		Issuer: &iss, SupplyType: Variable, PartSize: 4}
	a, err := New(p)
	if err != nil {
		t.Fatal(err)
	}
	const k = 3000
	want := naive(a, k)
	if id, _ := a.PartID(k); id != want[k-1] {
		t.Fatal("part id differs")
	}
	b, _ := New(p)
	if b.parts != a.parts || b.parts.next < k {
		t.Fatalf("a series built again works its chain out again (%d parts known)", b.parts.next)
	}
	for _, n := range []uint64{1, 1024, 1025, k} {
		if id, _ := b.PartID(n); id != want[n-1] {
			t.Fatalf("part %d differs", n)
		}
	}
	q := p
	q.PartSize = 8
	c, _ := New(q)
	if c.parts == a.parts {
		t.Fatal("another series shares the chain")
	}
	if id, _ := c.PartID(2); id == want[1] {
		t.Fatal("another series has the same part 2 id")
	}
	f, _ := New(Params{HashID: 1, Mints: 8, Supply: 100, PoolSize: 4, PolicyTypes: []policy.TypeID{1}, KeyAlgs: []policy.AlgID{1}})
	if f.parts != nil || IsVariableID(f.ID()) || !IsVariableID(a.ID()) {
		t.Fatal("a fixed series has a chain or reads as variable")
	}
}
