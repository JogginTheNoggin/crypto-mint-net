// Package weight implements the arbitrary-precision quantity that decides
// which ledger is selected.
//
// The credit of a hash is 2 to the power of its strength. A strength can be
// as large as 256, so credits and their sums do not fit in a machine integer;
// Weight is backed by math/big and is immutable: every operation returns a
// new value and never changes its operands.
package weight

import (
	"fmt"
	"math/big"
)

// Weight is a non-negative amount of credit. The zero value is 0.
type Weight struct {
	v *big.Int
}

// Zero is weight 0.
var Zero = Weight{}

func (w Weight) int() *big.Int {
	if w.v == nil {
		return new(big.Int)
	}
	return w.v
}

// FromUint64 returns a weight equal to n.
func FromUint64(n uint64) Weight {
	return Weight{v: new(big.Int).SetUint64(n)}
}

// Credit returns 2^strength, the weight one hash of that strength adds.
func Credit(strength int) Weight {
	if strength < 0 {
		panic("weight: negative strength")
	}
	return Weight{v: new(big.Int).Lsh(big.NewInt(1), uint(strength))}
}

// Add returns w + o.
func (w Weight) Add(o Weight) Weight {
	return Weight{v: new(big.Int).Add(w.int(), o.int())}
}

// Sum adds up weights in one value that grows in place, so adding n
// weights allocates once instead of n times (ORANGE-008). The zero value
// is an empty sum. Weight hands the result over and empties the sum.
type Sum struct{ v *big.Int }

// Add adds w to the sum.
func (s *Sum) Add(w Weight) {
	if s.v == nil {
		s.v = new(big.Int)
	}
	if w.v != nil {
		s.v.Add(s.v, w.v)
	}
}

// Weight returns the sum and empties s. The result is never changed by s
// again, so it is as immutable as any Weight.
func (s *Sum) Weight() Weight {
	w := Weight{v: s.v}
	s.v = nil
	return w
}

// Sub returns w - o. It panics if the result would be negative, since a
// weight is never negative; callers subtract only a part from its whole.
func (w Weight) Sub(o Weight) Weight {
	r := new(big.Int).Sub(w.int(), o.int())
	if r.Sign() < 0 {
		panic("weight: negative result")
	}
	return Weight{v: r}
}

// Cmp returns -1, 0 or +1 as w is less than, equal to or greater than o.
func (w Weight) Cmp(o Weight) int { return w.int().Cmp(o.int()) }

// Greater reports whether w > o.
func (w Weight) Greater(o Weight) bool { return w.Cmp(o) > 0 }

// IsZero reports whether w is 0.
func (w Weight) IsZero() bool { return w.v == nil || w.v.Sign() == 0 }

// String returns the decimal form.
func (w Weight) String() string { return w.int().String() }

// Bytes returns the minimal big-endian bytes of w (empty for 0). This is the
// form weights take on the wire.
func (w Weight) Bytes() []byte { return w.int().Bytes() }

// FromBytes decodes big-endian bytes. Leading zero bytes are refused, so
// each weight has exactly one encoding.
func FromBytes(b []byte) (Weight, error) {
	if len(b) > 0 && b[0] == 0 {
		return Zero, fmt.Errorf("weight: non-minimal encoding")
	}
	return Weight{v: new(big.Int).SetBytes(b)}, nil
}

// Uint64 returns w as a uint64 and whether it fits.
func (w Weight) Uint64() (uint64, bool) {
	i := w.int()
	return i.Uint64(), i.IsUint64()
}

// MarshalText encodes w in decimal.
func (w Weight) MarshalText() ([]byte, error) { return []byte(w.String()), nil }

// UnmarshalText decodes a decimal weight.
func (w *Weight) UnmarshalText(text []byte) error {
	v, ok := new(big.Int).SetString(string(text), 10)
	if !ok || v.Sign() < 0 {
		return fmt.Errorf("weight: invalid %q", text)
	}
	w.v = v
	return nil
}
