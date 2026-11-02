//go:build goexperiment.simd && amd64

package grind

import (
	"crypto/sha256"
	"encoding/binary"
	"math/bits"
	"simd/archsimd"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
)

// sha256LayerLanes8 searches nonces [from, from+n) of layers on entry with
// SHA-256 eight at a time, one nonce per lane of an AVX2 vector
// (ORANGE-012, a prototype behind GOEXPERIMENT=simd). A layer hash is
// SHA-256(entry ∥ nonce): 40 bytes, one padded block, so a try is one
// compression. Words 0-7 (the entry) and 10-15 (the padding) are the same
// in every lane and every try, so the first eight rounds are done once,
// and only words 8 and 9 (the nonce) differ. It returns the strongest
// nonce, the lowest among equals, as layer.Grind does. n must be a
// multiple of 8.
func sha256LayerLanes8(entry hash.Hash, from, n uint64) (uint64, int) {
	bc := archsimd.BroadcastUint32x8
	var w0 [16]uint32
	for i := 0; i < 8; i++ {
		w0[i] = binary.BigEndian.Uint32(entry[4*i:])
	}
	w0[10], w0[15] = 0x80000000, 40*8
	// Rounds 0-7 use only the entry's words: the same for every nonce.
	a, b, c, d, e, f, g, h := sha256IV[0], sha256IV[1], sha256IV[2], sha256IV[3], sha256IV[4], sha256IV[5], sha256IV[6], sha256IV[7]
	for i := 0; i < 8; i++ {
		t1 := h + (bits.RotateLeft32(e, -6) ^ bits.RotateLeft32(e, -11) ^ bits.RotateLeft32(e, -25)) + ((e & f) ^ (^e & g)) + sha256K[i] + w0[i]
		t2 := (bits.RotateLeft32(a, -2) ^ bits.RotateLeft32(a, -13) ^ bits.RotateLeft32(a, -22)) + ((a & b) ^ (a & c) ^ (b & c))
		h, g, f, e, d, c, b, a = g, f, e, d+t1, c, b, a, t1+t2
	}
	pre := [8]archsimd.Uint32x8{bc(a), bc(b), bc(c), bc(d), bc(e), bc(f), bc(g), bc(h)}
	// Words 17, 19 and 21 are the same for every nonce too, and 16, 18,
	// 20 and 22 are a constant plus one varying term. kw[i] is K[i]+W[i].
	sig0 := func(x uint32) uint32 { return bits.RotateLeft32(x, -7) ^ bits.RotateLeft32(x, -18) ^ x>>3 }
	sig1 := func(x uint32) uint32 { return bits.RotateLeft32(x, -17) ^ bits.RotateLeft32(x, -19) ^ x>>10 }
	w := w0[:]
	w = append(w, 0, w0[1]+sig0(w0[2])+w0[10]+sig1(w0[15]), 0)
	w = append(w, w0[3]+sig0(w0[4])+w0[12]+sig1(w[17]), 0)
	w = append(w, w0[5]+sig0(w0[6])+w0[14]+sig1(w[19]))
	c16 := bc(w0[0] + sig0(w0[1]) + sig1(w0[14]) + sha256K[16])
	c18 := bc(w0[2] + sig0(w0[3]) + w0[11])
	c20 := bc(w0[4] + sig0(w0[5]) + w0[13])
	c22 := bc(w0[6] + sig0(w0[7]) + w0[15])
	var k [64]archsimd.Uint32x8
	for i := range k {
		k[i] = bc(sha256K[i])
	}
	var W, kw [64]archsimd.Uint32x8
	for _, i := range []int{0, 1, 2, 3, 4, 5, 6, 7, 10, 11, 12, 13, 14, 15, 17, 19, 21} {
		W[i], kw[i] = bc(w[i]), bc(w[i]+sha256K[i])
	}
	r := newShaRots8()
	sh3, sh10 := bc(3), bc(10)
	s1 := func(y archsimd.Uint32x8) archsimd.Uint32x8 {
		return r.rot(y, 17).Xor(r.rot(y, 19)).Xor(y.ShiftRight(sh10))
	}
	iv0 := bc(sha256IV[0])
	bestNonce, best := from, -1
	var lo, hi, out [8]uint32
	for base := from; base < from+n; base += 8 {
		for l := uint64(0); l < 8; l++ {
			hi[l], lo[l] = uint32((base+l)>>32), uint32(base+l)
		}
		W[8], W[9] = archsimd.LoadUint32x8Array(&hi), archsimd.LoadUint32x8Array(&lo)
		kw[8], kw[9] = W[8].Add(k[8]), W[9].Add(k[9])
		kw[16] = c16.Add(W[9])
		W[16] = kw[16].Sub(k[16])
		W[18] = c18.Add(s1(W[16]))
		W[20] = c20.Add(s1(W[18]))
		W[22] = c22.Add(s1(W[20]))
		kw[18], kw[20], kw[22] = W[18].Add(k[18]), W[20].Add(k[20]), W[22].Add(k[22])
		// Rounds 8-63, eight at a time so that no state is copied: each
		// round changes only d and h. maj(a, b, c) is b ^ ((a^b) & (b^c)),
		// and b^c is the last round's a^b.
		a, b, c, d, e, f, g, h := pre[0], pre[1], pre[2], pre[3], pre[4], pre[5], pre[6], pre[7]
		x := b.Xor(c)
		for i := 8; i < 64; i += 8 {
			// The words of the next eight rounds, beside this eight's: the
			// two chains run side by side.
			for j := max(i+8, 23); j < min(i+16, 64); j++ {
				y := W[j-15]
				s0 := r.rot(y, 7).Xor(r.rot(y, 18)).Xor(y.ShiftRight(sh3))
				W[j] = W[j-16].Add(s0).Add(W[j-7]).Add(s1(W[j-2]))
				kw[j] = W[j].Add(k[j])
			}
			d, h, x = shaRound8(a, b, c, d, e, f, g, h, kw[i], x, r)
			c, g, x = shaRound8(h, a, b, c, d, e, f, g, kw[i+1], x, r)
			b, f, x = shaRound8(g, h, a, b, c, d, e, f, kw[i+2], x, r)
			a, e, x = shaRound8(f, g, h, a, b, c, d, e, kw[i+3], x, r)
			h, d, x = shaRound8(e, f, g, h, a, b, c, d, kw[i+4], x, r)
			g, c, x = shaRound8(d, e, f, g, h, a, b, c, kw[i+5], x, r)
			f, b, x = shaRound8(c, d, e, f, g, h, a, b, kw[i+6], x, r)
			e, a, x = shaRound8(b, c, d, e, f, g, h, a, kw[i+7], x, r)
		}
		// The hash's first word decides the strength unless it is 0: only
		// lanes below 2^(31-best) can beat the best, tested all at once.
		h0 := a.Add(iv0)
		h0.StoreArray(&out)
		for l, w := range out {
			if w == 0 {
				// Strength 32 or more: rare, so hash this nonce in full.
				var msg [40]byte
				copy(msg[:], entry[:])
				binary.BigEndian.PutUint64(msg[32:], base+uint64(l))
				if s := hash.Hash(sha256.Sum256(msg[:])).Strength(); s > best {
					bestNonce, best = base+uint64(l), s
				}
				continue
			}
			if s := bits.LeadingZeros32(w); s > best {
				bestNonce, best = base+uint64(l), s
			}
		}
	}
	return bestNonce, best
}

