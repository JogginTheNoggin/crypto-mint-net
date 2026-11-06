package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/bounty"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/wallet"
)

// A payment of a claim whose leg is in a state a later release added, or
// whose memo a later release extended, is found as sent, so bounty pay
// records it instead of paying again (W-34, D202). A dropped one is found
// to be sent again, not replaced (RED-120, D203).
func TestW34UnknownLegStateIsSent(t *testing.T) {
	o := bounty.Offer{Kind: bounty.KindLayer, Series: hash.Hash{1}, Entry: hash.Hash{2}, Strength: 20, Amount: 10, Count: 1, Salt: 3}
	m := &bounty.Mine{ID: o.ID(), Offer: o}
	c := &bounty.ClaimRecord{Nonce: 1, PayTo: hash.Hash{9}}
	open := func(state, memo string) *wallet.Wallet {
		t.Helper()
		f := map[string]any{"format": 1, "sent": []any{map[string]any{
			"series": o.Series, "to": c.PayTo, "amount": o.Amount, "memo": memo,
			"legs": []any{map[string]any{"serial": 1, "tid": hash.Hash{7}, "tx": "", "amount": 10, "state": state}},
		}}}
		raw, _ := json.Marshal(f)
		path := filepath.Join(t.TempDir(), "w.json")
		if err := os.WriteFile(path, raw, 0o600); err != nil {
			t.Fatal(err)
		}
		w, err := wallet.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		return w
	}
	memo := BountyMemo + BountyMemoID(m.ID)
	for _, tc := range []struct {
		state, memo  string
		sent, resend bool
	}{
		{wallet.StatePending, memo, true, false},
		{wallet.StateSelected, memo, true, false},
		{"settled-later", memo, true, false},
		{wallet.StatePending, memo + " (a later release's note)", true, false},
		{wallet.StateVoid, memo, false, false},
		{wallet.StateDropped, memo, true, true},
		{wallet.StateUnsigned, memo, false, false},
		{wallet.StatePending, "other", false, false},
		// Four bytes of the id name no offer (RED-121).
		{wallet.StatePending, BountyMemo + m.ID.Short(), false, false},
	} {
		p, resend, err := sentFor(open(tc.state, tc.memo), m, c)
		if err != nil {
			t.Fatalf("%+v: %v", tc, err)
		}
		if (p != nil) != tc.sent || resend != tc.resend {
			t.Errorf("state %q memo %q: found %v resend %v, want %v %v", tc.state, tc.memo, p != nil, resend, tc.sent, tc.resend)
		}
	}
}
