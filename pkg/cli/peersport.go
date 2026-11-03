package cli

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/fsutil"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/nodepolicy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/p2p"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/peerport"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/peers"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/rpc"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tracking"
)

// The peer book from the command line (D157): filters on poo peers and
// poo tracking, and a peer file to export and import (package peerport).

// peersCmd is poo peers: the running node's connections and peer book,
// filtered, or with export or import a peer file.
func peersCmd(args []string, out io.Writer) error {
	if len(args) > 0 {
		switch args[0] {
		case "export":
			return peersExportCmd(args[1:], out)
		case "import":
			return peersImportCmd(args[1:], out)
		}
	}
	fs := newFlagSet("peers", out)
	w := addWhere(fs)
	o := addOutput(fs)
	cats := fs.String("category", "", "only addresses in one of these book states for some mint policy, comma-separated: "+strings.Join(stateNames(), ", "))
	pol := fs.String("mint-policy", "", "only addresses with a mint policy whose text holds this (a series id, or a part of one)")
	label := fs.String("label", "", "only addresses whose label holds this (any case)")
	addr := fs.String("addr", "", "only addresses that hold this")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := o.check(); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("usage: poo peers [-category LIST] [-mint-policy TEXT] [-label TEXT] [-addr TEXT], or poo peers export|import")
	}
	var want []peers.State
	for _, c := range splitList(*cats) {
		st, err := stateNamed(c)
		if err != nil {
			return fmt.Errorf("-category: %w", err)
		}
		want = append(want, st)
	}
	cl, err := dialNode(out, w.resolve())
	if err != nil {
		return err
	}
	r, err := cl.Peers()
	if err != nil {
		return err
	}
	f := peerFilter{states: want, policy: *pol, label: strings.ToLower(*label), addr: *addr}
	if !f.empty() {
		book := []peers.Record{}
		kept := map[string]bool{}
		for _, rec := range r.Book {
			if f.match(rec) {
				book = append(book, rec)
				kept[rec.Addr] = true
			}
		}
		conns := []p2p.PeerInfo{}
		for _, p := range r.Peers {
			if kept[p.Addr] || kept[p.Listen] {
				conns = append(conns, p)
			}
		}
		r.Book, r.Peers = book, conns
	}
	return o.write(out, r)
}

// peerFilter picks peer book records.
type peerFilter struct {
	states              []peers.State
	policy, label, addr string
}

func (f peerFilter) empty() bool {
	return len(f.states) == 0 && f.policy == "" && f.label == "" && f.addr == ""
}

func (f peerFilter) match(r peers.Record) bool {
	if f.addr != "" && !strings.Contains(r.Addr, f.addr) {
		return false
	}
	if f.label != "" && !strings.Contains(strings.ToLower(r.Label), f.label) {
		return false
	}
	if len(f.states) == 0 && f.policy == "" {
		return true
	}
	for _, p := range r.Policies {
		if (len(f.states) == 0 || slices.Contains(f.states, p.State)) && (f.policy == "" || strings.Contains(p.Text, f.policy)) {
			return true
		}
	}
	return false
}

func stateNames() []string {
	var out []string
	for _, s := range peers.States {
		out = append(out, s.String())
	}
	return out
}

