package policy

import (
	"errors"
	"testing"
)

// TestVerifiedNeverChangesAResult checks that a remembered pass is reused
// only for the same policy, data and tid, that failures are never
// remembered, and that the cheap checks still run on a remembered witness.
func TestVerifiedNeverChangesAResult(t *testing.T) {
	h := sha(t)
	alice := signer(t, "alice")
	pol := SingleKey(AlgEd25519, alice.Public())
	addr := Address(h, pol)
	tid := h([]byte("some tid"))
	v := NewVerified(4)

	w := SingleKeyWitness(alice, tid)
	for i := 0; i < 2; i++ {
		if err := Default().CheckCached(h, allowAll, w, addr, tid, v); err != nil {
			t.Fatalf("pass %d: valid witness refused: %v", i, err)
		}
	}
	if v.Len() != 1 {
		t.Fatalf("remembered %d, want 1", v.Len())
	}

	// The same witness for another tid is still a replay.
	other := h([]byte("other tid"))
	if err := Default().CheckCached(h, allowAll, w, addr, other, v); !errors.Is(err, ErrUnsatisfied) {
		t.Fatalf("replay: want ErrUnsatisfied, got %v", err)
	}
	// A bad signature is refused every time and never remembered.
	bad := w
	bad.Data = append([]byte(nil), w.Data...)
	bad.Data[len(bad.Data)-1] ^= 1
	for i := 0; i < 2; i++ {
		if err := Default().CheckCached(h, allowAll, bad, addr, tid, v); !errors.Is(err, ErrUnsatisfied) {
			t.Fatalf("pass %d: tampered witness: want ErrUnsatisfied, got %v", i, err)
		}
	}
	if v.Len() != 1 {
		t.Fatalf("remembered %d after failures, want 1", v.Len())
	}
	// A remembered witness still has to open the address and fit the rules.
	bob := Address(h, SingleKey(AlgEd25519, signer(t, "bob").Public()))
	if err := Default().CheckCached(h, allowAll, w, bob, tid, v); !errors.Is(err, ErrAddressMismatch) {
		t.Fatalf("want ErrAddressMismatch, got %v", err)
	}
	if err := Default().CheckCached(h, rules{}, w, addr, tid, v); err == nil {
		t.Fatal("a remembered witness passed rules that allow nothing")
	}
	small := allowAll
	small.max = 1
	if err := Default().CheckCached(h, small, w, addr, tid, v); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("want ErrTooLarge, got %v", err)
	}
}

// TestVerifiedForgetsOldest checks the ring's bound.
func TestVerifiedForgetsOldest(t *testing.T) {
	h := sha(t)
	alice := signer(t, "alice")
	addr := Address(h, SingleKey(AlgEd25519, alice.Public()))
	v := NewVerified(2)
	for _, s := range []string{"a", "b", "c", "c"} {
		tid := h([]byte(s))
		if err := Default().CheckCached(h, allowAll, SingleKeyWitness(alice, tid), addr, tid, v); err != nil {
			t.Fatal(err)
		}
	}
	if v.Len() != 2 {
		t.Fatalf("remembered %d, want 2", v.Len())
	}
	if v.has(verifiedKey(SingleKeyWitness(alice, h([]byte("a"))), h([]byte("a")))) {
		t.Fatal("oldest witness not forgotten")
	}
}
