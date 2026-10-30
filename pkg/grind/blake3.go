package grind

import (
	"encoding/binary"
	"math/bits"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
)

// A BLAKE3 grinder that keeps only the 32-byte chaining values a search
// needs, not a whole hasher (ORANGE-009). The general BLAKE3 hasher
// buffers up to a full 1 KB chunk and carries an 8 KB buffer, so saving
// and restoring its state for every try costs more than the hash itself.
// This one compresses the blocks of the prefix that every try shares
// once, then for each try only the last block or two of the last chunk
// and the parents above it. It follows the BLAKE3 specification for the
// default 32-byte, unkeyed hash, and is checked against package hash's
// BLAKE3 for every length up to several chunks.

const (
	b3ChunkLen   = 1024
	b3BlockLen   = 64
	b3ChunkStart = 1
	b3ChunkEnd   = 2
	b3ParentFlag = 4
	b3Root       = 8
)

var b3IV = [8]uint32{0x6A09E667, 0xBB67AE85, 0x3C6EF372, 0xA54FF53A, 0x510E527F, 0x9B05688C, 0x1F83D9AB, 0x5BE0CD19}

// b3Compress is the BLAKE3 compression function, returning the first
// half of its output: the chaining value, or the hash at the root.
func b3Compress(cv *[8]uint32, m *[16]uint32, counter uint64, blockLen, flags uint32) [8]uint32 {
	s0, s1, s2, s3, s4, s5, s6, s7 := cv[0], cv[1], cv[2], cv[3], cv[4], cv[5], cv[6], cv[7]
	s8, s9, s10, s11 := b3IV[0], b3IV[1], b3IV[2], b3IV[3]
	s12, s13, s14, s15 := uint32(counter), uint32(counter>>32), blockLen, flags
	m0, m1, m2, m3, m4, m5, m6, m7 := m[0], m[1], m[2], m[3], m[4], m[5], m[6], m[7]
	m8, m9, m10, m11, m12, m13, m14, m15 := m[8], m[9], m[10], m[11], m[12], m[13], m[14], m[15]
	for r := 0; r < 7; r++ {
		// Columns.
		s0, s4, s8, s12 = b3G(s0, s4, s8, s12, m0, m1)
		s1, s5, s9, s13 = b3G(s1, s5, s9, s13, m2, m3)
		s2, s6, s10, s14 = b3G(s2, s6, s10, s14, m4, m5)
		s3, s7, s11, s15 = b3G(s3, s7, s11, s15, m6, m7)
		// Diagonals.
		s0, s5, s10, s15 = b3G(s0, s5, s10, s15, m8, m9)
		s1, s6, s11, s12 = b3G(s1, s6, s11, s12, m10, m11)
		s2, s7, s8, s13 = b3G(s2, s7, s8, s13, m12, m13)
		s3, s4, s9, s14 = b3G(s3, s4, s9, s14, m14, m15)
		// Permute the message words: 2 6 3 10 7 0 4 13 1 11 12 5 9 14 15 8.
		m0, m1, m2, m3, m4, m5, m6, m7, m8, m9, m10, m11, m12, m13, m14, m15 =
			m2, m6, m3, m10, m7, m0, m4, m13, m1, m11, m12, m5, m9, m14, m15, m8
	}
	return [8]uint32{s0 ^ s8, s1 ^ s9, s2 ^ s10, s3 ^ s11, s4 ^ s12, s5 ^ s13, s6 ^ s14, s7 ^ s15}
}

func b3G(a, b, c, d, mx, my uint32) (uint32, uint32, uint32, uint32) {
	a += b + mx
	d = bits.RotateLeft32(d^a, -16)
	c += d
	b = bits.RotateLeft32(b^c, -12)
	a += b + my
	d = bits.RotateLeft32(d^a, -8)
	c += d
	b = bits.RotateLeft32(b^c, -7)
	return a, b, c, d
}

func b3Words(m *[16]uint32, block []byte) {
	var buf [b3BlockLen]byte
	copy(buf[:], block)
	for i := range m {
		m[i] = binary.LittleEndian.Uint32(buf[4*i:])
	}
}

func b3Parent(left, right *[8]uint32, flags uint32) [8]uint32 {
	var m [16]uint32
	copy(m[:8], left[:])
	copy(m[8:], right[:])
	return b3Compress(&b3IV, &m, 0, b3BlockLen, b3ParentFlag|flags)
}

// blake3Hasher grinds BLAKE3 over a fixed prefix and a suffix of at most
// one block (a nonce is 8 bytes).
type blake3Hasher struct {
	stack  [][8]uint32 // chaining values of whole chunks before chunk, merged as BLAKE3 merges them
	chunk  uint64      // index of the chunk where each try starts
	cv     [8]uint32   // that chunk's chaining value after its saved blocks
	blocks int         // blocks of that chunk already compressed into cv
	tail   []byte      // the prefix bytes after them
	buf    []byte      // tail followed by the suffix, reused
	prefix []byte      // the whole prefix, for a suffix too long for the fast path
}

const suffixMax = b3BlockLen

// newBLAKE3 makes the BLAKE3 grinder for a prefix.
func newBLAKE3(prefix ...[]byte) (hash.Hasher, error) {
	var p []byte
	for _, b := range prefix {
		p = append(p, b...)
	}
	return newBLAKE3Bytes(p), nil
}

