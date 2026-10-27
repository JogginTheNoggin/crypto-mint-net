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

const usage = `poo: Proof of Ownership full node tool

Usage:
  poo init [-example] [-series FILE]... [-p2p ADDR] [-peer ADDR]...
                              write a node policy and make it current
  poo node                    run the node until interrupted
  poo peers                   the running node's peers, their categories
                              and points, and its peer book
  poo peer timeout|block|unblock|clear|template|delete ADDR
           [-seconds N] [-range] [-what flags|measures|all] [-template NAME]
                              act on one peer
  poo tracking [ADDR]         what the tracker keeps: every address's
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
  poo address [-series ID] [-limit N] [FILTER] ADDRESS
                              an address's balance, history and pending
  poo watch [-notices] [-once] [FILTER]
                              live events, one JSON object per line
                              FILTER: -series -pool -serial -address
                              -address-policy -tid -kind (repeatable, comma lists)
  poo claim -seed HEX -serial N
                              claim a whole mint for a key's address
  poo pay -seed HEX -serial N -to ADDR -amount A
                              pay from a key's address, change back to it
  poo build -pool K           build an entry from the pending transactions
  poo work -pool K [-entry HASH] [-tries N]
                              add a layer (default: on the selected tip)
  poo payment -serial N -tid HASH
                              where a transaction stands, and its weight
  poo holdings -serial N      unspent portions of a mint
  poo policy list|use|archive|unarchive|delete [-root DIR] [NAME]
                              manage saved and archived node policies
  poo wallet COMMAND          keys from one seed, payments, requests,
                              co-signing and light mode (poo wallet help)

  poo version                 print the version
  poo crypto                  list the hashes, key algorithms and policy
                              types a series can choose from
  poo keygen [-alg NAME] [-hash NAME] [-seed HEX]
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

Node commands pick a data directory with -dir, or with -root (default
$POO_HOME or ~/.poo) and -policy (default: the current node policy).
Client commands take -series ID when the node follows more than one series.
`

// Stdio is where a command reads and writes. In is read for passphrases
// and confirmations; nil reads nothing.
type Stdio struct {
	In       io.Reader
	Out, Err io.Writer
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
	return run(args, &stdio{Writer: s.Out, in: bufio.NewReader(in), ctx: ctx})
}

// stdio is the out of one invocation: commands write to it, and the few
// that need input or cancellation get them from it (inputOf, contextOf).
// Carrying them here keeps every command's signature (args, out) and
// leaves no state shared between invocations.
type stdio struct {
	io.Writer
	in  *bufio.Reader // one reader, so successive lines are not lost
	ctx context.Context
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

func run(args []string, out io.Writer) error {
	if len(args) == 0 {
		fmt.Fprint(out, usage)
		return nil
	}
	switch args[0] {
	case "version":
		fmt.Fprintln(out, Version)
		return nil
	case "keygen":
		return keygen(args[1:], out)
	case "crypto":
		return cryptoCmd(out)
	case "series":
		return seriesCmd(args[1:], out)
	case "demo":
		fs := flag.NewFlagSet("demo", flag.ContinueOnError)
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
	case "payment":
		return paymentCmd(args[1:], out)
	case "peers":
		return peersCmd(args[1:], out)
	case "peer":
		return peerCmd(args[1:], out)
	case "tracking":
		return trackingCmd(args[1:], out)
	case "chases":
		return chasesCmd(args[1:], out)
	case "holdings":
		return holdingsCmd(args[1:], out)
	case "policy":
		return policyCmd(args[1:], out)
	case "wallet":
		return walletCmd(args[1:], out)
	case "sim":
		return simCmd(args[1:], out)
	case "help", "-h", "--help":
		return helpCmd(args[1:], out)
	default:
		return unknownCommand(args[0])
	}
}

func keygen(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("keygen", flag.ContinueOnError)
	seedHex := fs.String("seed", "", "32-byte seed in hex (random if empty)")
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
