//go:build goexperiment.simd && amd64

package grind

import (
	"context"
	"crypto/sha256"
	"simd/archsimd"
	"testing"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/layer"
)

func TestSHA256LanesMatchGrind(t *testing.T) {
	if !archsimd.X86.AVX512() {
		t.Skip("no AVX-512")
	}
	f, _ := hash.Lookup(hash.SHA256)
	for _, c := range []struct{ from, n uint64 }{{0, 16}, {0, 4096}, {1<<32 - 8, 32}, {1 << 40, 1 << 16}, {12345, 160}} {
		for e := byte(0); e < 3; e++ {
			entry := fullEntry(e)
			want, ws := layer.Grind(f, entry, c.from, c.n)
			n, s := sha256LayerLanes(entry, c.from, c.n)
			if n != want.Nonce || s != ws {
				t.Fatalf("%+v entry %d: %d/%d, want %d/%d", c, e, n, s, want.Nonce, ws)
			}
		}
	}
}

func BenchmarkSHA256Lanes(b *testing.B) {
	if !archsimd.X86.AVX512() {
		b.Skip("no AVX-512")
	}
	n := uint64(b.N+15) &^ 15
	b.ResetTimer()
	sha256LayerLanes(hash.Hash{1}, 0, n)
}

func TestSHA256Lanes8MatchGrind(t *testing.T) {
	if !archsimd.X86.AVX2() {
		t.Skip("no AVX2")
	}
	f, _ := hash.Lookup(hash.SHA256)
	for _, c := range []struct{ from, n uint64 }{{0, 8}, {0, 4096}, {1<<32 - 4, 16}, {1 << 40, 1 << 16}, {12345, 160}} {
		for e := byte(0); e < 3; e++ {
			entry := fullEntry(e)
			want, ws := layer.Grind(f, entry, c.from, c.n)
			n, s := sha256LayerLanes8(entry, c.from, c.n)
			if n != want.Nonce || s != ws {
				t.Fatalf("%+v entry %d: %d/%d, want %d/%d", c, e, n, s, want.Nonce, ws)
			}
		}
	}
}

func BenchmarkSHA256Lanes8(b *testing.B) {
	if !archsimd.X86.AVX2() {
		b.Skip("no AVX2")
	}
	n := uint64(b.N+7) &^ 7
	b.ResetTimer()
	sha256LayerLanes8(hash.Hash{1}, 0, n)
}

// TestLanesGrinders: with AVX-512, SHA-256 and BLAKE3 layers are searched
// with the lanes, and a search finds what layer.GrindHash finds, for any
// number of nonces and workers (ORANGE-012).
func TestLanesGrinders(t *testing.T) {
	if !archsimd.X86.AVX512() {
		t.Skip("no AVX-512")
	}
	for _, c := range []struct {
		id   hash.ID
		name string
	}{{hash.SHA256, "cpu-sha256-avx512"}, {hash.BLAKE3, "cpu-blake3-avx512"}} {
		p, err := Default.Choose(Default.Detect(), c.id, "", 3)
		if err != nil || p.Grinder != c.name || p.search == nil {
			t.Fatalf("%s: plan %+v %v", c.id.Name(), p, err)
		}
		for _, tries := range []uint64{1, 15, 17, 10007, 1 << 16} {
			entry := fullEntry(byte(tries))
			want, ws, _ := layer.GrindHash(c.id, entry, 99, tries, 1)
			got, gs, err := layer.GrindSearch(context.Background(), p.search, entry, 99, tries, p.Threads, nil)
			if err != nil || got != want || gs != ws {
				t.Fatalf("%s %d tries: %+v/%d, want %+v/%d (%v)", c.id.Name(), tries, got, gs, want, ws, err)
			}
		}
		b, err := RunBench(context.Background(), p, 200*time.Millisecond)
		if err != nil || b.Rate == 0 {
			t.Fatalf("bench %+v %v", b, err)
		}
	}
}

// fullEntry is an entry hash with every word set, so that a search that
// skips a word of the entry fails.
func fullEntry(e byte) hash.Hash { return hash.Hash(sha256.Sum256([]byte{e})) }
