package cli

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/nodepolicy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/policy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/rpc"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/series"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tx"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/wallet"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/weight"
)

const walletUsage = `poo wallet: keys, payments and requests from one seed

Usage: poo wallet COMMAND [flags]

  create [-restore -|HEX] [-gap N] make a wallet (or restore one from its seed;
                                  - reads it hidden)
  create -watch-only               a wallet with no private key: it watches
                                   keys kept elsewhere (D193)
  seed                             print the seed (keep it secret)
  passwd                           seal the seed under a new passphrase
  address [-label TEXT]            give out a fresh address
  addresses                        every address, used or not, and shared ones
  watch [-serial N]... [-pool K]... add mints to look for portions in
  follow                           make the node policy follow only the pools
                                   of the watched mints (light mode)
  balance                          spendable units per watched mint
  claim -serial N                  claim a whole mint for a fresh address
  pay -to ADDR -amount A [-mint N]... [-memo TEXT] [-out FILE]
  pay -request FILE [-out FILE]    pay an address or a payment request; with
                                   a shared policy, -out writes what
                                   co-signers sign
  request -amount A [-memo TEXT] [-out FILE]
                                   ask to be paid, to a fresh address
  receive (-address ADDR | -request FILE) [-serial N -tid HASH]...
          [-threshold W]           what has arrived for a request
  status [-threshold W]            where every payment sent stands
  resend                           send dropped legs again
  policy -name NAME -k K -key self|ALG:HEX|HEX... [-label TEXT]
                                   record a shared k-of-n policy
  cosign FILE                      add this wallet's signatures to FILE
  submit FILE                      send a payment whose signatures are complete

The wallet file is -wallet, default wallet.json in -root. The passphrase
comes from $POO_PASSPHRASE, -passphrase-file, or a line on standard input.
The series and node come from the node policy flags, as for other commands.
`

// createIterations is the work factor of new wallets (Argon2id passes,
// decision D95); zero means the default. Tests lower it.
var createIterations int

// walletFlags are the flags shared by wallet commands.
type walletFlags struct {
	c        *clientFlags
	path     *string
	passFile *string
}

func addWallet(fs *flag.FlagSet) *walletFlags {
	return &walletFlags{
		c:        addClient(fs),
		path:     fs.String("wallet", "", "wallet file (default: wallet.json in -root)"),
		passFile: fs.String("passphrase-file", "", "read the passphrase from this file"),
	}
}

func (f *walletFlags) file() string {
	if *f.path != "" {
		return *f.path
	}
	return filepath.Join(*f.c.w.root, wallet.FileName)
}

// passphrase reads the passphrase; prompt is shown when reading stdin.
func (f *walletFlags) passphrase(out io.Writer, prompt string) (string, error) {
	if p, ok := os.LookupEnv("POO_PASSPHRASE"); ok {
		return p, nil
	}
	if *f.passFile != "" {
		b, err := os.ReadFile(*f.passFile)
		if err != nil {
			return "", err
		}
		return strings.TrimRight(string(b), "\r\n"), nil
	}
	p, err := readSecret(out, prompt)
	if errors.Is(err, errNoInput) {
		return "", errors.New("no passphrase: set POO_PASSPHRASE or use -passphrase-file")
	}
	return p, err
}

// newPassphrase reads a new wallet's passphrase as passphrase does,
// asking for it twice when it is typed (GUI-020, D145).
func (f *walletFlags) newPassphrase(out io.Writer) (string, error) {
	if _, ok := os.LookupEnv("POO_PASSPHRASE"); ok || *f.passFile != "" {
		return f.passphrase(out, "")
	}
	p, err := readNewSecret(out, "new passphrase: ")
	if errors.Is(err, errNoInput) {
		return "", errors.New("no passphrase: set POO_PASSPHRASE or use -passphrase-file")
	}
	return p, err
}

func (f *walletFlags) open() (*wallet.Wallet, error) {
	w, err := wallet.Open(f.file())
	if errors.Is(err, os.ErrNotExist) {
		// Say what to do, not only which file is missing (GREEN-040).
		return nil, fmt.Errorf("no wallet (%w): make one with poo wallet create (or -restore), or name another with -wallet", err)
	}
	return w, err
}

