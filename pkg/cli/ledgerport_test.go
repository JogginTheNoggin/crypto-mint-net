package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mineEntry claims serial on the node at root and works the entry that
// holds it into pool 1's selected ledger.
func mineEntry(t *testing.T, root, seed, serial string) {
	t.Helper()
	r := []string{"-root", root}
	cli(t, nil, append([]string{"claim", "-seed-file", seedFile(t, seed), "-serial", serial}, r...)...)
	var built struct{ Hash string }
	cli(t, &built, append([]string{"build", "-pool", "1"}, r...)...)
	cli(t, nil, append([]string{"work", "-pool", "1", "-entry", built.Hash, "-tries", "4000"}, r...)...)
}

// ledgerHeight is pool 1's selected height on the node at root.
func ledgerHeight(t *testing.T, root string) int {
	t.Helper()
	var lp struct{ Height int }
	cli(t, &lp, "ledger", "-pool", "1", "-root", root)
	return lp.Height
}

// D188: a stretch of a ledger exported from one node imports into
// another, which checks every entry in full; importing it again keeps
// nothing new, a changed item is refused, and a range that starts past
// what the node holds is refused for its unknown parent.
func TestLedgerExportImport(t *testing.T) {
	src, dst, gap := t.TempDir(), t.TempDir(), t.TempDir()
	for _, root := range []string{src, dst, gap} {
		cli(t, nil, "init", "-example", "-root", root, "-listen", "127.0.0.1:0", "-p2p", "off")
		stop := startNode(t, root+"/policies/default")
		defer stop()
	}
	mineEntry(t, src, strings.Repeat("a1", 32), "5")
	mineEntry(t, src, strings.Repeat("b2", 32), "6")
	if h := ledgerHeight(t, src); h != 2 {
		t.Fatalf("source height %d, want 2", h)
	}

	file := filepath.Join(t.TempDir(), "ledger.json")
	if out := runOut(t, "ledger", "export", "-pool", "1", "-root", src, "-out", file); !strings.Contains(out, "wrote 2 entries") {
		t.Fatalf("export: %s", out)
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	var f ledgerFile
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatalf("the ledger file is not JSON: %v\n%s", err, raw)
	}
	if f.Kind != LedgerFileKind || f.Height != 2 || f.From != 1 || f.To != 2 || len(f.Entries) != 2 ||
		f.Entries[0].Height != 1 || f.Entries[1].Parent != f.Entries[0].Hash || f.Entries[1].Hash != f.Tip || f.Entries[0].TxCount != 1 {
		t.Fatalf("ledger file %+v", f)
	}

	// -to stops the range; -series names the series.
	one := filepath.Join(t.TempDir(), "one.json")
	runOut(t, "ledger", "export", "-pool", "1", "-to", "1", "-series", f.Series.String(), "-root", src, "-out", one)
	var f1 ledgerFile
	if b, _ := os.ReadFile(one); json.Unmarshal(b, &f1) != nil || len(f1.Entries) != 1 || f1.To != 1 || f1.Entries[0].Hash != f.Entries[0].Hash {
		t.Fatalf("-to 1: %+v", f1)
	}

	// Into an empty node: every entry is checked and kept.
	var res struct {
		Kept, Known int
		Refused     []struct{ Reason string }
		Switched    bool
	}
	cli(t, &res, "ledger", "import", file, "-root", dst)
	if res.Kept != 2 || res.Known != 0 || len(res.Refused) != 0 || !res.Switched {
		t.Fatalf("import %+v", res)
	}
	if h := ledgerHeight(t, dst); h != 2 {
		t.Fatalf("imported height %d, want 2", h)
	}
	// Again: nothing new.
	cli(t, &res, "ledger", "import", file, "-root", dst)
	if res.Kept != 0 || res.Known != 2 || len(res.Refused) != 0 {
		t.Fatalf("second import %+v", res)
	}

	// From standard input, with the first entry's bytes changed: refused,
	// and so is the second, whose parent the node now lacks.
	bad := f
	bad.Entries = append([]ledgerFileItem(nil), f.Entries...)
	item := append([]byte(nil), bad.Entries[0].Item...)
	item[len(item)/2] ^= 0xff
	bad.Entries[0].Item = item
	badFile := filepath.Join(t.TempDir(), "bad.json")
	b, _ := json.Marshal(bad)
	if err := os.WriteFile(badFile, b, 0o600); err != nil {
		t.Fatal(err)
	}
	cli(t, &res, "ledger", "import", badFile, "-root", gap)
	if res.Kept != 0 || len(res.Refused) != 2 {
		t.Fatalf("tampered import %+v", res)
	}

	// Only height 2: its parent is unknown to a node with nothing.
	two := filepath.Join(t.TempDir(), "two.json")
	runOut(t, "ledger", "export", "-pool", "1", "-from", "2", "-root", src, "-out", two)
	cli(t, &res, "ledger", "import", two, "-root", gap)
	if res.Kept != 0 || len(res.Refused) != 1 {
		t.Fatalf("gap import %+v", res)
	}
	if h := ledgerHeight(t, gap); h != 0 {
		t.Fatalf("gap height %d, want 0", h)
	}

	// A file that is not a ledger file is refused before the node is
	// asked.
	if _, err := runErr("ledger", "import", file+".missing", "-root", dst); err == nil {
		t.Fatal("a missing file imported")
	}
	notLedger := filepath.Join(t.TempDir(), "x.json")
	os.WriteFile(notLedger, []byte(`{"kind":"poo peers","format":1}`), 0o600)
	if out, err := runErr("ledger", "import", notLedger, "-root", dst); err == nil || !strings.Contains(err.Error(), "not a ledger file") {
		t.Fatalf("a peers file imported: %v %s", err, out)
	}
	// Pool 0 is a pool like any other: -pool 0 exports it (empty here).
	zero := filepath.Join(t.TempDir(), "zero.json")
	if out := runOut(t, "ledger", "export", "-pool", "0", "-root", src, "-out", zero); !strings.Contains(out, "wrote 0 entries") {
		t.Fatalf("-pool 0: %s", out)
	}
	var f0 ledgerFile
	if b, _ := os.ReadFile(zero); json.Unmarshal(b, &f0) != nil || f0.Pool != 0 || len(f0.Entries) != 0 {
		t.Fatalf("-pool 0 file: %+v", f0)
	}
	// Export needs -pool, and view-only mode refuses both.
	if _, err := runErr("ledger", "export", "-root", src); err == nil {
		t.Fatal("export without -pool")
	}
	if _, err := runErr("-view-only", "ledger", "import", file, "-root", dst); err == nil {
		t.Fatal("view-only imported")
	}
}
