package cli

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/daemon"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/logging"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/nodepolicy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/peers"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/policy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/rpc"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/series"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/testutil"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tx"
)

// DefaultPolicyName is the node policy used when none is named or current.
const DefaultPolicyName = "default"

// defaultRoot is where node policies live: $POO_HOME, or ~/.poo.
func defaultRoot() string {
	if r := os.Getenv("POO_HOME"); r != "" {
		return r
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".poo"
	}
	return filepath.Join(home, ".poo")
}

// where holds the flags that pick a data directory.
type where struct {
	root, name, dir *string
}

func addWhere(fs *flag.FlagSet) *where {
	return &where{
		root: fs.String("root", defaultRoot(), "directory holding the node policies ($POO_HOME)"),
		name: fs.String("policy", "", "node policy name (default: the current one, else \"default\")"),
		dir:  fs.String("dir", "", "use this data directory directly instead of -root and -policy"),
	}
}

func (w *where) manager() nodepolicy.Manager { return nodepolicy.Manager{Root: *w.root} }

func (w *where) policyName() string {
	if *w.name != "" {
		return *w.name
	}
	if cur, err := w.manager().Current(); err == nil {
		return cur
	}
	return DefaultPolicyName
}

func (w *where) resolve() string {
	if *w.dir != "" {
		return *w.dir
	}
	return w.manager().Dir(w.policyName())
}

type stringList []string

func (s *stringList) String() string     { return strings.Join(*s, ",") }
func (s *stringList) Set(v string) error { *s = append(*s, v); return nil }

// initCmd writes a new node policy and makes it current.
func initCmd(args []string, out io.Writer) error {
	fs := newFlagSet("init", out)
	w := addWhere(fs)
	var files stringList
	fs.Var(&files, "series", "series parameters JSON file to know and follow (repeatable)")
	example := fs.Bool("example", false, "know and follow the worked example's series")
	listen := fs.String("listen", "", "RPC listen address (default "+nodepolicy.DefaultListen+"; \"off\" to disable)")
	p2pListen := fs.String("p2p", "", "address for peers (default "+nodepolicy.DefaultP2PListen+"; \"off\" to accept none)")
	var peerList stringList
	fs.Var(&peerList, "peer", "host:port of a peer to stay connected to (repeatable)")
	var registries stringList
	fs.Var(&registries, "registry", "address registry URL or file to find first peers from (repeatable)")
	advertise := fs.String("advertise", "", "host:port peers should connect to (default: none, so the node is not offered as a contact)")
	force := fs.Bool("force", false, "replace the settings of an existing node policy (its data is kept)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	p := nodepolicy.Default()
	p.RPC.Listen = *listen
	p.P2P.Listen = *p2pListen
	p.P2P.Peers = peerList
	p.P2P.Registries = registries
	p.P2P.Advertise = *advertise
	var params []series.Params
	if *example {
		params = append(params, testutil.ExampleParams())
	}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			return err
		}
		var sp series.Params
		if err := json.Unmarshal(b, &sp); err != nil {
			return fmt.Errorf("%s: %w", f, err)
		}
		params = append(params, sp)
	}
	for _, sp := range params {
		s, err := series.New(sp)
		if err != nil {
			return err
		}
		id := s.ID()
		p.Series = append(p.Series, nodepolicy.SeriesInfo{Params: sp})
		p.Follow = append(p.Follow, nodepolicy.Follow{Series: &id})
	}
	path := filepath.Join(w.resolve(), nodepolicy.FileName)
	if _, err := os.Stat(path); err == nil && !*force {
		return fmt.Errorf("%s exists; use -force to replace its settings", path)
	}
	if *w.dir != "" {
		// Not behind a running node, nor while another command writes
		// (RED-030, D136).
		unlock, err := lockPolicy(w.resolve(), "node policy", nil)
		if err != nil {
			return err
		}
		defer unlock()
		if err := nodepolicy.Save(path, p); err != nil {
			return err
		}
	} else {
		m, name := w.manager(), w.policyName()
		if err := m.Save(name, p); err != nil {
			return err
		}
		if err := m.Use(name); err != nil {
			return err
		}
	}
	fmt.Fprintf(out, "wrote %s (%d series)\n", path, len(p.Series))
	return nil
}

// nodeCmd runs the node until interrupted.
func nodeCmd(args []string, out io.Writer) error {
	fs := newFlagSet("node", out)
	w := addWhere(fs)
	level := fs.String("log-level", "", "debug, info, warn or error (default: the node policy's log.level, else info)")
	format := fs.String("log-format", "", "plain, text or json (default: the node policy's log.format, else plain)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := logging.Check(*level, *format); err != nil {
		return err
	}
	return runNode(contextOf(out), w.resolve(), out, nil, daemon.Options{LogLevel: *level, LogFormat: *format})
}

