package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// D193: a key kept off the node. A watch-only wallet holds no private key;
// it watches the key's address, makes the payment unsigned, poo sign adds
// the signature with no node and no wallet, and the wallet submits it.
// The change goes back to the key's address.
func TestColdKeyPayment(t *testing.T) {
	createIterations = 1
	defer func() { createIterations = 0 }()
	root := t.TempDir()
	r := []string{"-root", root}
	cli(t, nil, append([]string{"init", "-example", "-listen", "127.0.0.1:0", "-p2p", "127.0.0.1:0"}, r...)...)
	stop := startNode(t, filepath.Join(root, "policies", "default"))
	defer stop()

	// The cold key, made "elsewhere": only its public key reaches the node.
	coldSeed := strings.Repeat("c3", 32)
	keyFile := seedFile(t, coldSeed)
	var k struct {
		PublicKey string `json:"public_key"`
		Seed      string
	}
	cli(t, &k, "keygen", "-seed-file", keyFile)
	var raw struct {
		PublicKey string `json:"public_key"`
		Seed      string
	}
	cli(t, &raw, "keygen", "-raw", "-seed-file", keyFile) // an Ed25519 raw key is its seed
	if raw.PublicKey != k.PublicKey || raw.Seed != "" {
		t.Fatalf("keygen -raw %+v, want public key %s and no seed printed", raw, k.PublicKey)
	}

	t.Setenv("POO_PASSPHRASE", "") // a watch-only wallet asks for none
	ww := append([]string{"-wallet", filepath.Join(root, "watch.json")}, r...)
	wcmd := func(v any, sub string, args ...string) {
		t.Helper()
		cli(t, v, append(append([]string{"wallet", sub}, ww...), args...)...)
	}
	wcmd(nil, "create", "-watch-only")
	if b, _ := os.ReadFile(filepath.Join(root, "watch.json")); !strings.Contains(string(b), `"watch_only": true`) || !strings.Contains(string(b), `"sealed": ""`) {
		t.Fatalf("watch-only wallet file:\n%s", b)
	}
	var pol struct{ Address string }
	wcmd(&pol, "policy", "-name", "cold", "-k", "1", "-key", "ed25519:"+k.PublicKey)
	for _, refused := range [][]string{{"address"}, {"seed"}, {"claim", "-serial", "6"}} {
		if _, err := runErr(append(append([]string{"wallet"}, refused[0]), append(ww, refused[1:]...)...)...); err == nil || !strings.Contains(err.Error(), "watch-only") {
			t.Fatalf("watch-only wallet %v: %v", refused, err)
		}
	}

	// The cold key claims mint 5 (as a claim made elsewhere would), and the
	// watch-only wallet sees it.
	cli(t, nil, append([]string{"claim", "-seed-file", keyFile, "-serial", "5"}, r...)...)
	work := func() {
		t.Helper()
		var b struct{ Layer struct{ Switched bool } }
		cli(t, &b, append([]string{"build", "-pool", "1", "-work", "4000"}, r...)...)
		if !b.Layer.Switched {
			t.Fatalf("build -work did not select the new entry: %+v", b)
		}
	}
	work()
	wcmd(nil, "watch", "-serial", "5")
	var bal struct{ Total uint64 }
	wcmd(&bal, "balance")
	if bal.Total != 100 {
		t.Fatalf("watch-only balance %+v", bal)
	}

	// Pay 30 to bob: unsigned, written for the signer.
	bob := strings.Repeat("b0", 32)
	var bk struct{ Address string }
	cli(t, &bk, "keygen", "-seed-file", seedFile(t, bob), "-hash", "sha256")
	u := filepath.Join(t.TempDir(), "pay.json")
	out := runOut(t, append(append([]string{"wallet", "pay"}, ww...), "-to", bk.Address, "-amount", "30", "-out", u)...)
	if !strings.Contains(out, "signatures missing") {
		t.Fatalf("pay: %s", out)
	}

	// poo sign: -show signs nothing; a wrong key is refused; the right one
	// signs, with no node or wallet named.
	var shown SignReport
	cli(t, &shown, "sign", u, "-key-file", keyFile, "-show")
	if !shown.Shown || shown.Added != 0 || len(shown.Legs) != 1 || shown.Legs[0].Serial != 5 || len(shown.Legs[0].Outputs) != 2 {
		t.Fatalf("sign -show %+v", shown)
	}
	if _, err := runErr("sign", u, "-key-file", seedFile(t, bob)); err == nil || !strings.Contains(err.Error(), "in none of the payment's inputs") {
		t.Fatalf("a key not in the payment signed: %v", err)
	}
	signed := filepath.Join(t.TempDir(), "signed.json")
	var rep SignReport
	cli(t, &rep, "sign", u, "-key-file", keyFile, "-out", signed)
	if rep.Added != 1 || rep.Missing[0][0] != 0 || rep.Wrote != signed {
		t.Fatalf("sign %+v", rep)
	}
	if fi, err := os.Stat(signed); err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("signed file: %v %v", fi, err)
	}

	// The wallet submits it; once worked in, bob holds 30 and the change
	// is back at the cold key's address.
	var sent struct{ Legs []struct{ State string } }
	wcmd(&sent, "submit", signed)
	work()
	var b struct{ Balance uint64 }
	cli(t, &b, append(append([]string{"address"}, r...), bk.Address)...)
	if b.Balance != 30 {
		t.Fatalf("bob holds %d, want 30", b.Balance)
	}
	cli(t, &b, append(append([]string{"address"}, r...), pol.Address)...)
	if b.Balance != 70 {
		t.Fatalf("the cold key's address holds %d, want 70 (the change)", b.Balance)
	}
	var file map[string]any
	data, _ := os.ReadFile(u)
	json.Unmarshal(data, &file)
	if file["hash"] == "" || file["hash"] == nil {
		t.Fatalf("the unsigned payment does not name its hash: %s", data)
	}
}
