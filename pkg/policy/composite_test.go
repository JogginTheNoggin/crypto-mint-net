package policy

import (
	"errors"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
)

// keyData signs tid with s under its 1-of-1 policy and returns the policy
// and the witness data.
func keyData(s Signer, tid hash.Hash) (Policy, []byte) {
	w := SingleKeyWitness(s, tid)
	return w.Policy, w.Data
}

func TestHashlock(t *testing.T) {
	h := sha(t)
	tid := h([]byte("tx"))
	for _, id := range hash.IDs() {
		hl, err := NewHashlock(id, []byte("secret"))
		if err != nil {
			t.Fatal(err)
		}
		p := hl.Policy()
		addr := Address(h, p)
		if err := Default().Check(h, everything(), Witness{Policy: p, Data: HashlockData([]byte("secret"))}, addr, tid); err != nil {
			t.Fatalf("%s: preimage refused: %v", id.Name(), err)
		}
		if err := Default().Check(h, everything(), Witness{Policy: p, Data: HashlockData([]byte("guess"))}, addr, tid); !errors.Is(err, ErrUnsatisfied) {
			t.Fatalf("%s: wrong preimage: %v", id.Name(), err)
		}
	}
	bad := HashlockParams{Hash: 999}.Policy()
	if err := Default().CheckPolicy(everything(), bad); !errors.Is(err, ErrBadParams) {
		t.Fatalf("unknown hash: %v", err)
	}
}

// A hashlock beside a key: the shape of a hash time-locked contract's
// claim branch, here without the time lock.
func TestAllOfKeyAndHashlock(t *testing.T) {
	h := sha(t)
	tid := h([]byte("tx"))
	alice := newSigner(t, AlgSchnorrSecp256k1, "alice")
	kp, kd := keyData(alice, tid)
	hl, _ := NewHashlock(hash.SHA256, []byte("secret"))
	p := AllOf(kp, hl.Policy())
	addr := Address(h, p)
	data := KOfPoliciesData{Parts: []Part{{0, kd}, {1, HashlockData([]byte("secret"))}}}.Encode()
	if err := Default().Check(h, everything(), Witness{Policy: p, Data: data}, addr, tid); err != nil {
		t.Fatalf("both parts refused: %v", err)
	}
	only := KOfPoliciesData{Parts: []Part{{1, HashlockData([]byte("secret"))}}}.Encode()
	if err := Default().Check(h, everything(), Witness{Policy: p, Data: only}, addr, tid); !errors.Is(err, ErrBadData) {
		t.Fatalf("one part of all: %v", err)
	}
	// The key part is bound to the tid: replaying it on another fails.
	other := h([]byte("other"))
	if err := Default().Check(h, everything(), Witness{Policy: p, Data: data}, addr, other); !errors.Is(err, ErrUnsatisfied) {
		t.Fatalf("replay: %v", err)
	}
	// Every policy inside must be allowed by the series.
	noHashlock := everything()
	delete(noHashlock.types, TypeHashlock)
	if err := Default().Check(h, noHashlock, Witness{Policy: p, Data: data}, addr, tid); !errors.Is(err, ErrTypeNotAllowed) {
		t.Fatalf("inner type not allowed: %v", err)
	}
	noSchnorr := everything()
	delete(noSchnorr.algs, AlgSchnorrSecp256k1)
	if err := Default().Check(h, noSchnorr, Witness{Policy: p, Data: data}, addr, tid); !errors.Is(err, ErrAlgNotAllowed) {
		t.Fatalf("inner algorithm not allowed: %v", err)
	}
}

// Any one of keys of different algorithms: a holder keeps a post-quantum
// key beside an elliptic-curve one.
func TestAnyOfMixedAlgorithms(t *testing.T) {
	h := sha(t)
	tid := h([]byte("tx"))
	ec := newSigner(t, AlgEd25519, "ec")
	pq := newSigner(t, AlgMLDSA65, "pq")
	ecp, ecd := keyData(ec, tid)
	pqp, pqd := keyData(pq, tid)
	p := AnyOf(ecp, pqp)
	addr := Address(h, p)
	for i, d := range [][]byte{
		KOfPoliciesData{Parts: []Part{{0, ecd}}}.Encode(),
		KOfPoliciesData{Parts: []Part{{1, pqd}}}.Encode(),
	} {
		if err := Default().Check(h, everything(), Witness{Policy: p, Data: d}, addr, tid); err != nil {
			t.Fatalf("part %d alone refused: %v", i, err)
		}
	}
	swapped := KOfPoliciesData{Parts: []Part{{0, pqd}}}.Encode()
	if err := Default().Check(h, everything(), Witness{Policy: p, Data: swapped}, addr, tid); err == nil {
		t.Fatal("data for the wrong part accepted")
	}
}

func TestKOfPoliciesShape(t *testing.T) {
	h := sha(t)
	tid := h([]byte("tx"))
	a, _ := keyData(newSigner(t, AlgEd25519, "a"), tid)
	b, _ := keyData(newSigner(t, AlgEd25519, "b"), tid)
	cases := map[string]Policy{
		"k zero":   KOfPoliciesParams{K: 0, Policies: []Policy{a}}.Policy(),
		"k over n": KOfPoliciesParams{K: 3, Policies: []Policy{a, b}}.Policy(),
		"empty":    KOfPoliciesParams{K: 1}.Policy(),
		"repeat":   AnyOf(a, a),
	}
	deep := a
	for i := 0; i <= MaxPolicyDepth; i++ {
		deep = AnyOf(deep, b)
	}
	cases["too deep"] = deep
	for name, p := range cases {
		if err := Default().CheckPolicy(everything(), p); !errors.Is(err, ErrBadParams) {
			t.Errorf("%s: %v", name, err)
		}
	}
	ok := a
	for i := 1; i < MaxPolicyDepth; i++ {
		ok = AnyOf(ok, b)
	}
	if err := Default().CheckPolicy(everything(), AnyOf(ok, b)); err != nil {
		t.Errorf("deepest allowed: %v", err)
	}
	// Parts out of order or repeated are refused.
	p := AnyOf(a, b)
	two := KOfPoliciesParams{K: 2, Policies: []Policy{a, b}}.Policy()
	for name, d := range map[string]KOfPoliciesData{
		"repeat": {Parts: []Part{{0, nil}, {0, nil}}},
		"order":  {Parts: []Part{{1, nil}, {0, nil}}},
		"range":  {Parts: []Part{{5, nil}}},
	} {
		pol := p
		if len(d.Parts) == 2 {
			pol = two
		}
		if err := Default().Check(h, everything(), Witness{Policy: pol, Data: d.Encode()}, Address(h, pol), tid); !errors.Is(err, ErrBadData) {
			t.Errorf("%s: %v", name, err)
		}
	}
}
