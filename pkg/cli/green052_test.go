package cli

import (
	"strings"
	"testing"
)

// poon keygen gives the key's address in each series the node policy lists:
// under the default reserve series (SHA-512/256) the sha256 "address" is
// not the key's, so a note says to use series_addresses. A node policy
// whose series use the -hash gives no note, and no node policy gives
// neither (GREEN-052).
func TestKeygenSeriesAddresses(t *testing.T) {
	root := t.TempDir()
	cli(t, nil, "init", "-p2p", "off", "-root", root) // the reserve series
	seed := seedFile(t, strings.Repeat("33", 32))
	var k struct {
		Address string
		Note    string
		Series  []struct {
			Series, Builtin, Hash, Address string
			Allowed                        bool
		} `json:"series_addresses"`
	}
	cli(t, &k, "keygen", "-seed-file", seed, "-alg", "slh-dsa-sha2-256s", "-root", root)
	var want struct{ Address string }
	cli(t, &want, "keygen", "-seed-file", seed, "-alg", "slh-dsa-sha2-256s", "-hash", "sha512-256", "-root", t.TempDir())
	if len(k.Series) != 1 || k.Series[0].Builtin != "default" || k.Series[0].Hash != "sha512-256" || !k.Series[0].Allowed || k.Series[0].Address != want.Address || k.Address == want.Address {
		t.Fatalf("keygen under the reserve series: %+v, want address %s", k, want.Address)
	}
	if !strings.Contains(k.Note, "series_addresses") {
		t.Errorf("note %q", k.Note)
	}
	// ECDSA P-256 is not one of the reserve series' algorithms.
	cli(t, &k, "keygen", "-seed-file", seed, "-alg", "ecdsa-p256-sha256", "-root", root)
	if len(k.Series) != 1 || k.Series[0].Allowed {
		t.Errorf("a key the reserve series does not allow: %+v", k.Series)
	}
	// The same hash as the policy's series: no note.
	k.Note, k.Series = "", nil
	cli(t, &k, "keygen", "-seed-file", seed, "-hash", "sha512-256", "-root", root)
	if k.Note != "" || len(k.Series) != 1 || k.Series[0].Address != k.Address {
		t.Errorf("keygen -hash sha512-256: %+v", k)
	}
	// No node policy: neither.
	k.Note, k.Series = "", nil
	cli(t, &k, "keygen", "-seed-file", seed, "-root", t.TempDir())
	if k.Note != "" || k.Series != nil {
		t.Errorf("keygen with no node policy: %+v", k)
	}
}

// poon crypto -o table lists the key algorithms one row each (GREEN-052).
func TestCryptoTable(t *testing.T) {
	out := runOut(t, "crypto", "-o", "table")
	if !strings.Contains(out, "MAX_SIGNATURES_PER_WITNESS") || !strings.Contains(out, "slh-dsa-sha2-256s") || strings.Contains(out, "{") {
		t.Fatalf("poon crypto -o table:\n%s", out)
	}
	if _, err := runErr("crypto", "-o", "yaml"); err == nil {
		t.Error("-o yaml taken")
	}
}
