//go:build goexperiment.simd && amd64

package grind

import (
	"crypto/sha256"
	"encoding/binary"
	"math/bits"
	"simd/archsimd"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
)

// sha256K are SHA-256's round constants.
var sha256K = [64]uint32{
	0x428a2f98, 0x71374491, 0xb5c0fbcf, 0xe9b5dba5, 0x3956c25b, 0x59f111f1, 0x923f82a4, 0xab1c5ed5,
	0xd807aa98, 0x12835b01, 0x243185be, 0x550c7dc3, 0x72be5d74, 0x80deb1fe, 0x9bdc06a7, 0xc19bf174,
	0xe49b69c1, 0xefbe4786, 0x0fc19dc6, 0x240ca1cc, 0x2de92c6f, 0x4a7484aa, 0x5cb0a9dc, 0x76f988da,
	0x983e5152, 0xa831c66d, 0xb00327c8, 0xbf597fc7, 0xc6e00bf3, 0xd5a79147, 0x06ca6351, 0x14292967,
	0x27b70a85, 0x2e1b2138, 0x4d2c6dfc, 0x53380d13, 0x650a7354, 0x766a0abb, 0x81c2c92e, 0x92722c85,
	0xa2bfe8a1, 0xa81a664b, 0xc24b8b70, 0xc76c51a3, 0xd192e819, 0xd6990624, 0xf40e3585, 0x106aa070,
	0x19a4c116, 0x1e376c08, 0x2748774c, 0x34b0bcb5, 0x391c0cb3, 0x4ed8aa4a, 0x5b9cca4f, 0x682e6ff3,
	0x748f82ee, 0x78a5636f, 0x84c87814, 0x8cc70208, 0x90befffa, 0xa4506ceb, 0xbef9a3f7, 0xc67178f2,
}

var sha256IV = [8]uint32{0x6a09e667, 0xbb67ae85, 0x3c6ef372, 0xa54ff53a, 0x510e527f, 0x9b05688c, 0x1f83d9ab, 0x5be0cd19}

// sha256LayerLanes searches nonces [from, from+n) of layers on entry with
// SHA-256 sixteen at a time, one nonce per lane of an AVX-512 vector
// (ORANGE-012, a prototype behind GOEXPERIMENT=simd). A layer hash is
// SHA-256(entry ∥ nonce): 40 bytes, one padded block, so a try is one
// compression. Words 0-7 (the entry) and 10-15 (the padding) are the same
// in every lane and every try, so the first eight rounds are done once,
// and only words 8 and 9 (the nonce) differ. It returns the strongest
// nonce, the lowest among equals, as layer.Grind does. n must be a
// multiple of 16.
func sha256LayerLanes(entry hash.Hash, from, n uint64) (uint64, int) {
	// The check lets the compiler fuse the logic into three-input
	// VPTERNLOGD instructions; the caller checks it already.
	if !archsimd.X86.AVX512() {
		panic("grind: sha256LayerLanes without AVX-512")
	}
	bc := func(x uint32) archsimd.Int32x16 { return archsimd.BroadcastInt32x16(int32(x)) }
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
	pre := [8]archsimd.Int32x16{bc(a), bc(b), bc(c), bc(d), bc(e), bc(f), bc(g), bc(h)}
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
	var k [64]archsimd.Int32x16
	for i := range k {
		k[i] = bc(sha256K[i])
	}
	var W, kw [64]archsimd.Int32x16
	for _, i := range []int{0, 1, 2, 3, 4, 5, 6, 7, 10, 11, 12, 13, 14, 15, 17, 19, 21} {
		W[i], kw[i] = bc(w[i]), bc(w[i]+sha256K[i])
	}
	r := shaRots{bc(2), bc(13), bc(22), bc(6), bc(11), bc(25), bc(7), bc(18), bc(17), bc(19)}
	sh3, sh10 := bc(3), bc(10)
	s1 := func(y archsimd.Int32x16) archsimd.Int32x16 {
		return y.RotateRight(r.r17).Xor(y.RotateRight(r.r19)).Xor(srl(y, sh10))
	}
	iv0 := bc(sha256IV[0])
	bestNonce, best := from, -1
	var lo, hi, out [16]uint32
	for base := from; base < from+n; base += 16 {
		for l := uint64(0); l < 16; l++ {
			hi[l], lo[l] = uint32((base+l)>>32), uint32(base+l)
		}
		W[8], W[9] = archsimd.LoadUint32x16Array(&hi).AsInt32x16(), archsimd.LoadUint32x16Array(&lo).AsInt32x16()
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
				s0 := y.RotateRight(r.r7).Xor(y.RotateRight(r.r18)).Xor(srl(y, sh3))
				W[j] = W[j-16].Add(s0).Add(W[j-7]).Add(s1(W[j-2]))
				kw[j] = W[j].Add(k[j])
			}
			d, h, x = shaRound(a, b, c, d, e, f, g, h, kw[i], x, &r)
			c, g, x = shaRound(h, a, b, c, d, e, f, g, kw[i+1], x, &r)
			b, f, x = shaRound(g, h, a, b, c, d, e, f, kw[i+2], x, &r)
			a, e, x = shaRound(f, g, h, a, b, c, d, e, kw[i+3], x, &r)
			h, d, x = shaRound(e, f, g, h, a, b, c, d, kw[i+4], x, &r)
			g, c, x = shaRound(d, e, f, g, h, a, b, c, kw[i+5], x, &r)
			f, b, x = shaRound(c, d, e, f, g, h, a, b, kw[i+6], x, &r)
			e, a, x = shaRound(b, c, d, e, f, g, h, a, kw[i+7], x, &r)
		}
		// The hash's first word decides the strength unless it is 0: only
		// lanes below 2^(31-best) can beat the best, tested all at once.
		h0 := a.Add(iv0)
		if best >= 0 && best < 31 && h0.AsUint32x16().Less(archsimd.BroadcastUint32x16(1<<(31-best))).ToBits() == 0 {
			continue
		}
		h0.AsUint32x16().StoreArray(&out)
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

// shaRound is one SHA-256 round on state a-h with K[i]+W[i] in kw and
// b^c in bxc. It returns the new e and a, which take d's and h's places,
// and a^b, the next round's b^c.
func shaRound(a, b, c, d, e, f, g, h, kw, bxc archsimd.Int32x16, r *shaRots) (archsimd.Int32x16, archsimd.Int32x16, archsimd.Int32x16) {
	S1 := e.RotateRight(r.r6).Xor(e.RotateRight(r.r11)).Xor(e.RotateRight(r.r25))
	t1 := h.Add(S1).Add(g.Xor(e.And(f.Xor(g)))).Add(kw)
	S0 := a.RotateRight(r.r2).Xor(a.RotateRight(r.r13)).Xor(a.RotateRight(r.r22))
	axb := a.Xor(b)
	return d.Add(t1), t1.Add(S0).Add(b.Xor(axb.And(bxc))), axb
}

// srl shifts right logically; Int32x16's ShiftRight is arithmetic. The
// lanes are Int32x16, not Uint32x16, because the compiler fuses only
// signed vectors' logic into VPTERNLOGD (Go 1.27).
func srl(x, n archsimd.Int32x16) archsimd.Int32x16 {
	return x.AsUint32x16().ShiftRight(n.AsUint32x16()).AsInt32x16()
}

type shaRots struct{ r2, r13, r22, r6, r11, r25, r7, r18, r17, r19 archsimd.Int32x16 }
