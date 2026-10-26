// Package hash defines the 32-byte Hash type, the public number list of hash
// functions a series can name, domain tags, and the strength measure
// (leading zero bits) that every weight in the protocol is built on.
//
// A series names its hash function by number (see Lookup). Every hash in that
// series, from addresses to layers, is computed with the same function, and
// every function in this implementation has a 32-byte output.
package hash

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math/bits"
)

// Size is the output length, in bytes, of every hash function in the list.
const Size = 32

// Hash is a 32-byte hash value.
type Hash [Size]byte

// Zero is the all-zero hash. It is never the output of a real hash in
// practice and is used as "none" where a field is optional.
var Zero Hash

// String returns the full lowercase hex form.
func (h Hash) String() string { return hex.EncodeToString(h[:]) }

// Short returns the first 8 hex digits, the form used in the design
// documents' worked examples.
func (h Hash) Short() string { return hex.EncodeToString(h[:4]) }

// IsZero reports whether h is the all-zero hash.
func (h Hash) IsZero() bool { return h == Zero }

// Compare orders hashes as big-endian numbers from the first byte, the order
// used for the canonical order of a bundle.
func (h Hash) Compare(o Hash) int { return bytes.Compare(h[:], o[:]) }

// Less reports whether h sorts before o.
func (h Hash) Less(o Hash) bool { return h.Compare(o) < 0 }

// Strength returns the number of leading zero bits of h. The verifier
// always computes it; nobody declares it.
func (h Hash) Strength() int {
	for i, b := range h {
		if b != 0 {
			return i*8 + bits.LeadingZeros8(b)
		}
	}
	return Size * 8
}

// MarshalText encodes h as hex, so hashes read well in JSON.
func (h Hash) MarshalText() ([]byte, error) {
	return []byte(h.String()), nil
}

// UnmarshalText decodes a 64-digit hex string.
func (h *Hash) UnmarshalText(text []byte) error {
	p, err := Parse(string(text))
	if err != nil {
		return err
	}
	*h = p
	return nil
}

// Parse decodes a 64-digit hex string.
func Parse(s string) (Hash, error) {
	var h Hash
	b, err := hex.DecodeString(s)
	if err != nil {
		return h, fmt.Errorf("hash: %w", err)
	}
	if len(b) != Size {
		return h, fmt.Errorf("hash: want %d bytes, got %d", Size, len(b))
	}
	copy(h[:], b)
	return h, nil
}

// MustParse is Parse that panics on error, for tests and constants.
func MustParse(s string) Hash {
	h, err := Parse(s)
	if err != nil {
		panic(err)
	}
	return h
}

// ID is a number from the public number list of hash functions.
type ID uint32

// Known hash function numbers. Entries in the list are never changed or
// removed; new functions get new numbers.
const (
	// SHA256 is SHA-256 (FIPS 180-4).
	SHA256 ID = 1
)

// Func hashes the concatenation of its arguments.
type Func func(parts ...[]byte) Hash

// ErrUnknown is returned for a hash number this implementation does not
// know. A series that names one is unreadable at this node.
var ErrUnknown = errors.New("hash: unknown hash function number")

// Lookup returns the hash function for a number.
func Lookup(id ID) (Func, error) {
	switch id {
	case SHA256:
		return sha256Func, nil
	default:
		return nil, fmt.Errorf("%w: %d", ErrUnknown, id)
	}
}

func sha256Func(parts ...[]byte) Hash {
	d := sha256.New()
	for _, p := range parts {
		d.Write(p)
	}
	var h Hash
	d.Sum(h[:0])
	return h
}

// Domain tags. Each kind of hashed object starts with its own tag, so a hash
// of one kind can never be confused with a hash of another.
var (
	TagAddress = []byte("PoO-address")
	TagTx      = []byte("PoO-tx")
	TagEntry   = []byte("PoO-entry")
	TagPool    = []byte("PoO-pool")
	TagSeries  = []byte("PoO-series")
	TagWitness = []byte("PoO-witness-v1")
	TagIssuer  = []byte("PoO-issuer-v1")
)
