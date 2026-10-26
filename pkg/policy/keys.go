package policy

import (
	"crypto/ed25519"
	"fmt"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
)

type ed25519Alg struct{}

func (ed25519Alg) ID() AlgID          { return AlgEd25519 }
func (ed25519Alg) Name() string       { return "ed25519" }
func (ed25519Alg) PublicKeySize() int { return ed25519.PublicKeySize }
func (ed25519Alg) SignatureSize() int { return ed25519.SignatureSize }

// Verify uses the standard library, which rejects non-canonical signature
// scalars, so one signature has one accepted encoding.
func (ed25519Alg) Verify(pub, msg, sig []byte) bool {
	if len(pub) != ed25519.PublicKeySize || len(sig) != ed25519.SignatureSize {
		return false
	}
	return ed25519.Verify(ed25519.PublicKey(pub), msg, sig)
}

// Signer can sign witness messages for one key.
type Signer interface {
	Alg() AlgID
	Public() []byte
	Sign(msg []byte) []byte
}

// Ed25519Signer signs with an Ed25519 private key.
type Ed25519Signer struct {
	priv ed25519.PrivateKey
}

// NewEd25519Signer returns a signer for a 32-byte seed.
func NewEd25519Signer(seed []byte) (*Ed25519Signer, error) {
	if len(seed) != ed25519.SeedSize {
		return nil, fmt.Errorf("policy: ed25519 seed must be %d bytes", ed25519.SeedSize)
	}
	return &Ed25519Signer{priv: ed25519.NewKeyFromSeed(seed)}, nil
}

// Alg returns AlgEd25519.
func (s *Ed25519Signer) Alg() AlgID { return AlgEd25519 }

// Public returns the 32-byte public key.
func (s *Ed25519Signer) Public() []byte {
	return []byte(s.priv.Public().(ed25519.PublicKey))
}

// Sign signs msg.
func (s *Ed25519Signer) Sign(msg []byte) []byte { return ed25519.Sign(s.priv, msg) }

// Seed returns the private seed. Handle with care.
func (s *Ed25519Signer) Seed() []byte { return s.priv.Seed() }

// SignThreshold builds a witness for a threshold policy. signers maps key
// indices to the signers for those keys; exactly K of them must be given.
func SignThreshold(p ThresholdParams, signers map[uint16]Signer, tid hash.Hash) (Witness, error) {
	if len(signers) != int(p.K) {
		return Witness{}, fmt.Errorf("policy: need exactly %d signers, got %d", p.K, len(signers))
	}
	msg := Message(tid)
	var d ThresholdData
	for i := range p.Keys {
		s, ok := signers[uint16(i)]
		if !ok {
			continue
		}
		if s.Alg() != p.Keys[i].Alg || string(s.Public()) != string(p.Keys[i].Public) {
			return Witness{}, fmt.Errorf("policy: signer for index %d does not match the key", i)
		}
		d.Sigs = append(d.Sigs, Sig{Index: uint16(i), Signature: s.Sign(msg)})
	}
	if len(d.Sigs) != int(p.K) {
		return Witness{}, fmt.Errorf("policy: signer index out of range")
	}
	return Witness{Policy: p.Policy(), Data: d.Encode()}, nil
}

// SingleKeyWitness signs tid with one signer under its 1-of-1 policy.
func SingleKeyWitness(s Signer, tid hash.Hash) Witness {
	p := ThresholdParams{K: 1, Keys: []Key{{Alg: s.Alg(), Public: s.Public()}}}
	w, err := SignThreshold(p, map[uint16]Signer{0: s}, tid)
	if err != nil {
		panic(err) // unreachable: one signer for one matching key
	}
	return w
}
