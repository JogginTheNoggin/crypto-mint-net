// Package cli is the poo command: the Proof of Ownership full node's
// command-line tool, as a library. cmd/poo is a few lines around Main, and
// the GUI's terminal and every GUI action call the same Main, so the
// headless tool and the GUI always run the same code (docs/gui.md).
//
// It runs the node daemon (poo node), which stores what it accepts and
// serves a local RPC, and talks to a running node: status, claims and
// payments from a single key, building entries and adding layers, and
// reading where a payment stands. It also manages node policies and has
// offline commands for keys, series and an in-process demo. The node
// finds peers from its node policy, address registries and its peers, ranks
// them, and tracks how they behave; the owner can inspect and act on them
// (poo peers, poo peer, poo tracking, poo chases). poo wallet keeps a
// holder's keys and pays, requests and watches payments through the node.
package cli

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/node"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/nodepolicy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/policy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/rpc"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/series"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/testutil"
)

// Version is the tool's version, set at build time with -ldflags.
var Version = "0.1.0-alpha.1-dev"

// Commit is the source commit, set by scripts/release.sh with -X, where
// the build info has none (GREEN-013).
var Commit = ""

// A release build records no VCS information, so the node names the
// build its snapshots are written by from these (RED-087, D181).
func init() { node.SetRelease(Version, releaseCommit()) }

