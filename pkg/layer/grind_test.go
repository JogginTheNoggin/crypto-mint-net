package layer

import (
	"fmt"
	"runtime"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
)

// TestGrindHashMatchesGrind checks that GrindHash returns exactly what
// Grind returns, for every known hash and any number of workers.
func TestGrindHashMatchesGrind(t *testing.T) {
	for _, id := range hash.IDs() {
		h, _ := hash.Lookup(id)
		for _, c := range []struct{ start, tries uint64 }{{0, 0}, {0, 1}, {7, 3}, {1 << 40, 5000}, {^uint64(0) - 10, 5}} {
			want, ws := Grind(h, hash.Hash{9}, c.start, c.tries)
			for _, workers := range []int{0, 1, 2, 3, 8, 64} {
				got, gs, err := GrindHash(id, hash.Hash{9}, c.start, c.tries, workers)
				if err != nil {
					t.Fatal(err)
				}
				if got != want || gs != ws {
					t.Fatalf("%s %+v workers %d: %v/%d, want %v/%d", id.Name(), c, workers, got, gs, want, ws)
				}
			}
		}
	}
}

// BenchmarkGrindHash is GrindHash per try, for each known hash, on one
// worker and on one per core.
func BenchmarkGrindHash(b *testing.B) {
	for _, id := range hash.IDs() {
		for _, w := range []int{1, runtime.GOMAXPROCS(0)} {
			b.Run(fmt.Sprintf("%s/workers=%d", id.Name(), w), func(b *testing.B) {
				b.ReportAllocs()
				if _, _, err := GrindHash(id, hash.Hash{1}, 0, uint64(b.N), w); err != nil {
					b.Fatal(err)
				}
			})
		}
	}
}
