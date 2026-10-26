// Command poo is the command-line tool for the Proof of Ownership full node.
//
// Milestone 1 ships the consensus core without networking, so the tool
// offers offline commands: making keys and addresses, computing series ids
// and pools, and a demo that runs several in-process full nodes through the
// design's worked example. See docs/roadmap.md for what comes next.
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
  poo version                 print the version
  poo keygen [-seed HEX]      make an Ed25519 key and its 1-of-1 address
  poo series example          print the worked example's series as JSON
  poo series info FILE        print the id and pools of a series JSON file
  poo demo [-quiet]           run in-process full nodes through a payment,
                              a double spend and its resolution
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
