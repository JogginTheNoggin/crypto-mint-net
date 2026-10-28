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
	"strings"
	"sync"
	"syscall"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/daemon"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/logging"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/nodepolicy"
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
	return &clientFlags{w: addWhere(fs), series: fs.String("series", "", "series id (default: the only followed series)")}
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
	if err := fs.Parse(args); err != nil {
		return err
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
	r, err := cl.SubmitTx(t)
	if err != nil {
		return err
	}
	return writeJSON(out, r)
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
	if err := fs.Parse(args); err != nil {
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
	r, err := cl.Build(ref, *tries)
	if err != nil {
		return err
	}
	r.Entry = "" // the hash is what a user needs; the bytes are for tools
	rep := struct {
		rpc.EntryReply
		Layer *rpc.LayerReply `json:"layer,omitempty"`
	}{EntryReply: r}
	if *work > 0 {
		l, err := cl.Work(ref, r.Hash, 0, *work)
		if err != nil {
			return err
		}
		l.Layer = ""
		rep.Layer = &l
	}
	return writeJSON(out, rep)
}

func workCmd(args []string, out io.Writer) error {
	fs := newFlagSet("work", out)
	c := addClient(fs)
	pf := addPool(fs)
	entryHex := fs.String("entry", "", "entry hash (default: the pool's selected tip)")
	start := fs.Uint64("start", 0, "first nonce")
	tries := fs.Uint64("tries", 1<<16, "nonces to try")
	if err := fs.Parse(args); err != nil {
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
	r, err := cl.Work(rpc.PoolRef{Series: s.ID(), Pool: pool}, target, *start, *tries)
	if err != nil {
		return err
	}
	r.Layer = ""
	return writeJSON(out, r)
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

func peersCmd(args []string, out io.Writer) error {
	fs := newFlagSet("peers", out)
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
	r, err := cl.Peers()
	if err != nil {
		return err
	}
	return o.write(out, r)
}

// peerCmd acts on one peer of the running node: a timeout, a block, lifting
// them, clearing its records, choosing its tracking template, deleting it.
func peerCmd(args []string, out io.Writer) error {
	const use = "usage: poo peer timeout|block|unblock|clear|template|delete ADDR [-seconds N] [-range] [-what flags|measures|all] [-template NAME]"
	if len(args) == 0 {
		return errors.New(use)
	}
	fs := newFlagSet("peer", out)
	w := addWhere(fs)
	secs := fs.Int("seconds", 0, "timeout or block: how long (0: the policy's length; block -1: until unblocked)")
	rng := fs.Bool("range", false, "block: the address's whole network range")
	what := fs.String("what", "all", "clear: flags, measures or all")
	tpl := fs.String("template", "", "template: the tracking template (empty: its groups and the default)")
	action := args[0]
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if err := flagsFor(fs, action, map[string][]string{
		"seconds": {"timeout", "block"}, "range": {"block"}, "what": {"clear"}, "template": {"template"},
	}); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New(use)
	}
	cl, err := dialNode(out, w.resolve())
	if err != nil {
		return err
	}
	r, err := cl.Peer(rpc.PeerRequest{Addr: fs.Arg(0), Action: action, Seconds: *secs, Range: *rng, What: *what, Template: *tpl})
	if err != nil {
		return err
	}
	return writeJSON(out, r)
}

// trackingCmd shows what the running node's tracker keeps: every tracked
// address's status, or one address's whole record.
func trackingCmd(args []string, out io.Writer) error {
	fs := newFlagSet("tracking", out)
	w := addWhere(fs)
	o := addOutput(fs)
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
	return o.write(out, r)
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
// node policies.
func policyCmd(args []string, out io.Writer) error {
	fs := newFlagSet("policy", out)
	root := fs.String("root", defaultRoot(), "directory holding the node policies ($POO_HOME)")
	fresh := fs.Bool("fresh", false, "unarchive: drop the stored data and start from scratch")
	if len(args) == 0 {
		return fmt.Errorf("usage: poo policy list|use|archive|unarchive|delete [-root DIR] [NAME]")
	}
	sub := args[0]
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