// newBLAKE3Best uses the general hasher's assembly for a prefix of less
// than one block (a layer: the entry hash), where there is no state to
// save, and this grinder for longer ones (an entry: the tag, parent and
// tids), where restoring the general hasher's 8 KB state dominates.
func newBLAKE3Best(prefix ...[]byte) (hash.Hasher, error) {
	n := 0
	for _, b := range prefix {
		n += len(b)
	}
	if n < b3BlockLen {
		return hash.NewGrinder(hash.BLAKE3, prefix...)
	}
	return newBLAKE3(prefix...)
}

func newBLAKE3Bytes(p []byte) *blake3Hasher {
	h := &blake3Hasher{cv: b3IV, prefix: p}
	// Whole chunks before the one holding the prefix's last byte (or, for
	// a prefix that ends a chunk, before the next one) are followed by
	// more input in every try, so they and their merges are done once.
	var m [16]uint32
	for c := uint64(0); (c+1)*b3ChunkLen <= uint64(len(p)) && (c+1)*b3ChunkLen != uint64(len(p)); c++ {
		cv := b3IV
		for b := 0; b < b3ChunkLen/b3BlockLen; b++ {
			flags := uint32(0)
			if b == 0 {
				flags |= b3ChunkStart
			}
			if b == b3ChunkLen/b3BlockLen-1 {
				flags |= b3ChunkEnd
			}
			b3Words(&m, p[int(c)*b3ChunkLen+b*b3BlockLen:])
			cv = b3Compress(&cv, &m, c, b3BlockLen, flags)
		}
		h.push(cv, c+1)
		h.chunk = c + 1
	}
	rest := p[h.chunk*b3ChunkLen:]
	// Whole blocks of that chunk with at least one prefix byte after them
	// are never the chunk's last block, so they are done once too.
	for (h.blocks+1)*b3BlockLen < len(rest) {
		flags := uint32(0)
		if h.blocks == 0 {
			flags |= b3ChunkStart
		}
		b3Words(&m, rest[h.blocks*b3BlockLen:])
		h.cv = b3Compress(&h.cv, &m, h.chunk, b3BlockLen, flags)
		h.blocks++
	}
	h.tail = append([]byte(nil), rest[h.blocks*b3BlockLen:]...)
	h.buf = make([]byte, 0, len(h.tail)+suffixMax)
	return h
}

// push adds a finished chunk's chaining value, total chunks so far, and
// merges as BLAKE3 does: one parent for each trailing zero bit of total.
func (h *blake3Hasher) push(cv [8]uint32, total uint64) {
	for total&1 == 0 {
		left := h.stack[len(h.stack)-1]
		h.stack = h.stack[:len(h.stack)-1]
		cv = b3Parent(&left, &cv, 0)
		total >>= 1
	}
	h.stack = append(h.stack, cv)
}

// Hash returns BLAKE3(prefix ∥ suffix).
func (h *blake3Hasher) Hash(suffix []byte) hash.Hash {
	if len(suffix) > suffixMax {
		f, _ := hash.Lookup(hash.BLAKE3)
		return f(h.prefix, suffix)
	}
	buf := append(h.buf[:0], h.tail...)
	buf = append(buf, suffix...)
	// The tail and suffix are at most 1 KB + 64 bytes from the start of
	// h.chunk: they finish that chunk and, rarely, start one more.
	stack := h.stack
	chunk, cv, b := h.chunk, h.cv, h.blocks
	var m [16]uint32
	var out [8]uint32
	extra := false // whether stack was copied for a chunk finished here
	for {
		room := b3ChunkLen - b*b3BlockLen // bytes left in this chunk
		n := min(len(buf), b3BlockLen)
		endOfChunk := len(buf) <= b3BlockLen || room == b3BlockLen
		flags := uint32(0)
		if b == 0 {
			flags |= b3ChunkStart
		}
		if endOfChunk {
			flags |= b3ChunkEnd
		}
		b3Words(&m, buf[:n])
		buf = buf[n:]
		if endOfChunk && len(buf) == 0 {
			if len(stack) == 0 {
				flags |= b3Root
			}
			out = b3Compress(&cv, &m, chunk, uint32(n), flags)
			break
		}
		cv = b3Compress(&cv, &m, chunk, uint32(n), flags)
		b++
		if endOfChunk {
			// The chunk is full and more follows: push it and go on in
			// the next chunk.
			if !extra {
				stack = append([][8]uint32(nil), stack...)
				extra = true
			}
			total := chunk + 1
			for total&1 == 0 {
				left := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				cv = b3Parent(&left, &cv, 0)
				total >>= 1
			}
			stack = append(stack, cv)
			chunk, cv, b = chunk+1, b3IV, 0
		}
	}
	for i := len(stack) - 1; i >= 0; i-- {
		flags := uint32(0)
		if i == 0 {
			flags = b3Root
		}
		out = b3Parent(&stack[i], &out, flags)
	}
	return b3Bytes(&out)
}

func b3Bytes(w *[8]uint32) hash.Hash {
	var out hash.Hash
	for i, v := range w {
		binary.LittleEndian.PutUint32(out[4*i:], v)
	}
	return out
}
