// Package testutil provides deterministic keys, a standard example series,
// and helpers to build signed transactions. It is used by tests across the
// codebase and by the demo command. Keys are derived from labels and must
// never be used for real funds.
package testutil

import (
	"crypto/sha256"
	"fmt"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/policy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/series"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tx"
)

// ExampleParams returns the series of the design's worked examples: 8 mints
// of 100 units, pool size 4, SHA-256, 1-of-1 Ed25519 addresses, minimum layer
// strength 0, no minimum entry strength and no entry hash credit.
func ExampleParams() series.Params {
	return series.Params{
		HashID:      hash.SHA256,
		Mints:       8,
		Supply:      100,
		PoolSize:    4,
		PolicyTypes: []policy.TypeID{policy.TypeThreshold},
		KeyAlgs:     []policy.AlgID{policy.AlgEd25519},
	}
}

// ExampleSeries returns the series built from ExampleParams.
func ExampleSeries() *series.Series {
	return MustSeries(ExampleParams())
}

// MustSeries builds a series or panics.
func MustSeries(p series.Params) *series.Series {
	s, err := series.New(p)
	if err != nil {
		panic(err)
	}
	return s
}

// Key is a labelled single-key owner: its signer, policy and address.
type Key struct {
	Label   string
	Signer  *policy.Ed25519Signer
	Policy  policy.Policy
	Address hash.Hash
}

// NewKey derives a key from a label. The same label always gives the same
// key, under any series using the same hash.
func NewKey(s *series.Series, label string) *Key {
	seed := sha256.Sum256([]byte("PoO-test-key:" + label))
	signer, err := policy.NewEd25519Signer(seed[:])
	if err != nil {
		panic(err)
	}
	pol := policy.SingleKey(policy.AlgEd25519, signer.Public())
	return &Key{Label: label, Signer: signer, Policy: pol, Address: s.Address(pol)}
}

// Keys holds labelled keys, creating each on first use.
type Keys struct {
	s *series.Series
	m map[string]*Key
}

// NewKeys returns an empty key set for a series.
func NewKeys(s *series.Series) *Keys { return &Keys{s: s, m: map[string]*Key{}} }

// Get returns the key for a label.
func (k *Keys) Get(label string) *Key {
	if key, ok := k.m[label]; ok {
		return key
	}
	key := NewKey(k.s, label)
	k.m[label] = key
	return key
}

// Addr returns the address for a label.
func (k *Keys) Addr(label string) hash.Hash { return k.Get(label).Address }

// Pay is one output written with a key label.
type Pay struct {
	To     string
	Amount uint64
}

// Claim builds and signs a claim of mint serial paying the given outputs.
// Each output's owner signs for its own output.
func (k *Keys) Claim(serial uint64, outs ...Pay) *tx.Tx {
	t := &tx.Tx{Mint: k.s.Mint(serial)}
	for _, o := range outs {
		t.Outputs = append(t.Outputs, tx.Output{Address: k.Addr(o.To), Amount: o.Amount})
	}
	tid := t.TID(k.s.Hash())
	for _, o := range outs {
		t.Witnesses = append(t.Witnesses, policy.SingleKeyWitness(k.Get(o.To).Signer, tid))
	}
	return t
}

// In names an input and the label of the key that owns it.
type In struct {
	Input tx.Input
	Owner string
}

// Spend builds and signs a spend of mint serial.
func (k *Keys) Spend(serial uint64, ins []In, outs ...Pay) *tx.Tx {
	t := &tx.Tx{Mint: k.s.Mint(serial)}
	for _, in := range ins {
		t.Inputs = append(t.Inputs, in.Input)
	}
	for _, o := range outs {
		t.Outputs = append(t.Outputs, tx.Output{Address: k.Addr(o.To), Amount: o.Amount})
	}
	tid := t.TID(k.s.Hash())
	for _, in := range ins {
		t.Witnesses = append(t.Witnesses, policy.SingleKeyWitness(k.Get(in.Owner).Signer, tid))
	}
	return t
}

// Out returns the input naming output index of t, owned by owner.
func (k *Keys) Out(t *tx.Tx, index uint32, owner string) In {
	return In{Input: tx.Input{TID: t.TID(k.s.Hash()), Index: index}, Owner: owner}
}

// Must panics if err is not nil.
func Must(err error) {
	if err != nil {
		panic(fmt.Sprintf("testutil: %v", err))
	}
}

// P is shorthand for Pay{To: to, Amount: amount}.
func P(to string, amount uint64) Pay { return Pay{To: to, Amount: amount} }
