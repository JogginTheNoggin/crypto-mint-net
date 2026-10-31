package tx_test

import (
	"errors"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/state"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/testutil"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tx"
)

var P = testutil.P

func setup() (*testutil.Keys, *state.State, *tx.Tx) {
	s := testutil.ExampleSeries()
	k := testutil.NewKeys(s)
	st := state.New()
	claim := k.Claim(4, P("alice", 100))
	st.Apply([]state.Item{{TID: claim.TID(s.Hash()), Tx: claim}})
	return k, st, claim
}

func check(t *testing.T, k *testutil.Keys, v tx.View, x *tx.Tx) error {
	t.Helper()
	s := testutil.ExampleSeries()
	return tx.Check(s, v, x, x.TID(s.Hash()))
}

func TestValidClaimAndSpend(t *testing.T) {
	k, st, claim := setup()
	s := testutil.ExampleSeries()
	if err := check(t, k, state.New(), claim); err != nil {
		t.Fatalf("claim refused: %v", err)
	}
	split := k.Spend(4, []testutil.In{k.Out(claim, 0, "alice")}, P("carol", 30), P("alice2", 70))
	if err := check(t, k, st, split); err != nil {
		t.Fatalf("split refused: %v", err)
	}
	// The tid ignores witnesses.
	stripped := *split
	stripped.Witnesses = nil
	if stripped.TID(s.Hash()) != split.TID(s.Hash()) {
		t.Fatal("witnesses changed the tid")
	}
}

func TestClaimRules(t *testing.T) {
	k, st, _ := setup()
	cases := map[string]struct {
		tx   *tx.Tx
		view tx.View
		want error
	}{
		"short of supply":     {k.Claim(5, P("a", 99)), state.New(), tx.ErrClaimSupply},
		"over supply":         {k.Claim(5, P("a", 60), P("b", 41)), state.New(), tx.ErrClaimSupply},
		"repeated address":    {k.Claim(5, P("a", 50), P("a", 50)), state.New(), tx.ErrRepeatedAddress},
		"zero amount":         {k.Claim(5, P("a", 100), P("b", 0)), state.New(), tx.ErrZeroAmount},
		"already claimed":     {k.Claim(4, P("b", 100)), st, tx.ErrAlreadyClaimed},
		"serial out of range": {k.Claim(8, P("a", 100)), state.New(), tx.ErrSerialRange},
	}
	for name, c := range cases {
		if err := check(t, k, c.view, c.tx); !errors.Is(err, c.want) {
			t.Errorf("%s: want %v, got %v", name, c.want, err)
		}
	}
	// Several addresses are fine when they add up.
	if err := check(t, k, state.New(), k.Claim(7, P("hank", 60), P("gina", 40))); err != nil {
		t.Errorf("two-output claim refused: %v", err)
	}
	// A claim needs a witness from each output's owner.
	c := k.Claim(7, P("hank", 60), P("gina", 40))
	c.Witnesses[1] = c.Witnesses[0]
	if err := check(t, k, state.New(), c); !errors.Is(err, tx.ErrWitness) {
		t.Errorf("want ErrWitness, got %v", err)
	}
	c.Witnesses = c.Witnesses[:1]
	if err := check(t, k, state.New(), c); !errors.Is(err, tx.ErrWitnessCount) {
		t.Errorf("want ErrWitnessCount, got %v", err)
	}
}

func TestSpendRules(t *testing.T) {
	k, st, claim := setup()
	in := k.Out(claim, 0, "alice")
	cases := map[string]struct {
		tx   *tx.Tx
		want error
	}{
		"conservation":     {k.Spend(4, []testutil.In{in}, P("bob", 101)), tx.ErrConservation},
		"repeated address": {k.Spend(4, []testutil.In{in}, P("bob", 50), P("bob", 50)), tx.ErrRepeatedAddress},
		"repeated input":   {k.Spend(4, []testutil.In{in, in}, P("bob", 200)), tx.ErrRepeatedInput},
		"missing input":    {k.Spend(4, []testutil.In{{Input: tx.Input{TID: hash.Zero}, Owner: "alice"}}, P("bob", 100)), tx.ErrMissingInput},
		"unclaimed mint":   {k.Spend(5, []testutil.In{in}, P("bob", 100)), tx.ErrNotClaimed},
		"wrong signer":     {k.Spend(4, []testutil.In{{Input: in.Input, Owner: "mallory"}}, P("bob", 100)), tx.ErrWitness},
		"no outputs":       {k.Spend(4, []testutil.In{in}), tx.ErrNoOutputs},
	}
	for name, c := range cases {
		if err := check(t, k, st, c.tx); !errors.Is(err, c.want) {
			t.Errorf("%s: want %v, got %v", name, c.want, err)
		}
	}
	// An input of another mint is refused even if that mint is claimed.
	k2 := k.Claim(5, P("bob", 100))
	s := testutil.ExampleSeries()
	st.Apply([]state.Item{{TID: k2.TID(s.Hash()), Tx: k2}})
	cross := k.Spend(5, []testutil.In{in}, P("bob2", 100))
	if err := check(t, k, st, cross); !errors.Is(err, tx.ErrWrongMint) {
		t.Errorf("want ErrWrongMint, got %v", err)
	}
	// Wrong series.
	other := *claim
	other.Mint.Series = hash.Zero
	if err := check(t, k, state.New(), &other); !errors.Is(err, tx.ErrWrongSeries) {
		t.Errorf("want ErrWrongSeries, got %v", err)
	}
}

func TestOverflow(t *testing.T) {
	k, _, _ := setup()
	x := k.Claim(5, P("a", ^uint64(0)), P("b", 2))
	if err := check(t, k, state.New(), x); !errors.Is(err, tx.ErrOverflow) {
		t.Fatalf("want ErrOverflow, got %v", err)
	}
}

func TestEncodeDecode(t *testing.T) {
	k, _, claim := setup()
	s := testutil.ExampleSeries()
	split := k.Spend(4, []testutil.In{k.Out(claim, 0, "alice")}, P("carol", 30), P("alice2", 70))
	for _, x := range []*tx.Tx{claim, split} {
		back, err := tx.Decode(x.Encode())
		if err != nil {
			t.Fatal(err)
		}
		if back.TID(s.Hash()) != x.TID(s.Hash()) || len(back.Witnesses) != len(x.Witnesses) {
			t.Fatal("round trip changed the transaction")
		}
		if string(back.Encode()) != string(x.Encode()) {
			t.Fatal("re-encoding differs")
		}
	}
	if _, err := tx.Decode(append(claim.Encode(), 1)); err == nil {
		t.Fatal("trailing bytes must be refused")
	}
	if _, err := tx.Decode(claim.Encode()[:10]); err == nil {
		t.Fatal("short input must be refused")
	}
}

// No count is capped in decoding (consensus audit C2, D117): a
// transaction of more than 65,536 inputs and outputs round-trips, and a
// count larger than the bytes could hold is still refused.
func TestNoCountCap(t *testing.T) {
	big := &tx.Tx{}
	for i := 0; i < 1<<16+1; i++ {
		big.Inputs = append(big.Inputs, tx.Input{Index: uint32(i)})
		big.Outputs = append(big.Outputs, tx.Output{Amount: 1})
	}
	got, err := tx.Decode(big.Encode())
	if err != nil || len(got.Inputs) != 1<<16+1 || len(got.Outputs) != 1<<16+1 {
		t.Fatalf("decoded %v", err)
	}
	b := big.Encode()
	b = b[:len(b)-100]
	if _, err := tx.Decode(b); err == nil {
		t.Fatal("a cut transaction decoded")
	}
}