const usage = `poo: Proof of Ownership full node tool

Usage:
  poo init [-example] [-series FILE]... [-p2p ADDR] [-peer ADDR]...
                              write a node policy and make it current
  poo node [-log-level L] [-log-format F]
                              run the node until interrupted
  poo peers [-category LIST] [-mint-policy TEXT] [-label TEXT] [-addr TEXT]
                              the running node's peers, their categories
                              and points, and its peer book, filtered
                              (blocked, timed out: poo tracking)
  poo peers -counts           only the book's size and counts per state
  poo peers export [-category LIST] [-out FILE]
                              write the peer book, or some categories of
                              it, and the blocks to one shareable file
  poo peers import FILE|- [-apply] [-blocks [-ranges]] [-if-digest D]
                              add such a file's addresses, unlisted, and
                              with -blocks its blocks; shows the plan first
  poo peer timeout|block|unblock|clear|template|delete ADDR
           [-seconds N] [-range] [-what flags|measures|all] [-template NAME]
                              act on one peer
  poo peer add|remove|label|connect ADDR [-label TEXT] [-keep]
                              add a peer by hand, take one out, label it,
                              or dial it now (connect -imported: every
                              address an imported file gave)
  poo tracking [ADDR] [-blocked] [-timed-out]
                              what the tracker keeps: every address's
                              status, or one address's record
  poo chases                  the chase queue of heavier stated weights
  poo status                  every followed pool of the running node
  poo doctor [-json]          check the node's setup and say what to fix
  poo follow [-issuer ID | -series ID [-pool K]... [-mint N]...] [-no-reload]
                              follow more; applied by a running node at once
                              (no flags: show what is followed)
  poo unfollow -issuer ID | -series ID [-pool K]... [-mint N]...
                              follow less, the same way
  poo reload                  make the running node apply its node policy
  poo pools [FILTER]          followed pools: tip, height, weight, pending
  poo ledger -pool K [-below H] [-limit N] [FILTER]
                              the selected ledger, newest entry first
  poo ledger export -pool K [-from H] [-to H] [-out FILE]
                              a stretch of the selected ledger, oldest
                              first, as raw entries anyone can check
  poo ledger import [-dry-run] FILE|-
                              hand a ledger file's entries to the node,
                              which checks every one in full
  poo pending [FILTER]        pending transactions
  poo unspent [FILTER]        unspent portions
  poo tx [-series ID] [-in-serial N] TID
                              one transaction and where it stands
  poo address [-series ID] [-below H] [-limit N] [FILTER] ADDRESS
                              an address's balance, history and pending
  poo watch [-notices] [-once] [FILTER]
                              live events, one JSON object per line
                              FILTER: -series -pool -serial -address
                              -address-policy -tid -kind (repeatable, comma lists)
  poo claim -seed-file FILE [-alg NAME] -serial N [-parent HASH|root|before-claim [-work N]]
                              claim a whole mint for a key's address; with
                              -parent, build it on that branch to outweigh
                              a claim the selected ledger holds
  poo pay -seed-file FILE [-alg NAME] -serial N -to ADDR -amount A
                              pay from a key's address, change back to it
  poo build -pool K | -serial N [-parent HASH|root|before-claim] [-work N] [-device D] [-threads N] [-for 30s]
                              build an entry from the pending transactions,
                              on the selected tip or the -parent named
  poo work -pool K | -serial N [-entry HASH] [-tries N | -for 30s] [-device D] [-threads N]
                              add a layer (default: on the selected tip)
  poo submit entry|layer -pool K | -serial N HEX
                              hand the node an entry or layer made elsewhere
  poo grind devices|bench|start|run|schedule|jobs|stop
                              the node's hash work: devices, speed, and jobs
                              by hand, scheduled or automatic (poo grind help)
  poo payment -serial N -tid HASH
                              where a transaction stands (older form of
                              poo tx, kept for scripts)
  poo holdings -serial N      unspent portions of a mint (older form of
                              poo unspent -serial N, kept for scripts)
  poo policy list|use|archive|unarchive|delete [-root DIR] [NAME]
                              manage saved and archived node policies
  poo policy limits [-set NAME=VALUE]... [-o FORMAT]
                              the settable limits: range, default, value;
                              -set changes them, live in a running node
  poo policy export [-all | -series ID] [-full] [-out FILE]
                              write node policies to one shareable file
  poo policy import FILE|- [-apply] [-settings | -full] [-if-digest D]
                              merge such a file into the node policies here:
                              shows the plan, changes them only with -apply
  poo settings export [-full] [-out FILE]
                              write this node's settings (not what it
                              follows) to one file, for another machine
  poo settings import FILE|- [-apply] [-full] [-map-path OLD=NEW]
                              merge such a file into this node's settings:
                              shows the plan and what may not fit here
  poo wallet COMMAND          keys from one seed, payments, requests,
                              co-signing and light mode (poo wallet help)

  poo version [-v]            print the version (-v: Go, platform, commit
                              and dependencies, for a bug report)
  poo crypto                  list the hashes, key algorithms and policy
                              types a series can choose from
  poo keygen [-alg NAME] [-hash NAME] [-seed-file FILE [-raw]]
                              make a key (default Ed25519) and its 1-of-1
                              address under a hash (default SHA-256); with
                              -raw, show an existing key's public key and
                              address (D193)
  poo sign FILE|- -key-file F [-alg NAME] [-raw] [-hash NAME] [-out FILE] [-show]
                              sign a payment waiting for signatures with a
                              key kept off the node; needs no node (D193)
  poo bounty post -entry HASH [-pool K | -serial N] -strength S -amount A [-count N] [-open SECONDS]
                              offer to pay for layers on an entry (D196)
  poo bounty list [-ask] [-max-strength S] [-done N]
                              this node's offers and those peers sent; -done
                              adds its last N finished ones
  poo bounty work ID -to ADDR [-max-seconds N] [-max-strength S]
                              lease a peer's offer, grind the layer, claim it
  poo bounty pay ID [-claim NONCE] [-wallet FILE]
                              pay an accepted claim from the wallet
  poo bounty cancel ID        close one of this node's offers
  poo series example          print the worked example's series as JSON
  poo series new [-hash NAME] [-algs LIST] [-types LIST] [-mints N] ...
                              print a new series' params, checked
  poo series info FILE        print the id, supply type and pools of a series
                              file (params or signed)
  poo series sign FILE [-seed-file F] [-alg NAME]
                              sign a series as its issuer: the
                              issuer_witness for node.json
  poo series release FILE -part K [-seed-file F] [-alg NAME] | -check RELEASE
                              release part K of a variable series as its
                              issuer, or check a release (D163)
  poo series parts FILE [-from K] [-to K] [-o table]
                              a variable series' parts: serials, pools
                              and part ids
  poo series add-release RELEASE
                              give the node a release of a variable
                              series: it follows the new parts' mints
  poo series followed         the series the running node follows, with
                              the parts it has released (D166)
  poo demo [-quiet]           run in-process full nodes through a payment,
                              a double spend and its resolution
  poo sim load|partition|attack|sizing [flags]
                              run the deterministic network simulator
                              (poo sim help)
  poo help [COMMAND]          this text, or one command's usage
  poo completion bash|zsh|fish
                              print a shell completion script

Node commands pick a data directory with -dir, or with -root (default
$POO_HOME or ~/.poo) and -policy (default: the current node policy).
Client commands take -series ID when the node follows more than one series.
Read commands print JSON; -o table prints columns for reading (-wide: full hashes).

poo -view-only COMMAND (or POO_VIEW_ONLY=1) only looks: it dials the node
with rpc.view-token, which the node lets read and nothing else, and refuses
every command that would change something.
`