func (f *walletFlags) unlocked(out io.Writer) (*wallet.Wallet, error) {
	w, err := f.open()
	if err != nil {
		return nil, err
	}
	if w.WatchOnly() {
		return w, nil // no seed, so no passphrase (D193)
	}
	p, err := f.passphrase(out, "passphrase: ")
	if err != nil {
		return nil, err
	}
	return w, w.Unlock(p)
}

// series returns the chosen series from the node policy, so commands that
// do not need the node work offline.
func (f *walletFlags) series() (*series.Series, error) {
	path := filepath.Join(f.c.w.resolve(), nodepolicy.FileName)
	p, err := nodepolicy.Load(path)
	if err != nil {
		return nil, err
	}
	accepted, _ := p.Accepted()
	var pick []*series.Series
	for _, a := range accepted {
		if *f.c.series == "" || a.Series.ID().String() == *f.c.series {
			pick = append(pick, a.Series)
		}
	}
	switch {
	case len(pick) == 1:
		return pick[0], nil
	case *f.c.series != "":
		return nil, fmt.Errorf("%s does not know series %s", path, *f.c.series)
	default:
		return nil, fmt.Errorf("%s knows %d series; name one with -series", path, len(pick))
	}
}

// node dials the running node as the wallet's Node.
func (f *walletFlags) node(out io.Writer, s *series.Series) (wallet.Node, error) {
	cl, err := f.c.dial(out)
	if err != nil {
		return nil, err
	}
	h := s.Hash()
	return rpc.NewWalletNode(cl, func(t *tx.Tx) hash.Hash { return t.TID(h) }), nil
}

func parseThreshold(s string) (weight.Weight, error) {
	var w weight.Weight
	if s == "" {
		return w, nil
	}
	if err := w.UnmarshalText([]byte(s)); err != nil {
		return w, fmt.Errorf("-threshold: %w", err)
	}
	return w, nil
}

func readJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, v); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

// writeOut writes v as JSON to path, or to out when path is empty.
func writeOut(out io.Writer, path string, v any) error {
	if path == "" {
		return writeJSON(out, v)
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, append(b, '\n'), 0o644); err != nil {
		return err
	}
	fmt.Fprintf(out, "wrote %s\n", path)
	return nil
}

type uintList []uint64

func (u *uintList) String() string { return fmt.Sprint(*u) }
func (u *uintList) Set(v string) error {
	n, err := strconv.ParseUint(v, 10, 64)
	if err != nil {
		return err
	}
	*u = append(*u, n)
	return nil
}

func walletCmd(args []string, out io.Writer) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		fmt.Fprint(out, walletUsage)
		return nil
	}
	cmds := map[string]func(*flag.FlagSet, *walletFlags, []string, io.Writer) error{
		"create": walletCreate, "seed": walletSeed, "passwd": walletPasswd,
		"address": walletAddress, "addresses": walletAddresses, "watch": walletWatch,
		"follow": walletFollow, "balance": walletBalance, "claim": walletClaim,
		"pay": walletPay, "request": walletRequest, "receive": walletReceive,
		"status": walletStatus, "resend": walletResend, "policy": walletPolicy,
		"cosign": walletCosign, "submit": walletSubmit,
	}
	cmd, ok := cmds[args[0]]
	if !ok {
		return fmt.Errorf("unknown wallet command %q\n\n%s", args[0], walletUsage)
	}
	fs := newFlagSet("wallet "+args[0], out)
	return cmd(fs, addWallet(fs), args[1:], out)
}

