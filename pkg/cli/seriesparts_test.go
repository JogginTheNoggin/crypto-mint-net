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

// series info, the signed release and its check print a part's serials and
// pools as ranges, never lists (GUI ORANGE-018, ORANGE-034). The release
// file keeps its own fields first and -check reads it back, and a fixed
// series' info is unchanged.
func TestSeriesPartsAsRanges(t *testing.T) {
	dir := t.TempDir()
	seedFile := filepath.Join(dir, "seed")
	if err := os.WriteFile(seedFile, []byte(strings.Repeat("cd", 32)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var key struct{ Address string }
	cli(t, &key, "keygen", "-seed-file", seedFile)
	file := filepath.Join(dir, "series.json")
	if err := os.WriteFile(file, []byte(runOut(t, "series", "new", "-mints", "8", "-pool-size", "4", "-variable", "-part-size", "1000000", "-issuer", key.Address)), 0o600); err != nil {
		t.Fatal(err)
	}
	type row struct {
		Part      uint64
		First     uint64 `json:"first_serial"`
		Last      uint64 `json:"last_serial"`
		FirstPool uint64 `json:"first_pool"`
		LastPool  uint64 `json:"last_pool"`
		Mints     uint64
		Units     uint64
		ID        string `json:"part_id"`
	}
	var info struct {
		SupplyType string `json:"supply_type"`
		PartSize   uint64 `json:"part_size"`
		Pools      []struct{ Index uint64 }
		Parts      []row
	}
	out := runOut(t, "series", "info", file)
	cli(t, &info, "series", "info", file)
	if info.SupplyType != "variable" || info.PartSize != 1000000 || len(info.Pools) != 2 || len(info.Parts) != 10 {
		t.Fatalf("series info:\n%s", out)
	}
	if p := info.Parts[1]; p.Part != 2 || p.First != 8 || p.Last != 1000007 || p.FirstPool != 2 || p.LastPool != 250001 || p.Mints != 1000000 || p.Units != 100000000 {
		t.Fatalf("part 2: %+v", p)
	}
	if len(out) > 8000 {
		t.Fatalf("series info is %d bytes for parts of a million mints: not ranges", len(out))
	}
	var parts struct{ Parts []row }
	cli(t, &parts, "series", "parts", file, "-from", "2", "-to", "2")
	if len(parts.Parts) != 1 || parts.Parts[0] != info.Parts[1] {
		t.Fatalf("series parts %+v, series info %+v", parts.Parts, info.Parts[1])
	}

	rel := filepath.Join(dir, "part2.json")
	out = runOut(t, "series", "release", file, "-part", "2", "-seed-file", seedFile)
	if err := os.WriteFile(rel, []byte(out), 0o600); err != nil {
		t.Fatal(err)
	}
	var r struct {
		Series, Previous, Witness string
		row
	}
	cli(t, &r, "series", "release", file, "-part", "2", "-seed-file", seedFile)
	if r.Witness == "" || r.Previous == "" || r.row != info.Parts[1] {
		t.Fatalf("release:\n%s", out)
	}
	var check struct {
		Valid     bool
		ID        string `json:"part_id"`
		First     uint64 `json:"first_serial"`
		Last      uint64 `json:"last_serial"`
		FirstPool uint64 `json:"first_pool"`
		LastPool  uint64 `json:"last_pool"`
	}
	cli(t, &check, "series", "release", file, "-check", rel)
	if !check.Valid || check.ID != r.ID || check.First != 8 || check.Last != 1000007 || check.FirstPool != 2 || check.LastPool != 250001 {
		t.Fatalf("check: %+v", check)
	}

	fixed := filepath.Join(dir, "fixed.json")
	if err := os.WriteFile(fixed, []byte(runOut(t, "series", "new", "-mints", "8")), 0o600); err != nil {
		t.Fatal(err)
	}
	if out := runOut(t, "series", "info", fixed); strings.Contains(out, "part") || strings.Contains(out, "supply_type") {
		t.Fatalf("fixed series info gained part keys:\n%s", out)
	}
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
