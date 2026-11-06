package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A key made with poon keygen -alg can claim, pay, sign a series and
// release its parts: the commands that take a seed take -alg too. Without
// it the seed's Ed25519 key is used, and the issuer check says which -alg
// is the issuer's; a key the series does not allow is refused before it is
// sent, with the ones it allows (GREEN-051).
func TestSeedCommandsTakeAlg(t *testing.T) {
	const slh = "slh-dsa-sha2-128s"
	dir := t.TempDir()
	seed := seedFile(t, strings.Repeat("11", 32))
	var key struct{ Address string }
	cli(t, &key, "keygen", "-alg", slh, "-seed-file", seed)
	var bob struct{ Address string }
	cli(t, &bob, "keygen", "-seed", strings.Repeat("22", 32))

	file := filepath.Join(dir, "series.json")
	if err := os.WriteFile(file, []byte(runOut(t, "series", "new", "-algs", "ed25519,"+slh, "-issuer", key.Address, "-variable", "-part-size", "4")), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := runErr("series", "sign", file, "-seed-file", seed); err == nil || !strings.Contains(err.Error(), "(ed25519)") || !strings.Contains(err.Error(), "add -alg "+slh) {
		t.Fatalf("sign with the Ed25519 key of an SLH-DSA issuer's seed: %v", err)
	}
	signed := filepath.Join(dir, "signed.json")
	if err := os.WriteFile(signed, []byte(runOut(t, "series", "sign", file, "-seed-file", seed, "-alg", slh)), 0o600); err != nil {
		t.Fatal(err)
	}
	runOut(t, "series", "release", signed, "-part", "2", "-seed-file", seed, "-alg", slh)
	if _, err := runErr("series", "sign", file, "-seed-file", seed, "-alg", "nope"); err == nil || !strings.Contains(err.Error(), "-alg") {
		t.Fatalf("-alg nope: %v", err)
	}

	root := t.TempDir()
	r := []string{"-root", root}
	runOut(t, append([]string{"init", "-series", signed, "-listen", "127.0.0.1:0", "-p2p", "off"}, r...)...)
	stop := startNode(t, filepath.Join(root, "policies", "default"))
	defer stop()
	cli(t, nil, append([]string{"claim", "-seed-file", seed, "-alg", slh, "-serial", "1"}, r...)...)
	var built struct{ Layer struct{ Switched bool } }
	cli(t, &built, append([]string{"build", "-serial", "1", "-work", "2000"}, r...)...)
	var hs []struct{ Address string }
	cli(t, &hs, append([]string{"holdings", "-serial", "1"}, r...)...)
	if len(hs) != 1 || hs[0].Address != key.Address {
		t.Fatalf("mint 1 is held by %+v, want the SLH-DSA key %s", hs, key.Address)
	}
	cli(t, nil, append([]string{"pay", "-seed-file", seed, "-alg", slh, "-serial", "1", "-to", bob.Address, "-amount", "10"}, r...)...)
}

// A series that allows only Ed25519 refuses an SLH-DSA claim before it is
// sent, naming what it allows.
func TestClaimRefusesAKeyTheSeriesDoesNotAllow(t *testing.T) {
	root := t.TempDir()
	r := []string{"-root", root}
	cli(t, nil, append([]string{"init", "-example", "-listen", "127.0.0.1:0", "-p2p", "off"}, r...)...)
	stop := startNode(t, filepath.Join(root, "policies", "default"))
	defer stop()
	_, err := runErr(append([]string{"claim", "-seed-file", seedFile(t, strings.Repeat("11", 32)), "-alg", "slh-dsa-sha2-128s", "-serial", "1"}, r...)...)
	if err == nil || !strings.Contains(err.Error(), "allows the key algorithms ed25519, not this key's slh-dsa-sha2-128s") {
		t.Fatalf("claim with a key the series does not allow: %v", err)
	}
}
