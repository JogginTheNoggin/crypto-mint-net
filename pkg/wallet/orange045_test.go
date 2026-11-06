package wallet_test

import (
	"crypto/rand"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/policy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/series"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tx"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/wallet"
)

type countingSigner struct {
	policy.Signer
	n int
}

func (c *countingSigner) Sign(msg []byte) []byte { c.n++; return c.Signer.Sign(msg) }

// ORANGE-045: a key that owns several inputs of one transaction signs its
// message once, and every input's witness still satisfies its policy.
func TestSignWithOncePerLeg(t *testing.T) {
	h, _ := hash.Lookup(hash.SHA512_256)
	reg := policy.Default()
	for _, alg := range []policy.AlgID{policy.AlgEd25519, policy.AlgMLDSA65} {
		seed := make([]byte, policy.SeedSize)
		rand.Read(seed)
		s, err := policy.NewSigner(alg, seed)
		if err != nil {
			t.Fatal(err)
		}
		other, _ := policy.NewSigner(alg, make([]byte, policy.SeedSize))
		mine := policy.SingleKey(alg, s.Public())
		// A 2-of-2 shared with another key, to show the index is kept.
		shared := policy.ThresholdParams{K: 2, Keys: []policy.Key{{Alg: alg, Public: other.Public()}, {Alg: alg, Public: s.Public()}}}.Policy()
		var sid hash.Hash
		rand.Read(sid[:])
		u := wallet.Unsigned{Series: sid}
		const legs, inputs = 3, 4
		for l := 0; l < legs; l++ {
			tt := &tx.Tx{Mint: series.MintID{Series: sid, Serial: uint64(l + 1)}, Outputs: []tx.Output{{Amount: 1}}}
			leg := wallet.UnsignedLeg{Serial: uint64(l + 1)}
			for i := 0; i < inputs; i++ {
				var tid hash.Hash
				rand.Read(tid[:])
				p := mine
				if i == inputs-1 {
					p = shared
				}
				tt.Inputs = append(tt.Inputs, tx.Input{TID: tid})
				tt.Witnesses = append(tt.Witnesses, policy.Witness{Policy: p})
				leg.Inputs = append(leg.Inputs, wallet.PartialWitness{Policy: p.Encode()})
			}
			leg.Tx = tt.Encode()
			u.Legs = append(u.Legs, leg)
		}
		cs := &countingSigner{Signer: s}
		added, err := wallet.SignWith(&u, h, alg, cs)
		if err != nil {
			t.Fatal(err)
		}
		if added != legs*inputs || cs.n != legs {
			t.Fatalf("%d: added %d signatures with %d signings, want %d with %d", alg, added, cs.n, legs*inputs, legs)
		}
		// The other key of the 2-of-2 signs too, then every witness is checked.
		if _, err := wallet.SignWith(&u, h, alg, other); err != nil {
			t.Fatal(err)
		}
		for _, l := range u.Legs {
			ft, err := l.Finish()
			if err != nil {
				t.Fatal(err)
			}
			msg := policy.Message(ft.TID(h))
			for i, w := range ft.Witnesses {
				typ, _ := reg.Type(w.Policy.Type)
				if err := typ.Satisfy(reg, w.Policy.Params, w.Data, msg); err != nil {
					t.Fatalf("%d: input %d: %v", alg, i, err)
				}
			}
		}
	}
}
