package cli

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/policy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/series"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/wallet"
)

func TestCryptoLists(t *testing.T) {
	var out bytes.Buffer
	if err := run([]string{"crypto"}, &out); err != nil {
		t.Fatal(err)
	}
	type named struct {
		ID   uint32 `json:"id"`
		Name string `json:"name"`
	}
	var v struct {
		Hashes []named `json:"hashes"`
		Algs   []named `json:"key_algs"`
		Types  []named `json:"policy_types"`
	}
	if err := json.Unmarshal(out.Bytes(), &v); err != nil {
		t.Fatal(err)
	}
	if len(v.Hashes) != len(hash.IDs()) || len(v.Algs) != len(policy.Default().Algs()) || len(v.Types) != len(policy.Default().Types()) {
		t.Fatalf("lists %s", out.String())
	}
}

// keygen with every algorithm gives the address a series of the chosen
// hash computes for the key's policy.
func TestKeygenEveryAlgorithm(t *testing.T) {
	seed := strings.Repeat("ab", 32)
	for _, a := range policy.Default().Algs() {
		for _, id := range hash.IDs() {
			var out bytes.Buffer
			if err := run([]string{"keygen", "-seed", seed, "-alg", a.Name(), "-hash", id.Name()}, &out); err != nil {
				t.Fatalf("%s %s: %v", a.Name(), id.Name(), err)
			}
			var m map[string]string
			json.Unmarshal(out.Bytes(), &m)
			pub, _ := hex.DecodeString(m["public_key"])
			h, _ := hash.Lookup(id)
			if want := policy.Address(h, policy.SingleKey(a.ID(), pub)).String(); m["address"] != want || m["alg"] != a.Name() {
				t.Fatalf("%s %s: %s", a.Name(), id.Name(), out.String())
			}
			if _, ok := m["address_sha256"]; ok != (id == hash.SHA256) {
				t.Fatalf("address_sha256 under %s", id.Name())
			}
		}
	}
	for _, bad := range [][]string{{"-alg", "rsa"}, {"-alg", "99"}, {"-hash", "md5"}} {
		if err := run(append([]string{"keygen"}, bad...), &bytes.Buffer{}); err == nil {
			t.Fatalf("%v accepted", bad)
		}
	}
}

func TestSeriesNew(t *testing.T) {
	var out bytes.Buffer
	if err := run([]string{"series", "new", "-hash", "blake3", "-algs", "ml-dsa-65, ed25519,1", "-types", "k-of-policies,threshold,hashlock", "-mints", "16"}, &out); err != nil {
		t.Fatal(err)
	}
	var p series.Params
	if err := json.Unmarshal(out.Bytes(), &p); err != nil {
		t.Fatal(err)
	}
	if p.HashID != hash.BLAKE3 || p.Mints != 16 || len(p.KeyAlgs) != 2 || p.KeyAlgs[0] != policy.AlgEd25519 || p.KeyAlgs[1] != policy.AlgMLDSA65 || len(p.PolicyTypes) != 3 {
		t.Fatalf("params %+v", p)
	}
	path := filepath.Join(t.TempDir(), "s.json")
	os.WriteFile(path, out.Bytes(), 0o600)
	if err := run([]string{"series", "info", path}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	for _, bad := range [][]string{{"-algs", ""}, {"-types", "timelock"}, {"-pool-size", "0"}} {
		if err := run(append([]string{"series", "new"}, bad...), &bytes.Buffer{}); err == nil {
			t.Fatalf("%v accepted", bad)
		}
	}
}

func TestWalletCreateAlg(t *testing.T) {
	createIterations = 1
	defer func() { createIterations = 0 }()
	t.Setenv("POO_PASSPHRASE", "pass")
	path := filepath.Join(t.TempDir(), "w.json")
	if err := run([]string{"wallet", "create", "-wallet", path, "-alg", "schnorr-secp256k1-bip340"}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	w, err := wallet.Open(path)
	if err != nil || w.KeyAlg() != policy.AlgSchnorrSecp256k1 {
		t.Fatalf("%v %v", err, w)
	}
	if err := run([]string{"wallet", "create", "-wallet", path + "2", "-alg", "rsa"}, &bytes.Buffer{}); err == nil {
		t.Fatal("unknown algorithm accepted")
	}
	pk, err := parsePolicyKey("ml-dsa-44:00ff")
	if err != nil || pk.Alg != uint32(policy.AlgMLDSA44) {
		t.Fatalf("%v %+v", err, pk)
	}
}
