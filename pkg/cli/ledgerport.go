package cli

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/node"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/rpc"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/watch"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/weight"
)

// Ledger files (D188): poo ledger export writes a stretch of a pool's
// selected ledger, oldest first, as raw entry items with a readable
// summary of each; poo ledger import hands the items to the node, which
// checks every one as it checks an entry a peer sent. Nothing in a ledger
// file is trusted.

// LedgerFileKind and LedgerFileFormat name a ledger file.
const (
	LedgerFileKind   = "poo ledger"
	LedgerFileFormat = 1
)

// ledgerFile is the file's JSON form.
type ledgerFile struct {
	Kind     string           `json:"kind"`
	Format   int              `json:"format"`
	Series   hash.Hash        `json:"series"`
	Pool     uint64           `json:"pool"`
	From     int              `json:"from"`
	To       int              `json:"to"`
	Height   int              `json:"height"`
	Tip      hash.Hash        `json:"tip"`
	Weight   weight.Weight    `json:"weight"`
	Exported string           `json:"exported"`
	Entries  []ledgerFileItem `json:"entries"`
}

// ledgerFileItem is one entry: its summary and its raw item (the entry's
// byte form and layer nonces, as a peer sends them; base64).
type ledgerFileItem struct {
	watch.ExportEntry
	Item []byte `json:"item"`
}

func ledgerPortCmd(args []string, out io.Writer) error {
	switch args[0] {
	case "export":
		return ledgerExportCmd(args[1:], out)
	case "import":
		return ledgerImportCmd(args[1:], out)
	}
	return fmt.Errorf("unknown ledger command %q", args[0])
}

func ledgerExportCmd(args []string, out io.Writer) error {
	fs := newFlagSet("ledger export", out)
	w := addWhere(fs)
	series := fs.String("series", "", "the series (default: the only one the node follows)")
	pool := fs.Uint64("pool", 0, "the pool to export (needed)")
	from := fs.Int("from", 1, "the first height to export")
	to := fs.Int("to", 0, "the last height to export (default: the tip)")
	file := fs.String("out", "", "write the ledger file here (default: print it)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("poo ledger export takes flags only")
	}
	// Pools are numbered from 0, so ask whether -pool was given rather
	// than whether it is 0.
	gotPool := false
	fs.Visit(func(f *flag.Flag) { gotPool = gotPool || f.Name == "pool" })
	if !gotPool {
		return errors.New("poo ledger export needs -pool")
	}
	if *from < 1 || (*to != 0 && *to < *from) {
		return fmt.Errorf("-from %d -to %d: from is at least 1 and to, when given, is at least from", *from, *to)
	}
	var sid *hash.Hash
	if *series != "" {
		h, err := hash.Parse(*series)
		if err != nil {
			return fmt.Errorf("-series: %w", err)
		}
		sid = &h
	}
	cl, err := dialNode(out, w.resolve())
	if err != nil {
		return err
	}
	dst, finish, err := ledgerOut(out, *file)
	if err != nil {
		return err
	}
	n, err := writeLedgerFile(dst, cl, sid, *pool, *from, *to)
	if err = finish(err); err != nil {
		return err
	}
	if *file != "" {
		fmt.Fprintf(out, "wrote %d entries to %s\n", n, *file)
	}
	return nil
}

// ledgerOut opens where the file goes: standard output, or a temporary
// file renamed into place when the export is whole.
func ledgerOut(out io.Writer, file string) (*bufio.Writer, func(error) error, error) {
	if file == "" {
		bw := bufio.NewWriter(out)
		return bw, func(err error) error {
			if ferr := bw.Flush(); err == nil {
				err = ferr
			}
			return err
		}, nil
	}
	tmp, err := os.CreateTemp(filepath.Dir(file), ".ledger-*.tmp")
	if err != nil {
		return nil, nil, err
	}
	bw := bufio.NewWriter(tmp)
	return bw, func(err error) error {
		if err == nil {
			err = bw.Flush()
		}
		if err == nil {
			err = tmp.Sync()
		}
		if cerr := tmp.Close(); err == nil {
			err = cerr
		}
		if err == nil {
			err = os.Chmod(tmp.Name(), 0o600)
		}
		if err == nil {
			err = os.Rename(tmp.Name(), file)
		}
		if err != nil {
			os.Remove(tmp.Name())
		}
		return err
	}, nil
}

