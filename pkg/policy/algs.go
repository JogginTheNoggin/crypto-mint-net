package policy

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hkdf"
	"crypto/mldsa"
	"crypto/rand"
	"crypto/sha256"
	"crypto/sha512"
	"errors"
	"fmt"
	"math/big"

	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	k1ecdsa "github.com/decred/dcrd/dcrec/secp256k1/v4/ecdsa"
)

// SeedSize is the size of the seed every Signer is derived from.
const SeedSize = 32

// ErrUnknownSigner is returned by NewSigner for an algorithm it cannot sign
// with.
var ErrUnknownSigner = errors.New("policy: no signer for this key algorithm")

// NewSigner returns the signer for algorithm alg derived from a 32-byte
// seed. The same seed and algorithm always give the same key (decision
// D81), so a wallet need only keep its seed.
func NewSigner(alg AlgID, seed []byte) (Signer, error) {
	if len(seed) != SeedSize {
		return nil, fmt.Errorf("policy: a seed is %d bytes", SeedSize)
	}
	switch alg {
	case AlgEd25519:
		return NewEd25519Signer(seed)
	case AlgECDSAP256, AlgECDSAP384, AlgECDSAP521:
		a := nistAlgs[alg]
		return a.signer(seed)
	case AlgECDSASecp256k1, AlgSchnorrSecp256k1:
		k := secp256k1Key(seed, alg)
		if alg == AlgSchnorrSecp256k1 {
			return schnorrSigner{k}, nil
		}
		return k1Signer{k}, nil
	case AlgMLDSA44, AlgMLDSA65, AlgMLDSA87:
		a := mldsaAlgs[alg]
		sk, err := mldsa.NewPrivateKey(a.params, seed)
		if err != nil {
			return nil, err
		}
		return mldsaSigner{alg: alg, sk: sk}, nil
	}
	return nil, fmt.Errorf("%w: %d", ErrUnknownSigner, alg)
}

// expand derives n bytes from seed for one algorithm and attempt, with
// HKDF-SHA512 (RFC 5869).
func expand(seed []byte, label string, attempt byte, n int) []byte {
	b, err := hkdf.Expand(sha512.New, seed, fmt.Sprintf("PoO-key-v1 %s %d", label, attempt), n)
	if err != nil {
		panic(err) // unreachable: n is far below HKDF's limit
	}
	return b
}

// ECDSA over the NIST curves (FIPS 186-5). A public key is the 1 + 2n byte
// uncompressed SEC 1 point; a signature is r ∥ s, each n bytes, with s in
// the lower half of the group order so that each signature has one
// encoding. The signed digest is the curve's paired SHA-2 hash of the
// witness message.
type nistAlg struct {
	id    AlgID
	name  string
	curve elliptic.Curve
	size  int // bytes in a scalar
	sum   func([]byte) []byte
}

var nistAlgs = map[AlgID]nistAlg{
	AlgECDSAP256: {AlgECDSAP256, "ecdsa-p256-sha256", elliptic.P256(), 32, func(m []byte) []byte { s := sha256.Sum256(m); return s[:] }},
	AlgECDSAP384: {AlgECDSAP384, "ecdsa-p384-sha384", elliptic.P384(), 48, func(m []byte) []byte { s := sha512.Sum384(m); return s[:] }},
	AlgECDSAP521: {AlgECDSAP521, "ecdsa-p521-sha512", elliptic.P521(), 66, func(m []byte) []byte { s := sha512.Sum512(m); return s[:] }},
}

func (a nistAlg) ID() AlgID          { return a.id }
func (a nistAlg) Name() string       { return a.name }
func (a nistAlg) PublicKeySize() int { return 1 + 2*a.size }
func (a nistAlg) SignatureSize() int { return 2 * a.size }

