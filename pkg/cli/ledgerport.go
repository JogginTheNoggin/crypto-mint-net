package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/engine"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/node"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/nodepolicy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/rpc"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/series"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/watch"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/weight"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/wire"
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

// ledgerExporter and ledgerImporter are what export and import need of
// the node (*rpc.Client); tests give their own.
type (
	ledgerExporter interface {
		LedgerExport(series *hash.Hash, pool uint64, from, to int) (watch.ExportPage, error)
	}
	ledgerImporter interface {
		LedgerImport(rpc.LedgerImportRequest) (node.ImportResult, error)
	}
)

// writeLedgerFile streams the export page by page, so a long ledger never
// sits whole in memory. It returns the number of entries written.
func writeLedgerFile(dst io.Writer, cl ledgerExporter, sid *hash.Hash, pool uint64, from, to int) (int, error) {
	page, err := cl.LedgerExport(sid, pool, from, to)
	if err != nil {
		return 0, err
	}
	// A start past the tip would write an empty file whose header says
	// from 5 to 1; say so instead (GREEN-058). A pool with no selected
	// entry still exports as an empty file.
	if page.Height > 0 && from > page.Height {
		return 0, fmt.Errorf("-from %d is past the tip: pool %d's selected ledger is %d high", from, pool, page.Height)
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
	lastHeight := 0
	switched := errors.New("the node switched ledgers during the export; run it again")
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
			lastHash, lastHeight = page.Entries[i].Hash, page.Entries[i].Height
		}
		if page.Next == 0 || page.Next > last {
			break
		}
		from = page.Next
		// Later pages stop at the header's end, so the file holds what
		// the header says even if the ledger grows meanwhile (RED-097).
		if page, err = cl.LedgerExport(&head.Series, pool, from, last); err != nil {
			return n, err
		}
		// Each page must carry on from the last entry written: if the
		// selected ledger moved under the export, the file would mix two
		// ledgers. A page with nothing, after one that said more was to
		// come, is a switch to a shorter ledger (RED-097).
		if len(page.Entries) == 0 || page.Entries[0].Parent != lastHash || page.Entries[0].Height != from {
			return n, switched
		}
	}
	// The file must end where its header says: at height To, and at the
	// header's tip when it runs to the tip (RED-097).
	if last > 0 && lastHeight != last || last > 0 && last == head.Height && lastHash != head.Tip {
		return n, switched
	}
	_, err = fmt.Fprint(dst, "\n  ]\n}\n")
	return n, err
}

// importErr says what a failed import batch means: which series and pool
// the file is for when the node does not follow them, and what earlier
// batches already kept, so a runner knows where the import stopped
// (GREEN-058).
func importErr(err error, f ledgerFile, done int, so node.ImportResult) error {
	msg := err.Error()
	switch {
	case strings.Contains(msg, engine.ErrUnknownSeries.Error()):
		err = fmt.Errorf("%w: the file is for series %s, which this node does not know; add the series to the node policy, follow pool %d, then import again", err, f.Series, f.Pool)
	case strings.Contains(msg, engine.ErrNotFollowed.Error()):
		err = fmt.Errorf("%w: the file is for pool %d of series %s; poo follow -series %s -pool %d, then import again", err, f.Pool, f.Series, f.Series, f.Pool)
	}
	if done > 0 {
		// The file is read as it streams, so how many entries it holds in
		// all is not known yet.
		err = fmt.Errorf("%w (the first %d entries were sent: %d kept, %d already held)", err, done, so.Kept, so.Known)
	}
	return err
}

// LedgerImportReply is what poo ledger import prints: the node's totals
// over every batch sent, how many entries were sent, and, when the import
// stopped before the end of the file, why. Batches sent before a stop
// are kept and may have switched the selected ledger, so the totals are
// printed then too (GUI-031).
type LedgerImportReply struct {
	node.ImportResult
	Sent int `json:"sent"`
	// Stopped says why the import ended before the file did: an error, or
	// the command being stopped. The exit status is not 0 then.
	Stopped string `json:"stopped,omitempty"`
}

// LedgerDryRun is what poo ledger import -dry-run prints: the file read
// and checked against itself, with nothing sent to the node (RED-098).
type LedgerDryRun struct {
	Series hash.Hash `json:"series"`
	Pool   uint64    `json:"pool"`
	// Entries counts the items; Decoded those whose bytes read as an
	// entry item.
	Entries int `json:"entries"`
	Decoded int `json:"decoded"`
	// SeriesKnown says node.json knows the series, so the entry hashes
	// could be worked out and compared.
	SeriesKnown bool `json:"series_known"`
	// Problems lists where the items differ from their summaries or the
	// header, or do not chain (at most MaxDryRunProblems; ProblemCount
	// counts them all).
	Problems     []string `json:"problems,omitempty"`
	ProblemCount int      `json:"problem_count,omitempty"`
}

