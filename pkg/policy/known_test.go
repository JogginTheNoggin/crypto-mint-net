package policy

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
)

var update = flag.Bool("update", false, "rewrite testdata/known.json")

// knownKey pins key derivation for one algorithm: the public key a seed
// gives, and one signature of msg that must verify. NIST ECDSA and ML-DSA
// sign with fresh randomness, so for them the signature is one valid
// signature, not the only one.
type knownKey struct {
	Alg    string `json:"alg"`
	Seed   string `json:"seed"`
	Public string `json:"public"`
	Msg    string `json:"msg"`
	Sig    string `json:"sig"`
}

// knownPolicy pins a policy's bytes and its address under every hash.
type knownPolicy struct {
	Name      string            `json:"name"`
	Policy    string            `json:"policy"`
	Addresses map[string]string `json:"addresses"`
}

type known struct {
	Keys     []knownKey    `json:"keys"`
	Policies []knownPolicy `json:"policies"`
}

func knownSeed(alg string) []byte { s := sha256.Sum256([]byte("PoON-known-" + alg)); return s[:] }

// computeKnown builds the file. Signatures are made fresh, so when the
// file exists its signatures are kept: they must still verify.
func computeKnown(t *testing.T, old map[string]knownKey) known {
	var k known
	msg := Message(hash.Hash{1, 2, 3})
	pubs := map[AlgID][]byte{}
	for _, a := range Default().Algs() {
		seed := knownSeed(a.Name())
		s, err := NewSigner(a.ID(), seed)
		if err != nil {
			t.Fatal(err)
		}
		pubs[a.ID()] = s.Public()
		sig := hex.EncodeToString(s.Sign(msg))
		if o, ok := old[a.Name()]; ok && o.Public == hex.EncodeToString(s.Public()) {
			sig = o.Sig
		}
		k.Keys = append(k.Keys, knownKey{a.Name(), hex.EncodeToString(seed), hex.EncodeToString(s.Public()), hex.EncodeToString(msg), sig})
	}
	hl, _ := NewHashlock(hash.SHA3_256, []byte("PoON-known-preimage"))
	ps := []struct {
		name string
		p    Policy
	}{
		{"1-of-1 schnorr-secp256k1-bip340", SingleKey(AlgSchnorrSecp256k1, pubs[AlgSchnorrSecp256k1])},
		{"2-of-3 ed25519, ecdsa-p256-sha256, ml-dsa-44", ThresholdParams{K: 2, Keys: []Key{
			{AlgEd25519, pubs[AlgEd25519]}, {AlgECDSAP256, pubs[AlgECDSAP256]}, {AlgMLDSA44, pubs[AlgMLDSA44]}}}.Policy()},
		{"hashlock sha3-256", hl.Policy()},
		{"all of (ecdsa-secp256k1-sha256, hashlock)", AllOf(SingleKey(AlgECDSASecp256k1, pubs[AlgECDSASecp256k1]), hl.Policy())},
		{"any of (ed25519, ml-dsa-65)", AnyOf(SingleKey(AlgEd25519, pubs[AlgEd25519]), SingleKey(AlgMLDSA65, pubs[AlgMLDSA65]))},
	}
	for _, p := range ps {
		kp := knownPolicy{Name: p.name, Policy: hex.EncodeToString(p.p.Encode()), Addresses: map[string]string{}}
		for _, id := range hash.IDs() {
			h, _ := hash.Lookup(id)
			kp.Addresses[id.Name()] = Address(h, p.p).String()
		}
		k.Policies = append(k.Policies, kp)
	}
	return k
}

// TestKnownAnswers checks key derivation, signatures and policy addresses
// against testdata/known.json. A change there changes users' keys and
// addresses, so it needs a decision in docs/decisions.md.
func TestKnownAnswers(t *testing.T) {
	path := filepath.Join("testdata", "known.json")
	old := map[string]knownKey{}
	if b, err := os.ReadFile(path); err == nil {
		var k known
		if err := json.Unmarshal(b, &k); err != nil {
			t.Fatal(err)
		}
		for _, kk := range k.Keys {
			old[kk.Alg] = kk
		}
	}
	got, _ := json.MarshalIndent(computeKnown(t, old), "", "  ")
	got = append(got, '\n')
	if *update {
		os.MkdirAll("testdata", 0o755)
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run go test ./pkg/policy -run TestKnownAnswers -update)", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("known answers changed: key derivation, a policy encoding or a hash changed. If intended, record it in docs/decisions.md and run with -update.\n got:\n%s", got)
	}
	var k known
	json.Unmarshal(want, &k)
	for _, kk := range k.Keys {
		id, _ := Default().AlgByName(kk.Alg)
		a, _ := Default().Alg(id)
		pub, _ := hex.DecodeString(kk.Public)
		msg, _ := hex.DecodeString(kk.Msg)
		sig, _ := hex.DecodeString(kk.Sig)
		if !a.Verify(pub, msg, sig) {
			t.Errorf("%s: the recorded signature does not verify", kk.Alg)
		}
	}
}
