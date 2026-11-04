package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/nodepolicy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/series"
)

// D170: poo init with no series follows the built-in reserve series, and
// says so; -no-series follows none.
func TestInitWritesTheReserveSeries(t *testing.T) {
	root := t.TempDir()
	out := runOut(t, "init", "-root", root, "-p2p", "off")
	if !strings.Contains(out, series.ReserveID) || !strings.Contains(out, "the built-in reserve series") {
		t.Fatalf("init:\n%s", out)
	}
	p, err := nodepolicy.Load(filepath.Join(root, "policies", "default", nodepolicy.FileName))
	must(t, err)
	if len(p.Series) != 1 || len(p.Follow) != 1 {
		t.Fatalf("%+v", p)
	}
	s, err := series.New(p.Series[0].Params)
	must(t, err)
	if !series.IsReserve(s.ID()) || p.Follow[0].Series == nil || *p.Follow[0].Series != s.ID() {
		t.Fatalf("not the reserve series: %s", s.ID())
	}

	root = t.TempDir()
	runOut(t, "init", "-no-series", "-root", root, "-p2p", "off")
	p, err = nodepolicy.Load(filepath.Join(root, "policies", "default", nodepolicy.FileName))
	must(t, err)
	if len(p.Series) != 0 {
		t.Fatalf("-no-series: %+v", p.Series)
	}
	if _, err := runErr("init", "-no-series", "-example", "-root", t.TempDir()); err == nil {
		t.Fatal("-no-series with -example")
	}

	// -example alone is the worked example only; -reserve adds the reserve.
	root = t.TempDir()
	runOut(t, "init", "-example", "-reserve", "-root", root, "-p2p", "off")
	p, err = nodepolicy.Load(filepath.Join(root, "policies", "default", nodepolicy.FileName))
	must(t, err)
	if len(p.Series) != 2 {
		t.Fatalf("-example -reserve: %d series", len(p.Series))
	}
}

// D170, Red's condition 1 as tool policy: a series with no witness ceiling,
// or one above 4 MiB, is refused by poo series new and poo init -series
// unless -any-ceiling forces it.
func TestToolsRefuseAMissingOrHugeCeiling(t *testing.T) {
	var p series.Params
	must(t, json.Unmarshal([]byte(runOut(t, "series", "new")), &p))
	if p.MaxWitness != series.ReserveMaxWitness {
		t.Fatalf("series new's default ceiling is %d", p.MaxWitness)
	}
	for _, mw := range []string{"0", "4194305"} {
		if out, err := runErr("series", "new", "-max-witness", mw); err == nil || !strings.Contains(err.Error(), "-any-ceiling") {
			t.Fatalf("-max-witness %s: %v\n%s", mw, err, out)
		}
		runOut(t, "series", "new", "-max-witness", mw, "-any-ceiling")
	}
	runOut(t, "series", "new", "-max-witness", "4194304")

	file := filepath.Join(t.TempDir(), "s.json")
	must(t, os.WriteFile(file, []byte(runOut(t, "series", "new", "-max-witness", "0", "-any-ceiling")), 0o644))
	if _, err := runErr("init", "-series", file, "-root", t.TempDir(), "-p2p", "off"); err == nil || !strings.Contains(err.Error(), "no witness ceiling") {
		t.Fatalf("init -series with no ceiling: %v", err)
	}
	runOut(t, "init", "-series", file, "-any-ceiling", "-root", t.TempDir(), "-p2p", "off")
}

// D170, Red's condition 5: poo crypto shows each algorithm's largest k under
// a ceiling, the reserve series' by default.
func TestCryptoMaxK(t *testing.T) {
	type alg struct {
		Name string `json:"name"`
		Max  int    `json:"max_signatures_per_witness"`
	}
	var v struct {
		MaxWitness int   `json:"max_witness"`
		Algs       []alg `json:"key_algs"`
	}
	must(t, json.Unmarshal([]byte(runOut(t, "crypto")), &v))
	got := map[string]int{}
	for _, a := range v.Algs {
		got[a.Name] = a.Max
	}
	if v.MaxWitness != 131072 || got["ml-dsa-87"] != 16 || got["slh-dsa-sha2-128s"] != 16 || got["slh-dsa-sha2-256s"] != 4 {
		t.Fatalf("%+v", v)
	}
	must(t, json.Unmarshal([]byte(runOut(t, "crypto", "-max-witness", "65536")), &v))
	for _, a := range v.Algs {
		got[a.Name] = a.Max
	}
	if got["ml-dsa-87"] != 9 || got["slh-dsa-sha2-128s"] != 8 || got["slh-dsa-sha2-256s"] != 2 {
		t.Fatalf("64 KiB: %+v", got)
	}
	must(t, json.Unmarshal([]byte(runOut(t, "crypto", "-max-witness", "0")), &v))
	for _, a := range v.Algs {
		if a.Max != 16 {
			t.Fatalf("no ceiling: %+v", a)
		}
	}
}

// D170: poo series info names the built-in reserve series on its full id.
func TestSeriesInfoNamesTheReserve(t *testing.T) {
	file := filepath.Join(t.TempDir(), "r.json")
	b, err := json.Marshal(series.Reserve())
	must(t, err)
	must(t, os.WriteFile(file, b, 0o644))
	var info struct {
		ID      string `json:"series_id"`
		Builtin string `json:"builtin"`
	}
	must(t, json.Unmarshal([]byte(runOut(t, "series", "info", file)), &info))
	if info.ID != series.ReserveID || info.Builtin != "reserve" {
		t.Fatalf("%+v", info)
	}
	p := series.Reserve()
	p.Supply--
	b, _ = json.Marshal(p)
	must(t, os.WriteFile(file, b, 0o644))
	info.Builtin = ""
	must(t, json.Unmarshal([]byte(runOut(t, "series", "info", file)), &info))
	if info.Builtin != "" {
		t.Fatalf("a look-alike is named the reserve: %+v", info)
	}
}

// GUI-028: poo series parts and poo series release on a far part stop when
// the command's context is done, as a GUI job's Cancel does.
func TestFarPartIsCancellable(t *testing.T) {
	var key struct {
		Address string `json:"address"`
	}
	seed := filepath.Join(t.TempDir(), "seed")
	must(t, os.WriteFile(seed, []byte(strings.Repeat("5e", 32)), 0o600))
	must(t, json.Unmarshal([]byte(runOut(t, "keygen", "-seed-file", seed)), &key))
	file := filepath.Join(t.TempDir(), "v.json")
	must(t, os.WriteFile(file, []byte(runOut(t, "series", "new", "-mints", "8", "-pool-size", "4", "-variable", "-part-size", "4", "-issuer", key.Address)), 0o644))
	for _, args := range [][]string{
		{"series", "parts", file, "-from", "1000000000000"},
		{"series", "release", file, "-part", "1000000000000", "-seed-file", seed},
	} {
		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		start := time.Now()
		var out, errOut bytes.Buffer
		err := Run(ctx, args, Stdio{Out: &out, Err: &errOut})
		cancel()
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("%v: %v", args, err)
		}
		if d := time.Since(start); d > 10*time.Second {
			t.Errorf("%v took %v to stop", args, d)
		}
	}
}