func (a nistAlg) Verify(pub, msg, sig []byte) bool {
	if len(pub) != a.PublicKeySize() || len(sig) != a.SignatureSize() {
		return false
	}
	pk, err := ecdsa.ParseUncompressedPublicKey(a.curve, pub)
	if err != nil {
		return false
	}
	n := a.curve.Params().N
	r := new(big.Int).SetBytes(sig[:a.size])
	s := new(big.Int).SetBytes(sig[a.size:])
	half := new(big.Int).Rsh(n, 1)
	if r.Sign() == 0 || r.Cmp(n) >= 0 || s.Sign() == 0 || s.Cmp(half) > 0 {
		return false
	}
	return ecdsa.Verify(pk, a.sum(msg), r, s)
}

func (a nistAlg) signer(seed []byte) (Signer, error) {
	for attempt := byte(0); ; attempt++ {
		d := expand(seed, a.name, attempt, a.size)
		if a.id == AlgECDSAP521 {
			d[0] &= 1 // the order of P-521 is a 521-bit number
		}
		if sk, err := ecdsa.ParseRawPrivateKey(a.curve, d); err == nil {
			return nistSigner{a, sk}, nil
		}
		if attempt == 255 {
			return nil, errors.New("policy: no valid key from this seed")
		}
	}
}

type nistSigner struct {
	a  nistAlg
	sk *ecdsa.PrivateKey
}

func (s nistSigner) Alg() AlgID { return s.a.id }

func (s nistSigner) Public() []byte {
	b, err := s.sk.PublicKey.Bytes()
	if err != nil {
		panic(err) // unreachable: a valid key
	}
	return b
}

func (s nistSigner) Sign(msg []byte) []byte {
	r, ss, err := ecdsa.Sign(rand.Reader, s.sk, s.a.sum(msg))
	if err != nil {
		panic(err)
	}
	n := s.a.curve.Params().N
	if ss.Cmp(new(big.Int).Rsh(n, 1)) > 0 {
		ss.Sub(n, ss) // the lower-half form
	}
	out := make([]byte, 2*s.a.size)
	r.FillBytes(out[:s.a.size])
	ss.FillBytes(out[s.a.size:])
	return out
}

// ECDSA over secp256k1 (SEC 2), as Bitcoin uses it: a 33-byte compressed
// public key, a 64-byte r ∥ s signature with s in the lower half, over
// SHA-256 of the witness message.
type k1Alg struct{}

func (k1Alg) ID() AlgID          { return AlgECDSASecp256k1 }
func (k1Alg) Name() string       { return "ecdsa-secp256k1-sha256" }
func (k1Alg) PublicKeySize() int { return 33 }
func (k1Alg) SignatureSize() int { return 64 }

func (k1Alg) Verify(pub, msg, sig []byte) bool {
	if len(pub) != 33 || (pub[0] != 2 && pub[0] != 3) || len(sig) != 64 {
		return false
	}
	pk, err := secp256k1.ParsePubKey(pub)
	if err != nil {
		return false
	}
	var r, s secp256k1.ModNScalar
	if r.SetByteSlice(sig[:32]) || s.SetByteSlice(sig[32:]) || r.IsZero() || s.IsZero() || s.IsOverHalfOrder() {
		return false
	}
	d := sha256.Sum256(msg)
	return k1ecdsa.NewSignature(&r, &s).Verify(d[:], pk)
}

// secp256k1Key derives a secp256k1 private key in [1, n-1] from seed.
func secp256k1Key(seed []byte, alg AlgID) *secp256k1.PrivateKey {
	label := "secp256k1"
	if alg == AlgSchnorrSecp256k1 {
		label = "schnorr-secp256k1"
	}
	for attempt := byte(0); ; attempt++ {
		var k secp256k1.ModNScalar
		if overflow := k.SetByteSlice(expand(seed, label, attempt, 32)); !overflow && !k.IsZero() {
			return secp256k1.NewPrivateKey(&k)
		}
	}
}

type k1Signer struct{ sk *secp256k1.PrivateKey }

func (k1Signer) Alg() AlgID       { return AlgECDSASecp256k1 }
func (s k1Signer) Public() []byte { return s.sk.PubKey().SerializeCompressed() }

