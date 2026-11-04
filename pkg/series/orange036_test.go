package series_test

import (
	"sync/atomic"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/policy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/series"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/testutil"
)

// countingAlg is Ed25519 that counts its verifies.
type countingAlg struct {
	policy.KeyAlg
	n *atomic.Int32
}

func (a countingAlg) Verify(pub, msg, sig []byte) bool {
	a.n.Add(1)
	return a.KeyAlg.Verify(pub, msg, sig)
}

// countingRegistry is a registry whose Ed25519 counts its verifies.
func countingRegistry(n *atomic.Int32) *policy.Registry {
	r := policy.NewRegistry()
	t, _ := policy.Default().Type(policy.TypeThreshold)
	r.RegisterType(t)
	a, _ := policy.Default().Alg(policy.AlgEd25519)
	r.RegisterAlg(countingAlg{a, n})
	return r
}

// The same release checked again in a process costs no witness verify
// (ORANGE-036): its answer depends only on the series, the registry and
// its own bytes. A release with the same fields and another witness is
// checked in full, so a forged one is still refused (RED-074), and a
// series built with another registry checks for itself.
func TestReleaseCheckedOnce(t *testing.T) {
	var n atomic.Int32
	params := testutil.VariableParams(8, 4, 4)
	s, err := series.NewWithRegistry(params, countingRegistry(&n))
	if err != nil {
		t.Fatal(err)
	}
	r := testutil.Release(s, 2)
	for i := 0; i < 3; i++ {
		if err := s.CheckRelease(r); err != nil {
			t.Fatal(err)
		}
	}
	if n.Load() != 1 {
		t.Fatalf("3 checks of one release: %d verifies, want 1", n.Load())
	}
	// A new series value with the same id and registry finds it too.
	again, _ := series.NewWithRegistry(params, s.Registry())
	if err := again.CheckRelease(r); err != nil || n.Load() != 1 {
		t.Fatalf("rebuilt series: %v, %d verifies", err, n.Load())
	}

	// The issuer's policy with a signature changed in its last byte.
	forged := r
	forged.Witness.Data = append([]byte(nil), r.Witness.Data...)
	forged.Witness.Data[len(forged.Witness.Data)-1] ^= 1
	for i := 0; i < 2; i++ {
		if err := s.CheckRelease(forged); err == nil {
			t.Fatal("a forged release checked")
		}
	}
	if n.Load() != 3 {
		t.Fatalf("a forged release checked twice: %d verifies in all, want 3", n.Load())
	}

	var m atomic.Int32
	other, _ := series.NewWithRegistry(params, countingRegistry(&m))
	if err := other.CheckRelease(r); err != nil || m.Load() != 1 {
		t.Fatalf("another registry: %v, %d verifies, want 1", err, m.Load())
	}

	// The memory holds a few releases per series, the newest last.
	for k := uint64(3); k <= 7; k++ {
		if err := s.CheckRelease(testutil.Release(s, k)); err != nil {
			t.Fatal(err)
		}
	}
	before := n.Load()
	if err := s.CheckRelease(testutil.Release(s, 7)); err != nil || n.Load() != before {
		t.Fatalf("the newest again: %v, %d new verifies", err, n.Load()-before)
	}
	if err := s.CheckRelease(r); err != nil || n.Load() != before+1 {
		t.Fatalf("an old release again: %v, %d new verifies, want 1", err, n.Load()-before)
	}
}
