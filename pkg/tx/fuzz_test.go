package tx_test

import (
	"bytes"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/testutil"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tx"
)

// FuzzDecode checks that Decode never panics and that whatever it accepts
// re-encodes to the same bytes: the byte form has one encoding per value.
func FuzzDecode(f *testing.F) {
	s := testutil.ExampleSeries()
	k := testutil.NewKeys(s)
	claim := k.Claim(4, P("alice", 100))
	f.Add(claim.Encode())
	f.Add(k.Spend(4, []testutil.In{{Input: tx.Input{TID: claim.TID(s.Hash())}, Owner: "alice"}}, P("bob", 30), P("alice", 70)).Encode())
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, b []byte) {
		got, err := tx.Decode(b)
		if err != nil {
			return
		}
		if !bytes.Equal(got.Encode(), b) {
			t.Fatal("re-encoding differs")
		}
		got.TID(s.Hash())
	})
}
