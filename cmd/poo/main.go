// Command poo is the command-line tool for the Proof of Ownership full node.
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
package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

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
  poo keygen [-seed HEX]      make an Ed25519 key and its 1-of-1 address
  poo series example          print the worked example's series as JSON
  poo series info FILE        print the id and pools of a series JSON file
  poo demo [-quiet]           run in-process full nodes through a payment,
                              a double spend and its resolution
  poo sim load|partition|attack|sizing [flags]
                              run the deterministic network simulator
                              (poo sim help)

Node commands pick a data directory with -dir, or with -root (default
$POO_HOME or ~/.poo) and -policy (default: the current node policy).
Client commands take -series ID when the node follows more than one series.
`

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "poo:", err)
		os.Exit(1)
	}
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
		fmt.Fprint(out, usage)
		return nil
	default:
		return fmt.Errorf("unknown command %q\n\n%s", args[0], usage)
	}
}

func keygen(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("keygen", flag.ContinueOnError)
	seedHex := fs.String("seed", "", "32-byte seed in hex (random if empty)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	seed := make([]byte, 32)
	if *seedHex != "" {
		b, err := hex.DecodeString(*seedHex)
		if err != nil || len(b) != 32 {
			return fmt.Errorf("seed must be 64 hex digits")
		}
		seed = b
	} else if _, err := rand.Read(seed); err != nil {
		return err
	}
	signer, err := policy.NewEd25519Signer(seed)
	if err != nil {
		return err
	}
	pol := policy.SingleKey(policy.AlgEd25519, signer.Public())
	// The address depends only on the series' hash; every series using
	// SHA-256 gives the same address for this policy.
	s := testutil.ExampleSeries()
	return writeJSON(out, map[string]string{
		"seed":           hex.EncodeToString(seed),
		"public_key":     hex.EncodeToString(signer.Public()),
		"policy":         hex.EncodeToString(pol.Encode()),
		"address_sha256": s.Address(pol).String(),
	})
}

func seriesCmd(args []string, out io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("series needs a subcommand: example or info")
	}
	switch args[0] {
	case "example":
		return writeJSON(out, testutil.ExampleParams())
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
