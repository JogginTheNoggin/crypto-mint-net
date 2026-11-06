package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/bounty"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/wallet"
)

// BLUE-040 (RED-120, RED-121).

// sentWallet is a wallet holding one payment with these leg states.
func sentWallet(t *testing.T, to hash.Hash, series hash.Hash, amount uint64, memo string, states ...string) *wallet.Wallet {
	t.Helper()
	var legs []any
	for i, st := range states {
		legs = append(legs, map[string]any{"serial": i + 1, "tid": hash.Hash{byte(7 + i)}, "tx": "", "amount": amount / uint64(len(states)), "state": st})
	}
	f := map[string]any{"format": 1, "sent": []any{map[string]any{
		"series": series, "to": to, "amount": amount, "memo": memo, "legs": legs,
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

// RED-121: the memo names the offer by 16 bytes of its id, so an offer
// whose id shares the first 4 bytes with a paid one does not take its
// payment.
func TestBlue040MemoID(t *testing.T) {
	o := bounty.Offer{Kind: bounty.KindLayer, Series: hash.Hash{1}, Entry: hash.Hash{2}, Strength: 20, Amount: 10, Count: 1, Salt: 3}
	a := &bounty.Mine{ID: o.ID(), Offer: o}
	b := &bounty.Mine{ID: a.ID, Offer: o}
	b.ID[31] ^= 0xff
	b.ID[8] ^= 0xff
	c := &bounty.ClaimRecord{Nonce: 1, PayTo: hash.Hash{9}}
	if a.ID.Short() != b.ID.Short() {
		t.Fatal("the test needs ids with the same short form")
	}
	if got := BountyMemoID(a.ID); len(got) != 32 || !strings.HasPrefix(a.ID.String(), got) {
		t.Fatalf("memo id %q", got)
	}
	w := sentWallet(t, c.PayTo, o.Series, o.Amount, BountyMemo+BountyMemoID(a.ID), wallet.StateSelected)
	if p, _, err := sentFor(w, b, c); p != nil || err != nil {
		t.Fatalf("offer B took offer A's payment: %v %v", p != nil, err)
	}
	if p, _, err := sentFor(w, a, c); p == nil || err != nil {
		t.Fatalf("offer A's own payment not found: %v", err)
	}
}

// RED-120: a payment with a dropped leg is found, to be sent again; one
// with a dead leg beside a dropped or sent one is refused, never paid
// anew.
func TestBlue040DroppedIsResent(t *testing.T) {
	o := bounty.Offer{Kind: bounty.KindLayer, Series: hash.Hash{1}, Entry: hash.Hash{2}, Strength: 20, Amount: 10, Count: 1, Salt: 3}
	m := &bounty.Mine{ID: o.ID(), Offer: o}
	c := &bounty.ClaimRecord{Nonce: 1, PayTo: hash.Hash{9}}
	memo := BountyMemo + BountyMemoID(m.ID)
	for _, tc := range []struct {
		states        []string
		found, resend bool
		err           bool
	}{
		{[]string{wallet.StateDropped, wallet.StatePending}, true, true, false},
		{[]string{wallet.StateDropped, wallet.StateDropped}, true, true, false},
		{[]string{wallet.StateVoid, wallet.StateVoid}, false, false, false},
		{[]string{wallet.StateVoid, wallet.StateDropped}, false, false, true},
		{[]string{wallet.StateVoid, wallet.StatePending}, false, false, true},
	} {
		p, resend, err := sentFor(sentWallet(t, c.PayTo, o.Series, o.Amount, memo, tc.states...), m, c)
		if (p != nil) != tc.found || resend != tc.resend || (err != nil) != tc.err {
			t.Errorf("%v: found %v resend %v err %v", tc.states, p != nil, resend, err)
		}
	}
}
