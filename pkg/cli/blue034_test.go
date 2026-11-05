package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/node"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/rpc"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/watch"
)

// BLUE-034 (RED-097, RED-098, GUI-031).

// fakePages answers export pages from a list, one per call.
type fakePages struct {
	pages []watch.ExportPage
	asked []int // the to of each call
}

func (f *fakePages) LedgerExport(_ *hash.Hash, _ uint64, _, to int) (watch.ExportPage, error) {
	f.asked = append(f.asked, to)
	p := f.pages[0]
	f.pages = f.pages[1:]
	return p, nil
}

func hashN(i int) hash.Hash {
	var h hash.Hash
	h[0], h[31] = byte(i), 0x34
	return h
}

// page makes a page of heights from..to of a ledger height high whose
// entry at height i has hash hashN(i).
func page(from, to, height, next int) watch.ExportPage {
	p := watch.ExportPage{Height: height, Tip: hashN(height), Next: next, Entries: []watch.ExportEntry{}, Items: [][]byte{}}
	for i := from; i <= to; i++ {
		p.Entries = append(p.Entries, watch.ExportEntry{Height: i, Hash: hashN(i), Parent: hashN(i - 1)})
		p.Items = append(p.Items, []byte{byte(i)})
	}
	return p
}

// RED-097: an export whose ledger becomes shorter between pages fails
// rather than ending short; one that stays whole ends where its header
// says, and later pages are asked only up to the header's end.
func TestBlue034ExportEndsWhereItsHeaderSays(t *testing.T) {
	for name, tc := range map[string]struct {
		pages []watch.ExportPage
		ok    bool
	}{
		"empty page after more was promised": {[]watch.ExportPage{page(1, 2, 5, 3), page(1, 0, 2, 0)}, false},
		"ends below the header's end":        {[]watch.ExportPage{page(1, 2, 5, 3), page(3, 4, 5, 0)}, false},
		"a page from another height":         {[]watch.ExportPage{page(1, 2, 5, 3), page(4, 5, 5, 0)}, false},
		"whole":                              {[]watch.ExportPage{page(1, 2, 5, 3), page(3, 5, 5, 0)}, true},
		"grows meanwhile":                    {[]watch.ExportPage{page(1, 2, 5, 3), page(3, 5, 9, 0)}, true},
	} {
		t.Run(name, func(t *testing.T) {
			f := &fakePages{pages: tc.pages}
			var buf bytes.Buffer
			n, err := writeLedgerFile(&buf, f, nil, 1, 1, 0)
			if tc.ok {
				if err != nil || n != 5 {
					t.Fatalf("n %d, %v", n, err)
				}
				var lf ledgerFile
				if err := json.Unmarshal(buf.Bytes(), &lf); err != nil || lf.To != 5 || len(lf.Entries) != 5 {
					t.Fatalf("file %+v %v", lf, err)
				}
				if f.asked[1] != 5 {
					t.Fatalf("the second page was asked up to %d, not the header's 5", f.asked[1])
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), "switched ledgers") {
				t.Fatalf("n %d, %v", n, err)
			}
		})
	}
}

// fakeImporter takes batches until fail, then fails.
type fakeImporter struct {
	batches []int
	fail    int // the batch that fails (from 1); 0 never
	cancel  func()
}

func (f *fakeImporter) LedgerImport(req rpc.LedgerImportRequest) (node.ImportResult, error) {
	f.batches = append(f.batches, len(req.Items))
	if len(f.batches) == f.fail {
		return node.ImportResult{}, errors.New("connection refused")
	}
	if f.cancel != nil {
		f.cancel()
	}
	return node.ImportResult{Kept: len(req.Items), Switched: true}, nil
}

// ledgerJSON is a ledger file of n items of size bytes each.
func ledgerJSON(n, size int) string {
	var b strings.Builder
	fmt.Fprintf(&b, `{"kind":"poo-ledger","format":1,"series":"%s","pool":1,"entries":[`, hashN(1))
	item, _ := json.Marshal(make([]byte, size))
	for i := range n {
		if i > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, `{"height":%d,"item":%s}`, i+1, item)
	}
	b.WriteString("]}")
	return b.String()
}

