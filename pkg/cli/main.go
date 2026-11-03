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
	"strings"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/policy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/series"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/testutil"
)

// Version is the tool's version, set at build time with -ldflags.
var Version = "0.1.0-dev"

// Commit is the source commit, set by scripts/release.sh with -X, where
// the build info has none (GREEN-013).
var Commit = ""

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
  poo claim -seed-file FILE -serial N
                              claim a whole mint for a key's address
  poo pay -seed-file FILE -serial N -to ADDR -amount A
                              pay from a key's address, change back to it
  poo build -pool K | -serial N [-work N] [-device D] [-threads N] [-for 30s]
                              build an entry from the pending transactions
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
  poo keygen [-alg NAME] [-hash NAME] [-seed-file FILE]
                              make a key (default Ed25519) and its 1-of-1
                              address under a hash (default SHA-256)
  poo series example          print the worked example's series as JSON
  poo series new [-hash NAME] [-algs LIST] [-types LIST] [-mints N] ...
                              print a new series' params, checked
  poo series info FILE        print the id and pools of a series JSON file
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
		return 1
	}
	return 0
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
		return cryptoCmd(out)
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
	case "pools", "ledger", "pending", "unspent", "tx", "address":
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
	if err := fs.Parse(args); err != nil {
		return err
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
		s, err := f.read(out, false)
		if err != nil {
			return err
		}
		*seedHex = s
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
	pol := policy.SingleKey(alg, signer.Public())
	// The address depends only on the series' hash: every series using it
	// gives the same address for this policy.
	h, _ := hash.Lookup(hid)
	addr := policy.Address(h, pol).String()
	a, _ := policy.Default().Alg(alg)
	v := map[string]string{
		"seed":       hex.EncodeToString(seed),
		"alg":        a.Name(),
		"public_key": hex.EncodeToString(signer.Public()),
		"policy":     hex.EncodeToString(pol.Encode()),
		"hash":       hid.Name(),
		"address":    addr,
	}
	if hid == hash.SHA256 {
		v["address_sha256"] = addr // the field's name before there was a choice
	}
	return writeJSON(out, v)
}

func seriesCmd(args []string, out io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("series needs a subcommand: example, new or info")
	}
	switch args[0] {
	case "example":
		return writeJSON(out, testutil.ExampleParams())
	case "new":
		return seriesNew(args[1:], out)
	case "info":
		if len(args) != 2 {
			return fmt.Errorf("usage: poo series info FILE")
		}
		b, err := os.ReadFile(args[1])
		if err != nil {
			return err
		}
		var p series.Params
		if err := json.Unmarshal(b, &p); err != nil {
			return err
		}
		s, err := series.New(p)
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
			ID    string `json:"series_id"`
			Pools []pool `json:"pools"`
		}{ID: s.ID().String()}
		for k := uint64(0); k < s.PoolCount() && k < 1000; k++ {
			f, l := s.PoolRange(k)
			info.Pools = append(info.Pools, pool{k, f, l, s.PoolID(k).String()})
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
var ownHelp = map[string]bool{"grind": true, "sim": true, "wallet": true}

// runsBare names the commands made of subcommands that also run with no
// subcommand, with flags of their own: -h lists those flags, and "help"
// the usage lines (poo peers, D157).
var runsBare = map[string]bool{"peers": true}

func isHelpWord(s string) bool { return s == "help" || s == "-h" || s == "--help" }
