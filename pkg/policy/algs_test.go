package policy

import (
	"bytes"
	"crypto/elliptic"
	"crypto/sha256"
	"errors"
	"math/big"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
)

// everything allows every registered type and algorithm.
func everything() rules {
	r := rules{types: map[TypeID]bool{}, algs: map[AlgID]bool{}}
	for _, t := range Default().Types() {
		r.types[t.ID()] = true
	}
	for _, a := range Default().Algs() {
		r.algs[a.ID()] = true
	}
	return r
}

func seedOf(label string) []byte { s := sha256.Sum256([]byte(label)); return s[:] }

func newSigner(t *testing.T, alg AlgID, label string) Signer {
	t.Helper()
	s, err := NewSigner(alg, seedOf(label))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// Every algorithm: keys derive the same way twice, sizes match, a
// signature verifies for its message only, and a changed byte breaks it.
func TestEveryAlgorithm(t *testing.T) {
	h := sha(t)
	tid := h([]byte("tx"))
	msg := Message(tid)
	if len(Default().Algs()) != 9 {
		t.Fatalf("%d algorithms", len(Default().Algs()))
	}
	for _, a := range Default().Algs() {
		t.Run(a.Name(), func(t *testing.T) {
			s := newSigner(t, a.ID(), "alice")
			if s.Alg() != a.ID() {
				t.Fatalf("signer alg %d", s.Alg())
			}
			if again := newSigner(t, a.ID(), "alice"); !bytes.Equal(again.Public(), s.Public()) {
				t.Fatal("key derivation is not deterministic")
			}
			if bob := newSigner(t, a.ID(), "bob"); bytes.Equal(bob.Public(), s.Public()) {
				t.Fatal("two seeds give one key")
			}
			if len(s.Public()) != a.PublicKeySize() {
				t.Fatalf("public key %d bytes, want %d", len(s.Public()), a.PublicKeySize())
			}
			sig := s.Sign(msg)
			if len(sig) != a.SignatureSize() {
				t.Fatalf("signature %d bytes, want %d", len(sig), a.SignatureSize())
			}
			if !a.Verify(s.Public(), msg, sig) {
				t.Fatal("valid signature refused")
			}
			if a.Verify(s.Public(), Message(h([]byte("other"))), sig) {
				t.Fatal("signature verified for another message")
			}
			for _, i := range []int{0, len(sig) / 2, len(sig) - 1} {
				bad := append([]byte(nil), sig...)
				bad[i] ^= 1
				if a.Verify(s.Public(), msg, bad) {
					t.Fatalf("signature with byte %d changed verified", i)
				}
			}
			if a.Verify(s.Public(), msg, sig[:len(sig)-1]) || a.Verify(s.Public()[1:], msg, sig) {
				t.Fatal("wrong sizes verified")
			}
			// Through a witness, as consensus checks it.
			w := SingleKeyWitness(s, tid)
			addr := Address(h, w.Policy)
			if err := Default().Check(h, everything(), w, addr, tid); err != nil {
				t.Fatalf("witness refused: %v", err)
			}
			only := rules{types: map[TypeID]bool{TypeThreshold: true}, algs: map[AlgID]bool{}}
			if err := Default().Check(h, only, w, addr, tid); !errors.Is(err, ErrAlgNotAllowed) {
				t.Fatalf("an algorithm the series does not allow: %v", err)
			}
		})
	}
}

// ECDSA signatures with s in the upper half are the other encoding of a
// valid signature, and are refused, so each signature has one encoding.
func TestECDSAHighSRefused(t *testing.T) {
	msg := Message(hash.Hash{1})
	for alg, curve := range map[AlgID]elliptic.Curve{AlgECDSAP256: elliptic.P256(), AlgECDSAP384: elliptic.P384(), AlgECDSAP521: elliptic.P521()} {
		a := nistAlgs[alg]
		s := newSigner(t, alg, "alice")
		sig := s.Sign(msg)
		n := curve.Params().N
		hi := new(big.Int).Sub(n, new(big.Int).SetBytes(sig[a.size:]))
		flipped := append([]byte(nil), sig...)
		hi.FillBytes(flipped[a.size:])
		if a.Verify(s.Public(), msg, flipped) {
			t.Errorf("%s: high-s signature accepted", a.name)
		}
	}
	// secp256k1: the same.
	s := newSigner(t, AlgECDSASecp256k1, "alice")
	sig := s.Sign(msg)
	n, _ := new(big.Int).SetString("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFEBAAEDCE6AF48A03BBFD25E8CD0364141", 16)
	hi := new(big.Int).Sub(n, new(big.Int).SetBytes(sig[32:]))
	hi.FillBytes(sig[32:])
	if (k1Alg{}).Verify(s.Public(), msg, sig) {
		t.Error("secp256k1: high-s signature accepted")
	}
}

// NewSigner refuses what it cannot sign with.
func TestNewSignerErrors(t *testing.T) {
	if _, err := NewSigner(999, seedOf("x")); !errors.Is(err, ErrUnknownSigner) {
		t.Fatalf("unknown algorithm: %v", err)
	}
	if _, err := NewSigner(AlgEd25519, []byte{1}); err == nil {
		t.Fatal("short seed accepted")
	}
}

// Names map back to numbers.
func TestNames(t *testing.T) {
	r := Default()
	for _, a := range r.Algs() {
		if id, ok := r.AlgByName(a.Name()); !ok || id != a.ID() {
			t.Errorf("algorithm %s", a.Name())
		}
	}
	for _, ty := range r.Types() {
		if id, ok := r.TypeByName(ty.Name()); !ok || id != ty.ID() {
			t.Errorf("type %s", ty.Name())
		}
	}
	if _, ok := r.AlgByName("rsa"); ok {
		t.Error("unknown name found")
	}
}