func stateNamed(name string) (peers.State, error) {
	for _, s := range peers.States {
		if s.String() == name {
			return s, nil
		}
	}
	return 0, fmt.Errorf("no category %q: %s", name, strings.Join(stateNames(), ", "))
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// filterTracking keeps the blocked, the timed-out, or both.
func filterTracking(r rpc.TrackingReply, blocked, timedOut bool) rpc.TrackingReply {
	if !blocked && !timedOut {
		return r
	}
	out := rpc.TrackingReply{Tracked: map[string]tracking.Status{}, Ranges: map[string]time.Time{}}
	for k, st := range r.Tracked {
		if (blocked && st.Blocked) || (timedOut && st.TimedOut) {
			out.Tracked[k] = st
		}
	}
	if blocked {
		now := time.Now()
		for k, until := range r.Ranges {
			if until.After(now) {
				out.Ranges[k] = until
			}
		}
	}
	return out
}

// peerBook is the peer book and tracker of a data directory: the running
// node's through its interface, or with no node the files themselves,
// under the directory's lock.
type peerBook struct {
	cl    *rpc.Client
	book  *peers.Book
	tr    *tracking.Tracker
	pol   nodepolicy.Policy
	close func()
}

// openPeerBook dials the node in dir, or opens its files. With write, the
// files are opened under the data directory's lock.
func openPeerBook(out io.Writer, dir string, write bool) (*peerBook, error) {
	var nodeErr error
	if cl, err := dialNode(out, dir); err == nil {
		if _, err := cl.Peers(); err == nil {
			return &peerBook{cl: cl, close: func() {}}, nil
		} else {
			nodeErr = err
			var re *rpc.Error
			if errors.As(err, &re) {
				return nil, err
			}
		}
	}
	pol, err := nodepolicy.Load(filepath.Join(dir, nodepolicy.FileName))
	if err != nil {
		return nil, err
	}
	unlock := func() {}
	if write {
		u, err := lockPolicy(dir, "peer book", nodeErr)
		if err != nil {
			return nil, err
		}
		unlock = u
	}
	book, err := peers.Open(filepath.Join(dir, peers.FileName), peers.Limits{})
	if err != nil {
		unlock()
		return nil, err
	}
	tr, err := tracking.New(pol.TrackingConfig(), filepath.Join(dir, tracking.FileName))
	if err != nil {
		unlock()
		return nil, err
	}
	return &peerBook{book: book, tr: tr, pol: pol, close: unlock}, nil
}

// peersExportCmd writes the peer book, or the categories asked for, to a
// peer file.
func peersExportCmd(args []string, out io.Writer) error {
	fs := newFlagSet("peers export", out)
	w := addWhere(fs)
	cats := fs.String("category", "", "only these categories, comma-separated (default: all): "+strings.Join(peerport.Categories(), ", "))
	file := fs.String("out", "", "write the peer file here (default: print it)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("poo peers export takes flags only")
	}
	pb, err := openPeerBook(out, w.resolve(), false)
	if err != nil {
		return err
	}
	defer pb.close()
	var recs []peers.Record
	var tracked map[string]tracking.Status
	var ranges map[string]time.Time
	now := time.Now()
	if pb.cl != nil {
		r, err := pb.cl.Peers()
		if err != nil {
			return err
		}
		t, err := pb.cl.Tracking()
		if err != nil {
			return err
		}
		recs, tracked, ranges = r.Book, t.Tracked, t.Ranges
	} else {
		recs, ranges = pb.book.Records(), pb.tr.Ranges()
		tracked = map[string]tracking.Status{}
		for _, k := range pb.tr.Keys() {
			tracked[k] = pb.tr.Status(k, now)
		}
	}
	f, err := peerport.Export(recs, tracked, ranges, splitList(*cats), now)
	if err != nil {
		return err
	}
	if *file == "" {
		return f.Write(out)
	}
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return err
	}
	if err := fsutil.WriteFile(*file, buf.Bytes(), 0o600); err != nil {
		return err
	}
	fmt.Fprintf(out, "exported %d %s and %d %s to %s\n", len(f.Peers), plural(len(f.Peers), "address", "addresses"), len(f.Blocks), plural(len(f.Blocks), "block", "blocks"), *file)
	return nil
}

// peersImportReport is what poo peers import did, or would do.
type peersImportReport struct {
	Running bool          `json:"running"`
	Plan    peerport.Plan `json:"plan"`
	Applied bool          `json:"applied"`
	// Digest covers the file, the options and the plan: pass it back with
	// -if-digest to apply only what was previewed.
	Digest string `json:"digest"`
}

