// Package layer implements layers: unsigned work attached to one entry hash.
//
//	Layer { entry : hash, nonce : u64 }
//	hash(layer)  = H(entry ∥ nonce)        // nonce as 8 big-endian bytes
//	strength(L)  = leading zero bits of hash(layer)
//	credit(L)    = 2^strength(L)
//
// A layer needs only the entry hash to be made or checked. There is no
// signature, no role and no declared difficulty: the verifier counts the
// leading zeros itself, and lucky strength counts in full.
package layer

import (
	"encoding/binary"
	"fmt"
	"sync"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/encoding"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/weight"
)

// Size is the encoded size of a layer.
const Size = hash.Size + 8

// Layer is work on one entry hash.
type Layer struct {
	Entry hash.Hash `json:"entry"`
	Nonce uint64    `json:"nonce"`
}

// Hash returns H(entry ∥ nonce).
func (l Layer) Hash(h hash.Func) hash.Hash {
	return h(l.Entry[:], encoding.NewWriter(8).U64(l.Nonce).Bytes())
}

// Strength returns the leading zero bits of the layer hash.
func (l Layer) Strength(h hash.Func) int { return l.Hash(h).Strength() }

// Credit returns 2^strength.
func (l Layer) Credit(h hash.Func) weight.Weight { return weight.Credit(l.Strength(h)) }

// Encode returns entry ∥ nonce.
func (l Layer) Encode() []byte {
	return encoding.NewWriter(Size).Raw(l.Entry[:]).U64(l.Nonce).Bytes()
}

// EncodeTo writes the layer to w.
func (l Layer) EncodeTo(w *encoding.Writer) { w.Raw(l.Entry[:]).U64(l.Nonce) }

// DecodeFrom reads one layer from r.
func DecodeFrom(r *encoding.Reader) Layer {
	var l Layer
	r.Into(l.Entry[:])
	l.Nonce = r.U64()
	return l
}

// Decode reads a layer written by Encode.
func Decode(b []byte) (Layer, error) {
	r := encoding.NewReader(b)
	l := DecodeFrom(r)
	if err := r.Finish(); err != nil {
		return l, fmt.Errorf("layer: decode: %w", err)
	}
	return l, nil
}

// Grind tries tries nonces from start against entry and returns the
// strongest layer found and its strength. A worker may try nonces freely and
// keep the best.
func Grind(h hash.Func, entry hash.Hash, start, tries uint64) (Layer, int) {
	best := Layer{Entry: entry, Nonce: start}
	bestStrength := -1
	buf := make([]byte, 8)
	for i := uint64(0); i < tries; i++ {
		n := start + i
		binary.BigEndian.PutUint64(buf, n)
		s := h(entry[:], buf).Strength()
		if s > bestStrength {
			best, bestStrength = Layer{Entry: entry, Nonce: n}, s
		}
	}
	return best, bestStrength
}

// GrindTo tries nonces from start until a layer reaches strength min, or
// maxTries nonces have been tried. It reports whether one was found.
func GrindTo(h hash.Func, entry hash.Hash, start uint64, min int, maxTries uint64) (Layer, bool) {
	for i := uint64(0); i < maxTries; i++ {
		l := Layer{Entry: entry, Nonce: start + i}
		if l.Strength(h) >= min {
			return l, true
		}
	}
	return Layer{}, false
}

// GrindHash is Grind for the hash function id, with the same result: the
// strongest layer of the nonces tried, the lowest nonce among equals. It
// hashes with a hash.Grinder, which allocates nothing per try
// (ORANGE-006), and splits the nonces between workers goroutines, each
// on its own range, when workers is above 1 (ORANGE-007).
func GrindHash(id hash.ID, entry hash.Hash, start, tries uint64, workers int) (Layer, int, error) {
	if workers < 1 {
		workers = 1
	}
	if uint64(workers) > tries {
		workers = int(max(tries, 1))
	}
	type result struct {
		l   Layer
		s   int
		err error
	}
	res := make([]result, workers)
	per := tries / uint64(workers)
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		from := start + uint64(w)*per
		n := per
		if w == workers-1 {
			n = tries - uint64(w)*per
		}
		wg.Add(1)
		go func(w int, from, n uint64) {
			defer wg.Done()
			g, err := hash.NewGrinder(id, entry[:])
			if err != nil {
				res[w].err = err
				return
			}
			best, bestStrength := Layer{Entry: entry, Nonce: from}, -1
			var buf [8]byte
			for i := uint64(0); i < n; i++ {
				nonce := from + i
				binary.BigEndian.PutUint64(buf[:], nonce)
				if s := g.Hash(buf[:]).Strength(); s > bestStrength {
					best, bestStrength = Layer{Entry: entry, Nonce: nonce}, s
				}
			}
			res[w] = result{best, bestStrength, nil}
		}(w, from, n)
	}
	wg.Wait()
	// Ranges are in nonce order, so keeping the first of equal strengths
	// keeps the lowest nonce, as Grind does.
	best := res[0]
	for _, r := range res {
		if r.err != nil {
			return Layer{}, -1, r.err
		}
		if r.s > best.s {
			best = r
		}
	}
	return best.l, best.s, nil
}
