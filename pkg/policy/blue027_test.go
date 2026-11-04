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

// RED-077, RED-079, D169, D170: there is no network-wide size limit, so
// only the series' witness ceiling decides whether a policy can be spent,
// and every set of signers it allows must fit (Red's condition 5).
func TestCheckSpendable(t *testing.T) {
	reg := Default()
	const reserve = 128 * 1024
	cases := []struct {
		name string
		p    Policy
		max  int
		ok   bool
	}{
		// What the old 64 KiB caps refused now fits the reserve ceiling.
		{"16 of 16 ML-DSA-87", ThresholdParams{K: 16, Keys: keysOf(t, AlgMLDSA87, 16)}.Policy(), reserve, true},
		{"16 of 16 SLH-DSA-128s", ThresholdParams{K: 16, Keys: keysOf(t, AlgSLHDSASHA2128s, 16)}.Policy(), reserve, true},
		{"4 of 4 SLH-DSA-256s", ThresholdParams{K: 4, Keys: keysOf(t, AlgSLHDSASHA2256s, 4)}.Policy(), reserve, true},
		{"5 of 5 SLH-DSA-256s", ThresholdParams{K: 5, Keys: keysOf(t, AlgSLHDSASHA2256s, 5)}.Policy(), reserve, false},
		// Under a 64 KiB ceiling the old limits come back from the series.
		{"14 of 16 ML-DSA-87, 64 KiB", ThresholdParams{K: 14, Keys: keysOf(t, AlgMLDSA87, 16)}.Policy(), 64 * 1024, false},
		{"8 of 8 SLH-DSA-128s, 64 KiB", ThresholdParams{K: 8, Keys: keysOf(t, AlgSLHDSASHA2128s, 8)}.Policy(), 70 * 1024, true},
		// The worst case counts: 1 of an Ed25519 and an SLH-DSA-256s key
		// fits with Ed25519 but not with SLH-DSA-256s.
		{"1 of Ed25519 and SLH-DSA-256s, 8 KB", ThresholdParams{K: 1, Keys: append(keysOf(t, AlgEd25519, 1), keysOf(t, AlgSLHDSASHA2256s, 1)...)}.Policy(), 8000, false},
		{"1 of Ed25519 and SLH-DSA-256s, 64 KB", ThresholdParams{K: 1, Keys: append(keysOf(t, AlgEd25519, 1), keysOf(t, AlgSLHDSASHA2256s, 1)...)}.Policy(), 64000, true},
		{"all of two halves", AllOf(ThresholdParams{K: 8, Keys: keysOf(t, AlgSLHDSASHA2128s, 8)}.Policy(), ThresholdParams{K: 8, Keys: keysOf(t, AlgSLHDSASHA2128s, 9)[1:]}.Policy()), reserve, true},
		{"all of two halves, 64 KiB", AllOf(ThresholdParams{K: 8, Keys: keysOf(t, AlgSLHDSASHA2128s, 8)}.Policy(), ThresholdParams{K: 8, Keys: keysOf(t, AlgSLHDSASHA2128s, 9)[1:]}.Policy()), 64 * 1024, false},
		// A hashlock's worst case is a preimage of MaxPreimageSize.
		{"ed25519 and a hashlock, 1 KB", AllOf(SingleKey(AlgEd25519, keysOf(t, AlgEd25519, 1)[0].Public), HashlockParams{Hash: 1}.Policy()), 1000, false},
		{"ed25519 and a hashlock, 2 KB", AllOf(SingleKey(AlgEd25519, keysOf(t, AlgEd25519, 1)[0].Public), HashlockParams{Hash: 1}.Policy()), 2000, true},
		// No ceiling: every policy fits.
		{"16 of 16 SLH-DSA-256s, no ceiling", ThresholdParams{K: 16, Keys: keysOf(t, AlgSLHDSASHA2256s, 16)}.Policy(), 0, true},
	}
	for _, c := range cases {
		err := reg.CheckPolicy(ceiling{max: c.max}, c.p)
		if c.ok != (err == nil) {
			t.Errorf("%s: %v", c.name, err)
		}
		if !c.ok && !errors.Is(err, ErrUnspendable) {
			t.Errorf("%s: %v is not ErrUnspendable", c.name, err)
		}
	}
	err := reg.CheckPolicy(ceiling{max: reserve}, ThresholdParams{K: 5, Keys: keysOf(t, AlgSLHDSASHA2256s, 6)}.Policy())
	if err == nil || !strings.Contains(err.Error(), "at most 4") || !strings.Contains(err.Error(), "smallest") {
		t.Fatalf("the error names how many fit: %v", err)
	}
	err = reg.CheckPolicy(ceiling{max: 8000}, ThresholdParams{K: 1, Keys: append(keysOf(t, AlgEd25519, 1), keysOf(t, AlgSLHDSASHA2256s, 1)...)}.Policy())
	if err == nil || !strings.Contains(err.Error(), "some sets of signers") {
		t.Fatalf("the error says some signers could not spend: %v", err)
	}
}

// MaxK is Red's table for the reserve ceiling, and every algorithm fits 16
// with no ceiling.
func TestMaxK(t *testing.T) {
	want := map[AlgID]int{AlgEd25519: 16, AlgMLDSA65: 16, AlgMLDSA87: 16, AlgSLHDSASHA2128s: 16, AlgSLHDSASHA2256s: 4}
	for id, k := range want {
		a, _ := Default().Alg(id)
		if got := MaxK(a, 128*1024); got != k {
			t.Errorf("%s: %d, want %d", a.Name(), got, k)
		}
		if got := MaxK(a, 0); got != MaxThresholdKeys {
			t.Errorf("%s with no ceiling: %d", a.Name(), got)
		}
	}
	a, _ := Default().Alg(AlgMLDSA87)
	if got := MaxK(a, 115754); got != 16 {
		t.Errorf("16 ML-DSA-87 is 115,754 bytes, MaxK %d", got)
	}
	if got := MaxK(a, 115753); got != 15 {
		t.Errorf("one byte less: %d", got)
	}
}
