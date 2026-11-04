package policy

import (
	"errors"
	"strings"
	"testing"
)

type ceiling struct {
	anyRules
	max int
}

func (c ceiling) MaxWitnessSize() int { return c.max }

func keysOf(t *testing.T, alg AlgID, n int) []Key {
	t.Helper()
	a, ok := Default().Alg(alg)
	if !ok {
		t.Fatalf("alg %d", alg)
	}
	keys := make([]Key, n)
	for i := range keys {
		pub := make([]byte, a.PublicKeySize())
		pub[0], pub[1] = byte(i), byte(alg)
		keys[i] = Key{Alg: alg, Public: pub}
	}
	return keys
}

// RED-077, RED-079, D169: a policy whose smallest witness cannot decode is
// refused where the address is made; one that fits is not.
func TestCheckSpendable(t *testing.T) {
	reg := Default()
	cases := []struct {
		name string
		p    Policy
		ok   bool
	}{
		{"14 of 16 ML-DSA-87", ThresholdParams{K: 14, Keys: keysOf(t, AlgMLDSA87, 16)}.Policy(), true},
		{"15 of 16 ML-DSA-87", ThresholdParams{K: 15, Keys: keysOf(t, AlgMLDSA87, 16)}.Policy(), false},
		{"16 of 16 ML-DSA-65", ThresholdParams{K: 16, Keys: keysOf(t, AlgMLDSA65, 16)}.Policy(), true},
		{"8 of 10 SLH-DSA", ThresholdParams{K: 8, Keys: keysOf(t, AlgSLHDSASHA2128s, 10)}.Policy(), true},
		{"9 of 10 SLH-DSA", ThresholdParams{K: 9, Keys: keysOf(t, AlgSLHDSASHA2128s, 10)}.Policy(), false},
		// 6 SLH-DSA and 4 ML-DSA-87, all needed: over the limit.
		{"10 of a mix", ThresholdParams{K: 10, Keys: append(keysOf(t, AlgSLHDSASHA2128s, 6), keysOf(t, AlgMLDSA87, 4)...)}.Policy(), false},
		// 9 of the mix fits with the smallest signatures (4 ML-DSA-87 and
		// 5 SLH-DSA would not, but the owner chooses which keys sign).
		{"9 of SLH-DSA and Ed25519", ThresholdParams{K: 9, Keys: append(keysOf(t, AlgSLHDSASHA2128s, 8), keysOf(t, AlgEd25519, 1)...)}.Policy(), true},
		{"all of two halves", AllOf(ThresholdParams{K: 8, Keys: keysOf(t, AlgSLHDSASHA2128s, 8)}.Policy(), ThresholdParams{K: 8, Keys: keysOf(t, AlgSLHDSASHA2128s, 9)[1:]}.Policy()), false},
		{"one of a bad and a good", AnyOf(ThresholdParams{K: 9, Keys: keysOf(t, AlgSLHDSASHA2128s, 9)}.Policy(), SingleKey(AlgEd25519, keysOf(t, AlgEd25519, 1)[0].Public)), true},
	}
	for _, c := range cases {
		err := reg.CheckPolicy(anyRules{}, c.p)
		if c.ok != (err == nil) {
			t.Errorf("%s: %v", c.name, err)
		}
		if !c.ok && !errors.Is(err, ErrUnspendable) {
			t.Errorf("%s: %v is not ErrUnspendable", c.name, err)
		}
	}
	err := reg.CheckPolicy(anyRules{}, ThresholdParams{K: 15, Keys: keysOf(t, AlgMLDSA87, 16)}.Policy())
	if err == nil || !strings.Contains(err.Error(), "at most 14") {
		t.Fatalf("the error names how many fit: %v", err)
	}
	// The series' witness ceiling counts too.
	p := ThresholdParams{K: 2, Keys: keysOf(t, AlgMLDSA87, 2)}.Policy()
	if err := reg.CheckPolicy(ceiling{max: 8000}, p); !errors.Is(err, ErrUnspendable) {
		t.Fatalf("over max_witness: %v", err)
	}
	if err := reg.CheckPolicy(ceiling{max: 20000}, p); err != nil {
		t.Fatal(err)
	}
}
