package hash

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"testing"
)

// TestGrinderMatchesFunc checks that a Grinder gives exactly the hash
// Func gives, for every known hash and prefixes shorter and longer than a
// block, with and without a midstate.
func TestGrinderMatchesFunc(t *testing.T) {
	for _, id := range IDs() {
		f, _ := Lookup(id)
		for _, n := range []int{0, 1, 32, 63, 64, 127, 128, 129, 200, 1000, 9000} {
			t.Run(fmt.Sprintf("%s/%d", id.Name(), n), func(t *testing.T) {
				prefix := bytes.Repeat([]byte{0xa5}, n)
				// Split the prefix in two parts, as callers pass several.
				g, err := NewGrinder(id, prefix[:n/3], prefix[n/3:])
				if err != nil {
					t.Fatal(err)
				}
				var nonce [8]byte
				for i := uint64(0); i < 5; i++ {
					binary.BigEndian.PutUint64(nonce[:], i)
					if got, want := g.Hash(nonce[:]), f(prefix, nonce[:]); got != want {
						t.Fatalf("nonce %d: %s, want %s", i, got, want)
					}
				}
			})
		}
	}
}

func BenchmarkGrinder(b *testing.B) {
	for _, id := range IDs() {
		for _, n := range []int{32, 32 * 17, 32 * 257} {
			b.Run(fmt.Sprintf("%s/prefix=%d", id.Name(), n), func(b *testing.B) {
				g, _ := NewGrinder(id, make([]byte, n))
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
