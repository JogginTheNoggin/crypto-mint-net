package policy

import (
	"bytes"
	"crypto/rand"

	"github.com/cloudflare/circl/sign/slhdsa"
)

// SLH-DSA (FIPS 205), the stateless hash-based signature NIST
// standardised for resistance to quantum computers (D168). It rests only
// on its hash, so it holds even if the lattice problems under ML-DSA
// fall. It signs the witness message itself, with an empty context, as a
// pure (not pre-hashed) signature.
//
// Two sets are registered: SLH-DSA-SHA2-128s (NIST category 1, 32-byte
// keys, 7,856-byte signatures, D168) and SLH-DSA-SHA2-256s (category 5,
// 64-byte keys, 29,792-byte signatures, D170). There is no network-wide
// size limit (D170): how many signatures one witness holds is set by the
// series' witness ceiling, which CheckPolicy (CheckSpendable) applies
// where an address is made. The 192-bit set is left out on purpose.
type slhdsaAlg struct {
	id   AlgID
	name string
	set  slhdsa.ID
	pub  int
	sig  int
}

var slhdsaAlgs = map[AlgID]slhdsaAlg{
	AlgSLHDSASHA2128s: {AlgSLHDSASHA2128s, "slh-dsa-sha2-128s", slhdsa.SHA2_128s, 32, 7856},
	AlgSLHDSASHA2256s: {AlgSLHDSASHA2256s, "slh-dsa-sha2-256s", slhdsa.SHA2_256s, 64, 29792},
}

func (a slhdsaAlg) ID() AlgID          { return a.id }
func (a slhdsaAlg) Name() string       { return a.name }
func (a slhdsaAlg) PublicKeySize() int { return a.pub }
func (a slhdsaAlg) SignatureSize() int { return a.sig }

func (a slhdsaAlg) Verify(pub, msg, sig []byte) bool {
	if len(pub) != a.pub || len(sig) != a.sig {
		return false
	}
	pk := slhdsa.PublicKey{ID: a.set}
	if err := pk.UnmarshalBinary(pub); err != nil {
		return false
	}
	return slhdsa.Verify(&pk, slhdsa.NewMessage(msg), sig, nil)
}

// signer derives the key from the seed: FIPS 205's key generation reads
// SK.seed, SK.prf and PK.seed (n bytes each) from HKDF-SHA512 of the seed,
// info "PoO-key-v1 <name> 0", so the same seed always gives the same key.
func (a slhdsaAlg) signer(seed []byte) (Signer, error) {
	n := a.pub / 2
	_, sk, err := slhdsa.GenerateKey(bytes.NewReader(expand(seed, a.name, 0, 3*n)), a.set)
	if err != nil {
		return nil, err
	}
	pub, err := sk.PublicKey().MarshalBinary()
	if err != nil {
		return nil, err
	}
	return slhdsaSigner{alg: a.id, sk: sk, pub: pub}, nil
}

type slhdsaSigner struct {
	alg AlgID
	sk  slhdsa.PrivateKey
	pub []byte
}

func (s slhdsaSigner) Alg() AlgID     { return s.alg }
func (s slhdsaSigner) Public() []byte { return s.pub }

// Sign uses FIPS 205's randomized (hedged) signing.
func (s slhdsaSigner) Sign(msg []byte) []byte {
	sig, err := slhdsa.SignRandomized(&s.sk, rand.Reader, slhdsa.NewMessage(msg), nil)
	if err != nil {
		panic(err)
	}
	return sig
}
