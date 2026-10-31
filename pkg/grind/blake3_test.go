package grind

import (
	"encoding/binary"
	"fmt"
	"math/rand"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
)

// TestBLAKE3GrinderMatches checks the BLAKE3 grinder against package
// hash's BLAKE3 for every prefix length up to past four chunks, and
// suffixes of 0 to 65 bytes, across chunk and block boundaries.
func TestBLAKE3GrinderMatches(t *testing.T) {
	f, _ := hash.Lookup(hash.BLAKE3)
	rng := rand.New(rand.NewSource(1))
	data := make([]byte, 5*b3ChunkLen)
	rng.Read(data)
	for n := 0; n <= 4*b3ChunkLen+130; n++ {
		g := newBLAKE3Bytes(data[:n])
		for _, s := range []int{0, 1, 8, 63, 64, 65} {
			suffix := data[len(data)-s:]
			if got, want := g.Hash(suffix), f(data[:n], suffix); got != want {
				t.Fatalf("prefix %d, suffix %d: %s, want %s", n, s, got, want)
			}
		}
	}
	// Many tries on one grinder, as a search makes.
	g, _ := newBLAKE3(data[:100], data[100:3000])
	var nonce [8]byte
	for i := uint64(0); i < 1000; i++ {
		binary.BigEndian.PutUint64(nonce[:], i)
		if got, want := g.Hash(nonce[:]), f(data[:3000], nonce[:]); got != want {
			t.Fatalf("nonce %d", i)
		}
	}
}

func BenchmarkBLAKE3Grinder(b *testing.B) {
	for _, n := range []int{32, 32 * 17, 32 * 257} {
		for _, which := range []string{"midstate", "native"} {
			b.Run(fmt.Sprintf("prefix=%d/%s", n, which), func(b *testing.B) {
				var g hash.Hasher
				if which == "native" {
					g, _ = newBLAKE3(make([]byte, n))
				} else {
					g, _ = hash.NewGrinder(hash.BLAKE3, make([]byte, n))
				}
				var nonce [8]byte
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					binary.BigEndian.PutUint64(nonce[:], uint64(i))
					g.Hash(nonce[:])
				}
			})
		}
	}
}
