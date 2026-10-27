package ledger

import "github.com/JogginTheNoggin/crypto-mint-net/pkg/weight"

// fenwick is a Fenwick (binary indexed) tree over the own weights of the
// selected ledger, indexed by height from 1. It answers "weight of the
// ledger up to height h" in O(log n) and takes a layer on any entry in
// O(log n), so a layer on an old entry does not touch every entry after it.
type fenwick struct {
	t []weight.Weight // t[0] unused
}

func newFenwick() *fenwick { return &fenwick{t: make([]weight.Weight, 1, 64)} }

// push appends the own weight of the next height.
func (f *fenwick) push(w weight.Weight) {
	i := len(f.t)
	sum := w
	low := i & -i
	for j := i - 1; j > i-low; j -= j & -j {
		sum = sum.Add(f.t[j])
	}
	f.t = append(f.t, sum)
}

// truncate keeps heights 1..n. Entries at or below n depend only on
// heights at or below n, so nothing needs recomputing.
func (f *fenwick) truncate(n int) {
	for i := n + 1; i < len(f.t); i++ {
		f.t[i] = weight.Zero
	}
	f.t = f.t[:n+1]
}

// add adds w to the own weight at height i.
func (f *fenwick) add(i int, w weight.Weight) {
	for ; i < len(f.t); i += i & -i {
		f.t[i] = f.t[i].Add(w)
	}
}

// prefix returns the sum of own weights at heights 1..i.
func (f *fenwick) prefix(i int) weight.Weight {
	sum := weight.Zero
	for ; i > 0; i -= i & -i {
		sum = sum.Add(f.t[i])
	}
	return sum
}
