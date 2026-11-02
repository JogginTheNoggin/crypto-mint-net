//go:build goexperiment.simd && amd64

package grind

import (
	"encoding/binary"
	"math/bits"
	"simd/archsimd"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
)

// blake3LayerLanes searches nonces [from, from+n) of layers on entry with
// BLAKE3 sixteen at a time, one nonce per lane of an AVX-512 vector
// (ORANGE-011, a prototype behind GOEXPERIMENT=simd). A layer hash is
// BLAKE3(entry ∥ nonce), one 40-byte block, so a try is one compression
// with only the two nonce words differing between lanes. It returns the
// strongest nonce, the lowest among equals, as layer.Grind does. n must
// be a multiple of 16.
func blake3LayerLanes(entry hash.Hash, from, n uint64) (uint64, int) {
	bc := archsimd.BroadcastUint32x16
	var ew [8]uint32
	for i := range ew {
		ew[i] = binary.LittleEndian.Uint32(entry[4*i:])
	}
	iv := b3IV
	zero := bc(0)
	blockLen := bc(40)
	flags := bc(b3ChunkStart | b3ChunkEnd | b3Root)
	r := rots{bc(16), bc(12), bc(8), bc(7)}
	low8 := bc(0xFF)
	bestNonce, best := from, -1
	var lo, hi [16]uint32
	var out [16]uint32
	for base := from; base < from+n; base += 16 {
		// The nonce is 8 big-endian bytes: message words 8 and 9 read them
		// little-endian.
		for l := uint64(0); l < 16; l++ {
			var nb [8]byte
			binary.BigEndian.PutUint64(nb[:], base+l)
			lo[l] = binary.LittleEndian.Uint32(nb[0:])
			hi[l] = binary.LittleEndian.Uint32(nb[4:])
		}
		m0, m1, m2, m3, m4, m5, m6, m7 := bc(ew[0]), bc(ew[1]), bc(ew[2]), bc(ew[3]), bc(ew[4]), bc(ew[5]), bc(ew[6]), bc(ew[7])
		m8, m9 := archsimd.LoadUint32x16Array(&lo), archsimd.LoadUint32x16Array(&hi)
		m10, m11, m12, m13, m14, m15 := zero, zero, zero, zero, zero, zero
		s0, s1, s2, s3 := bc(iv[0]), bc(iv[1]), bc(iv[2]), bc(iv[3])
		s4, s5, s6, s7 := bc(iv[4]), bc(iv[5]), bc(iv[6]), bc(iv[7])
		s8, s9, s10, s11 := bc(iv[0]), bc(iv[1]), bc(iv[2]), bc(iv[3])
		s12, s13, s14, s15 := zero, zero, blockLen, flags
		for round := 0; round < 7; round++ {
			s0, s4, s8, s12 = laneG(s0, s4, s8, s12, m0, m1, r)
			s1, s5, s9, s13 = laneG(s1, s5, s9, s13, m2, m3, r)
			s2, s6, s10, s14 = laneG(s2, s6, s10, s14, m4, m5, r)
			s3, s7, s11, s15 = laneG(s3, s7, s11, s15, m6, m7, r)
			s0, s5, s10, s15 = laneG(s0, s5, s10, s15, m8, m9, r)
			s1, s6, s11, s12 = laneG(s1, s6, s11, s12, m10, m11, r)
			s2, s7, s8, s13 = laneG(s2, s7, s8, s13, m12, m13, r)
			s3, s4, s9, s14 = laneG(s3, s4, s9, s14, m14, m15, r)
			m0, m1, m2, m3, m4, m5, m6, m7, m8, m9, m10, m11, m12, m13, m14, m15 =
				m2, m6, m3, m10, m7, m0, m4, m13, m1, m11, m12, m5, m9, m14, m15, m8
		}
		// The hash's first word decides the strength unless it is 0. Once
		// the best has strength 8 or more, only lanes whose first byte is
		// zero can beat it: test that for all lanes at once.
		h0 := s0.Xor(s8)
		if best >= 8 && h0.And(low8).Equal(zero).ToBits() == 0 {
			continue
		}
		h0.StoreArray(&out)
		for l, w := range out {
			if w == 0 {
				// Strength 32 or more: rare, so hash this nonce in full.
				var nb [8]byte
				binary.BigEndian.PutUint64(nb[:], base+uint64(l))
				f, _ := hash.Lookup(hash.BLAKE3)
				if s := f(entry[:], nb[:]).Strength(); s > best {
					bestNonce, best = base+uint64(l), s
				}
				continue
			}
			if s := bits.LeadingZeros32(bits.ReverseBytes32(w)); s > best {
				bestNonce, best = base+uint64(l), s
			}
		}
	}
	return bestNonce, best
}

// rots holds the rotation counts as vectors, for AVX-512's per-lane
// rotate (VPRORVD); RotateAllRight is emulated with two shifts and an or.
type rots struct{ r16, r12, r8, r7 archsimd.Uint32x16 }

func laneG(a, b, c, d, mx, my archsimd.Uint32x16, r rots) (archsimd.Uint32x16, archsimd.Uint32x16, archsimd.Uint32x16, archsimd.Uint32x16) {
	a = a.Add(b).Add(mx)
	d = d.Xor(a).RotateRight(r.r16)
	c = c.Add(d)
	b = b.Xor(c).RotateRight(r.r12)
	a = a.Add(b).Add(my)
	d = d.Xor(a).RotateRight(r.r8)
	c = c.Add(d)
	b = b.Xor(c).RotateRight(r.r7)
	return a, b, c, d
}