func walletCreate(fs *flag.FlagSet, f *walletFlags, args []string, out io.Writer) error {
	restore := fs.String("restore", "", "restore from a seed: - to type it hidden (or read a line of standard input), or 64 hex digits, which other users can read on the command line")
	gap := fs.Int("gap", 0, fmt.Sprintf("unused keys to keep ahead (default %d)", wallet.DefaultGap))
	algName := fs.String("alg", "ed25519", "key algorithm of every key, by name or number (poo crypto); fixed for the wallet's life")
	watchOnly := fs.Bool("watch-only", false, "hold no private key at all: add keys kept elsewhere with poo wallet policy -k 1 -key ALG:HEX, pay with -out, sign with poo sign, then submit (D193)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *watchOnly {
		if *restore != "" {
			return errors.New("-watch-only holds no seed, so it takes no -restore")
		}
		if _, err := wallet.Create(f.file(), "", wallet.Options{WatchOnly: true}); err != nil {
			return err
		}
		fmt.Fprintf(out, "wrote %s (watch-only: no seed, no passphrase)\n", f.file())
		return nil
	}
	alg, err := parseAlg(*algName)
	if err != nil {
		return err
	}
	o := wallet.Options{Gap: *gap, Iterations: createIterations, Alg: alg}
	if *restore != "" {
		text := *restore
		if text == "-" {
			// Read like a passphrase, so a terminal or the GUI hides it
			// (GUI-006).
			if text, err = readSecret(out, "seed: "); err != nil {
				return err
			}
			text = strings.TrimSpace(text)
		}
		seed, err := hex.DecodeString(text)
		if err != nil || len(seed) != wallet.SeedSize {
			return fmt.Errorf("-restore must be %d hex digits", 2*wallet.SeedSize)
		}
		o.Seed = seed
	}
	if _, err := os.Stat(f.file()); err == nil {
		return fmt.Errorf("%s exists", f.file())
	}
	p, err := f.newPassphrase(out)
	if err != nil {
		return err
	}
	if p == "" {
		return errors.New("the passphrase must not be empty")
	}
	if _, err := wallet.Create(f.file(), p, o); err != nil {
		return err
	}
	fmt.Fprintf(out, "wrote %s\n", f.file())
	if *restore == "" {
		fmt.Fprintln(out, "back up the seed (poo wallet seed): it restores every key")
	}
	return nil
}

func walletSeed(fs *flag.FlagSet, f *walletFlags, args []string, out io.Writer) error {
	if err := fs.Parse(args); err != nil {
		return err
	}
	w, err := f.unlocked(out)
	if err != nil {
		return err
	}
	seed, err := w.Seed()
	if err != nil {
		return err
	}
	fmt.Fprintln(out, hex.EncodeToString(seed))
	return nil
}