// MaxDryRunProblems caps the problems a dry run lists.
const MaxDryRunProblems = 20

// ErrDryRunProblems is the error of a dry run that found problems.
var ErrDryRunProblems = errors.New("the ledger file does not match itself; nothing was sent")

func ledgerImportCmd(args []string, out io.Writer) error {
	fs := newFlagSet("ledger import", out)
	w := addWhere(fs)
	o := addOutput(fs)
	dry := fs.Bool("dry-run", false, "read and check the file against itself, and print what it holds; send nothing (the node need not run)")
	pos, err := parseMixed(fs, args)
	if err != nil {
		return err
	}
	if err := o.check(); err != nil {
		return err
	}
	if len(pos) != 1 {
		return errors.New("usage: poo ledger import [-dry-run] FILE|-")
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
	if *dry {
		return ledgerDryRun(r, w.resolve(), o, out)
	}
	total, err := importLedger(contextOf(out), r, func() (ledgerImporter, error) { return dialNode(out, w.resolve()) })
	if err != nil {
		if total.Sent == 0 {
			return err
		}
		// Earlier batches are kept: say what they did before the error
		// (GUI-031).
		total.Stopped = err.Error()
		if werr := o.write(out, total); werr != nil {
			return werr
		}
		return err
	}
	return o.write(out, total)
}

// importLedger reads a ledger file from r and sends its items to the
// node in batches, dialling it once the header is read. It stops at the
// first error, or when ctx ends, between batches, returning the totals
// of the batches already sent with the error (GUI-031).
func importLedger(ctx context.Context, r io.Reader, dial func() (ledgerImporter, error)) (LedgerImportReply, error) {
	var (
		f     ledgerFile
		cl    ledgerImporter
		total LedgerImportReply
		batch [][]byte
	)
	size := 0
	send := func() error {
		if len(batch) == 0 {
			return nil
		}
		// A stop ends the import between batches (GUI-031).
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("stopped: %w", err)
		}
		res, err := cl.LedgerImport(rpc.LedgerImportRequest{Series: f.Series, Pool: f.Pool, Items: batch})
		if err != nil {
			return importErr(err, f, total.Sent, total.ImportResult)
		}
		total.Kept += res.Kept
		total.Known += res.Known
		total.LayersKept += res.LayersKept
		total.LayersKnown += res.LayersKnown
		total.Switched = total.Switched || res.Switched
		for _, rf := range res.Refused {
			rf.Index += total.Sent
			total.Refused = append(total.Refused, rf)
		}
		total.Sent += len(batch)
		batch, size = nil, 0
		return nil
	}
	head := func(h ledgerFile) error {
		if h.Kind != LedgerFileKind || h.Format != LedgerFileFormat {
			return fmt.Errorf("not a ledger file of format %d (kind %q, format %d)", LedgerFileFormat, h.Kind, h.Format)
		}
		f = h
		var err error
		cl, err = dial()
		return err
	}
	item := func(it ledgerFileItem) error {
		if len(batch) == node.MaxImportItems || (len(batch) > 0 && size+len(it.Item) > rpc.MaxImportBatchBytes) {
			if err := send(); err != nil {
				return err
			}
		}
		batch = append(batch, it.Item)
		size += len(it.Item)
		return nil
	}
	err := readLedgerFile(r, head, item)
	if err == nil {
		err = send()
	}
	return total, err
}

