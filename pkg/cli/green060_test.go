package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The cold-key steps of running-a-node.md with the key never on the
// node's machine: poon keygen -out makes the key file and prints no seed,
// the watch-only wallet prepares the claim (wallet claim -to-policy),
// poon sign signs it with no node, and wallet submit sends it (GREEN-060).
func TestColdClaimWithoutTheKeyOnTheNode(t *testing.T) {
	createIterations = 1
	defer func() { createIterations = 0 }()
	root := t.TempDir()
	r := []string{"-root", root}
	cli(t, nil, append([]string{"init", "-example", "-listen", "127.0.0.1:0", "-p2p", "127.0.0.1:0"}, r...)...)
	stop := startNode(t, filepath.Join(root, "policies", "default"))
	defer stop()

	// On the key's machine: a new key file, the seed never printed.
	keyFile := filepath.Join(t.TempDir(), "key.hex")
	var k struct {
		PublicKey string `json:"public_key"`
		Seed      string
		KeyFile   string `json:"key_file"`
	}
	cli(t, &k, "keygen", "-out", keyFile)
	if k.Seed != "" || k.KeyFile != keyFile || k.PublicKey == "" {
		t.Fatalf("keygen -out %+v", k)
	}
	if fi, err := os.Stat(keyFile); err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("key file: %v %v", fi, err)
	}
	var again struct {
		PublicKey string `json:"public_key"`
	}
	cli(t, &again, "keygen", "-seed-file", keyFile)
	if again.PublicKey != k.PublicKey {
		t.Fatalf("the key file holds another key: %s, want %s", again.PublicKey, k.PublicKey)
	}
	if _, err := runErr("keygen", "-out", keyFile); err == nil || !strings.Contains(err.Error(), "never replaced") {
		t.Fatalf("keygen -out replaced a key file: %v", err)
	}
	if _, err := runErr("keygen", "-out", keyFile+"2", "-seed-file", keyFile); err == nil {
		t.Fatal("keygen -out with -seed-file")
	}

	// On the node's machine: a watch-only wallet, claim prepared unsigned.
	t.Setenv("POON_PASSPHRASE", "")
	ww := append([]string{"-wallet", filepath.Join(root, "watch.json")}, r...)
	wcmd := func(v any, sub string, args ...string) {
		t.Helper()
		cli(t, v, append(append([]string{"wallet", sub}, ww...), args...)...)
	}
	wcmd(nil, "create", "-watch-only")
	var pol struct{ Address string }
	wcmd(&pol, "policy", "-name", "cold", "-k", "1", "-key", "ed25519:"+k.PublicKey)
	if _, err := runErr(append(append([]string{"wallet", "claim"}, ww...), "-serial", "5")...); err == nil || !strings.Contains(err.Error(), "-to-policy NAME") {
		t.Fatalf("a plain watch-only claim does not say how: %v", err)
	}
	if _, err := runErr(append(append([]string{"wallet", "claim"}, ww...), "-serial", "5", "-to-policy", "cold")...); err == nil {
		t.Fatal("-to-policy without -out")
	}
	claim := filepath.Join(t.TempDir(), "claim.json")
	runOut(t, append(append([]string{"wallet", "claim"}, ww...), "-serial", "5", "-to-policy", "cold", "-out", claim)...)

	// The key signs it, with no node: -show marks the claim.
	var shown SignReport
	cli(t, &shown, "sign", claim, "-key-file", keyFile, "-show")
	if len(shown.Legs) != 1 || !shown.Legs[0].Claim || shown.Legs[0].Outputs[0].Address.String() != pol.Address {
		t.Fatalf("sign -show %+v", shown)
	}
	var rep SignReport
	cli(t, &rep, "sign", claim, "-key-file", keyFile)
	if rep.Added != 1 || rep.Missing[0][0] != 0 {
		t.Fatalf("sign %+v", rep)
	}

	// Submitted and worked in: the cold key's address holds the mint, and
	// the wallet watches it.
	wcmd(nil, "submit", claim)
	var b struct{ Layer struct{ Switched bool } }
	cli(t, &b, append([]string{"build", "-pool", "1", "-work", "4000"}, r...)...)
	var addr struct{ Balance uint64 }
	cli(t, &addr, append(append([]string{"address"}, r...), pol.Address)...)
	if addr.Balance != 100 {
		t.Fatalf("the cold key's address holds %d, want 100", addr.Balance)
	}
	var bal struct{ Total uint64 }
	wcmd(&bal, "balance")
	if bal.Total != 100 {
		t.Fatalf("watch-only balance %+v", bal)
	}
}