// Stdio is where a command reads and writes. In is read for passphrases
// and confirmations; nil reads nothing.
type Stdio struct {
	In       io.Reader
	Out, Err io.Writer
	// Secret, if set, reads a passphrase or a seed in place of In, so the
	// caller can hide what is typed: a terminal with echo off, a GUI's
	// masked field. prompt says what is asked for. Nil reads a line of In.
	Secret func(prompt string) (string, error)
	// ViewOnly runs the invocation in view-only mode (see scope.go), as
	// $POO_VIEW_ONLY=1 or a leading -view-only argument also do.
	ViewOnly bool
}

// Main runs the command line args (without the program name) and returns
// the process exit code: errors are printed to Err as "poo: <error>".
// Cancelling ctx stops a long-running command such as poo node, as an
// interrupt does from a shell.
func Main(ctx context.Context, args []string, s Stdio) int {
	if err := Run(ctx, args, s); err != nil {
		fmt.Fprintln(s.Err, "poo:", err)
		return ExitCode(err)
	}
	return 0
}

// The exit codes of poo: 0 done, 1 refused or failed, and ExitWaiting.
const (
	ExitFailed = 1
	// ExitWaiting: refused only because the mint's part is not released at
	// this node yet (D166), as the RPC's 409 waiting says. It is not an
	// invalid mint, and the same command works once the release arrives,
	// from a peer or from poo series add-release (D168).
	ExitWaiting = 3
)

// ExitCode returns the exit code for a command's error.
func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	var re *rpc.Error
	if errors.Is(err, series.ErrWaiting) || (errors.As(err, &re) && re.Code == rpc.CodeWaiting) {
		return ExitWaiting
	}
	return ExitFailed
}

// Run runs the command line args and returns its error.
func Run(ctx context.Context, args []string, s Stdio) error {
	in := s.In
	if in == nil {
		in = strings.NewReader("")
	}
	view := s.ViewOnly || viewOnlyEnv()
	if len(args) > 0 && (args[0] == "-view-only" || args[0] == "--view-only") {
		view, args = true, args[1:]
	}
	if view {
		if err := checkViewOnly(args); err != nil {
			return err
		}
	}
	err := run(args, &stdio{Writer: s.Out, err: s.Err, in: bufio.NewReader(in), ctx: ctx, secret: s.Secret, viewOnly: view})
	if errors.Is(err, flag.ErrHelp) {
		// -h printed the usage the person asked for: that is not a failure,
		// so it neither exits 1 nor adds "flag: help requested" (GREEN-032).
		return nil
	}
	return err
}

// stdio is the out of one invocation: commands write to it, and the few
// that need input or cancellation get them from it (inputOf, contextOf).
// Carrying them here keeps every command's signature (args, out) and
// leaves no state shared between invocations.
type stdio struct {
	io.Writer
	err io.Writer     // where flag errors and -h go; nil: os.Stderr
	in  *bufio.Reader // one reader, so successive lines are not lost
	ctx context.Context
	// probe, when set, is given every flag set a command makes, and the
	// flag sets print nothing: poo __complete runs a command with -h to
	// learn its flags (completion.go).
	probe func(*flag.FlagSet)
	// secret reads a secret (Stdio.Secret); nil reads a line of in.
	secret func(prompt string) (string, error)
	// viewOnly: the invocation may only look (scope.go).
	viewOnly bool
}

// newFlagSet is the flag set of a command writing to out. Its usage and
// errors go to the invocation's Err, so the GUI's terminal shows them as a
// shell does, rather than the GUI process's own stderr.
func newFlagSet(name string, out io.Writer) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	if s, ok := out.(*stdio); ok {
		if s.err != nil {
			fs.SetOutput(s.err)
		}
		if s.probe != nil {
			fs.SetOutput(io.Discard)
			s.probe(fs)
		}
	}
	return fs
}

// inputOf returns the input of the invocation writing to out.
func inputOf(out io.Writer) *bufio.Reader {
	if s, ok := out.(*stdio); ok {
		return s.in
	}
	return bufio.NewReader(strings.NewReader(""))
}

// contextOf returns the context of the invocation writing to out.
func contextOf(out io.Writer) context.Context {
	if s, ok := out.(*stdio); ok && s.ctx != nil {
		return s.ctx
	}
	return context.Background()
}

