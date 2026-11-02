package policy

import (
	"errors"
	"fmt"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
)

// TestCheckAllMatchesInOrder: CheckAll returns what CheckCached on each
// witness in turn returns, the first failure's index and error, for few
// and many witnesses, bad signatures and failed cheap checks anywhere;
// and a set it passed is remembered as one key (ORANGE-014).
func TestCheckAllMatchesInOrder(t *testing.T) {
	h := sha(t)
	tid := h([]byte("tid"))
	keys := make([]*Ed25519Signer, 8)
	for i := range keys {
		keys[i] = signer(t, fmt.Sprint("k", i))
	}
	build := func(n int) ([]Witness, []hash.Hash) {
		ws, addrs := make([]Witness, n), make([]hash.Hash, n)
		for i := range ws {
			k := keys[i%len(keys)]
			ws[i] = SingleKeyWitness(k, tid)
			addrs[i] = Address(h, SingleKey(AlgEd25519, k.Public()))
		}
		return ws, addrs
	}
	inOrder := func(ws []Witness, addrs []hash.Hash) (int, error) {
		for i, w := range ws {
			if err := Default().CheckCached(h, allowAll, w, addrs[i], tid, nil); err != nil {
				return i, err
			}
		}
		return -1, nil
	}
	badSig := func(w Witness) Witness {
		w.Data = append([]byte(nil), w.Data...)
		w.Data[len(w.Data)-1] ^= 1
		return w
	}
	for _, n := range []int{3, ManyWitnesses, 500} {
		for _, c := range []struct {
			name       string
			sig, cheap []int
		}{
			{"valid", nil, nil},
			{"bad first", []int{0}, nil},
			{"bad last", []int{n - 1}, nil},
			{"bad two", []int{n / 2, n - 1}, nil},
			{"address", nil, []int{n / 3}},
			{"bad before address", []int{n / 4}, []int{n / 2}},
			{"address before bad", []int{n - 1}, []int{1}},
		} {
			ws, addrs := build(n)
			for _, i := range c.sig {
				ws[i] = badSig(ws[i])
			}
			for _, i := range c.cheap {
				addrs[i] = h([]byte("nobody"))
			}
			wi, werr := inOrder(ws, addrs)
			v := NewVerified(16)
			for pass := 0; pass < 2; pass++ {
				gi, gerr := Default().CheckAll(h, allowAll, ws, addrs, tid, v)
				if gi != wi || !errors.Is(gerr, werr) && fmt.Sprint(gerr) != fmt.Sprint(werr) {
					t.Fatalf("%d %s pass %d: %d %v, want %d %v", n, c.name, pass, gi, gerr, wi, werr)
				}
			}
			switch {
			case werr != nil && v.Len() != 0 && n >= ManyWitnesses:
				t.Fatalf("%d %s: a failed set remembered %d", n, c.name, v.Len())
			case werr == nil && n >= ManyWitnesses && v.Len() != 1:
				t.Fatalf("%d %s: a passed set remembered as %d keys, want 1", n, c.name, v.Len())
			}
		}
	}
}

func BenchmarkCheckAll(b *testing.B) {
	h := sha(&testing.T{})
	tid := h([]byte("tid"))
	k := signer(&testing.T{}, "k")
	const n = 2000
	ws, addrs := make([]Witness, n), make([]hash.Hash, n)
	for i := range ws {
		ws[i] = SingleKeyWitness(k, tid)
		addrs[i] = Address(h, SingleKey(AlgEd25519, k.Public()))
	}
	b.Run("in order", func(b *testing.B) {
		for range b.N {
			for i, w := range ws {
				if err := Default().CheckCached(h, allowAll, w, addrs[i], tid, nil); err != nil {
					b.Fatal(err)
				}
			}
		}
	})
	b.Run("CheckAll", func(b *testing.B) {
		for range b.N {
			if _, err := Default().CheckAll(h, allowAll, ws, addrs, tid, nil); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("CheckAll remembered", func(b *testing.B) {
		v := NewVerified(0)
		for range b.N {
			if _, err := Default().CheckAll(h, allowAll, ws, addrs, tid, v); err != nil {
				b.Fatal(err)
			}
		}
	})
}