// shaRound8 is one SHA-256 round on state a-h with K[i]+W[i] in kw and
// b^c in bxc. It returns the new e and a, which take d's and h's places,
// and a^b, the next round's b^c.
func shaRound8(a, b, c, d, e, f, g, h, kw, bxc archsimd.Uint32x8, r *shaRots8) (archsimd.Uint32x8, archsimd.Uint32x8, archsimd.Uint32x8) {
	S1 := r.rot(e, 6).Xor(r.rot(e, 11)).Xor(r.rot(e, 25))
	t1 := h.Add(S1).Add(g.Xor(e.And(f.Xor(g)))).Add(kw)
	S0 := r.rot(a, 2).Xor(r.rot(a, 13)).Xor(r.rot(a, 22))
	axb := a.Xor(b)
	return d.Add(t1), t1.Add(S0).Add(b.Xor(axb.And(bxc))), axb
}

// shaRots8 holds shift counts for rotations: AVX2 has no rotate, so a
// rotation is two shifts and an or.
type shaRots8 struct{ right, left [32]archsimd.Uint32x8 }

func newShaRots8() *shaRots8 {
	r := &shaRots8{}
	for n := 1; n < 32; n++ {
		r.right[n], r.left[n] = archsimd.BroadcastUint32x8(uint32(n)), archsimd.BroadcastUint32x8(uint32(32-n))
	}
	return r
}

func (r *shaRots8) rot(x archsimd.Uint32x8, n int) archsimd.Uint32x8 {
	return x.ShiftRight(r.right[n]).Or(x.ShiftLeft(r.left[n]))
}
