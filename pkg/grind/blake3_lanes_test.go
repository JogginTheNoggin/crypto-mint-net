//go:build goexperiment.simd && amd64

package grind

import (
	"simd/archsimd"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/layer"
)

func TestBLAKE3LanesMatchGrind(t *testing.T) {
	if !archsimd.X86.AVX512() {
		t.Skip("no AVX-512")
	}
	f, _ := hash.Lookup(hash.BLAKE3)
	for _, c := range []struct{ from, n uint64 }{{0, 16}, {0, 4096}, {1 << 40, 1 << 16}, {12345, 160}} {
		for e := byte(0); e < 3; e++ {
			entry := fullEntry(e)
			want, ws := layer.Grind(f, entry, c.from, c.n)
			n, s := blake3LayerLanes(entry, c.from, c.n)
			if n != want.Nonce || s != ws {
				t.Fatalf("%+v entry %d: %d/%d, want %d/%d", c, e, n, s, want.Nonce, ws)
			}
		}
	}
}

func BenchmarkBLAKE3Lanes(b *testing.B) {
	if !archsimd.X86.AVX512() {
		b.Skip("no AVX-512")
	}
	n := uint64(b.N+15) &^ 15
	b.ResetTimer()
	blake3LayerLanes(hash.Hash{1}, 0, n)
}
