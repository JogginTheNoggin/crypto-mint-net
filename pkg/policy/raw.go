package policy

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/mldsa"
	"errors"
	"fmt"

	"github.com/cloudflare/circl/sign/slhdsa"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
)

// NewRawSigner returns the signer for an existing private key in its
// algorithm's usual raw form, as other tools keep it, rather than derived
// from a seed as NewSigner does (D193). So a key made elsewhere signs
// here as the same key:
//   - Ed25519: the 32-byte RFC 8032 seed, or the 64-byte seed ∥ public key;
//   - ECDSA P-256, P-384, P-521: the big-endian scalar (32, 48, 66 bytes);
//   - secp256k1 ECDSA and BIP-340 Schnorr: the 32-byte scalar;
//   - ML-DSA-44/65/87: the 32-byte FIPS 204 seed ξ (the same as NewSigner);
//   - SLH-DSA: the FIPS 205 private key, SK.seed ∥ SK.prf ∥ PK.seed ∥ PK.root.
//
// It is node-side only: what a key signs and how a witness is checked are
// unchanged.
//
// The key is self-tested: it signs a fixed message, and the signature
// must verify under the public key it gives, so a key whose stated public
// half does not match its secret (an SLH-DSA PK.root mistyped or from
// another key) is refused rather than giving an address no signature can
// spend (RED-106).
func NewRawSigner(alg AlgID, raw []byte) (Signer, error) {
	s, err := newRawSigner(alg, raw)
	if err != nil {
		return nil, err
	}
	if err := SelfTest(s); err != nil {
		return nil, err
	}
	return s, nil
}

// SelfTest signs a fixed message with s and checks the signature under
// s's public key with its algorithm's verifier (RED-106).
func SelfTest(s Signer) error {
	a, ok := Default().Alg(s.Alg())
	if !ok {
		return fmt.Errorf("%w: %d", ErrUnknownSigner, s.Alg())
	}
	msg := []byte("poo key self-test")
	if !a.Verify(s.Public(), msg, s.Sign(msg)) {
		return fmt.Errorf("policy: the %s key's public half does not match its private key; it would give an address no signature can spend", a.Name())
	}
	return nil
}

func newRawSigner(alg AlgID, raw []byte) (Signer, error) {
	switch alg {
	case AlgEd25519:
		switch len(raw) {
		case ed25519.SeedSize:
			return NewEd25519Signer(raw)
		case ed25519.PrivateKeySize:
			s, err := NewEd25519Signer(raw[:ed25519.SeedSize])
			if err != nil {
				return nil, err
			}
			if string(s.Public()) != string(raw[ed25519.SeedSize:]) {
				return nil, errors.New("policy: the ed25519 key's public half does not match its seed")
			}
			return s, nil
		}
		return nil, fmt.Errorf("policy: an ed25519 key is %d or %d bytes", ed25519.SeedSize, ed25519.PrivateKeySize)
	case AlgECDSAP256, AlgECDSAP384, AlgECDSAP521:
		a := nistAlgs[alg]
		if len(raw) != a.size {
			return nil, fmt.Errorf("policy: a %s key is %d bytes", a.name, a.size)
		}
		sk, err := ecdsa.ParseRawPrivateKey(a.curve, raw)
		if err != nil {
			return nil, fmt.Errorf("policy: %s key: %w", a.name, err)
		}
		return nistSigner{a, sk}, nil
	case AlgECDSASecp256k1, AlgSchnorrSecp256k1:
		if len(raw) != 32 {
			return nil, errors.New("policy: a secp256k1 key is 32 bytes")
		}
		var k secp256k1.ModNScalar
		if overflow := k.SetByteSlice(raw); overflow || k.IsZero() {
			return nil, errors.New("policy: the secp256k1 key is zero or not below the group order")
		}
		sk := secp256k1.NewPrivateKey(&k)
		if alg == AlgSchnorrSecp256k1 {
			return schnorrSigner{sk}, nil
		}
		return k1Signer{sk}, nil
	case AlgMLDSA44, AlgMLDSA65, AlgMLDSA87:
		a := mldsaAlgs[alg]
		if len(raw) != SeedSize {
			return nil, fmt.Errorf("policy: an ML-DSA key is its %d-byte seed", SeedSize)
		}
		sk, err := mldsa.NewPrivateKey(a.params, raw)
		if err != nil {
			return nil, err
		}
		return mldsaSigner{alg: alg, sk: sk}, nil
	case AlgSLHDSASHA2128s, AlgSLHDSASHA2256s:
		a := slhdsaAlgs[alg]
		n := a.pub / 2
		if len(raw) != 4*n {
			return nil, fmt.Errorf("policy: a %s key is %d bytes", a.name, 4*n)
		}
		sk := slhdsa.PrivateKey{ID: a.set}
		if err := sk.UnmarshalBinary(raw); err != nil {
			return nil, fmt.Errorf("policy: %s key: %w", a.name, err)
		}
		pub, err := sk.PublicKey().MarshalBinary()
		if err != nil {
			return nil, err
		}
		return slhdsaSigner{alg: alg, sk: sk, pub: pub}, nil
	}
	return nil, fmt.Errorf("%w: %d", ErrUnknownSigner, alg)
}
