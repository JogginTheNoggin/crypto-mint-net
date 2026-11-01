package p2p

import (
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/weight"
)

// Black's C-2 (D121): past maxProgress the host forgets the yielded chase
// that has shown least, and never a worked one for one that is not.
func TestProgressForgetsLeastShown(t *testing.T) {
	h := &Host{}
	worked := newProgress(hash.Hash{}, nil, false)
	worked.worked = true
	h.saveProgress(progressKey{group: 1}, worked)
	heavy := newProgress(hash.Hash{}, nil, false)
	heavy.total = weight.FromUint64(1 << 20)
	h.saveProgress(progressKey{group: 2}, heavy)
	for i := 0; i < maxProgress; i++ {
		h.saveProgress(progressKey{group: uint64(10 + i)}, newProgress(hash.Hash{}, nil, false))
	}
	if len(h.prog) != maxProgress {
		t.Fatalf("%d remembered", len(h.prog))
	}
	if h.prog[progressKey{group: 1}] != worked || h.prog[progressKey{group: 2}] != heavy {
		t.Fatal("a chase that showed work was forgotten for ones that showed none")
	}
	// Full of worked chases, a new unworked one is the one let go.
	h = &Host{}
	for i := 0; i < maxProgress; i++ {
		st := newProgress(hash.Hash{}, nil, false)
		st.worked = true
		h.saveProgress(progressKey{group: uint64(i)}, st)
	}
	h.saveProgress(progressKey{group: 9999}, newProgress(hash.Hash{}, nil, false))
	if _, ok := h.prog[progressKey{group: 9999}]; ok || len(h.prog) != maxProgress {
		t.Fatal("a worked chase was forgotten for an unworked one")
	}
}
