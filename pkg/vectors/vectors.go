// Package vectors computes the published test vectors: the exact hashes of
// a fixed series, pool, address, claim, spend, entry and layer. Independent
// implementations can check their byte forms against them, and the golden
// test in this package catches any change to an encoding.
//
// Regenerate testdata/vectors.json with:
//
//	go test ./pkg/vectors -update
package vectors

import (
	"encoding/hex"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/entry"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/layer"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/testutil"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tx"
)

// Vectors are the values every implementation must reproduce.
type Vectors struct {
	SeriesEncoding string `json:"series_encoding"`
	SeriesID       string `json:"series_id"`
	Pool1ID        string `json:"pool_1_id"`
	AlicePublicKey string `json:"alice_public_key"`
	AlicePolicy    string `json:"alice_policy"`
	AliceAddress   string `json:"alice_address"`
	ClaimBody      string `json:"claim_body"`
	ClaimTID       string `json:"claim_tid"`
	ClaimEncoding  string `json:"claim_encoding"`
	SpendTID       string `json:"spend_tid"`
	Entry1Hash     string `json:"entry_1_hash"`
	Entry2Hash     string `json:"entry_2_hash"`
	LayerNonce     uint64 `json:"layer_nonce"`
	LayerHash      string `json:"layer_hash"`
	LayerStrength  int    `json:"layer_strength"`
}

// Compute builds the vectors from the example series. Keys come from
// testutil.NewKey labels, whose seeds are SHA-256("PoON-test-key:" + label).
func Compute() Vectors {
	s := testutil.ExampleSeries()
	k := testutil.NewKeys(s)
	alice := k.Get("alice")
	claim := k.Claim(4, testutil.P("alice", 100))
	spend := k.Spend(4, []testutil.In{k.Out(claim, 0, "alice")}, testutil.P("bob", 30), testutil.P("alice2", 70))
	e1, h1, err := entry.Build(s, s.PoolID(1), []*tx.Tx{claim}, 0)
	if err != nil {
		panic(err)
	}
	_, h2, err := entry.Build(s, h1, []*tx.Tx{spend}, 0)
	if err != nil {
		panic(err)
	}
	_ = e1
	l, strength := layer.Grind(s.Hash(), h1, 0, 4000)
	return Vectors{
		SeriesEncoding: hex.EncodeToString(s.Params().Encode()),
		SeriesID:       s.ID().String(),
		Pool1ID:        s.PoolID(1).String(),
		AlicePublicKey: hex.EncodeToString(alice.Signer.Public()),
		AlicePolicy:    hex.EncodeToString(alice.Policy.Encode()),
		AliceAddress:   alice.Address.String(),
		ClaimBody:      hex.EncodeToString(claim.Body()),
		ClaimTID:       claim.TID(s.Hash()).String(),
		ClaimEncoding:  hex.EncodeToString(claim.Encode()),
		SpendTID:       spend.TID(s.Hash()).String(),
		Entry1Hash:     h1.String(),
		Entry2Hash:     h2.String(),
		LayerNonce:     l.Nonce,
		LayerHash:      l.Hash(s.Hash()).String(),
		LayerStrength:  strength,
	}
}
