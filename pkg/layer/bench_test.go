package layer

import (
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
)

// BenchmarkHash is one layer hash: the cost of one try when grinding.
func BenchmarkHash(b *testing.B) {
	h, _ := hash.Lookup(hash.SHA256)
	l := Layer{Entry: hash.Hash{1}}
	for i := 0; i < b.N; i++ {
		l.Nonce = uint64(i)
		l.Hash(h)
	}
}

// BenchmarkGrind is the builder's loop, per try, for each known hash.
func BenchmarkGrind(b *testing.B) {
	for _, id := range hash.IDs() {
		b.Run(id.Name(), func(b *testing.B) {
			h, _ := hash.Lookup(id)
			b.ReportAllocs()
			Grind(h, hash.Hash{1}, 0, uint64(b.N))
		})
	}
}
