//go:build goexperiment.simd && amd64

package grind

import (
	"encoding/binary"
	"simd/archsimd"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/layer"
)

// With GOEXPERIMENT=simd on a CPU with AVX-512, SHA-256 and BLAKE3 layers
// are searched sixteen nonces at a time (ORANGE-011, ORANGE-012). A build
// without the experiment, or a CPU without AVX-512, keeps the grinders of
// newDefault.
func init() {
	if !archsimd.X86.AVX512() {
		return
	}
	Default.Register(Grinder{Name: "cpu-sha256-avx512", Kind: CPU, Hash: hash.SHA256,
		New: func(Device, hash.ID) (hash.NewHasher, error) { return hash.GrinderFor(hash.SHA256), nil },
		Search: func(Device, hash.ID) (layer.Search, error) {
			return lanesSearch(sha256LayerLanes, hash.GrinderFor(hash.SHA256)), nil
		}})
	Default.Register(Grinder{Name: "cpu-blake3-avx512", Kind: CPU, Hash: hash.BLAKE3,
		New: func(Device, hash.ID) (hash.NewHasher, error) { return newBLAKE3Best, nil },
		Search: func(Device, hash.ID) (layer.Search, error) {
			return lanesSearch(blake3LayerLanes, hash.GrinderFor(hash.BLAKE3)), nil
		}})
}

// lanesSearch makes a layer.Search of a sixteen-lane search, which takes
// multiples of 16 nonces: the nonces left over are hashed one at a time.
func lanesSearch(lanes func(hash.Hash, uint64, uint64) (uint64, int), nh hash.NewHasher) layer.Search {
	return func(entry hash.Hash, from, n uint64) (uint64, int) {
		m := n &^ 15
		bestNonce, best := from, -1
		if m > 0 {
			bestNonce, best = lanes(entry, from, m)
		}
		if m == n {
			return bestNonce, best
		}
		g, err := nh(entry[:])
		if err != nil {
			panic(err) // the grinders of package hash take any prefix
		}
		var buf [8]byte
		for nonce := from + m; nonce < from+n; nonce++ {
			binary.BigEndian.PutUint64(buf[:], nonce)
			if s := g.Hash(buf[:]).Strength(); s > best {
				bestNonce, best = nonce, s
			}
		}
		return bestNonce, best
	}
}