// errOf returns where the invocation writing to out sends warnings: its
// Err, as a shell's stderr, or os.Stderr.
func errOf(out io.Writer) io.Writer {
	if s, ok := out.(*stdio); ok && s.err != nil {
		return s.err
	}
	return os.Stderr
}

func run(args []string, out io.Writer) error {
	if len(args) == 0 {
		fmt.Fprint(out, usage)
		return nil
	}
	if len(args) == 2 && isHelpWord(args[1]) && !ownHelp[args[0]] && len(subcommands(args[0])) > 0 {
		if runsBare[args[0]] && args[1] != "help" {
			// Its usage lines, then the flags it takes with no
			// subcommand, from its own flag set (D157).
			if err := helpCmd(args[:1], out); err != nil {
				return err
			}
			fmt.Fprintln(out)
			if args[0] == "ledger" {
				return viewCmd(args[0], args[1:], out)
			}
			return peersCmd(args[1:], out)
		}
		// "poo policy help", "-h" or "--help": a command made of
		// subcommands prints its usage lines, as poo help policy does,
		// and exits 0, where it took the word for a subcommand or a
		// NAME and failed (GREEN-045).
		return helpCmd(args[:1], out)
	}
	switch args[0] {
	case "version":
		return versionCmd(args[1:], out)
	case "keygen":
		return keygen(args[1:], out)
	case "crypto":
		return cryptoCmd(args[1:], out)
	case "series":
		return seriesCmd(args[1:], out)
	case "demo":
		fs := newFlagSet("demo", out)
		quiet := fs.Bool("quiet", false, "print only the summary")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		return demo(out, *quiet)
	case "init":
		return initCmd(args[1:], out)
	case "node":
		return nodeCmd(args[1:], out)
	case "status":
		return statusCmd(args[1:], out)
	case "doctor":
		return doctorCmd(args[1:], out)
	case "ledger":
		if len(args) > 1 && (args[1] == "export" || args[1] == "import") {
			return ledgerPortCmd(args[1:], out)
		}
		return viewCmd(args[0], args[1:], out)
	case "pools", "pending", "unspent", "tx", "address":
		return viewCmd(args[0], args[1:], out)
	case "watch":
		return watchCmd(args[1:], out)
	case "follow":
		return followCmd(false, args[1:], out)
	case "unfollow":
		return followCmd(true, args[1:], out)
	case "reload":
		return reloadCmd(args[1:], out)
	case "claim":
		return claimCmd(args[1:], out)
	case "sign":
		return signCmd(args[1:], out)
	case "bounty":
		return bountyCmd(args[1:], out)
	case "pay":
		return payCmd(args[1:], out)
	case "build":
		return buildCmd(args[1:], out)
	case "work":
		return workCmd(args[1:], out)
	case "submit":
		return submitCmd(args[1:], out)
	case "payment":
		return paymentCmd(args[1:], out)
	case "peers":
		return peersCmd(args[1:], out)
	case "peer":
		return peerCmd(args[1:], out)
	case "grind":
		return grindCmd(args[1:], out)
	case "tracking":
		return trackingCmd(args[1:], out)
	case "chases":
		return chasesCmd(args[1:], out)
	case "holdings":
		return holdingsCmd(args[1:], out)
	case "policy":
		return policyCmd(args[1:], out)
	case "settings":
		return settingsCmd(args[1:], out)
	case "wallet":
		return walletCmd(args[1:], out)
	case "sim":
		return simCmd(args[1:], out)
	case "completion":
		return completionCmd(args[1:], out)
	case "__complete":
		return completeCmd(args[1:], out)
	case "help", "-h", "--help":
		return helpCmd(args[1:], out)
	default:
		return unknownCommand(args[0])
	}
}

