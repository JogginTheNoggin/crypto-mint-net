package ledger_test

import (
	"strconv"
	"testing"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/testutil"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tx"
)

// RED-068, D162: a build on a chosen parent over 20,000 pending
// transactions checks them in well under a second: the conflicts are set
// lookups (ORANGE-033) and the signatures the pending pool checked are not
// checked again (CheckPendingOn).
func TestCheckPendingOnIsLinear(t *testing.T) {
	if testing.Short() {
		t.Skip("signs 20,000 transactions")
	}
	const n = 20000
	s, k, p, claim := spread(t, n)
	ctid := claim.TID(s.Hash())
	txs := make([]*tx.Tx, n)
	for i := range txs {
		txs[i] = k.Spend(4, []testutil.In{{Input: tx.Input{TID: ctid, Index: uint32(i)}, Owner: "a" + strconv.Itoa(i)}}, P("bob", 1))
	}
	tip, _, _ := p.Tip()
	start := time.Now()
	ok, refused, err := p.CheckPendingOn(tip, txs)
	d := time.Since(start)
	if err != nil || len(ok) != n || len(refused) != 0 {
		t.Fatalf("kept %d, refused %d: %v", len(ok), len(refused), err)
	}
	if d > 500*time.Millisecond {
		t.Fatalf("CheckPendingOn of %d took %v", n, d)
	}
	t.Logf("CheckPendingOn of %d: %v", n, d)
	// A claim of another pool's mint is still left out.
	_, refused, _ = p.CheckPendingOn(tip, []*tx.Tx{k.Claim(0, P("bob", 1))})
	if len(refused) != 1 {
		t.Fatal("a claim of another pool's mint was kept")
	}
}

// D162: on valid pending transactions CheckPendingOn keeps and refuses
// what CheckOn does: conflicts, a spent or missing input, a mint claimed.
func TestCheckPendingOnMatchesCheckOn(t *testing.T) {
	s, k, p, claim := spread(t, 3)
	in := func(i int) testutil.In { return k.Out(claim, uint32(i), "a"+strconv.Itoa(i)) }
	ghost := testutil.In{Input: tx.Input{TID: claim.TID(s.Hash()), Index: 9}, Owner: "a0"}
	txs := []*tx.Tx{
		k.Spend(4, []testutil.In{in(0)}, P("bob", 1)),
		k.Spend(4, []testutil.In{in(0)}, P("carol", 1)),
		k.Spend(4, []testutil.In{in(1), in(2)}, P("carol", 2)),
		k.Spend(4, []testutil.In{ghost}, P("carol", 1)),
		k.Claim(4, P("bob", 3)), // already claimed
		k.Claim(5, P("bob", 3)),
		k.Claim(0, P("bob", 3)), // another pool
	}
	tip, _, _ := p.Tip()
	ok1, ref1, err1 := p.CheckOn(tip, txs)
	ok2, ref2, err2 := p.CheckPendingOn(tip, txs)
	if err1 != nil || err2 != nil || len(ok1) != len(ok2) || len(ref1) != len(ref2) {
		t.Fatalf("CheckOn kept %d refused %d (%v); CheckPendingOn kept %d refused %d (%v)", len(ok1), len(ref1), err1, len(ok2), len(ref2), err2)
	}
	for i := range ok1 {
		if ok1[i] != ok2[i] {
			t.Fatalf("kept %d differs", i)
		}
	}
	for i := range ref1 {
		if ref1[i].TID != ref2[i].TID {
			t.Fatalf("refused %d differs: %v and %v", i, ref1[i].Err, ref2[i].Err)
		}
	}
	if len(ok1) != 3 || len(ref1) != 4 {
		t.Fatalf("kept %d, refused %d", len(ok1), len(ref1))
	}
}
