package tx_test

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"math/rand/v2"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/encoding"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/policy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/series"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/testutil"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tx"
)

// bodyV1 is a transaction's body exactly as byte form v1 writes it, frozen
// here: series ∥ serial ∥ inputs ∥ outputs.
func bodyV1(t *tx.Tx) []byte {
	w := encoding.NewWriter(64)
	w.Raw(t.Mint.Series[:]).U64(t.Mint.Serial)
	w.Count(len(t.Inputs))
	for _, in := range t.Inputs {
		w.Raw(in.TID[:]).U32(in.Index)
	}
	w.Count(len(t.Outputs))
	for _, o := range t.Outputs {
		w.Raw(o.Address[:]).U64(o.Amount)
	}
	return w.Bytes()
}

// Every fixed transaction keeps its bytes and its tid, and reads back the
// same, whatever variable series the process knows (D166, Black's phase 2
// point 5).
func TestFixedTxUnchanged(t *testing.T) {
	// A variable series built first, so the reader knows one.
	testutil.MustSeries(testutil.VariableParams(8, 4, 4))
	s := testutil.ExampleSeries()
	r := rand.New(rand.NewPCG(3, 4))
	h := s.Hash()
	for i := 0; i < 2000; i++ {
		ids := []hash.Hash{s.ID(), sha256.Sum256([]byte{byte(i), 1})}
		tr := &tx.Tx{Mint: series.MintID{Series: ids[r.IntN(2)], Serial: r.Uint64()}}
		for range r.IntN(4) {
			tr.Inputs = append(tr.Inputs, tx.Input{TID: sha256.Sum256([]byte{byte(i), 2, byte(len(tr.Inputs))}), Index: r.Uint32()})
		}
		for range 1 + r.IntN(4) {
			tr.Outputs = append(tr.Outputs, tx.Output{Address: sha256.Sum256([]byte{byte(i), 3, byte(len(tr.Outputs))}), Amount: r.Uint64()})
		}
		for range r.IntN(3) {
			tr.Witnesses = append(tr.Witnesses, policy.Witness{Policy: policy.Policy{Type: 1, Params: []byte{byte(i)}}, Data: []byte{1, 2, 3}})
		}
		if !bytes.Equal(tr.Body(), bodyV1(tr)) {
			t.Fatalf("tx %d: body changed", i)
		}
		if tr.TID(h) != h(hash.TagTx, bodyV1(tr)) {
			t.Fatalf("tx %d: tid changed", i)
		}
		back, err := tx.Decode(tr.Encode())
		if err != nil || !bytes.Equal(back.Encode(), tr.Encode()) || back.Mint.Part != 0 {
			t.Fatalf("tx %d read back: %v", i, err)
		}
	}
	// A fixed mint that names a part is refused.
	k := testutil.NewKeys(s)
	c := k.Claim(1, testutil.P("alice", 100))
	c.Mint.Part = 1
	if err := tx.CheckForm(s, c); !errors.Is(err, tx.ErrWrongPart) {
		t.Fatalf("a fixed mint with a part: %v", err)
	}
}

// A variable series' mint is series ∥ part ∥ serial in the body, so the tid
// covers the part; its one name is the part that holds the serial; and a
// mint past the newest known release waits, never invalid (D166).
func TestVariableMints(t *testing.T) {
	s := testutil.MustSeries(testutil.VariableParams(8, 4, 4))
	k := testutil.NewKeys(s)
	c := k.Claim(9, testutil.P("alice", 100))
	if c.Mint.Part != 2 {
		t.Fatalf("serial 9 named part %d", c.Mint.Part)
	}
	w := encoding.NewWriter(64)
	w.Raw(c.Mint.Series[:]).U64(2).U64(9)
	if !bytes.HasPrefix(c.Body(), w.Bytes()) {
		t.Fatalf("body %x", c.Body()[:48])
	}
	back, err := tx.Decode(c.Encode())
	if err != nil || back.Mint != c.Mint || back.TID(s.Hash()) != c.TID(s.Hash()) {
		t.Fatalf("read back %+v: %v", back.Mint, err)
	}
	other := *c
	other.Mint.Part = 3
	if other.TID(s.Hash()) == c.TID(s.Hash()) {
		t.Fatal("the tid does not cover the part")
	}
	if err := tx.CheckForm(s, c); !errors.Is(err, tx.ErrWaiting) || errors.Is(err, tx.ErrSerialRange) {
		t.Fatalf("before the release: %v", err)
	}
	if err := tx.CheckForm(s, &other); !errors.Is(err, tx.ErrWrongPart) {
		t.Fatalf("a wrong part, unreleased: %v", err)
	}
	if _, err := s.AddRelease(testutil.Release(s, 2)); err != nil {
		t.Fatal(err)
	}
	if err := tx.CheckStateless(s, c, c.TID(s.Hash())); err != nil {
		t.Fatalf("after the release: %v", err)
	}
	if err := tx.CheckForm(s, &other); !errors.Is(err, tx.ErrWrongPart) {
		t.Fatalf("a wrong part, released: %v", err)
	}
	zero := *c
	zero.Mint.Part = 0
	if err := tx.CheckForm(s, &zero); !errors.Is(err, tx.ErrWrongPart) {
		t.Fatalf("a variable mint with no part: %v", err)
	}
	p1 := k.Claim(3, testutil.P("bob", 100))
	if p1.Mint.Part != 1 || tx.CheckStateless(s, p1, p1.TID(s.Hash())) != nil {
		t.Fatalf("a part 1 claim: %+v", p1.Mint)
	}
}
