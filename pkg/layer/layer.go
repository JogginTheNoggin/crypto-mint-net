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
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

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
	if tries == 0 {
		return Layer{Entry: entry, Nonce: start}, -1, nil
	}
	return GrindWith(context.Background(), hash.GrinderFor(id), entry, start, tries, workers, nil)
}

// checkEvery is how many nonces a worker tries between looks at its
// context and additions to the try counter.
const checkEvery = 1 << 12

// GrindWith is GrindHash with any hasher (package grind chooses one per
// device and hash, decision D109), a context and a counter. tries 0 means
// no limit: the search runs until ctx is done. It adds the nonces tried
// to tried, if not nil, as it goes, so a caller can report a rate. When
// ctx ends the search early it returns the strongest layer found so far
// (strength -1 if none) with ctx's error; with every nonce tried the
// result is GrindHash's, whatever the number of workers.
func GrindWith(ctx context.Context, newHasher hash.NewHasher, entry hash.Hash, start, tries uint64, workers int, tried *atomic.Uint64) (Layer, int, error) {
	return GrindWithKeep(ctx, newHasher, entry, start, tries, workers, tried, nil)
}

// Keep receives layers a search found besides its strongest (ORANGE-010,
// decision D118): every range of checkEvery nonces whose strongest layer
// reaches Floor hands that layer to Func, so a node can send more of the
// work it did. Func is called from the search's workers at once and must
// be safe for that. The layer returned at the end is passed too.
type Keep struct {
	Floor int
	Func  func(Layer, int)
}

// GrindWithKeep is GrindWith that also hands layers to keep, if not nil.
func GrindWithKeep(ctx context.Context, newHasher hash.NewHasher, entry hash.Hash, start, tries uint64, workers int, tried *atomic.Uint64, keep *Keep) (Layer, int, error) {
	return grind(ctx, entry, start, tries, workers, tried, keep, func() (Search, error) {
		g, err := newHasher(entry[:])
		if err != nil {
			return nil, err
		}
		var buf [8]byte
		return func(_ hash.Hash, from, n uint64) (uint64, int) {
			bestNonce, best := from, -1
			for nonce := from; nonce < from+n; nonce++ {
				binary.BigEndian.PutUint64(buf[:], nonce)
				if s := g.Hash(buf[:]).Strength(); s > best {
					bestNonce, best = nonce, s
				}
			}
			return bestNonce, best
		}, nil
	})
}

// Search tries the layers on entry of nonces [from, from+n) and returns
// the strongest, the lowest nonce among equals, with its strength (-1
// when n is 0). A grinder that hashes many nonces at once, such as with
// SIMD lanes (ORANGE-012), searches a range rather than hashing one try.
type Search func(entry hash.Hash, from, n uint64) (nonce uint64, strength int)

// GrindSearch is GrindWith with a Search, which each worker calls on
// ranges of checkEvery nonces.
func GrindSearch(ctx context.Context, search Search, entry hash.Hash, start, tries uint64, workers int, tried *atomic.Uint64) (Layer, int, error) {
	return GrindSearchKeep(ctx, search, entry, start, tries, workers, tried, nil)
}

// GrindSearchKeep is GrindSearch that also hands layers to keep, if not nil.
func GrindSearchKeep(ctx context.Context, search Search, entry hash.Hash, start, tries uint64, workers int, tried *atomic.Uint64, keep *Keep) (Layer, int, error) {
	return grind(ctx, entry, start, tries, workers, tried, keep, func() (Search, error) { return search, nil })
}

// grind splits the nonces between workers, each searching its own range
// with the Search newSearch makes for it.
func grind(ctx context.Context, entry hash.Hash, start, tries uint64, workers int, tried *atomic.Uint64, keep *Keep, newSearch func() (Search, error)) (Layer, int, error) {
	if workers < 1 {
		workers = 1
	}
	span := tries
	if tries == 0 {
		span = ^uint64(0) - start // as many as there are
	}
	if uint64(workers) > span {
		workers = int(max(span, 1))
	}
	type result struct {
		l   Layer
		s   int
		err error
	}
	res := make([]result, workers)
	per := span / uint64(workers)
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		from := start + uint64(w)*per
		n := per
		if w == workers-1 {
			n = span - uint64(w)*per
		}
		wg.Add(1)
		go func(w int, from, n uint64) {
			defer wg.Done()
			search, err := newSearch()
			if err != nil {
				res[w] = result{err: err, s: -1}
				return
			}
			best, bestStrength := Layer{Entry: entry, Nonce: from}, -1
			for done := uint64(0); done < n; {
				chunk := min(n-done, checkEvery)
				nonce, s := search(entry, from+done, chunk)
				if s > bestStrength {
					best, bestStrength = Layer{Entry: entry, Nonce: nonce}, s
				}
				if keep != nil && s >= keep.Floor && s >= 0 {
					keep.Func(Layer{Entry: entry, Nonce: nonce}, s)
				}
				done += chunk
				if tried != nil {
					tried.Add(chunk)
				}
				if err := ctx.Err(); err != nil {
					res[w] = result{best, bestStrength, err}
					return
				}
			}
			res[w] = result{best, bestStrength, nil}
		}(w, from, n)
	}
	wg.Wait()
	// Ranges are in nonce order, so keeping the first of equal strengths
	// keeps the lowest nonce, as Grind does.
	best := result{l: Layer{Entry: entry, Nonce: start}, s: -1}
	var stopped error
	for _, r := range res {
		if r.err != nil && !errors.Is(r.err, ctx.Err()) {
			return Layer{}, -1, r.err
		}
		if r.err != nil {
			stopped = r.err
		}
		if r.s > best.s {
			best = r
		}
	}
	return best.l, best.s, stopped
}
