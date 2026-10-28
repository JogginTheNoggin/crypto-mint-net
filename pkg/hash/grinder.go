package hash

import (
	"encoding"
	gohash "hash"

	"github.com/zeebo/blake3"
)

// Grinder hashes a fixed prefix followed by a varying suffix, many times
// over: the inner loop of building an entry (the prefix is the tag, the
// parent and the tids, the suffix the nonce) and of grinding a layer (the
// entry hash, then the nonce). It gives the same hash as Func(prefix...,
// suffix), without allocating, and, when the prefix fills at least one
// block, without hashing the prefix again for every suffix: it keeps the
// digest's state after the prefix (its midstate) and restores it.
//
// A Grinder is not safe for concurrent use; give each worker its own.
type Grinder struct {
	d      gohash.Hash
	prefix []byte // written for every suffix when there is no midstate
	// restore puts the digest back to its state after the prefix, or is
	// nil when the prefix is short and simply written again.
	restore func() error
	out     [Size]byte
}

// NewGrinder returns a Grinder for hash function id and the given prefix.
func NewGrinder(id ID, prefix ...[]byte) (*Grinder, error) {
	e, ok := known[id]
	if !ok {
		return nil, ErrUnknown
	}
	g := &Grinder{d: e.new()}
	n := 0
	for _, p := range prefix {
		n += len(p)
	}
	if n < g.d.BlockSize() {
		for _, p := range prefix {
			g.prefix = append(g.prefix, p...)
		}
		return g, nil
	}
	for _, p := range prefix {
		g.d.Write(p)
	}
	switch d := g.d.(type) {
	case *blake3.Hasher:
		// Its state is plain values: a copy restores it. The copy includes
		// its 8 KB buffer, which bounds BLAKE3 at about 750 ns a try
		// whatever the prefix.
		saved := d.Clone()
		g.restore = func() error { *d = *saved; return nil }
	case interface {
		encoding.BinaryMarshaler
		encoding.BinaryUnmarshaler
	}:
		state, err := d.MarshalBinary()
		if err != nil {
			return nil, err
		}
		g.restore = func() error { return d.UnmarshalBinary(state) }
	default:
		g.d.Reset()
		for _, p := range prefix {
			g.prefix = append(g.prefix, p...)
		}
	}
	return g, nil
}

// Hash returns the hash of the prefix followed by suffix.
func (g *Grinder) Hash(suffix []byte) Hash {
	if g.restore != nil {
		if err := g.restore(); err != nil {
			panic(err) // unreachable: the state was made by this digest
		}
	} else {
		g.d.Reset()
		g.d.Write(g.prefix)
	}
	g.d.Write(suffix)
	g.d.Sum(g.out[:0])
	return Hash(g.out)
}