func walletPasswd(fs *flag.FlagSet, f *walletFlags, args []string, out io.Writer) error {
	newFile := fs.String("new-passphrase-file", "", "read the new passphrase from this file (default: $POO_NEW_PASSPHRASE or standard input)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	w, err := f.unlocked(out)
	if err != nil {
		return err
	}
	var p string
	switch v, ok := os.LookupEnv("POO_NEW_PASSPHRASE"); {
	case *newFile != "":
		b, err := os.ReadFile(*newFile)
		if err != nil {
			return err
		}
		p = strings.TrimRight(string(b), "\r\n")
	case ok:
		p = v
	default:
		if p, err = readNewSecret(out, "new passphrase: "); err != nil && !errors.Is(err, errNoInput) {
			return err
		}
	}
	if p == "" {
		return errors.New("the new passphrase must not be empty")
	}
	release, err := holdWallet(w, out)
	if err != nil {
		return err
	}
	defer release()
	if err := w.Rekey(p); err != nil {
		return err
	}
	if err := w.Save(); err != nil {
		return err
	}
	fmt.Fprintln(out, "passphrase changed")
	return nil
}

func walletAddress(fs *flag.FlagSet, f *walletFlags, args []string, out io.Writer) error {
	label := fs.String("label", "", "what the address is for")
	if err := fs.Parse(args); err != nil {
		return err
	}
	s, err := f.series()
	if err != nil {
		return err
	}
	w, err := f.unlocked(out)
	if err != nil {
		return err
	}
	release, err := holdWallet(w, out)
	if err != nil {
		return err
	}
	defer release()
	o, err := w.NewAddress(s, *label)
	if err != nil {
		return err
	}
	if err := w.Save(); err != nil {
		return err
	}
	var public string
	for _, k := range w.Keys() {
		if k.Index == *o.Key {
			public = hex.EncodeToString(k.Public)
		}
	}
	// The public key is what a co-signer names in a shared policy.
	return writeJSON(out, map[string]any{"address": o.Address, "key": o.Key, "public": public, "policy": hex.EncodeToString(o.Policy.Encode())})
}

// ownerInfo is one row of poo wallet addresses.
type ownerInfo struct {
	Address hash.Hash `json:"address"`
	Key     *uint32   `json:"key,omitempty"`
	Shared  string    `json:"shared,omitempty"`
	Label   string    `json:"label,omitempty"`
	Used    bool      `json:"used"`
}

func walletAddresses(fs *flag.FlagSet, f *walletFlags, args []string, out io.Writer) error {
	o := addOutput(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := o.check(); err != nil {
		return err
	}
	s, err := f.series()
	if err != nil {
		return err
	}
	w, err := f.open()
	if err != nil {
		return err
	}
	var rows []ownerInfo
	for _, o := range w.Owners(s) {
		rows = append(rows, ownerInfo{o.Address, o.Key, o.Shared, o.Label, o.Used})
	}
	sortOwners(rows)
	return o.write(out, rows)
}

// sortOwners orders keys by index, then shared policies by name.
func sortOwners(rows []ownerInfo) {
	less := func(a, b ownerInfo) bool {
		switch {
		case a.Key != nil && b.Key != nil:
			return *a.Key < *b.Key
		case a.Key != nil || b.Key != nil:
			return a.Key != nil
		}
		return a.Shared < b.Shared
	}
	for i := 1; i < len(rows); i++ {
		for j := i; j > 0 && less(rows[j], rows[j-1]); j-- {
			rows[j], rows[j-1] = rows[j-1], rows[j]
		}
	}
}

func walletWatch(fs *flag.FlagSet, f *walletFlags, args []string, out io.Writer) error {
	var serials, pools uintList
	fs.Var(&serials, "serial", "a mint to watch (repeatable)")
	fs.Var(&pools, "pool", "watch every mint of this pool (repeatable)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	s, err := f.series()
	if err != nil {
		return err
	}
	w, err := f.open()
	if err != nil {
		return err
	}
	release, err := holdWallet(w, out)
	if err != nil {
		return err
	}
	defer release()
	for _, k := range pools {
		if k >= s.PoolCount() {
			return fmt.Errorf("pool %d out of range", k)
		}
		first, last := s.PoolRange(k)
		for n := first; n <= last; n++ {
			serials = append(serials, n)
		}
	}
	if err := w.WatchMints(s, serials...); err != nil {
		return err
	}
	if err := w.Save(); err != nil {
		return err
	}
	return writeJSON(out, map[string]any{"mints": w.Watched(s.ID()), "pools": w.Pools(s)})
}

// walletFollow points the node policy at the wallet's pools: light mode,
// where the node verifies and stores only what the wallet needs.
func walletFollow(fs *flag.FlagSet, f *walletFlags, args []string, out io.Writer) error {
	if err := fs.Parse(args); err != nil {
		return err
	}
	s, err := f.series()
	if err != nil {
		return err
	}
	w, err := f.open()
	if err != nil {
		return err
	}
	pools := w.Pools(s)
	if len(pools) == 0 {
		return errors.New("the wallet watches no mints of this series (poo wallet watch)")
	}
	dir := f.c.w.resolve()
	path := filepath.Join(dir, nodepolicy.FileName)
	id := s.ID()
	// A running node changes its own node.json: the series' rule is
	// replaced by the wallet's pools, then applied. Only with no node
	// answering is the file written here, under the lock a running node
	// holds (RED-030, D136).
	var nodeErr error
	if cl, err := dialNode(out, dir); err == nil {
		// One request, so the node never saves the series off its list
		// between a removal and an add (GUI-018, D139).
		if _, err = cl.Follow(rpc.FollowRequest{Replace: true, Series: &id, Pools: pools}); err == nil {
			waitBack(out, dir)
			fmt.Fprintf(out, "wrote %s: series %s follows pools %v; the node applied it\n", path, id.Short(), pools)
			return nil
		}
		var re *rpc.Error
		if errors.As(err, &re) {
			return err
		}
		nodeErr = err
	}
	unlock, err := lockPolicy(dir, "follow list", nodeErr)
	if err != nil {
		return err
	}
	defer unlock()
	p, err := nodepolicy.Load(path)
	if err != nil {
		return err
	}
	if err := p.ReplaceFollow(nodepolicy.Follow{Series: &id, Pools: pools}); err != nil {
		return err
	}
	if err := nodepolicy.Save(path, p); err != nil {
		return err
	}
	fmt.Fprintf(out, "wrote %s: series %s follows pools %v; the node applies it when it starts\n", path, id.Short(), pools)
	return nil
}

func walletBalance(fs *flag.FlagSet, f *walletFlags, args []string, out io.Writer) error {
	o := addOutput(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := o.check(); err != nil {
		return err
	}
	s, err := f.series()
	if err != nil {
		return err
	}
	w, err := f.open()
	if err != nil {
		return err
	}
	n, err := f.node(out, s)
	if err != nil {
		return err
	}
	b, err := w.Balance(s, n)
	if err != nil {
		return err
	}
	var total uint64
	mints := map[string]uint64{}
	for serial, a := range b {
		mints[strconv.FormatUint(serial, 10)] = a
		total += a
	}
	return o.write(out, map[string]any{"mints": mints, "total": total})
}

func walletClaim(fs *flag.FlagSet, f *walletFlags, args []string, out io.Writer) error {
	serial := fs.Uint64("serial", 0, "the mint's serial")
	polName := fs.String("to-policy", "", "claim to this wallet policy's address, unsigned, for poo sign where its keys are (with -out; how a watch-only wallet claims)")
	outFile := fs.String("out", "", "with -to-policy: write the claim waiting for signatures here")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if (*polName == "") != (*outFile == "") {
		return errors.New("-to-policy and -out go together: the claim is written unsigned for poo sign, then sent with poo wallet submit")
	}
	s, err := f.series()
	if err != nil {
		return err
	}
	w, err := f.unlocked(out)
	if err != nil {
		return err
	}
	if *polName != "" {
		// A claim to a policy whose keys are elsewhere: prepared here,
		// signed with poo sign, sent with poo wallet submit (GREEN-060).
		release, err := holdWallet(w, out)
		if err != nil {
			return err
		}
		defer release()
		p, u, err := w.ClaimUnsigned(s, *serial, *polName)
		if err != nil {
			return err
		}
		if err := saveSent(w, out, "claim", p, nil); err != nil {
			return err
		}
		return writeOut(out, *outFile, u)
	}
	if w.WatchOnly() {
		return fmt.Errorf("%w; to claim with a key kept off the node: poo wallet claim -serial %d -to-policy NAME -out claim.json, poo sign claim.json -key-file KEY, poo wallet submit claim.json", wallet.ErrWatchOnly, *serial)
	}
	n, err := f.node(out, s)
	if err != nil {
		return err
	}
	release, err := holdWallet(w, out)
	if err != nil {
		return err
	}
	defer release()
	p, err := w.Claim(s, n, *serial)
	if p != nil {
		if serr := saveSent(w, out, "claim", p, err); serr != nil {
			return serr
		}
	}
	if err != nil {
		return err
	}
	return writeJSON(out, p)
}

func walletPay(fs *flag.FlagSet, f *walletFlags, args []string, out io.Writer) error {
	to := fs.String("to", "", "the receiving address")
	amount := fs.Uint64("amount", 0, "units to pay")
	memo := fs.String("memo", "", "a note kept in the wallet")
	reqFile := fs.String("request", "", "pay this payment request file instead of -to and -amount")
	outFile := fs.String("out", "", "write the payment waiting for co-signers here")
	changeTo := fs.String("change", "", "send the change here (default: a fresh address; a watch-only wallet sends it back to where it came from)")
	var mints uintList
	fs.Var(&mints, "mint", "pay only from this mint (repeatable)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	var change hash.Hash
	if *changeTo != "" {
		c, err := hash.Parse(*changeTo)
		if err != nil {
			return fmt.Errorf("-change: %w", err)
		}
		change = c
	}
	s, err := f.series()
	if err != nil {
		return err
	}
	w, err := f.unlocked(out)
	if err != nil {
		return err
	}
	n, err := f.node(out, s)
	if err != nil {
		return err
	}
	var (
		sent *wallet.Sent
		u    *wallet.Unsigned
	)
	release, err := holdWallet(w, out)
	if err != nil {
		return err
	}
	defer release()
	if *reqFile != "" {
		var r wallet.Request
		if err := readJSON(*reqFile, &r); err != nil {
			return err
		}
		sent, u, err = w.PayRequestWith(s, n, r, wallet.PayOptions{Mints: mints, Change: change})
	} else {
		addr, perr := hash.Parse(*to)
		if perr != nil {
			return fmt.Errorf("-to: %w", perr)
		}
		sent, u, err = w.Pay(s, n, addr, *amount, wallet.PayOptions{Memo: *memo, Mints: mints, Change: change})
	}
	if sent != nil {
		if serr := saveSent(w, out, "payment", sent, err); serr != nil {
			return serr
		}
	}
	if err != nil {
		return err
	}
	if u != nil {
		fmt.Fprintf(out, "signatures missing (per leg, per input): %v\n", u.Missing())
		if *outFile == "" {
			fmt.Fprintln(out, "pass -out FILE to write the payment for co-signers (or for poo sign, with a key kept off the node)")
			return nil
		}
		return writeOut(out, *outFile, u)
	}
	return writeJSON(out, sent)
}

func walletRequest(fs *flag.FlagSet, f *walletFlags, args []string, out io.Writer) error {
	amount := fs.Uint64("amount", 0, "units asked for")
	memo := fs.String("memo", "", "what the payment is for")
	outFile := fs.String("out", "", "write the request here (default: print it)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	s, err := f.series()
	if err != nil {
		return err
	}
	w, err := f.unlocked(out)
	if err != nil {
		return err
	}
	release, err := holdWallet(w, out)
	if err != nil {
		return err
	}
	defer release()
	r, err := w.Request(s, *amount, *memo)
	if err != nil {
		return err
	}
	if err := w.Save(); err != nil {
		return err
	}
	return writeOut(out, *outFile, r)
}

func walletReceive(fs *flag.FlagSet, f *walletFlags, args []string, out io.Writer) error {
	address := fs.String("address", "", "the requested address")
	reqFile := fs.String("request", "", "the request file given out")
	threshold := fs.String("threshold", "", "from(e) the receiver needs for settled (default 0)")
	var serials uintList
	var tids stringList
	fs.Var(&serials, "serial", "mint of a transaction the payer named (pairs with -tid)")
	fs.Var(&tids, "tid", "a transaction the payer named (repeatable)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if len(serials) != len(tids) {
		return errors.New("give one -serial per -tid")
	}
	th, err := parseThreshold(*threshold)
	if err != nil {
		return err
	}
	s, err := f.series()
	if err != nil {
		return err
	}
	w, err := f.open()
	if err != nil {
		return err
	}
	var addr hash.Hash
	switch {
	case *reqFile != "":
		var r wallet.Request
		if err := readJSON(*reqFile, &r); err != nil {
			return err
		}
		addr = r.Address
	case *address != "":
		if addr, err = hash.Parse(*address); err != nil {
			return fmt.Errorf("-address: %w", err)
		}
	default:
		return errors.New("name the request with -address or -request")
	}
	refs := make([]wallet.TxRef, len(tids))
	for i, t := range tids {
		id, err := hash.Parse(t)
		if err != nil {
			return fmt.Errorf("-tid: %w", err)
		}
		refs[i] = wallet.TxRef{Serial: serials[i], TID: id}
	}
	if len(refs) > 0 {
		// Held only to note what the payer named, not while the node is
		// asked: the receipt itself only reads (RED-042, D148).
		release, err := holdWallet(w, out)
		if err != nil {
			return err
		}
		e, ok := w.Find(addr)
		if ok {
			for _, r := range refs {
				e.Tell(r)
			}
			err = w.Save()
		}
		release()
		if err != nil {
			return err
		}
	}
	e, ok := w.Find(addr)
	if !ok {
		return fmt.Errorf("no request to %s in this wallet", addr.Short())
	}
	n, err := f.node(out, s)
	if err != nil {
		return err
	}
	rc, err := w.Receive(s, n, e)
	if err != nil {
		return err
	}
	return writeJSON(out, struct {
		wallet.Receipt
		Settled bool `json:"settled"`
	}{rc, rc.Settled(th)})
}

// sentStatus is one row of poo wallet status.
type sentStatus struct {
	To      hash.Hash          `json:"to"`
	Amount  uint64             `json:"amount"`
	Memo    string             `json:"memo,omitempty"`
	Claim   bool               `json:"claim,omitempty"`
	Legs    []wallet.LegStatus `json:"legs"`
	Settled bool               `json:"settled"`
}

func walletStatus(fs *flag.FlagSet, f *walletFlags, args []string, out io.Writer) error {
	threshold := fs.String("threshold", "", "from(e) needed for settled (default 0)")
	o := addOutput(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := o.check(); err != nil {
		return err
	}
	th, err := parseThreshold(*threshold)
	if err != nil {
		return err
	}
	s, err := f.series()
	if err != nil {
		return err
	}
	w, err := f.open()
	if err != nil {
		return err
	}
	n, err := f.node(out, s)
	if err != nil {
		return err
	}
	// The node is asked without the wallet held, so status never waits
	// behind a command waiting for the node, or makes one wait. The legs'
	// new states are then saved under the hold, each only where the file
	// still has the state this command read, so a change another command
	// made meanwhile (a resend) stands (RED-042, D148).
	type change struct{ was, now string }
	changed := map[hash.Hash]change{}
	rows := []sentStatus{}
	for _, p := range w.Sent() {
		if p.Series != s.ID() {
			continue
		}
		was := make([]string, len(p.Legs))
		for i, l := range p.Legs {
			was[i] = l.State
		}
		legs, err := w.Refresh(s, n, p)
		if err != nil {
			return err
		}
		for i, l := range p.Legs {
			if l.State != was[i] {
				changed[l.TID] = change{was[i], l.State}
			}
		}
		rows = append(rows, sentStatus{p.To, p.Amount, p.Memo, p.Claim, legs, wallet.Settled(legs, th)})
	}
	if len(changed) > 0 {
		release, err := holdWallet(w, out)
		if err != nil {
			return err
		}
		for _, p := range w.Sent() {
			for i := range p.Legs {
				if c, ok := changed[p.Legs[i].TID]; ok && p.Legs[i].State == c.was {
					p.Legs[i].State = c.now
				}
			}
		}
		err = w.Save()
		release()
		if err != nil {
			return err
		}
	}
	return o.write(out, rows)
}

func walletResend(fs *flag.FlagSet, f *walletFlags, args []string, out io.Writer) error {
	if err := fs.Parse(args); err != nil {
		return err
	}
	s, err := f.series()
	if err != nil {
		return err
	}
	w, err := f.open()
	if err != nil {
		return err
	}
	n, err := f.node(out, s)
	if err != nil {
		return err
	}
	release, err := holdWallet(w, out)
	if err != nil {
		return err
	}
	defer release()
	total := 0
	for _, p := range w.Sent() {
		if p.Series != s.ID() {
			continue
		}
		if _, err := w.Refresh(s, n, p); err != nil {
			return err
		}
		k, err := w.Resend(s, n, p)
		total += k
		if err != nil {
			_ = w.Save()
			return err
		}
	}
	if err := w.Save(); err != nil {
		return err
	}
	fmt.Fprintf(out, "resent %d legs\n", total)
	return nil
}

// parsePolicyKey reads self, ALG:HEX or HEX (Ed25519). ALG is a key
// algorithm's name or number.
func parsePolicyKey(v string) (wallet.PolicyKey, error) {
	if v == "self" {
		return wallet.PolicyKey{}, nil
	}
	alg := uint32(policy.AlgEd25519)
	if i := strings.IndexByte(v, ':'); i >= 0 {
		a, err := parseAlg(v[:i])
		if err != nil {
			return wallet.PolicyKey{}, fmt.Errorf("-key %q: %v", v, err)
		}
		alg, v = uint32(a), v[i+1:]
	}
	pub, err := hex.DecodeString(v)
	if err != nil || len(pub) == 0 {
		return wallet.PolicyKey{}, fmt.Errorf("-key %q: want self, ALG:HEX or HEX", v)
	}
	return wallet.PolicyKey{Alg: alg, Public: pub}, nil
}

func walletPolicy(fs *flag.FlagSet, f *walletFlags, args []string, out io.Writer) error {
	name := fs.String("name", "", "the policy's name in this wallet")
	k := fs.Uint("k", 1, "signatures needed")
	label := fs.String("label", "", "what the policy is for")
	var keys stringList
	fs.Var(&keys, "key", "self (a fresh key of this wallet), or a co-signer's public key (repeatable)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *k == 0 || *k > 0xffff {
		return errors.New("-k out of range")
	}
	var pks []wallet.PolicyKey
	for _, v := range keys {
		pk, err := parsePolicyKey(v)
		if err != nil {
			return err
		}
		pks = append(pks, pk)
	}
	s, err := f.series()
	if err != nil {
		return err
	}
	w, err := f.unlocked(out)
	if err != nil {
		return err
	}
	release, err := holdWallet(w, out)
	if err != nil {
		return err
	}
	defer release()
	pi, err := w.AddPolicy(*name, uint16(*k), pks, *label)
	if err != nil {
		return err
	}
	p := pi.Params().Policy()
	if err := wallet.CheckPolicy(s, p); err != nil {
		return err
	}
	if err := w.Save(); err != nil {
		return err
	}
	return writeJSON(out, map[string]any{"policy": pi, "address": s.Address(p)})
}

func walletCosign(fs *flag.FlagSet, f *walletFlags, args []string, out io.Writer) error {
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: poo wallet cosign FILE")
	}
	file := fs.Arg(0)
	var u wallet.Unsigned
	if err := readJSON(file, &u); err != nil {
		return err
	}
	s, err := f.series()
	if err != nil {
		return err
	}
	w, err := f.unlocked(out)
	if err != nil {
		return err
	}
	added, err := w.Cosign(s, &u)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "added %d signatures; still missing %v\n", added, u.Missing())
	return writeOut(out, file, &u)
}

func walletSubmit(fs *flag.FlagSet, f *walletFlags, args []string, out io.Writer) error {
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: poo wallet submit FILE")
	}
	var u wallet.Unsigned
	if err := readJSON(fs.Arg(0), &u); err != nil {
		return err
	}
	s, err := f.series()
	if err != nil {
		return err
	}
	w, err := f.open()
	if err != nil {
		return err
	}
	n, err := f.node(out, s)
	if err != nil {
		return err
	}
	release, err := holdWallet(w, out)
	if err != nil {
		return err
	}
	defer release()
	p, err := w.Submit(s, n, &u)
	if p != nil {
		if serr := saveSent(w, out, "payment", p, err); serr != nil {
			return serr
		}
	}
	if err != nil {
		return err
	}
	return writeJSON(out, p)
}

// saveSent saves w after a payment or claim p went to the node (sendErr
// is what sending it returned). The caller holds w (Wallet.Hold), so no
// other command's save comes between; if the save still fails, the record
// is printed and the error says what was sent and that it must not be
// sent again, since running the command again would pay a second time
// (RED-039, D146).
func saveSent(w *wallet.Wallet, out io.Writer, what string, p *wallet.Sent, sendErr error) error {
	err := w.Save()
	if err == nil {
		return nil
	}
	sent := false
	for _, l := range p.Legs {
		if l.State == wallet.StatePending || l.State == wallet.StateSelected {
			sent = true
		}
	}
	if !sent && sendErr != nil {
		return fmt.Errorf("%w (and the wallet was not saved: %v)", sendErr, err)
	}
	if !sent {
		return err
	}
	writeJSON(out, p)
	why := err.Error()
	if errors.Is(err, wallet.ErrChanged) {
		// ErrChanged says to run the command again, which is wrong here.
		why = "the wallet file was changed or removed by something else"
	}
	return fmt.Errorf("the %s above was sent to the node, but the wallet could not record it (%s). Do not run the command again: that would send it a second time. Keep the record above", what, why)
}

// holdWallet holds w (Wallet.Hold) and, when another command holds it,
// says so on the invocation's error output before it waits, so a waiting
// command, the GUI's terminal included, is never silent (RED-042, D148).
func holdWallet(w *wallet.Wallet, out io.Writer) (func(), error) {
	w.OnBusy(func() {
		fmt.Fprintf(errOf(out), "waiting for another poo wallet command (or the GUI) to finish with %s\n", w.Path())
	})
	return w.Hold()
}