// runNode runs the node in dir with pkg/daemon until ctx is done,
// reloading its node policy on SIGHUP. ready, if set, is called once the
// RPC first accepts connections.
func runNode(ctx context.Context, dir string, out io.Writer, ready func(string), opt daemon.Options) error {
	hup := make(chan os.Signal, 1)
	signal.Notify(hup, syscall.SIGHUP)
	defer signal.Stop(hup)
	reload := make(chan struct{})
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		for {
			select {
			case <-hup:
				select {
				case reload <- struct{}{}:
				case <-ctx.Done():
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()
	var once sync.Once
	opt.Log, opt.Reload = out, reload
	opt.Ready = func(addr string) {
		if ready != nil {
			once.Do(func() { ready(addr) })
		}
	}
	return daemon.Run(ctx, dir, opt)
}

// poolFlags pick a pool by its index or by a mint in it, so commands
// about pools take the -serial that claim and pay take (GREEN-009).
type poolFlags struct {
	pool, serial *uint64
}

func addPool(fs *flag.FlagSet) *poolFlags {
	return &poolFlags{
		pool:   fs.Uint64("pool", 0, "pool index"),
		serial: fs.Uint64("serial", 0, "a mint's serial, for the pool holding it (instead of -pool)"),
	}
}

// resolve returns the pool the flags name in s.
func (p *poolFlags) resolve(fs *flag.FlagSet, s *series.Series) (uint64, error) {
	set := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { set[f.Name] = true })
	if !set["serial"] {
		return *p.pool, nil
	}
	if *p.serial >= s.Mints() {
		return 0, fmt.Errorf("-serial %d: the series has %d mints", *p.serial, s.Mints())
	}
	k := s.PoolOf(*p.serial)
	if set["pool"] && *p.pool != k {
		return 0, fmt.Errorf("-serial %d is in pool %d, not -pool %d", *p.serial, k, *p.pool)
	}
	return k, nil
}

// clientFlags are the flags shared by commands that talk to a running node.
type clientFlags struct {
	w      *where
	series *string
}

func addClient(fs *flag.FlagSet) *clientFlags {
	return addClientSeries(fs, "series id (default: the only followed series)")
}

// addClientSeries is addClient with -series described by usage, for a
// command whose default is not the only followed series (GREEN-042).
func addClientSeries(fs *flag.FlagSet, usage string) *clientFlags {
	return &clientFlags{w: addWhere(fs), series: fs.String("series", "", usage)}
}

func (c *clientFlags) dial(out io.Writer) (*rpc.Client, error) { return dialNode(out, c.w.resolve()) }

// seriesOf returns the chosen series, as the node knows it.
func (c *clientFlags) seriesOf(cl *rpc.Client) (*series.Series, error) {
	known, err := cl.Series()
	if err != nil {
		return nil, err
	}
	var pick *rpc.SeriesInfo
	switch {
	case *c.series != "":
		id, err := hash.Parse(*c.series)
		if err != nil {
			return nil, fmt.Errorf("-series: %w", err)
		}
		for i := range known {
			if known[i].ID == id {
				pick = &known[i]
			}
		}
		if pick == nil {
			return nil, fmt.Errorf("the node does not follow series %s", id.Short())
		}
	case len(known) == 1:
		pick = &known[0]
	default:
		return nil, fmt.Errorf("the node follows %d series; name one with -series", len(known))
	}
	return series.New(pick.Params)
}

func statusCmd(args []string, out io.Writer) error {
	fs := newFlagSet("status", out)
	c := addClient(fs)
	o := addOutput(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := o.check(); err != nil {
		return err
	}
	cl, err := c.dial(out)
	if err != nil {
		return err
	}
	st, err := cl.Status()
	if err != nil {
		return err
	}
	return o.write(out, st)
}

// errSeedFlag refuses a seed on the command line, where other users of
// the machine can read it in the process list and it stays in the shell's
// history (RED-005, decision D98).
var errSeedFlag = errors.New("-seed is not accepted: other users can read the command line; set POO_SEED or use -seed-file")

// seedFlags say where a command reads a key's seed: $POO_SEED, or a file
// (-seed-file, "-" for standard input), never the command line.
type seedFlags struct {
	file, flag *string
}

func addSeed(fs *flag.FlagSet, use string) *seedFlags {
	return &seedFlags{
		file: fs.String("seed-file", "", "file holding "+use+" in hex, - for standard input (or set POO_SEED)"),
		flag: fs.String("seed", "", "not accepted: use POO_SEED or -seed-file"),
	}
}

// read returns the seed in hex, or "" when none is given.
func (f *seedFlags) read(out io.Writer, env bool) (string, error) {
	if *f.flag != "" {
		return "", errSeedFlag
	}
	switch {
	case *f.file == "-":
		if s, ok := out.(*stdio); ok && s.secret != nil {
			v, err := s.secret("seed: ")
			return strings.TrimSpace(v), err
		}
		line, err := inputOf(out).ReadString('\n')
		if err != nil && (err != io.EOF || line == "") {
			return "", errors.New("no seed on standard input")
		}
		return strings.TrimSpace(line), nil
	case *f.file != "":
		b, err := os.ReadFile(*f.file)
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(b)), nil
	}
	if v, ok := os.LookupEnv("POO_SEED"); ok && env {
		return strings.TrimSpace(v), nil
	}
	return "", nil
}

// signer reads the seed and returns its Ed25519 key.
func (f *seedFlags) signer(out io.Writer) (*policy.Ed25519Signer, error) {
	seedHex, err := f.read(out, true)
	if err != nil {
		return nil, err
	}
	if seedHex == "" {
		return nil, errors.New("no seed: set POO_SEED or use -seed-file (see poo keygen)")
	}
	seed, err := hex.DecodeString(seedHex)
	if err != nil || len(seed) != 32 {
		return nil, fmt.Errorf("the seed must be 64 hex digits (see poo keygen)")
	}
	return policy.NewEd25519Signer(seed)
}

// claimCmd claims a whole mint for a key's 1-of-1 address and submits it.
func claimCmd(args []string, out io.Writer) error {
	fs := newFlagSet("claim", out)
	c := addClient(fs)
	seed := addSeed(fs, "the claiming key's seed")
	serial := fs.Uint64("serial", 0, "the mint's serial")
	parent := fs.String("parent", "", "build the claim into an entry on this parent instead of submitting it: an entry hash, root, or before-claim (the parent of the entry holding the mint's claim now), for a rival claim to grind (D161)")
	tries := fs.Uint64("max-tries", 0, "with -parent: nonces to try when the series sets a minimum entry strength (0: the node's limit)")
	work := fs.Uint64("work", 0, "with -parent: then add the strongest layer found in this many nonces")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := refuseEmptyFlag(fs, "parent"); err != nil {
		return err
	}
	if *parent == "" && (*tries != 0 || *work != 0) {
		return errors.New("-max-tries and -work go with -parent; a claim submitted to the pending pool is built with poo build")
	}
	signer, err := seed.signer(out)
	if err != nil {
		return err
	}
	cl, err := c.dial(out)
	if err != nil {
		return err
	}
	s, err := c.seriesOf(cl)
	if err != nil {
		return err
	}
	if *serial >= s.Mints() {
		return fmt.Errorf("serial %d out of range", *serial)
	}
	addr := s.Address(policy.SingleKey(policy.AlgEd25519, signer.Public()))
	t := &tx.Tx{Mint: s.Mint(*serial), Outputs: []tx.Output{{Address: addr, Amount: s.Params().Supply}}}
	t.Witnesses = []policy.Witness{policy.SingleKeyWitness(signer, t.TID(s.Hash()))}
	if *parent != "" {
		ref := rpc.PoolRef{Series: s.ID(), Pool: s.PoolOf(*serial)}
		return buildOnParent(out, cl, ref, poolArgs(c, ref), *serial, *parent, []*tx.Tx{t}, *tries, *work, rpc.GrindOptions{}, false)
	}
	r, err := cl.SubmitTx(t)
	if err != nil {
		return err
	}
	return writeJSON(out, r)
}

// parentSpec turns a -parent value into the RPC's form: before-claim
// takes the mint serial the command names.
func parentSpec(spec string, serial uint64, haveSerial bool) (string, error) {
	switch {
	case spec == "before-claim" && !haveSerial:
		return "", errors.New("-parent before-claim needs -serial N, the mint whose claim to build before")
	case spec == "before-claim":
		return fmt.Sprintf("before-claim:%d", serial), nil
	}
	return spec, nil
}

// buildOnParent builds an entry on a chosen parent (D161), from txs or
// the pending transactions, optionally adds a layer, and prints the result
// with the next step: an entry on a branch counts only once it outweighs
// the selected ledger.
func buildOnParent(out io.Writer, cl *rpc.Client, ref rpc.PoolRef, sel string, serial uint64, spec string, txs []*tx.Tx, tries, work uint64, o rpc.GrindOptions, keep bool) error {
	par, err := parentSpec(spec, serial, true)
	if err != nil {
		return err
	}
	r, err := cl.BuildOn(ref, tries, o, par, txs)
	if err != nil {
		if strings.HasPrefix(par, "before-claim:") && strings.Contains(err.Error(), "holds no claim") {
			// The claim may be pending, or in an entry no layer has
			// selected yet: say where to look and what else to build on
			// (GREEN-048).
			return fmt.Errorf("%w: a claim still pending, or in an entry with no layer yet, is not in it (poo pending %s shows the pending ones); -parent root builds on the pool's first entry", err, sel)
		}
		return err
	}
	if !keep {
		r.Entry = "" // -bytes keeps it, as for a build on the tip (GREEN-048)
	}
	rep := struct {
		rpc.EntryReply
		Layer *rpc.LayerReply `json:"layer,omitempty"`
		Next  string          `json:"next,omitempty"`
	}{EntryReply: r}
	if work > 0 {
		l, err := cl.WorkWith(ref, r.Hash, 0, work, rpc.GrindOptions{Device: o.Device, Threads: o.Threads})
		if err != nil {
			return err
		}
		if !keep {
			l.Layer = ""
		}
		rep.Layer = &l
	}
	if !r.Switched && (rep.Layer == nil || !rep.Layer.Switched) {
		// The pool as the person can name it again: -pool K always picks
		// this pool, where -serial 0 from a -pool build picked pool 0
		// (GREEN-048).
		rep.Next = fmt.Sprintf("the entry is kept as a branch; add weight with poo grind start %s %s (or poo work %s -entry %s) until it outweighs the selected ledger, and keep node.json's k high enough that it is not pruned meanwhile", r.Hash, sel, sel, r.Hash)
	}
	return writeJSON(out, rep)
}

// payCmd pays an amount of one mint from a key's 1-of-1 address, with
// change back to the same address, and submits it.
func payCmd(args []string, out io.Writer) error {
	fs := newFlagSet("pay", out)
	c := addClient(fs)
	seed := addSeed(fs, "the paying key's seed")
	serial := fs.Uint64("serial", 0, "the mint's serial")
	to := fs.String("to", "", "the receiving address")
	amount := fs.Uint64("amount", 0, "units to pay")
	if err := fs.Parse(args); err != nil {
		return err
	}
	signer, err := seed.signer(out)
	if err != nil {
		return err
	}
	dest, err := hash.Parse(*to)
	if err != nil {
		return fmt.Errorf("-to: %w", err)
	}
	if *amount == 0 {
		return fmt.Errorf("-amount must be at least 1")
	}
	cl, err := c.dial(out)
	if err != nil {
		return err
	}
	s, err := c.seriesOf(cl)
	if err != nil {
		return err
	}
	self := s.Address(policy.SingleKey(policy.AlgEd25519, signer.Public()))
	hs, err := cl.Holdings(s.ID(), *serial)
	if err != nil {
		return err
	}
	t := &tx.Tx{Mint: s.Mint(*serial)}
	var sum uint64
	for _, h := range hs {
		if h.Address != self || sum >= *amount {
			continue
		}
		t.Inputs = append(t.Inputs, tx.Input{TID: h.TID, Index: h.Index})
		sum += h.Amount
	}
	if sum < *amount {
		return fmt.Errorf("the key holds %d units of mint %d at the selected tip, fewer than %d", sum, *serial, *amount)
	}
	t.Outputs = []tx.Output{{Address: dest, Amount: *amount}}
	if change := sum - *amount; change > 0 {
		if dest == self {
			t.Outputs[0].Amount = sum
		} else {
			t.Outputs = append(t.Outputs, tx.Output{Address: self, Amount: change})
		}
	}
	tid := t.TID(s.Hash())
	for range t.Inputs {
		t.Witnesses = append(t.Witnesses, policy.SingleKeyWitness(signer, tid))
	}
	r, err := cl.SubmitTx(t)
	if err != nil {
		return err
	}
	return writeJSON(out, r)
}

func buildCmd(args []string, out io.Writer) error {
	fs := newFlagSet("build", out)
	c := addClient(fs)
	pf := addPool(fs)
	tries := fs.Uint64("max-tries", 0, "nonces to try when the series sets a minimum entry strength (0: the node's limit, 16777216)")
	work := fs.Uint64("work", 0, "then add the strongest layer found in this many nonces")
	parent := fs.String("parent", "", "build on this parent instead of the selected tip, from the pending transactions that fit there: an entry hash, root (the pool's first entry), or before-claim with -serial N (D161)")
	g := addGrindOptions(fs)
	keep := addBytes(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := refuseEmptyFlag(fs, "parent"); err != nil {
		return err
	}
	cl, err := c.dial(out)
	if err != nil {
		return err
	}
	s, err := c.seriesOf(cl)
	if err != nil {
		return err
	}
	pool, err := pf.resolve(fs, s)
	if err != nil {
		return err
	}
	ref := rpc.PoolRef{Series: s.ID(), Pool: pool}
	if *parent != "" {
		set := map[string]bool{}
		fs.Visit(func(f *flag.Flag) { set[f.Name] = true })
		spec, err := parentSpec(*parent, *pf.serial, set["serial"])
		if err != nil {
			return err
		}
		return buildOnParent(out, cl, ref, poolArgs(c, ref), *pf.serial, spec, nil, *tries, *work, g.options(), *keep)
	}
	r, err := cl.BuildWith(ref, *tries, g.options())
	if err != nil {
		return err
	}
	if !*keep {
		r.Entry = "" // the hash is what a user needs; the bytes are for tools
	}
	rep := struct {
		rpc.EntryReply
		Layer *rpc.LayerReply `json:"layer,omitempty"`
		Next  string          `json:"next,omitempty"`
	}{EntryReply: r}
	if *work > 0 {
		l, err := cl.WorkWith(ref, r.Hash, 0, *work, rpc.GrindOptions{Device: *g.device, Threads: *g.threads})
		if err != nil {
			return err
		}
		if !*keep {
			l.Layer = ""
		}
		rep.Layer = &l
	}
	if !r.Switched && (rep.Layer == nil || !rep.Layer.Switched) {
		// A new entry has no weight, and poo work without -entry works on
		// the selected tip, which this is not (GREEN-048).
		rep.Next = fmt.Sprintf("the entry is selected once a layer lands on it: poo work %s -entry %s, or add -work N to the build", poolArgs(c, ref), r.Hash)
	}
	return writeJSON(out, rep)
}

func workCmd(args []string, out io.Writer) error {
	fs := newFlagSet("work", out)
	c := addClient(fs)
	pf := addPool(fs)
	entryHex := fs.String("entry", "", "entry hash (default: the pool's selected tip)")
	start := fs.Uint64("start", 0, "first nonce")
	tries := fs.Uint64("tries", 1<<16, "nonces to try (with -for: 0, as many as fit)")
	g := addGrindOptions(fs)
	keep := addBytes(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	triesSet := false
	fs.Visit(func(f *flag.Flag) { triesSet = triesSet || f.Name == "tries" })
	if *g.dur != "" && !triesSet {
		*tries = 0
	}
	cl, err := c.dial(out)
	if err != nil {
		return err
	}
	s, err := c.seriesOf(cl)
	if err != nil {
		return err
	}
	pool, err := pf.resolve(fs, s)
	if err != nil {
		return err
	}
	var target hash.Hash
	if *entryHex != "" {
		if target, err = hash.Parse(*entryHex); err != nil {
			return fmt.Errorf("-entry: %w", err)
		}
	} else {
		st, err := cl.Status()
		if err != nil {
			return err
		}
		for _, p := range st.Pools {
			if p.Series == s.ID() && p.Pool == pool {
				target = p.Tip
			}
		}
		if target == (hash.Hash{}) || target == s.PoolID(pool) {
			return fmt.Errorf("pool %d has no selected entry; name one with -entry (a new entry has no weight until a layer lands on it)", pool)
		}
	}
	r, err := cl.WorkWith(rpc.PoolRef{Series: s.ID(), Pool: pool}, target, *start, *tries, g.options())
	if err != nil {
		return err
	}
	if !*keep {
		r.Layer = ""
	}
	return writeJSON(out, r)
}

// addBytes is -bytes, which keeps the encoded entry or layer in a build
// or work reply, as poo submit takes it on another node (GREEN-039).
func addBytes(fs *flag.FlagSet) *bool {
	return fs.Bool("bytes", false, "also print the encoded entry or layer (hex), for poo submit on another node")
}

func paymentCmd(args []string, out io.Writer) error {
	fs := newFlagSet("payment", out)
	c := addClient(fs)
	serial := fs.Uint64("serial", 0, "the mint's serial")
	tidHex := fs.String("tid", "", "the transaction id")
	o := addOutput(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := o.check(); err != nil {
		return err
	}
	tid, err := hash.Parse(*tidHex)
	if err != nil {
		return fmt.Errorf("-tid: %w", err)
	}
	cl, err := c.dial(out)
	if err != nil {
		return err
	}
	s, err := c.seriesOf(cl)
	if err != nil {
		return err
	}
	r, err := cl.Payment(s.ID(), *serial, tid)
	if err != nil {
		return err
	}
	return o.write(out, r)
}

func holdingsCmd(args []string, out io.Writer) error {
	fs := newFlagSet("holdings", out)
	c := addClient(fs)
	serial := fs.Uint64("serial", 0, "the mint's serial")
	o := addOutput(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := o.check(); err != nil {
		return err
	}
	cl, err := c.dial(out)
	if err != nil {
		return err
	}
	s, err := c.seriesOf(cl)
	if err != nil {
		return err
	}
	r, err := cl.Holdings(s.ID(), *serial)
	if err != nil {
		return err
	}
	return o.write(out, r)
}

// peerCmd acts on one peer of the running node: a timeout, a block, lifting
// them, clearing its records, choosing its tracking template, deleting it.
func peerCmd(args []string, out io.Writer) error {
	const use = "usage: poo peer timeout|block|unblock|clear|template|delete|add|remove|label|connect ADDR [-seconds N] [-range] [-what flags|measures|all] [-template NAME] [-label TEXT] [-keep], or poo peer connect|remove -imported"
	if len(args) == 0 {
		return errors.New(use)
	}
	fs := newFlagSet("peer", out)
	w := addWhere(fs)
	secs := fs.Int("seconds", 0, "timeout or block: how long (0: the policy's length; block -1: until unblocked)")
	rng := fs.Bool("range", false, "block: the address's whole network range")
	what := fs.String("what", "all", "clear: flags, measures or all")
	tpl := fs.String("template", "", "template: the tracking template (empty: its groups and the default)")
	label := fs.String("label", "", "add and label: the owner's own note on the address (label with none clears it)")
	keep := fs.Bool("keep", false, "add: also list it in node.json (p2p.peers), so it is always dialled, even with the node stopped")
	imported := fs.Bool("imported", false, "with no address: connect tries every unlisted address an imported peer file gave; remove forgets them all (RED-058)")
	action := args[0]
	rest, addr := args[1:], ""
	// The address may come before the flags: poo peer add ADDR -label X.
	if len(rest) > 0 && !strings.HasPrefix(rest[0], "-") {
		addr, rest = rest[0], rest[1:]
	}
	if err := fs.Parse(rest); err != nil {
		return err
	}
	if err := flagsFor(fs, action, map[string][]string{
		"seconds": {"timeout", "block"}, "range": {"block"}, "what": {"clear"}, "template": {"template"},
		"label": {"add", "label"}, "keep": {"add"}, "imported": {"connect", "remove"},
	}); err != nil {
		return err
	}
	if addr == "" && fs.NArg() == 1 {
		addr = fs.Arg(0)
	} else if fs.NArg() != 0 {
		return errors.New(use)
	}
	if (addr == "") != ((action == "connect" || action == "remove") && *imported) {
		return errors.New(use)
	}
	if action == "remove" && *imported {
		return forgetImported(out, w.resolve())
	}
	req := rpc.PeerRequest{Addr: addr, Action: action, Seconds: *secs, Range: *rng, What: *what, Template: *tpl, Label: *label, Keep: *keep, Imported: *imported}
	cl, err := dialNode(out, w.resolve())
	if err == nil {
		var r rpc.PeerReply
		if r, err = cl.Peer(req); err == nil {
			return writeJSON(out, r)
		}
		var re *rpc.Error
		if errors.As(err, &re) {
			return err
		}
	}
	// No node answered: what can be done on the files is (D157).
	r, ok, oerr := peerOffline(w.resolve(), req, err)
	if !ok {
		if action == "add" {
			// Only -keep works with the node stopped; say so (GREEN-046).
			return fmt.Errorf("%w (with the node stopped, add -keep lists the address in node.json)", err)
		}
		return err
	}
	if oerr != nil {
		return oerr
	}
	return writeJSON(out, r)
}

// peerOffline makes a peer action on the data directory's files when no
// node answered: add with -keep lists the address in node.json, remove
// takes it out of node.json and the book, and label labels it in the
// book. It reports whether the action can be made so.
func peerOffline(dir string, req rpc.PeerRequest, nodeErr error) (rpc.PeerReply, bool, error) {
	var out rpc.PeerReply
	switch {
	case req.Action == "add" && req.Keep, req.Action == "remove", req.Action == "label":
	default:
		return out, false, nil
	}
	// Everything is checked before any file changes (GREEN-046): an add
	// used to save node.json and then refuse its label, and to drop a
	// good one without a word.
	if err := peers.CheckLabel(req.Label); err != nil {
		return out, true, err
	}
	unlock, err := lockPolicy(dir, "node policy", nodeErr)
	if err != nil {
		return out, true, err
	}
	defer unlock()
	var book *peers.Book
	if req.Action != "add" || req.Label != "" {
		if book, err = peers.Open(filepath.Join(dir, peers.FileName), peers.Limits{}); err != nil {
			return out, true, err
		}
		if _, held := book.Record(req.Addr); req.Action == "add" && !held {
			return out, true, errors.New("the node is stopped and its peer book does not hold this address, so it cannot take a label yet: add it with -keep alone, then poo peer label ADDR -label TEXT once the node has it; nothing was changed")
		}
	}
	if req.Action != "label" {
		path := filepath.Join(dir, nodepolicy.FileName)
		p, err := nodepolicy.Load(path)
		if err != nil {
			return out, true, err
		}
		if nodepolicy.KeepPeer(&p, req.Addr, req.Action == "add") {
			if err := nodepolicy.Save(path, p); err != nil {
				return out, true, err
			}
			out.Kept = true
		}
	}
	if book == nil {
		return out, true, nil
	}
	if req.Action == "remove" {
		out.Held = book.Delete(req.Addr)
	} else if out.Held, err = book.SetLabel(req.Addr, req.Label); err != nil {
		return out, true, err
	}
	return out, true, book.Save()
}

// trackingCmd shows what the running node's tracker keeps: every tracked
// address's status, or one address's whole record.
func trackingCmd(args []string, out io.Writer) error {
	fs := newFlagSet("tracking", out)
	w := addWhere(fs)
	o := addOutput(fs)
	blocked := fs.Bool("blocked", false, "only the blocked addresses and network ranges")
	timedOut := fs.Bool("timed-out", false, "only the timed-out addresses")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := o.check(); err != nil {
		return err
	}
	if fs.NArg() > 1 {
		return errors.New("usage: poo tracking [ADDR]")
	}
	cl, err := dialNode(out, w.resolve())
	if err != nil {
		return err
	}
	if fs.NArg() == 1 {
		r, err := cl.TrackingOf(fs.Arg(0))
		if err != nil {
			return err
		}
		return o.write(out, r)
	}
	r, err := cl.Tracking()
	if err != nil {
		return err
	}
	return o.write(out, filterTracking(r, *blocked, *timedOut))
}

// chasesCmd shows the running node's chase queue.
func chasesCmd(args []string, out io.Writer) error {
	fs := newFlagSet("chases", out)
	w := addWhere(fs)
	o := addOutput(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := o.check(); err != nil {
		return err
	}
	cl, err := dialNode(out, w.resolve())
	if err != nil {
		return err
	}
	r, err := cl.Chases()
	if err != nil {
		return err
	}
	return o.write(out, r)
}

// policyCmd lists, switches, archives, takes out of the archive and deletes
// node policies, and exports and imports them (policyport.go, D149).
func policyCmd(args []string, out io.Writer) error {
	fs := newFlagSet("policy", out)
	root := fs.String("root", defaultRoot(), "directory holding the node policies ($POO_HOME)")
	fresh := fs.Bool("fresh", false, "unarchive: drop the stored data and start from scratch")
	if len(args) == 0 {
		return fmt.Errorf("usage: poo policy list|use|archive|unarchive|delete [-root DIR] [NAME], or poo policy export|import|limits")
	}
	sub := args[0]
	switch sub {
	case "export":
		return policyExportCmd(args[1:], out)
	case "import":
		return policyImportCmd(args[1:], out)
	case "limits":
		return policyLimitsCmd(args[1:], out)
	}
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if err := flagsFor(fs, sub, map[string][]string{"fresh": {"unarchive"}}); err != nil {
		return err
	}
	m := nodepolicy.Manager{Root: *root}
	if sub == "list" {
		list, err := m.List()
		if err != nil {
			return err
		}
		for _, e := range list {
			mark := " "
			if e.Current {
				mark = "*"
			}
			state := "saved"
			if e.Archived {
				state = "archived"
			}
			fmt.Fprintf(out, "%s %-20s %s\n", mark, e.Name, state)
		}
		return nil
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("poo policy %s needs one NAME", sub)
	}
	name := fs.Arg(0)
	var err error
	switch sub {
	case "use":
		err = m.Use(name)
	case "archive":
		err = m.Archive(name)
	case "unarchive":
		err = m.Unarchive(name, *fresh)
	case "delete":
		err = m.Delete(name)
	default:
		return fmt.Errorf("unknown policy subcommand %q", sub)
	}
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "%s: %s\n", sub, name)
	return nil
}

// grindOptions are the flags that choose how build and work grind
// (package grind, M11).
type grindOptions struct {
	device  *string
	threads *int
	dur     *string
}

func addGrindOptions(fs *flag.FlagSet) *grindOptions {
	return &grindOptions{
		device:  fs.String("device", "", "device: auto, cpu, gpu:0, ... (poo grind devices; default: node.json's grind section, else auto)"),
		threads: fs.Int("threads", 0, "threads (default: every core but one)"),
		dur:     fs.String("for", "", "grind for this long, up to 4m (30s), instead of a number of tries"),
	}
}

func (g *grindOptions) options() rpc.GrindOptions {
	return rpc.GrindOptions{Device: *g.device, Threads: *g.threads, For: *g.dur}
}

// policyLimitsCmd lists the node policy's bounded settings (D159): each
// one's place in node.json, its range, its default, what node.json sets
// and what the node uses, so a person can tailor them without reading
// the decisions (GREEN-047). With -set NAME=VALUE it changes them (D161):
// through the running node, which applies them at once, or in node.json
// when no node runs. Without -set it only reads node.json.
func policyLimitsCmd(args []string, out io.Writer) error {
	fs := newFlagSet("policy limits", out)
	w := addWhere(fs)
	o := addOutput(fs)
	var sets stringList
	fs.Var(&sets, "set", "set a limit, NAME=VALUE (repeatable; 0 puts it back to its default), checked against its range before anything is saved")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := o.check(); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("usage: poo policy limits [-set NAME=VALUE]... [-o FORMAT] [-policy NAME | -dir DIR]")
	}
	dir := w.resolve()
	path := filepath.Join(dir, nodepolicy.FileName)
	if len(sets) == 0 {
		p, err := nodepolicy.Load(path)
		if err != nil {
			return err
		}
		return o.write(out, p.Limits())
	}
	if viewOnlyOf(out) {
		return errViewOnly("policy limits -set")
	}
	set := map[string]int{}
	for _, kv := range sets {
		name, val, ok := strings.Cut(kv, "=")
		v, err := strconv.Atoi(strings.TrimSpace(val))
		if !ok || err != nil {
			return fmt.Errorf("-set %q: want NAME=VALUE, such as p2p.max_book=8192", kv)
		}
		name = strings.TrimSpace(name)
		if hint := limitHint(name); hint != "" {
			return fmt.Errorf("-set %q: %q is not a settable limit; did you mean %s? (poo policy limits lists them)", kv, name, hint)
		}
		set[name] = v
	}
	var nodeErr error
	if cl, err := dialNode(out, dir); err == nil {
		r, err := cl.SetLimits(rpc.LimitsRequest{Set: set})
		if err == nil {
			return o.write(out, r)
		}
		var re *rpc.Error
		if errors.As(err, &re) {
			return err
		}
		nodeErr = err
	}
	unlock, err := lockPolicy(dir, "node policy", nodeErr)
	if err != nil {
		return err
	}
	defer unlock()
	p, err := nodepolicy.Load(path)
	if err != nil {
		return err
	}
	changed, err := p.SetLimits(set)
	if err != nil {
		return err
	}
	if changed {
		if err := nodepolicy.Save(path, p); err != nil {
			return err
		}
	}
	return o.write(out, rpc.LimitsReply{Limits: p.Limits(), Saved: changed})
}

// poolArgs names a pool as poo's commands take it, for a next step: the
// series only when the person named one, since otherwise the only
// followed series is the default (GREEN-048).
func poolArgs(c *clientFlags, ref rpc.PoolRef) string {
	a := fmt.Sprintf("-pool %d", ref.Pool)
	if *c.series != "" {
		a = "-series " + ref.Series.String() + " " + a
	}
	return a
}

// limitHint names the settable limits a mistyped -set name most likely
// meant: the ones it ends (max_book for p2p.max_book), else the nearest
// by spelling. It is empty for a name that is a limit, or with no near
// one (GREEN-048).
func limitHint(name string) string {
	var names, ends []string
	for _, b := range nodepolicy.Bounds() {
		if b.Name == name {
			return ""
		}
		names = append(names, b.Name)
		if strings.HasSuffix(b.Name, "."+name) {
			ends = append(ends, b.Name)
		}
	}
	if len(ends) == 0 {
		ends = suggest(name, names)
	}
	return strings.Join(ends, " or ")
}

// refuseEmptyFlag refuses a flag given with an empty value, which would
// otherwise read as the flag left out: poo claim -parent "" submitted the
// claim to the pending pool instead of building it on a parent.
func refuseEmptyFlag(fs *flag.FlagSet, name string) error {
	var err error
	fs.Visit(func(f *flag.Flag) {
		if f.Name == name && f.Value.String() == "" {
			err = fmt.Errorf("-%s was given empty: name an entry hash, root or before-claim, or leave -%s out; nothing was done", name, name)
		}
	})
	return err
}