func keygen(args []string, out io.Writer) error {
	fs := newFlagSet("keygen", out)
	seedHex := fs.String("seed", "", "32-byte seed in hex, for test vectors only: other users can read the command line (random if empty)")
	seedFile := fs.String("seed-file", "", "file holding the seed in hex, - for standard input (random if neither)")
	algName := fs.String("alg", "ed25519", "key algorithm, by name or number (poo crypto)")
	hashName := fs.String("hash", "sha256", "hash of the series the address is for")
	raw := fs.Bool("raw", false, "with -seed-file: the file holds an existing private key in its algorithm's usual form, as other tools keep it; prints its public key and address, never the key (D193)")
	keyOut := fs.String("out", "", "write a new random seed to this file (readable by you only, never replacing a file) and print only the public key and addresses: a key file for poo sign, claim and pay -seed-file")
	w := addWhere(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *keyOut != "" && (*seedHex != "" || *seedFile != "" || *raw) {
		return errors.New("-out makes a new key; it does not go with -seed, -seed-file or -raw")
	}
	alg, err := parseAlg(*algName)
	if err != nil {
		return err
	}
	hid, err := parseHash(*hashName)
	if err != nil {
		return err
	}
	if *seedHex != "" && *seedFile != "" {
		return errors.New("give -seed or -seed-file, not both")
	}
	if *seedFile != "" {
		f := &seedFlags{file: seedFile, flag: new(string)}
		if *raw {
			f.what = "key"
		}
		s, err := f.read(out, false)
		if err != nil {
			return err
		}
		*seedHex = s
	}
	if *raw {
		if *seedFile == "" {
			return errors.New("-raw reads an existing key from -seed-file")
		}
		key, err := hex.DecodeString(*seedHex)
		if err != nil || len(key) == 0 {
			return errors.New("the key file must hold the key in hex")
		}
		signer, err := policy.NewRawSigner(alg, key)
		clear(key)
		if err != nil {
			return err
		}
		return keygenOut(out, w, alg, hid, signer, nil, "")
	}
	seed := make([]byte, policy.SeedSize)
	if *seedHex != "" {
		b, err := hex.DecodeString(*seedHex)
		if err != nil || len(b) != policy.SeedSize {
			return fmt.Errorf("seed must be 64 hex digits")
		}
		seed = b
	} else if _, err := rand.Read(seed); err != nil {
		return err
	}
	signer, err := policy.NewSigner(alg, seed)
	if err != nil {
		return err
	}
	if *keyOut != "" {
		// The seed goes to the file only, never to the screen, so a key
		// meant to stay off the node is not left in a terminal's
		// scrollback (GREEN-060).
		if err := writeNewKeyFile(*keyOut, seed); err != nil {
			return err
		}
		clear(seed)
		return keygenOut(out, w, alg, hid, signer, nil, *keyOut)
	}
	return keygenOut(out, w, alg, hid, signer, seed, "")
}

// writeNewKeyFile writes seed in hex to a new file only its owner can
// read, refusing to replace one: an existing key file may be the only
// copy of a key.
func writeNewKeyFile(path string, seed []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, os.ErrExist) {
		return fmt.Errorf("%s exists: a key file is never replaced; name a new file", path)
	}
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintln(f, hex.EncodeToString(seed)); err != nil {
		f.Close()
		os.Remove(path)
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// keygenOut prints a key's public key, policy and addresses, and its seed
// when it has one (a raw key's private half is never printed).
func keygenOut(out io.Writer, w *where, alg policy.AlgID, hid hash.ID, signer policy.Signer, seed []byte, keyFile string) error {
	pol := policy.SingleKey(alg, signer.Public())
	// The address depends only on the series' hash: every series using it
	// gives the same address for this policy.
	h, _ := hash.Lookup(hid)
	addr := policy.Address(h, pol).String()
	a, _ := policy.Default().Alg(alg)
	v := map[string]any{
		"alg":        a.Name(),
		"public_key": hex.EncodeToString(signer.Public()),
		"policy":     hex.EncodeToString(pol.Encode()),
		"hash":       hid.Name(),
		"address":    addr,
	}
	if seed != nil {
		v["seed"] = hex.EncodeToString(seed)
	}
	if keyFile != "" {
		v["key_file"] = keyFile
		v["key_note"] = "the seed is in key_file only, readable by you; back it up, it is the only copy"
	}
	if hid == hash.SHA256 {
		v["address_sha256"] = addr // the field's name before there was a choice
	}
	// The address under each series the node policy lists, which is the
	// one to give a payer: poo init follows the reserve series by default
	// (D170), whose hash is SHA-512/256, so the sha256 "address" above is
	// not this key's address there (GREEN-052).
	if fa, note := followedAddresses(w, pol, alg, hid); len(fa) > 0 {
		v["series_addresses"] = fa
		if note != "" {
			v["note"] = note
		}
	}
	return writeJSON(out, v)
}

// seriesAddress is a key's address in one series a node policy lists.
type seriesAddress struct {
	Series  string `json:"series"`
	Builtin string `json:"builtin,omitempty"`
	Hash    string `json:"hash"`
	Allowed bool   `json:"allowed"`
	Address string `json:"address"`
}

// followedAddresses gives pol's address in each series the node policy at
// w lists, and a note when "address" (hash hid) is the address in none of
// them. No node policy, or one that cannot be read, gives none: keygen
// works without a node.
func followedAddresses(w *where, pol policy.Policy, alg policy.AlgID, hid hash.ID) ([]seriesAddress, string) {
	p, err := nodepolicy.Load(filepath.Join(w.resolve(), nodepolicy.FileName))
	if err != nil {
		return nil, ""
	}
	var fa []seriesAddress
	matched := false
	for _, si := range p.Series {
		s, err := series.New(si.Params)
		if err != nil {
			continue
		}
		a := seriesAddress{Series: s.ID().String(), Hash: si.Params.HashID.Name(), Allowed: s.AllowsKeyAlg(alg), Address: s.Address(pol).String()}
		if s.ID().String() == series.ReserveID {
			a.Builtin = rpc.BuiltinDefault
		}
		matched = matched || si.Params.HashID == hid
		fa = append(fa, a)
	}
	if len(fa) == 0 || matched {
		return fa, ""
	}
	return fa, fmt.Sprintf("address is for a %s series; this node's series use another hash, so give a payer the address in series_addresses (or pass -hash)", hid.Name())
}

func seriesCmd(args []string, out io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("series needs a subcommand: example, new, info, sign, release, parts, add-release or followed")
	}
	switch args[0] {
	case "example":
		return writeJSON(out, testutil.ExampleParams())
	case "new":
		return seriesNew(args[1:], out)
	case "sign":
		return seriesSign(args[1:], out)
	case "release":
		return seriesRelease(args[1:], out)
	case "parts":
		return seriesParts(args[1:], out)
	case "add-release":
		return seriesAddRelease(args[1:], out)
	case "followed":
		return seriesFollowed(args[1:], out)
	case "info":
		const infoUsage = "usage: poo series info FILE (a series params file or a signed series file)"
		if len(args) == 2 && (args[1] == "-h" || args[1] == "-help" || args[1] == "--help") {
			fmt.Fprintln(out, infoUsage)
			return nil
		}
		if len(args) != 2 {
			return errors.New(infoUsage)
		}
		s, err := readSeriesFile(args[1])
		if err != nil {
			return err
		}
		type pool struct {
			Index uint64 `json:"index"`
			First uint64 `json:"first_serial"`
			Last  uint64 `json:"last_serial"`
			ID    string `json:"pool_id"`
		}
		info := struct {
			ID         string    `json:"series_id"`
			Builtin    string    `json:"builtin,omitempty"`
			SupplyType string    `json:"supply_type,omitempty"`
			PartSize   uint64    `json:"part_size,omitempty"`
			MaxParts   uint64    `json:"max_parts,omitempty"`
			Pools      []pool    `json:"pools"`
			Parts      []partRow `json:"parts,omitempty"`
			// Warning: no witness ceiling, or one above CeilingLimit
			// (RED-083).
			Warning string `json:"warning,omitempty"`
		}{ID: s.ID().String(), Builtin: rpc.Builtin(s.ID()), Warning: nodepolicy.CeilingWarning(s.Params())}
		for k := uint64(0); k < s.PoolCount() && k < 1000; k++ {
			f, l := s.PoolRange(k)
			info.Pools = append(info.Pools, pool{k, f, l, s.PoolID(k).String()})
		}
		// A variable series also lists its first parts, each as ranges
		// (poo series parts lists any others).
		if s.IsVariable() {
			info.SupplyType, info.PartSize, info.MaxParts = "variable", s.PartSize(), s.MaxParts()
			if info.Parts, err = partRows(contextOf(out), s, 1, 10); err != nil {
				return err
			}
		}
		return writeJSON(out, info)
	default:
		return fmt.Errorf("unknown series subcommand %q", args[0])
	}
}

func writeJSON(out io.Writer, v any) error {
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// ownHelp names the commands whose own usage answers "help" and "-h"
// (grindUsage, simUsage, walletUsage).
var ownHelp = map[string]bool{"grind": true, "sim": true, "wallet": true, "bounty": true}

// runsBare names the commands made of subcommands that also run with no
// subcommand, with flags of their own: -h lists those flags, and "help"
// the usage lines (poo peers, D157; poo ledger, D188).
var runsBare = map[string]bool{"peers": true, "ledger": true}

func isHelpWord(s string) bool { return s == "help" || s == "-h" || s == "--help" }