// writeLedgerFile streams the export page by page, so a long ledger never
// sits whole in memory. It returns the number of entries written.
func writeLedgerFile(dst io.Writer, cl *rpc.Client, sid *hash.Hash, pool uint64, from, to int) (int, error) {
	page, err := cl.LedgerExport(sid, pool, from, to)
	if err != nil {
		return 0, err
	}
	last := to
	if last == 0 || last > page.Height {
		last = page.Height
	}
	head := ledgerFile{Kind: LedgerFileKind, Format: LedgerFileFormat, Series: page.Series, Pool: pool, From: from, To: last,
		Height: page.Height, Tip: page.Tip, Weight: page.Weight, Exported: time.Now().UTC().Format(time.RFC3339), Entries: []ledgerFileItem{}}
	hb, err := json.MarshalIndent(head, "", "  ")
	if err != nil {
		return 0, err
	}
	// The header without its closing "entries": [] and brace, then the
	// entries one per line.
	hb = hb[:len(hb)-len("[]\n}")-1]
	if _, err := fmt.Fprintf(dst, "%s[", hb); err != nil {
		return 0, err
	}
	n := 0
	var lastHash hash.Hash
	for {
		if page.Unreadable > 0 {
			return n, fmt.Errorf("the entry at height %d cannot be read back from the node's log; poo doctor says more", from+len(page.Items))
		}
		for i, b := range page.Items {
			line, err := json.Marshal(ledgerFileItem{ExportEntry: page.Entries[i], Item: b})
			if err != nil {
				return n, err
			}
			sep := ","
			if n == 0 {
				sep = ""
			}
			if _, err := fmt.Fprintf(dst, "%s\n    %s", sep, line); err != nil {
				return n, err
			}
			n++
			lastHash = page.Entries[i].Hash
		}
		if page.Next == 0 || (to > 0 && page.Next > to) {
			break
		}
		from = page.Next
		if page, err = cl.LedgerExport(&head.Series, pool, from, to); err != nil {
			return n, err
		}
		// Each page must carry on from the last entry written: if the
		// selected ledger moved under the export, the file would mix two
		// ledgers.
		if len(page.Entries) > 0 && page.Entries[0].Parent != lastHash {
			return n, errors.New("the node switched ledgers during the export; run it again")
		}
	}
	_, err = fmt.Fprint(dst, "\n  ]\n}\n")
	return n, err
}

func ledgerImportCmd(args []string, out io.Writer) error {
	fs := newFlagSet("ledger import", out)
	w := addWhere(fs)
	o := addOutput(fs)
	pos, err := parseMixed(fs, args)
	if err != nil {
		return err
	}
	if err := o.check(); err != nil {
		return err
	}
	if len(pos) != 1 {
		return errors.New("usage: poo ledger import FILE|-")
	}
	var r io.Reader
	if pos[0] == "-" {
		r = inputOf(out)
	} else {
		fh, err := os.Open(pos[0])
		if err != nil {
			return err
		}
		defer fh.Close()
		r = fh
	}
	var f ledgerFile
	if err := json.NewDecoder(r).Decode(&f); err != nil {
		return fmt.Errorf("not a ledger file: %w", err)
	}
	if f.Kind != LedgerFileKind || f.Format != LedgerFileFormat {
		return fmt.Errorf("not a ledger file of format %d (kind %q, format %d)", LedgerFileFormat, f.Kind, f.Format)
	}
	cl, err := dialNode(out, w.resolve())
	if err != nil {
		return err
	}
	total := node.ImportResult{}
	var batch [][]byte
	size, base := 0, 0
	send := func() error {
		if len(batch) == 0 {
			return nil
		}
		res, err := cl.LedgerImport(rpc.LedgerImportRequest{Series: f.Series, Pool: f.Pool, Items: batch})
		if err != nil {
			return err
		}
		total.Kept += res.Kept
		total.Known += res.Known
		total.LayersKept += res.LayersKept
		total.LayersKnown += res.LayersKnown
		total.Switched = total.Switched || res.Switched
		for _, rf := range res.Refused {
			rf.Index += base
			total.Refused = append(total.Refused, rf)
		}
		base += len(batch)
		batch, size = nil, 0
		return nil
	}
	for _, it := range f.Entries {
		if len(batch) == node.MaxImportItems || (len(batch) > 0 && size+len(it.Item) > watch.ExportBytes) {
			if err := send(); err != nil {
				return err
			}
		}
		batch = append(batch, it.Item)
		size += len(it.Item)
	}
	if err := send(); err != nil {
		return err
	}
	return o.write(out, total)
}
