package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/brand"
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

// Ledger files (D188): poon ledger export writes a stretch of a pool's
// selected ledger, oldest first, as raw entry items with a readable
// summary of each; poon ledger import hands the items to the node, which
// checks every one as it checks an entry a peer sent. Nothing in a ledger
// file is trusted.

// LedgerFileKind and LedgerFileFormat name a ledger file.
const (
	LedgerFileKind   = brand.KindPrefix + "ledger"
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
		return errors.New("poon ledger export takes flags only")
	}
	// Pools are numbered from 0, so ask whether -pool was given rather
	// than whether it is 0.
	gotPool := false
	fs.Visit(func(f *flag.Flag) { gotPool = gotPool || f.Name == "pool" })
	if !gotPool {
		return errors.New("poon ledger export needs -pool")
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
			return n, fmt.Errorf("the entry at height %d cannot be read back from the node's log; poon doctor says more", from+len(page.Items))
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
		err = fmt.Errorf("%w: the file is for pool %d of series %s; poon follow -series %s -pool %d, then import again", err, f.Pool, f.Series, f.Series, f.Pool)
	}
	if done > 0 {
		// The file is read as it streams, so how many entries it holds in
		// all is not known yet.
		err = fmt.Errorf("%w (the first %d entries were sent: %d kept, %d already held)", err, done, so.Kept, so.Known)
	}
	return err
}

// LedgerImportReply is what poon ledger import prints: the node's totals
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