// peersImportCmd merges a peer file into the peer book: every address
// unlisted, and blocks only with -blocks. It shows the plan, and changes
// the book only with -apply.
func peersImportCmd(args []string, out io.Writer) error {
	fs := newFlagSet("peers import", out)
	w := addWhere(fs)
	apply := fs.Bool("apply", false, "make the change (default: only show the plan)")
	blocks := fs.Bool("blocks", false, "take the file's address blocks too, adding or lengthening only (default: list them and leave them)")
	ranges := fs.Bool("ranges", false, "with -blocks, take its network range blocks too")
	asJSON := fs.Bool("json", false, "print the plan as JSON")
	ifDigest := fs.String("if-digest", "", "apply only if the import digest (as the preview prints it) is this one")
	var file string
	if len(args) > 0 && (args[0] == "-" || !strings.HasPrefix(args[0], "-")) {
		file, args = args[0], args[1:]
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if file == "" && fs.NArg() == 1 {
		file = fs.Arg(0)
	} else if fs.NArg() != 0 || file == "" {
		return errors.New("usage: poo peers import FILE|- [-apply] [-blocks [-ranges]] [-if-digest D] [-json]")
	}
	if *apply && viewOnlyOf(out) {
		return errViewOnly("peers import -apply")
	}
	digestGiven := false
	fs.Visit(func(f *flag.Flag) { digestGiven = digestGiven || f.Name == "if-digest" })
	if digestGiven {
		if d, err := hex.DecodeString(*ifDigest); err != nil || len(d) != sha256.Size {
			return fmt.Errorf("-if-digest needs the digest the preview printed (64 hex digits), not %q; nothing was changed", *ifDigest)
		}
		if !*apply {
			return errors.New("-if-digest checks the file an -apply changes: add -apply, or leave it out to preview")
		}
	}
	if *ranges && !*blocks {
		return errors.New("-ranges goes with -blocks")
	}
	var r io.Reader
	if file == "-" {
		r = inputOf(out)
	} else {
		fh, err := os.Open(file)
		if err != nil {
			return err
		}
		defer fh.Close()
		r = fh
	}
	f, err := peerport.Read(r)
	if file == "-" {
		if cerr := contextOf(out).Err(); cerr != nil {
			return fmt.Errorf("standard input was cut off, so nothing was read: %w", cerr)
		}
	}
	if err != nil {
		return err
	}
	opt := peerport.Options{Blocks: *blocks, Ranges: *ranges}
	pb, err := openPeerBook(out, w.resolve(), *apply)
	if err != nil {
		return err
	}
	defer pb.close()
	// want, with an apply, is the digest the plan made in the same call
	// must have, so what is applied is what was checked (RED-062, D158).
	run := func(apply bool, want string) (peerport.Plan, error) {
		if pb.cl != nil {
			rep, err := pb.cl.ImportPeers(rpc.PeersImportRequest{File: f, Options: opt, DryRun: !apply, IfDigest: want})
			return rep.Plan, err
		}
		listed := append([]string(nil), pb.pol.P2P.Peers...)
		if pb.pol.P2P.Advertise != "" {
			listed = append(listed, pb.pol.P2P.Advertise)
		}
		n := peerport.Node{Book: pb.book, Tracker: pb.tr, AllowLocal: pb.pol.P2P.AllowLocalPeers, Listed: listed}
		if apply && want != "" {
			return peerport.ImportIf(f, opt, n, want, time.Now())
		}
		return peerport.Import(f, opt, n, apply, time.Now())
	}
	plan, err := run(false, "")
	if err != nil {
		return fmt.Errorf("%w; nothing was changed", err)
	}
	dg := peerport.Digest(f.Digest, opt, plan)
	if digestGiven && !strings.EqualFold(*ifDigest, dg) {
		return fmt.Errorf("this is not the one previewed: the file, -blocks, -ranges or what it would change differ (digest %s, not %s), so nothing was changed: preview it again", dg, *ifDigest)
	}
	rep := peersImportReport{Running: pb.cl != nil, Plan: plan, Digest: dg}
	if *apply && !plan.Empty() {
		want := ""
		if digestGiven {
			want = *ifDigest
		}
		if rep.Plan, err = run(true, want); err != nil {
			return err
		}
		rep.Applied = true
	}
	if *asJSON {
		return writeJSON(out, rep)
	}
	printPeersPlan(out, rep, *apply)
	return nil
}

// printPeersPlan prints a peer import's plan for reading. Every text in it
// from the file was checked to hold no control character (RED-053).
func printPeersPlan(out io.Writer, r peersImportReport, apply bool) {
	p := r.Plan
	state := "not running"
	if r.Running {
		state = "running"
	}
	if !apply && !p.Empty() {
		fmt.Fprintf(out, "import digest %s (-apply -if-digest applies only this file, with these options and this plan)\n", r.Digest)
	}
	fmt.Fprintf(out, "peer book (%s):\n", state)
	if len(p.Blocks) > 0 {
		fmt.Fprintln(out, "  read first: these blocks refuse addresses on this node:")
		for _, l := range p.Blocks {
			fmt.Fprintf(out, "  ! block %s %s\n", l.Addr, l.Detail)
		}
	}
	for _, l := range p.Added {
		fmt.Fprintf(out, "  + %s (%s)\n", l.Addr, l.Detail)
	}
	for _, l := range p.More {
		fmt.Fprintf(out, "  ~ %s (%s)\n", l.Addr, l.Detail)
	}
	for _, l := range p.Labels {
		fmt.Fprintf(out, "  label %s: %s\n", l.Addr, l.Detail)
	}
	for _, l := range p.LabelConflicts {
		fmt.Fprintf(out, "  ! label %s: %s\n", l.Addr, l.Detail)
	}
	for _, l := range p.Skipped {
		if l.Addr == "" {
			fmt.Fprintf(out, "  skipped: %s\n", l.Detail)
		} else {
			fmt.Fprintf(out, "  skipped %s: %s\n", l.Addr, l.Detail)
		}
	}
	for _, l := range p.Refused {
		fmt.Fprintf(out, "  refused %s: %s\n", l.Addr, l.Detail)
	}
	if p.Same > 0 {
		fmt.Fprintf(out, "  %d of the file's entries are already here\n", p.Same)
	}
	for _, n := range p.Notes {
		fmt.Fprintf(out, "  note: %s\n", n)
	}
	switch {
	case p.Empty():
		fmt.Fprintln(out, "  nothing to change")
	case !apply:
		fmt.Fprintln(out, "dry run: nothing was changed; add -apply to make this change")
	default:
		fmt.Fprintln(out, "  applied: the addresses are unlisted; the node tries them when it is short of peers, or now with poo peer connect -imported")
	}
}

// forgetImported is poo peer remove -imported: it takes out every
// address known only from an imported peer file, which an import may have
// put there by the thousand, in one step (RED-058, D158). Blocks, and
// addresses the node learned, promoted or verified, stay.
func forgetImported(out io.Writer, dir string) error {
	if viewOnlyOf(out) {
		return errViewOnly("peer remove -imported")
	}
	pb, err := openPeerBook(out, dir, true)
	if err != nil {
		return err
	}
	defer pb.close()
	var rep rpc.ForgetImportedReply
	if pb.cl != nil {
		if rep, err = pb.cl.ForgetImported(rpc.ForgetImportedRequest{}); err != nil {
			return err
		}
	} else {
		rep.Imported = pb.book.Unlisted()
		rep.Forgotten = pb.book.ForgetImported()
		if err := pb.book.Save(); err != nil {
			return err
		}
	}
	return writeJSON(out, rep)
}
