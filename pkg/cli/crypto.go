package cli

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/policy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/series"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/testutil"
)

// cryptoCmd lists the hashes, key algorithms and policy types this build
// knows, with the numbers a series names them by.
func cryptoCmd(out io.Writer) error {
	type named struct {
		ID   uint32 `json:"id"`
		Name string `json:"name"`
	}
	type alg struct {
		named
		PublicKey int `json:"public_key_bytes"`
		Signature int `json:"signature_bytes"`
	}
	var v struct {
		Hashes []named `json:"hashes"`
		Algs   []alg   `json:"key_algs"`
		Types  []named `json:"policy_types"`
	}
	for _, id := range hash.IDs() {
		v.Hashes = append(v.Hashes, named{uint32(id), id.Name()})
	}
	for _, a := range policy.Default().Algs() {
		v.Algs = append(v.Algs, alg{named{uint32(a.ID()), a.Name()}, a.PublicKeySize(), a.SignatureSize()})
	}
	for _, t := range policy.Default().Types() {
		v.Types = append(v.Types, named{uint32(t.ID()), t.Name()})
	}
	return writeJSON(out, v)
}

// number reads a decimal number, or looks a name up.
func number(kind, v string, byName func(string) (uint32, bool)) (uint32, error) {
	if n, err := strconv.ParseUint(v, 10, 32); err == nil {
		return uint32(n), nil
	}
	if n, ok := byName(v); ok {
		return n, nil
	}
	return 0, fmt.Errorf("unknown %s %q (poo crypto lists them)", kind, v)
}

func parseHash(v string) (hash.ID, error) {
	n, err := number("hash", v, func(s string) (uint32, bool) { id, ok := hash.ByName(s); return uint32(id), ok })
	if err == nil {
		if _, lerr := hash.Lookup(hash.ID(n)); lerr != nil {
			return 0, fmt.Errorf("unknown hash %q (poo crypto lists them)", v)
		}
	}
	return hash.ID(n), err
}

func parseAlg(v string) (policy.AlgID, error) {
	n, err := number("key algorithm", v, func(s string) (uint32, bool) { id, ok := policy.Default().AlgByName(s); return uint32(id), ok })
	if err == nil && !policy.Default().HasAlg(policy.AlgID(n)) {
		return 0, fmt.Errorf("unknown key algorithm %q (poo crypto lists them)", v)
	}
	return policy.AlgID(n), err
}

func parseType(v string) (policy.TypeID, error) {
	n, err := number("policy type", v, func(s string) (uint32, bool) { id, ok := policy.Default().TypeByName(s); return uint32(id), ok })
	if err == nil && !policy.Default().HasType(policy.TypeID(n)) {
		return 0, fmt.Errorf("unknown policy type %q (poo crypto lists them)", v)
	}
	return policy.TypeID(n), err
}

// parseList reads a comma-separated list, sorted with repeats dropped, as
// a series wants it.
func parseList[T ~uint32](v string, parse func(string) (T, error)) ([]T, error) {
	seen := map[T]bool{}
	var out []T
	for _, f := range strings.Split(v, ",") {
		if f = strings.TrimSpace(f); f == "" {
			continue
		}
		n, err := parse(f)
		if err != nil {
			return nil, err
		}
		if !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out, nil
}

// seriesNew prints the params of a new series, checked, starting from the
// worked example's and changed by the flags.
func seriesNew(args []string, out io.Writer) error {
	ex := testutil.ExampleParams()
	fs := newFlagSet("series new", out)
	h := fs.String("hash", "sha256", "the series' hash, by name or number")
	algs := fs.String("algs", "ed25519", "allowed key algorithms, comma-separated")
	types := fs.String("types", "threshold", "allowed policy types, comma-separated")
	mints := fs.Uint64("mints", ex.Mints, "number of mints")
	supply := fs.Uint64("supply", ex.Supply, "units in every mint")
	poolSize := fs.Uint64("pool-size", ex.PoolSize, "mints per pool")
	minLayer := fs.Uint("min-layer-strength", 0, "layers below this strength are ignored")
	minEntry := fs.Uint("min-entry-strength", 0, "entries below this strength are invalid (0: none)")
	credit := fs.Bool("entry-hash-credit", false, "an entry's own hash adds its credit")
	maxWitness := fs.Uint("max-witness", 0, "largest witness in bytes (0: not set)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *minLayer > 256 || *minEntry > 256 || *maxWitness > 1<<32-1 {
		return fmt.Errorf("a flag is out of range")
	}
	p := series.Params{
		Mints: *mints, Supply: *supply, PoolSize: *poolSize,
		MinLayerStrength: uint16(*minLayer), MinEntryStrength: uint16(*minEntry),
		EntryHashCredit: *credit, MaxWitness: uint32(*maxWitness),
	}
	var err error
	if p.HashID, err = parseHash(*h); err != nil {
		return err
	}
	if p.KeyAlgs, err = parseList(*algs, parseAlg); err != nil {
		return err
	}
	if p.PolicyTypes, err = parseList(*types, parseType); err != nil {
		return err
	}
	if _, err := series.New(p); err != nil {
		return err
	}
	return writeJSON(out, p)
}
