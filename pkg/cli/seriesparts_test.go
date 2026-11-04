package cli

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/nodepolicy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/rpc"
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
			Released    uint64 `json:"mints_released"`
			ID          string `json:"part_id"`
		}
	}
	cli(t, &parts, "series", "parts", file)
	if len(parts.Parts) != 3 || parts.Parts[1].FirstSerial != 8 || parts.Parts[1].LastSerial != 11 || parts.Parts[2].LastSerial != 15 ||
		parts.Parts[0].Released != 8 || parts.Parts[2].Released != 16 {
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
	// -check - reads the release from standard input (GUI-027).
	var piped bytes.Buffer
	b, _ := os.ReadFile(rel)
	if err := run([]string{"series", "release", file, "-check", "-"}, &stdio{Writer: &piped, in: bufio.NewReader(bytes.NewReader(b))}); err != nil || !strings.Contains(piped.String(), `"valid": true`) {
		t.Fatalf("-check -: %v\n%s", err, piped.String())
	}
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

	// A node policy that lists it with its issuer witness follows part 1
	// (D166).
	s, err := readSeriesFile(file)
	if err != nil {
		t.Fatal(err)
	}
	pol := nodepolicy.Default()
	pol.Series = []nodepolicy.SeriesInfo{{Params: s.Params(), IssuerWitness: signed.Witness}}
	if acc, ref := pol.Accepted(); len(acc) != 1 || len(ref) != 0 || acc[0].Series.Released() != 1 {
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

// A node keeps a variable series listed without its issuer witness: poo
// init says what it needs, and poo doctor says to add the witness, not to
// correct the series (GREEN-049, D166); and poo series parts -from past
// the last part names the cap.
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
	if !strings.Contains(init, "is a variable supply series") || !strings.Contains(init, "issuer_witness") {
		t.Errorf("init:\n%s", init)
	}
	c := checkNamed(doctor(io.Discard, node), "series")
	if c.Result != checkWarn || strings.Contains(c.Fix, "correct or remove") || !strings.Contains(c.Fix, "issuer_witness") {
		t.Errorf("doctor series: %+v", c)
	}
	// With a fixed series beside it, init says nothing extra.
	fixed := runOut(t, "init", "-dir", t.TempDir(), "-p2p", "off", "-example")
	if strings.Contains(fixed, "note:") {
		t.Errorf("init of a fixed series:\n%s", fixed)
	}
}

// D166: a node follows a variable series signed by its issuer. A claim on
// part 2 waits until the issuer's release of part 2 is added; add-release
// keeps it in node.json and the node reloads to follow part 2's pools.
func TestVariableSeriesOnANode(t *testing.T) {
	dir := t.TempDir()
	issuerSeed := filepath.Join(dir, "issuer")
	if err := os.WriteFile(issuerSeed, []byte(strings.Repeat("5e", 32)), 0o600); err != nil {
		t.Fatal(err)
	}
	var key struct{ Address string }
	cli(t, &key, "keygen", "-seed-file", issuerSeed)
	file := filepath.Join(dir, "series.json")
	if err := os.WriteFile(file, []byte(runOut(t, "series", "new", "-mints", "8", "-pool-size", "4", "-variable", "-part-size", "4", "-issuer", key.Address)), 0o600); err != nil {
		t.Fatal(err)
	}
	signed := filepath.Join(dir, "signed.json")
	if err := os.WriteFile(signed, []byte(runOut(t, "series", "sign", file, "-seed-file", issuerSeed)), 0o600); err != nil {
		t.Fatal(err)
	}
	// poo series info reads the signed file too, with the supply type,
	// part size and no max_parts for an uncapped series; -h is help, not a
	// file (GUI-GREEN-051).
	if a, b := runOut(t, "series", "info", signed), runOut(t, "series", "info", file); a != b || !strings.Contains(a, `"supply_type": "variable"`) || !strings.Contains(a, `"part_size": 4`) || strings.Contains(a, "max_parts") {
		t.Fatalf("series info of the signed file:\n%s\nof the params file:\n%s", a, b)
	}
	if out := runOut(t, "series", "info", "-h"); !strings.Contains(out, "usage: poo series info") {
		t.Fatalf("series info -h: %s", out)
	}
	rel := filepath.Join(dir, "part2.json")
	if err := os.WriteFile(rel, []byte(runOut(t, "series", "release", file, "-part", "2", "-seed-file", issuerSeed)), 0o600); err != nil {
		t.Fatal(err)
	}

	root := t.TempDir()
	r := []string{"-root", root}
	if out := runOut(t, append([]string{"init", "-series", signed, "-listen", "127.0.0.1:0", "-p2p", "off"}, r...)...); strings.Contains(out, "note:") {
		t.Fatalf("init of a signed variable series:\n%s", out)
	}
	stop := startNode(t, filepath.Join(root, "policies", "default"))
	defer stop()
	alice := seedFile(t, strings.Repeat("a1", 32))
	cli(t, nil, append([]string{"claim", "-seed-file", alice, "-serial", "2"}, r...)...)
	if _, err := runErr(append([]string{"claim", "-seed-file", alice, "-serial", "9"}, r...)...); err == nil || !strings.Contains(err.Error(), "waiting for a release") || !strings.Contains(err.Error(), "poo series add-release FILE") || ExitCode(err) != ExitWaiting {
		t.Fatalf("a claim on part 2 before its release: %v (exit %d)", err, ExitCode(err))
	}
	var followed struct{ Series []rpc.SeriesInfo }
	cli(t, &followed, append([]string{"series", "followed"}, r...)...)
	if len(followed.Series) != 1 || followed.Series[0].Variable == nil || followed.Series[0].Variable.Released != 1 || followed.Series[0].Variable.NextPart != 2 {
		t.Fatalf("series followed before the release: %+v", followed)
	}
	var added rpc.ReleaseReply
	cli(t, &added, append([]string{"series", "add-release", rel}, r...)...)
	if !added.Added || !added.Saved || added.Reloading || !added.InPlace || added.Released != 2 || added.Mints != 12 {
		t.Fatalf("add-release: %+v", added)
	}
	var err error
	for i := 0; i < 100; i++ {
		if _, err = runErr(append([]string{"claim", "-seed-file", alice, "-serial", "9"}, r...)...); err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("a claim on part 2 after its release: %v", err)
	}
	cli(t, &followed, append([]string{"series", "followed"}, r...)...)
	if v := followed.Series[0].Variable; v == nil || v.Released != 2 || v.NextPart != 3 || v.Mints != 12 || v.Units == "" || v.Release == nil || v.Release.Part != 2 {
		t.Fatalf("series followed after the release: %+v", v)
	}
	var again rpc.ReleaseReply
	cli(t, &again, append([]string{"series", "add-release", rel}, r...)...)
	if again.Added || again.Saved || again.Released != 2 {
		t.Fatalf("the same release again: %+v", again)
	}
	// GREEN-050: a release that changes nothing says why, and the table
	// of followed series has a row a reader can use.
	var note struct{ Note string }
	cli(t, &note, append([]string{"series", "add-release", rel}, r...)...)
	if !strings.Contains(note.Note, "already keeps") {
		t.Errorf("the same release again: note %q", note.Note)
	}
	rel3 := filepath.Join(dir, "part3.json")
	if err := os.WriteFile(rel3, []byte(runOut(t, "series", "release", file, "-part", "3", "-seed-file", issuerSeed)), 0o600); err != nil {
		t.Fatal(err)
	}
	cli(t, nil, append([]string{"series", "add-release", rel3}, r...)...)
	cli(t, &note, append([]string{"series", "add-release", rel}, r...)...)
	if !strings.Contains(note.Note, "part 2 is already released here: the kept release of part 3") {
		t.Errorf("an older release: note %q", note.Note)
	}
	var table string
	for i := 0; i < 100; i++ {
		// Read through the reload the release started.
		if table, _ = runErr(append([]string{"series", "followed", "-o", "table"}, r...)...); strings.Contains(table, "16") {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !strings.Contains(table, "SUPPLY") || !strings.Contains(table, "variable") || !strings.Contains(table, "RELEASED_THROUGH") || strings.Contains(table, "{") {
		t.Errorf("series followed -o table:\n%s", table)
	}
	p, err := nodepolicy.Load(filepath.Join(root, "policies", "default", nodepolicy.FileName))
	if err != nil || p.Series[0].Release == nil || p.Series[0].Release.Part != 3 {
		t.Fatalf("node.json keeps %+v: %v", p.Series, err)
	}
}

// poo series parts -o table lists one row per part (GREEN-049's idea).
func TestSeriesPartsTable(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "series.json")
	if err := os.WriteFile(file, []byte(runOut(t, "series", "new", "-mints", "8", "-pool-size", "4", "-variable", "-part-size", "4", "-max-parts", "3", "-issuer", strings.Repeat("9d", 32))), 0o600); err != nil {
		t.Fatal(err)
	}
	out := runOut(t, "series", "parts", file, "-o", "table")
	if strings.HasPrefix(strings.TrimSpace(out), "{") || !strings.Contains(out, "PART_ID") || !strings.Contains(out, "\n3 ") {
		t.Fatalf("series parts -o table:\n%s", out)
	}
	if _, err := runErr("series", "parts", file, "-o", "yaml"); err == nil {
		t.Fatal("-o yaml was taken")
	}
}

// A refusal only for a part not yet released exits 3, from the node's
// 409 waiting or a check made here, so the GUI need not read the message;
// anything else that fails exits 1 (GUI Grey, R26).
func TestExitCodeWaiting(t *testing.T) {
	for _, c := range []struct {
		err  error
		want int
	}{
		{nil, 0},
		{errors.New("no"), ExitFailed},
		{fmt.Errorf("serial 9: %w", series.ErrWaiting), ExitWaiting},
		{&rpc.Error{Code: rpc.CodeWaiting, Message: "waiting"}, ExitWaiting},
		{fmt.Errorf("claim: %w", &rpc.Error{Code: rpc.CodeWaiting}), ExitWaiting},
		{&rpc.Error{Code: rpc.CodeInvalid}, ExitFailed},
	} {
		if got := ExitCode(c.err); got != c.want {
			t.Errorf("%v: exit %d, want %d", c.err, got, c.want)
		}
	}
}