// Sign signs with RFC 6979 nonces; the library gives the lower-half s.
func (s k1Signer) Sign(msg []byte) []byte {
	d := sha256.Sum256(msg)
	sig := k1ecdsa.Sign(s.sk, d[:])
	r, ss := sig.R(), sig.S()
	var out [64]byte
	r.PutBytesUnchecked(out[:32])
	ss.PutBytesUnchecked(out[32:])
	return out[:]
}

// Schnorr signatures over secp256k1 as BIP-340 defines them: a 32-byte
// x-only public key and a 64-byte signature, over SHA-256 of the witness
// message (BIP-340 signs 32-byte messages).
type schnorrAlg struct{}

func (schnorrAlg) ID() AlgID          { return AlgSchnorrSecp256k1 }
func (schnorrAlg) Name() string       { return "schnorr-secp256k1-bip340" }
func (schnorrAlg) PublicKeySize() int { return 32 }
func (schnorrAlg) SignatureSize() int { return 64 }

// Verify parses the signature strictly (r below the field size, s below
// the group order), so each signature has one encoding.
func (schnorrAlg) Verify(pub, msg, sig []byte) bool {
	if len(pub) != 32 || len(sig) != 64 {
		return false
	}
	pk, err := schnorr.ParsePubKey(pub)
	if err != nil {
		return false
	}
	s, err := schnorr.ParseSignature(sig)
	if err != nil {
		return false
	}
	d := sha256.Sum256(msg)
	return s.Verify(d[:], pk)
}

type schnorrSigner struct{ sk *secp256k1.PrivateKey }

func (schnorrSigner) Alg() AlgID       { return AlgSchnorrSecp256k1 }
func (s schnorrSigner) Public() []byte { return schnorr.SerializePubKey(s.sk.PubKey()) }

func (s schnorrSigner) Sign(msg []byte) []byte {
	d := sha256.Sum256(msg)
	sig, err := schnorr.Sign(s.sk, d[:])
	if err != nil {
		panic(err)
	}
	return sig.Serialize()
}

// ML-DSA (FIPS 204), the lattice signature NIST standardised for
// resistance to quantum computers, in its three parameter sets. It signs
// the witness message itself, with an empty context.
type mldsaAlg struct {
	id     AlgID
	name   string
	params mldsa.Parameters
}

var mldsaAlgs = map[AlgID]mldsaAlg{
	AlgMLDSA44: {AlgMLDSA44, "ml-dsa-44", mldsa.MLDSA44()},
	AlgMLDSA65: {AlgMLDSA65, "ml-dsa-65", mldsa.MLDSA65()},
	AlgMLDSA87: {AlgMLDSA87, "ml-dsa-87", mldsa.MLDSA87()},
}

func (a mldsaAlg) ID() AlgID          { return a.id }
func (a mldsaAlg) Name() string       { return a.name }
func (a mldsaAlg) PublicKeySize() int { return a.params.PublicKeySize() }
func (a mldsaAlg) SignatureSize() int { return a.params.SignatureSize() }

func (a mldsaAlg) Verify(pub, msg, sig []byte) bool {
	if len(pub) != a.PublicKeySize() || len(sig) != a.SignatureSize() {
		return false
	}
	pk, err := mldsa.NewPublicKey(a.params, pub)
	if err != nil {
		return false
	}
	return mldsa.Verify(pk, msg, sig, nil) == nil
}

type mldsaSigner struct {
	alg AlgID
	sk  *mldsa.PrivateKey
}

func (s mldsaSigner) Alg() AlgID     { return s.alg }
func (s mldsaSigner) Public() []byte { return s.sk.PublicKey().Bytes() }

// Sign uses hedged signing: fresh randomness mixed with the key.
func (s mldsaSigner) Sign(msg []byte) []byte {
	sig, err := s.sk.Sign(rand.Reader, msg, &mldsa.Options{})
	if err != nil {
		panic(err)
	}
	return sig
}
