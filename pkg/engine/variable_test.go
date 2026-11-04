package engine_test

import (
	"errors"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/engine"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/ledger"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/testutil"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tx"
)

// A node follows a variable series' released parts (D166). A claim on a
// part it has not seen released waits: it is refused with tx.ErrWaiting,
// kept nowhere, and taken once the release arrives. An entry of a pool of
// that part is not a pool the node has yet, never an invalid entry, and is
// taken after the release. A wrong part is invalid either way.
func TestVariableSeriesWaitsForRelease(t *testing.T) {
	// Two nodes with their own copies of the series, so each knows its
	// own releases.
	a := testutil.MustSeries(testutil.VariableParams(8, 4, 4))
	b := testutil.MustSeries(testutil.VariableParams(8, 4, 4))
	na, nb := engine.New(engine.DefaultConfig()), engine.New(engine.DefaultConfig())
	na.AddSeries(a)
	nb.AddSeries(b)
	for _, e := range []*engine.Engine{na, nb} {
		if err := e.FollowSeries(a.ID()); err != nil {
			t.Fatal(err)
		}
	}
	if n := len(na.FollowedPools()); n != 2 {
		t.Fatalf("part 1 only: %d pools followed", n)
	}
	k := testutil.NewKeys(a)

	early := k.Claim(1, P("alice", 100))
	if _, err := na.HandleTx(early, engine.Pushed); err != nil {
		t.Fatalf("a part 1 claim: %v", err)
	}
	late := k.Claim(9, P("bob", 100))
	for i := 0; i < 2; i++ {
		if _, err := na.HandleTx(late, engine.Pushed); !errors.Is(err, tx.ErrWaiting) {
			t.Fatalf("try %d before the release: %v", i, err)
		}
	}
	wrong := k.Claim(9, P("bob", 100))
	wrong.Mint.Part = 3
	if _, err := na.HandleTx(wrong, engine.Pushed); !errors.Is(err, tx.ErrWrongPart) {
		t.Fatalf("a wrong part: %v", err)
	}

	if added, err := a.AddRelease(testutil.Release(a, 2)); err != nil || !added {
		t.Fatalf("release: %v %v", added, err)
	}
	if n := len(na.FollowedPools()); n != 3 {
		t.Fatalf("after the release: %d pools followed", n)
	}
	res, err := na.HandleTx(late, engine.Pushed)
	if err != nil {
		t.Fatalf("after the release: %v", err)
	}
	en, _, err := na.Build(res.Key, 0)
	if err != nil {
		t.Fatal(err)
	}
	// Node b has not seen the release: the entry's pool is not one it
	// has, which is not an invalid entry, and nothing about it is kept.
	if _, err := nb.HandleEntry(res.Key, en, engine.Pushed); err == nil || errors.Is(err, ledger.ErrInvalidEntry) {
		t.Fatalf("b before the release: %v", err)
	}
	if _, err := b.AddRelease(testutil.Release(b, 2)); err != nil {
		t.Fatal(err)
	}
	if _, err := nb.HandleEntry(res.Key, en, engine.Pushed); err != nil {
		t.Fatalf("b after the release: %v", err)
	}
	// An older release adds nothing and costs no part-id work.
	if added, err := b.AddRelease(testutil.Release(b, 2)); err != nil || added {
		t.Fatalf("the same release again: %v %v", added, err)
	}
}
