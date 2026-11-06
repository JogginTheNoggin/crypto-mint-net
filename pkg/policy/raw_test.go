package policy_test

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"testing"

	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/cloudflare/circl/sign/slhdsa"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/policy"
)

// D193: a key made by another tool, in its usual raw form, signs here as
// the same key: the public key is the one that tool shows, and the
// signature checks.
func TestRawSignerIsTheSameKey(t *testing.T) {
	edPub, edPriv, _ := ed25519.GenerateKey(rand.Reader)
	p256, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	p384, _ := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	p256raw, _ := p256.Bytes()
	p384raw, _ := p384.Bytes()
	p256pub, _ := p256.PublicKey.Bytes()
	p384pub, _ := p384.PublicKey.Bytes()
	k1, _ := secp256k1.GeneratePrivateKey()
	_, slh, _ := slhdsa.GenerateKey(rand.Reader, slhdsa.SHA2_128s)
	slhRaw, _ := slh.MarshalBinary()
	slhPub, _ := slh.PublicKey().MarshalBinary()
	for _, c := range []struct {
		name     string
		alg      policy.AlgID
		raw, pub []byte
	}{
		{"ed25519 seed", policy.AlgEd25519, edPriv.Seed(), edPub},
		{"ed25519 64 bytes", policy.AlgEd25519, edPriv, edPub},
		{"p256", policy.AlgECDSAP256, p256raw, p256pub},
		{"p384", policy.AlgECDSAP384, p384raw, p384pub},
		{"secp256k1", policy.AlgECDSASecp256k1, k1.Serialize(), k1.PubKey().SerializeCompressed()},
		{"schnorr", policy.AlgSchnorrSecp256k1, k1.Serialize(), schnorr.SerializePubKey(k1.PubKey())},
		{"slh-dsa-128s", policy.AlgSLHDSASHA2128s, slhRaw, slhPub},
	} {
		s, err := policy.NewRawSigner(c.alg, c.raw)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if !bytes.Equal(s.Public(), c.pub) {
			t.Fatalf("%s: public key %x, want %x", c.name, s.Public(), c.pub)
		}
		a, _ := policy.Default().Alg(c.alg)
		msg := []byte("a witness message")
		if !a.Verify(c.pub, msg, s.Sign(msg)) {
			t.Fatalf("%s: signature does not check", c.name)
		}
	}
	// ML-DSA's raw form is its seed, as NewSigner takes it.
	seed := make([]byte, policy.SeedSize)
	rand.Read(seed)
	a, _ := policy.NewRawSigner(policy.AlgMLDSA65, seed)
	b, _ := policy.NewSigner(policy.AlgMLDSA65, seed)
	if !bytes.Equal(a.Public(), b.Public()) {
		t.Fatal("ML-DSA raw and seed keys differ")
	}
	// Wrong sizes and bad keys are refused.
	bad := append(append([]byte(nil), edPriv...), 0)
	for _, c := range []struct {
		alg policy.AlgID
		raw []byte
	}{
		{policy.AlgEd25519, bad},
		{policy.AlgECDSAP256, make([]byte, 31)},
		{policy.AlgECDSASecp256k1, make([]byte, 32)}, // zero
		{policy.AlgECDSASecp256k1, bytes.Repeat([]byte{0xff}, 32)},
	} {
		if _, err := policy.NewRawSigner(c.alg, c.raw); err == nil {
			t.Fatalf("alg %d: a bad key was taken", c.alg)
		}
	}
	tampered := append([]byte(nil), edPriv...)
	tampered[40] ^= 1
	if _, err := policy.NewRawSigner(policy.AlgEd25519, tampered); err == nil {
		t.Fatal("an ed25519 key whose halves differ was taken")
	}
}