// ledgerDryRun reads a ledger file and checks it against itself: every
// item decodes, its summary matches it, the items chain, and the last
// one is where the header says. With the series in node.json it works
// out each entry's hash too. Nothing is sent and the node need not run.
func ledgerDryRun(r io.Reader, dir string, o *outputFlags, out io.Writer) error {
	var (
		rep      LedgerDryRun
		f        ledgerFile
		s        *series.Series
		prevHash hash.Hash
		prevH    int
	)
	problem := func(format string, a ...any) {
		rep.ProblemCount++
		if len(rep.Problems) < MaxDryRunProblems {
			rep.Problems = append(rep.Problems, fmt.Sprintf(format, a...))
		}
	}
	head := func(h ledgerFile) error {
		if h.Kind != LedgerFileKind || h.Format != LedgerFileFormat {
			return fmt.Errorf("not a ledger file of format %d (kind %q, format %d)", LedgerFileFormat, h.Kind, h.Format)
		}
		f, rep.Series, rep.Pool = h, h.Series, h.Pool
		if p, err := nodepolicy.Load(filepath.Join(dir, nodepolicy.FileName)); err == nil {
			accepted, _ := p.Accepted()
			for _, a := range accepted {
				if a.Series.ID() == h.Series {
					s, rep.SeriesKnown = a.Series, true
				}
			}
		}
		return nil
	}
	item := func(it ledgerFileItem) error {
		i := rep.Entries
		rep.Entries++
		e := it.ExportEntry
		if i == 0 && e.Height != f.From {
			problem("entry 0 is at height %d, but the file says it starts at %d", e.Height, f.From)
		}
		if i > 0 && e.Height != prevH+1 {
			problem("entry %d is at height %d, after height %d", i, e.Height, prevH)
		}
		if i > 0 && e.Parent != prevHash {
			problem("entry %d's parent %s is not entry %d's hash %s", i, e.Parent.Short(), i-1, prevHash.Short())
		}
		prevH, prevHash = e.Height, e.Hash
		di, err := wire.DecodeItem(it.Item)
		if err != nil {
			problem("entry %d does not decode: %v", i, err)
			return nil
		}
		rep.Decoded++
		if di.Entry.Parent != e.Parent {
			problem("entry %d's item has parent %s, its summary %s", i, di.Entry.Parent.Short(), e.Parent.Short())
		}
		if len(di.Entry.Txs) != e.TxCount {
			problem("entry %d's item holds %d transactions, its summary %d", i, len(di.Entry.Txs), e.TxCount)
		}
		if s != nil {
			if h := di.Entry.Hash(s); h != e.Hash {
				problem("entry %d's item hashes to %s, its summary says %s", i, h.Short(), e.Hash.Short())
			}
		}
		return nil
	}
	if err := readLedgerFile(r, head, item); err != nil {
		return err
	}
	if rep.Entries > 0 {
		if prevH != f.To {
			problem("the last entry is at height %d, but the file says it ends at %d", prevH, f.To)
		}
		if f.To == f.Height && prevHash != f.Tip {
			problem("the last entry is %s, but the file's tip is %s", prevHash.Short(), f.Tip.Short())
		}
	}
	if err := o.write(out, rep); err != nil {
		return err
	}
	if rep.ProblemCount > 0 {
		return ErrDryRunProblems
	}
	return nil
}

// readLedgerFile reads a ledger file from r, giving its header to head
// and then each entry to item, one at a time, so an import holds one
// batch of items rather than the whole file (ORANGE-043: a 292 MB file
// took 1.8 GB). The entries are read as they stream when the header
// fields that name the file and its pool come before them, as poo ledger
// export writes them; otherwise they are held until the header is read.
// An error from head or item stops the read.
func readLedgerFile(r io.Reader, head func(ledgerFile) error, item func(ledgerFileItem) error) error {
	bad := func(err error) error { return fmt.Errorf("not a ledger file: %w", err) }
	dec := json.NewDecoder(r)
	if t, err := dec.Token(); err != nil {
		return bad(err)
	} else if t != json.Delim('{') {
		return bad(errors.New("it is not a JSON object"))
	}
	fields := map[string]json.RawMessage{}
	header := func() (ledgerFile, error) {
		var h ledgerFile
		b, err := json.Marshal(fields)
		if err == nil {
			err = json.Unmarshal(b, &h)
		}
		if err != nil {
			return h, bad(err)
		}
		return h, nil
	}
	named := func() bool {
		for _, k := range []string{"kind", "format", "series", "pool"} {
			if _, ok := fields[k]; !ok {
				return false
			}
		}
		return true
	}
	started := false
	var held []ledgerFileItem
	for dec.More() {
		t, err := dec.Token()
		if err != nil {
			return bad(err)
		}
		k, _ := t.(string)
		if k != "entries" {
			var v json.RawMessage
			if err := dec.Decode(&v); err != nil {
				return bad(err)
			}
			fields[k] = v
			continue
		}
		if started || !named() {
			// Out of order, or entries twice: read them whole, as before.
			var es []ledgerFileItem
			if err := dec.Decode(&es); err != nil {
				return bad(err)
			}
			held = append(held, es...)
			continue
		}
		h, err := header()
		if err != nil {
			return err
		}
		if err := head(h); err != nil {
			return err
		}
		started = true
		t, err = dec.Token()
		if err != nil {
			return bad(err)
		}
		if t == nil { // "entries": null
			continue
		}
		if t != json.Delim('[') {
			return bad(errors.New("entries is not a list"))
		}
		for dec.More() {
			var it ledgerFileItem
			if err := dec.Decode(&it); err != nil {
				return bad(err)
			}
			if err := item(it); err != nil {
				return err
			}
		}
		if _, err := dec.Token(); err != nil {
			return bad(err)
		}
	}
	if _, err := dec.Token(); err != nil {
		return bad(err)
	}
	if !started {
		h, err := header()
		if err != nil {
			return err
		}
		if err := head(h); err != nil {
			return err
		}
	}
	for _, it := range held {
		if err := item(it); err != nil {
			return err
		}
	}
	return nil
}
