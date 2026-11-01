package series_test

import (
	"errors"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/policy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/testutil"
)

// ORANGE-021: WarmWitness remembers only a witness that passes for its
// tid, and CheckWitness still checks the address it must satisfy.
func TestWarmWitness(t *testing.T) {
	s := testutil.ExampleSeries()
	k := testutil.NewKeys(s)
	c := k.Claim(4, testutil.P("alice", 100))
	tid := c.TID(s.Hash())
	w := c.Witnesses[0]
	if s.WarmWitness(w, hash.Hash{1}) {
		t.Fatal("a witness for another tid passed")
	}
	if n := s.Remembered(); n != 0 {
		t.Fatalf("%d remembered after a failure", n)
	}
	if !s.WarmWitness(w, tid) || s.Remembered() != 1 {
		t.Fatalf("a valid witness: remembered %d", s.Remembered())
	}
	if err := s.CheckWitness(w, hash.Hash{2}, tid); !errors.Is(err, policy.ErrAddressMismatch) {
		t.Fatalf("wrong address after warming: %v", err)
	}
	if err := s.CheckWitness(w, c.Outputs[0].Address, tid); err != nil {
		t.Fatalf("right address after warming: %v", err)
	}
}