// GUI-031: an import that fails after a batch was kept returns that
// batch's totals with the error, and a stop ends it between batches.
func TestBlue034ImportReportsWhatWasKept(t *testing.T) {
	file := ledgerJSON(node.MaxImportItems+10, 1)
	f := &fakeImporter{fail: 2}
	got, err := importLedger(context.Background(), strings.NewReader(file), func() (ledgerImporter, error) { return f, nil })
	if err == nil || got.Sent != node.MaxImportItems || got.Kept != node.MaxImportItems || !got.Switched {
		t.Fatalf("after a failed second batch: %+v %v", got, err)
	}
	if !strings.Contains(err.Error(), "the first 256 entries were sent") {
		t.Fatalf("error %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	f = &fakeImporter{cancel: cancel}
	got, err = importLedger(ctx, strings.NewReader(file), func() (ledgerImporter, error) { return f, nil })
	if err == nil || !strings.Contains(err.Error(), "stopped") || len(f.batches) != 1 || got.Sent != node.MaxImportItems {
		t.Fatalf("stopped: %+v %v, batches %v", got, err, f.batches)
	}
	// A batch holds at most MaxImportBatchBytes past its first item.
	f = &fakeImporter{}
	big := rpc.MaxImportBatchBytes / 3
	if _, err := importLedger(context.Background(), strings.NewReader(ledgerJSON(7, big)), func() (ledgerImporter, error) { return f, nil }); err != nil {
		t.Fatal(err)
	}
	for _, n := range f.batches {
		if n > 3 {
			t.Fatalf("batches %v", f.batches)
		}
	}
}

// GUI-031 through the command: the partial totals are printed, with
// stopped, and the exit is not 0.
func TestBlue034ImportPrintsPartialTotals(t *testing.T) {
	var out bytes.Buffer
	o := &outputFlags{format: new(string), wide: new(bool)}
	*o.format = "json"
	rep := LedgerImportReply{ImportResult: node.ImportResult{Kept: 2, Switched: true}, Sent: 2, Stopped: "connection refused"}
	if err := o.write(&out, rep); err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(out.Bytes(), &m); err != nil || m["kept"] != 2.0 || m["sent"] != 2.0 || m["stopped"] != "connection refused" || m["switched"] != true {
		t.Fatalf("reply %s", out.String())
	}
}

// RED-098: -dry-run checks a file against itself with no node, using
// node.json for the hashes; a changed item is a problem.
func TestBlue034DryRun(t *testing.T) {
	src := t.TempDir()
	cli(t, nil, "init", "-example", "-root", src, "-listen", "127.0.0.1:0", "-p2p", "off")
	stop := startNode(t, src+"/policies/default")
	mineEntry(t, src, strings.Repeat("a1", 32), "5")
	mineEntry(t, src, strings.Repeat("b2", 32), "6")
	file := filepath.Join(t.TempDir(), "ledger.json")
	runOut(t, "ledger", "export", "-pool", "1", "-root", src, "-out", file)
	stop()

	var rep LedgerDryRun
	cli(t, &rep, "ledger", "import", "-dry-run", file, "-root", src)
	if rep.Entries != 2 || rep.Decoded != 2 || !rep.SeriesKnown || rep.ProblemCount != 0 {
		t.Fatalf("dry run %+v", rep)
	}
	raw, _ := os.ReadFile(file)
	var f ledgerFile
	json.Unmarshal(raw, &f)
	f.Entries[1].TxCount = 9
	f.Tip = hashN(7)
	b, _ := json.Marshal(f)
	bad := filepath.Join(t.TempDir(), "bad.json")
	os.WriteFile(bad, b, 0o600)
	out, err := runErr("ledger", "import", "-dry-run", bad, "-root", src)
	if !errors.Is(err, ErrDryRunProblems) || !strings.Contains(out, "transactions, its summary 9") || !strings.Contains(out, "the file's tip") {
		t.Fatalf("%v\n%s", err, out)
	}
}
