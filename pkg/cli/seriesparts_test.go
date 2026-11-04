package cli

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/nodepolicy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/series"
)

// D163: an issuer makes a variable series, signs it, lists its parts and
// releases part 2; the release checks, and a node that lists the series
// reports it unreadable until its transactions are built.
func TestSeriesReleaseCLI(t *testing.T) {
	dir := t.TempDir()
	seedFile := filepath.Join(dir, "seed")
	if err := os.WriteFile(seedFile, []byte(strings.Repeat("ab", 32)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var key struct{ Address string }
	cli(t, &key, "keygen", "-seed-file", seedFile)

	if out, err := runErr("series", "new", "-variable", "-part-size", "4"); err == nil || !strings.Contains(err.Error(), "must name an issuer") {
		t.Fatalf("variable without an issuer: %v %s", err, out)
	}
	if _, err := runErr("series", "new", "-part-size", "4"); err == nil {
		t.Fatal("-part-size without -variable")
	}
	file := filepath.Join(dir, "series.json")
	out := runOut(t, "series", "new", "-mints", "8", "-pool-size", "4", "-variable", "-part-size", "4", "-max-parts", "3", "-issuer", key.Address)
	if err := os.WriteFile(file, []byte(out), 0o600); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"supply_type": "variable"`) {
		t.Fatalf("series new:\n%s", out)
	}

	var parts struct {
		ID    string `json:"series_id"`
		Parts []struct {
			Part        uint64
			FirstSerial uint64 `json:"first_serial"`
			LastSerial  uint64 `json:"last_serial"`
			ID          string `json:"part_id"`
		}
	}
	cli(t, &parts, "series", "parts", file)
	if len(parts.Parts) != 3 || parts.Parts[1].FirstSerial != 8 || parts.Parts[1].LastSerial != 11 || parts.Parts[2].LastSerial != 15 {
		t.Fatalf("parts: %+v", parts)
	}

	var signed struct {
		Witness string `json:"issuer_witness"`
	}
	cli(t, &signed, "series", "sign", file, "-seed-file", seedFile)

	rel := filepath.Join(dir, "part2.json")
	out = runOut(t, "series", "release", file, "-part", "2", "-seed-file", seedFile)
	if err := os.WriteFile(rel, []byte(out), 0o600); err != nil {
		t.Fatal(err)
	}
	var check struct {
		Valid  bool
		PartID string `json:"part_id"`
		Mints  uint64 `json:"mints_released"`
	}
	cli(t, &check, "series", "release", file, "-check", rel)
	if !check.Valid || check.PartID != parts.Parts[1].ID || check.Mints != 12 {
		t.Fatalf("check: %+v", check)
	}
	// A release of part 2 does not release part 3 of another file, and a
	// changed part number fails its witness.
	bad := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(bad, []byte(strings.Replace(out, `"part": 2`, `"part": 3`, 1)), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := runErr("series", "release", file, "-check", bad); err == nil {
		t.Fatal("an edited release checked")
	}
	for _, args := range [][]string{
		{"series", "release", file, "-part", "1", "-seed-file", seedFile},
		{"series", "release", file, "-part", "4", "-seed-file", seedFile},
	} {
		if _, err := runErr(args...); err == nil {
			t.Fatalf("%v was released", args)
		}
	}
	other := filepath.Join(dir, "other")
	if err := os.WriteFile(other, []byte(strings.Repeat("cd", 32)), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := runErr("series", "release", file, "-part", "2", "-seed-file", other); err == nil || !strings.Contains(err.Error(), "not the series' issuer") {
		t.Fatalf("another key released: %v", err)
	}
	fixed := filepath.Join(dir, "fixed.json")
	if err := os.WriteFile(fixed, []byte(runOut(t, "series", "example")), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := runErr("series", "parts", fixed); err == nil || !strings.Contains(err.Error(), "fixed") {
		t.Fatalf("a fixed series' parts: %v", err)
	}

	// A node policy that lists it reports it unreadable, not invalid.
	s, err := readSeriesFile(file)
	if err != nil {
		t.Fatal(err)
	}
	pol := nodepolicy.Default()
	pol.Series = []nodepolicy.SeriesInfo{{Params: s.Params(), IssuerWitness: signed.Witness}}
	if acc, ref := pol.Accepted(); len(acc) != 0 || len(ref) != 1 || !strings.Contains(ref[0].Reason.Error(), "not followed yet") {
		t.Fatalf("accepted %d, refused %v", len(acc), ref)
	}
	if w, err := nodepolicy.DecodeWitness(signed.Witness); err != nil || s.CheckIssuer(w) != nil {
		t.Fatalf("issuer witness: %v", err)
	}
	if s.SupplyType() != series.Variable {
		t.Fatal("read back fixed")
	}
}

func runErr(args ...string) (string, error) {
	var out bytes.Buffer
	err := run(args, &out)
	return out.String(), err
}

// A node keeps a variable series it does not follow yet: poo init says so,
// and poo doctor says to leave it, not to correct it; and poo series parts
// -from past the last part names the cap (GREEN-049).
func TestVariableSeriesNotes(t *testing.T) {
	dir := t.TempDir()
	addr := strings.Repeat("9d", 32)
	file := filepath.Join(dir, "series.json")
	out := runOut(t, "series", "new", "-variable", "-part-size", "4", "-max-parts", "3", "-issuer", addr)
	if err := os.WriteFile(file, []byte(out), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := runErr("series", "parts", file, "-from", "5"); err == nil || !strings.Contains(err.Error(), "at most 3 parts") {
		t.Errorf("parts -from past the cap: %v", err)
	}
	node := filepath.Join(dir, "node")
	os.MkdirAll(node, 0o700)
	init := runOut(t, "init", "-dir", node, "-p2p", "off", "-series", file)
	if !strings.Contains(init, "is a variable supply series") || !strings.Contains(init, "leave it there") {
		t.Errorf("init:\n%s", init)
	}
	c := checkNamed(doctor(io.Discard, node), "series")
	if c.Result != checkWarn || strings.Contains(c.Fix, "correct or remove") || !strings.Contains(c.Fix, "leave it there") {
		t.Errorf("doctor series: %+v", c)
	}
	// With a fixed series beside it, init says nothing extra.
	fixed := runOut(t, "init", "-dir", t.TempDir(), "-p2p", "off", "-example")
	if strings.Contains(fixed, "note:") {
		t.Errorf("init of a fixed series:\n%s", fixed)
	}
}
