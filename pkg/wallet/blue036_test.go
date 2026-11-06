package wallet_test

import (
	"strings"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tx"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/wallet"
)

// RED-105: a payment of a variable series this process never built is
// read with its part when the file says the series is variable, and a
// file that does not say so gets an error naming why; RED-104: a leg of
// another series is refused.
func TestBlue036VariableLegRead(t *testing.T) {
	var id hash.Hash
	id[0], id[31] = 0x36, 0xb1
	tr := &tx.Tx{Inputs: []tx.Input{{Index: 2}}, Outputs: []tx.Output{{Amount: 7}}}
	tr.Mint.Series, tr.Mint.Part, tr.Mint.Serial = id, 3, 9
	leg := wallet.UnsignedLeg{Serial: 9, Tx: tr.Encode()}
	u := wallet.Unsigned{Series: id, Variable: true, Legs: []wallet.UnsignedLeg{leg}}
	got, err := u.DecodeLeg(0)
	if err != nil || got.Mint.Part != 3 || got.Mint.Serial != 9 || got.Outputs[0].Amount != 7 {
		t.Fatalf("variable leg: %+v %v", got, err)
	}
	u.Variable = false
	if _, err := u.DecodeLeg(0); err == nil || !strings.Contains(err.Error(), "variable") {
		t.Fatalf("no variable mark: %v", err)
	}
	u.Variable = true
	u.Series[5] ^= 1
	if _, err := u.DecodeLeg(0); err == nil || !strings.Contains(err.Error(), "but the payment says series") {
		t.Fatalf("another series: %v", err)
	}
}