// LedgerDryRun is what poon ledger import -dry-run prints: the file read
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
	// PoolFollowed says node.json's follow rules follow the file's pool,
	// so a real import would be taken rather than refused (GREEN-059).
	PoolFollowed bool `json:"pool_followed"`
	// Note says what a real import needs when it would be refused.
	Note string `json:"note,omitempty"`
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
		return errors.New("usage: poon ledger import [-dry-run] FILE|-")
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
	_, err := readLedgerFile(r, head, item)
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
		first    int
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
			rep.PoolFollowed = p.FollowsPool(accepted, h.Series, h.Pool)
		}
		switch {
		case !rep.SeriesKnown:
			rep.Note = fmt.Sprintf("this node does not know series %s, so a real import is refused; add the series to the node policy and follow pool %d first", h.Series, h.Pool)
		case !rep.PoolFollowed:
			rep.Note = fmt.Sprintf("this node does not follow pool %d, so a real import is refused; poon follow -series %s -pool %d first", h.Pool, h.Series, h.Pool)
		}
		return nil
	}
	item := func(it ledgerFileItem) error {
		i := rep.Entries
		rep.Entries++
		e := it.ExportEntry
		if i == 0 {
			first = e.Height
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
	// The range and tip are checked against the whole header, keys after
	// the entries included (RED-108).
	f, err := readLedgerFile(r, head, item)
	if err != nil {
		return err
	}
	if rep.Entries > 0 {
		if first != f.From {
			problem("entry 0 is at height %d, but the file says it starts at %d", first, f.From)
		}
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
// took 1.8 GB). Only the keys format 1 defines are taken, each header
// value at most MaxLedgerHeaderValue bytes, and the entries must come
// once (RED-102). It returns the whole header, with any keys that came
// after the entries, which head may not have seen (RED-108).
//
// Entries that come before the fields that name the file and its pool (a
// tool that sorts keys puts them first) are skipped on a first read, and
// the file is read again from where it started for them, so nothing is
// held or copied (RED-107). Input that cannot seek back, such as a pipe,
// is refused in that case. An error from head or item stops the read.
func readLedgerFile(r io.Reader, head func(ledgerFile) error, item func(ledgerFileItem) error) (ledgerFile, error) {
	start := int64(-1)
	if sk, ok := r.(io.Seeker); ok {
		if p, err := sk.Seek(0, io.SeekCurrent); err == nil {
			start = p
		}
	}
	h, early, err := readLedgerPass(r, head, item, nil, start >= 0)
	if err != nil || !early {
		return h, err
	}
	if _, err := r.(io.Seeker).Seek(start, io.SeekStart); err != nil {
		return h, err
	}
	h2, _, err := readLedgerPass(r, head, item, &h, false)
	return h2, err
}

// readLedgerPass is one read of a ledger file for readLedgerFile. With
// known set, the header is already read and is given to head when the
// entries start. Otherwise, entries that come before the naming fields
// are skipped when canRewind (early is then true) and refused if not.
func readLedgerPass(r io.Reader, head func(ledgerFile) error, item func(ledgerFileItem) error, known *ledgerFile, canRewind bool) (h ledgerFile, early bool, err error) {
	bad := func(err error) error { return fmt.Errorf("not a ledger file: %w", err) }
	lr := &limitedValue{r: r, limit: -1}
	dec := json.NewDecoder(lr)
	if t, err := dec.Token(); err != nil {
		return h, false, bad(err)
	} else if t != json.Delim('{') {
		return h, false, bad(errors.New("it is not a JSON object"))
	}
	seen := map[string]bool{}
	started := false
	unknown := 0
	for dec.More() {
		lr.limit = lr.read + MaxLedgerHeaderValue
		t, err := dec.Token()
		lr.limit = -1
		if err != nil {
			return h, early, bad(err)
		}
		k, _ := t.(string)
		if seen[k] {
			return h, early, bad(fmt.Errorf("%q appears twice", k))
		}
		seen[k] = true
		if k != "entries" {
			dst, ok := ledgerHeaderField(&h, k)
			if !ok {
				// A field a later release added: read under the same bound
				// and dropped, so an older release still imports the file
				// (W-20, D192).
				if unknown++; unknown > MaxLedgerUnknownKeys {
					return h, early, bad(fmt.Errorf("more than %d fields format %d does not define", MaxLedgerUnknownKeys, LedgerFileFormat))
				}
				dst = new(json.RawMessage)
			}
			lr.limit = lr.read + MaxLedgerHeaderValue
			err := dec.Decode(dst)
			lr.limit = -1
			if err != nil {
				return h, early, bad(fmt.Errorf("%s: %w", k, err))
			}
			continue
		}
		skip := false
		switch {
		case known != nil:
			if err := head(*known); err != nil {
				return h, early, err
			}
			started = true
		case seen["kind"] && seen["format"] && seen["series"] && seen["pool"]:
			if err := head(h); err != nil {
				return h, early, err
			}
			started = true
		case canRewind:
			skip, early = true, true
		default:
			return h, early, bad(errors.New("its entries come before kind, format, series and pool, and a pipe cannot be read twice: give the file's path instead"))
		}
		t, err = dec.Token()
		if err != nil {
			return h, early, bad(err)
		}
		if t == nil { // "entries": null
			continue
		}
		if t != json.Delim('[') {
			return h, early, bad(errors.New("entries is not a list"))
		}
		for dec.More() {
			if skip {
				var raw json.RawMessage
				if err := dec.Decode(&raw); err != nil {
					return h, early, bad(err)
				}
				continue
			}
			var it ledgerFileItem
			if err := dec.Decode(&it); err != nil {
				return h, early, bad(err)
			}
			if err := item(it); err != nil {
				return h, early, err
			}
		}
		if _, err := dec.Token(); err != nil {
			return h, early, bad(err)
		}
	}
	if _, err := dec.Token(); err != nil {
		return h, early, bad(err)
	}
	if !started && !early {
		hh := h
		if known != nil {
			hh = *known
		}
		if err := head(hh); err != nil {
			return h, early, err
		}
	}
	return h, early, nil
}

// MaxLedgerHeaderValue bounds each header value of a ledger file, read
// before it is held: the longest, the weight, is a few hundred bytes
// (RED-102).
const MaxLedgerHeaderValue = 4 << 10

// MaxLedgerUnknownKeys bounds the header fields a ledger file may carry
// that this release does not know; each is read and dropped (W-20).
const MaxLedgerUnknownKeys = 64

// ledgerHeaderField returns where key k of a ledger file's header goes,
// and false for a key format 1 does not define.
func ledgerHeaderField(h *ledgerFile, k string) (any, bool) {
	switch k {
	case "kind":
		return &h.Kind, true
	case "format":
		return &h.Format, true
	case "series":
		return &h.Series, true
	case "pool":
		return &h.Pool, true
	case "from":
		return &h.From, true
	case "to":
		return &h.To, true
	case "height":
		return &h.Height, true
	case "tip":
		return &h.Tip, true
	case "weight":
		return &h.Weight, true
	case "exported":
		return &h.Exported, true
	}
	return nil, false
}

// limitedValue reads from r, counting, and fails once read reaches limit
// (none while limit is negative).
type limitedValue struct {
	r     io.Reader
	read  int64
	limit int64
}

func (l *limitedValue) Read(p []byte) (int, error) {
	if l.limit >= 0 {
		if l.read >= l.limit {
			return 0, fmt.Errorf("a header value over %d bytes", MaxLedgerHeaderValue)
		}
		if left := l.limit - l.read; int64(len(p)) > left {
			p = p[:left]
		}
	}
	n, err := l.r.Read(p)
	l.read += int64(n)
	return n, err
}
