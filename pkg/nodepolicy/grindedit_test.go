package nodepolicy_test

import (
	"errors"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/nodepolicy"
)

// GUI-014 (D125): edits apply to the section where it is kept, and a
// numbered removal names the whole job.
func TestGrindEdit(t *testing.T) {
	tip := nodepolicy.GrindJob{Target: "tip"}
	night := nodepolicy.GrindJob{Target: "tip", Hours: "22-06"}
	pend := nodepolicy.GrindJob{Target: "pending"}
	g := nodepolicy.Grind{Threads: 2, Jobs: []nodepolicy.GrindJob{tip, night}}
	v := g.Version()

	// Two edits made against the same reading both apply in turn.
	g1, err := nodepolicy.GrindEdit{Add: []nodepolicy.GrindJob{pend}}.Apply(g)
	if err != nil || len(g1.Jobs) != 3 || len(g.Jobs) != 2 {
		t.Fatalf("add %+v %v", g1, err)
	}
	g2, err := nodepolicy.GrindEdit{Remove: []nodepolicy.GrindRemove{{N: 1, Job: tip}}}.Apply(g1)
	if err != nil || len(g2.Jobs) != 2 || !nodepolicy.SameJob(g2.Jobs[0], night) || !nodepolicy.SameJob(g2.Jobs[1], pend) {
		t.Fatalf("remove %+v %v", g2, err)
	}
	// A stale number does not remove another job, even one with the same
	// target.
	if _, err := (nodepolicy.GrindEdit{Remove: []nodepolicy.GrindRemove{{N: 1, Job: tip}}}).Apply(g2); !errors.Is(err, nodepolicy.ErrGrindChanged) {
		t.Fatalf("stale remove: %v", err)
	}
	if _, err := (nodepolicy.GrindEdit{Remove: []nodepolicy.GrindRemove{{N: 9, Job: tip}}}).Apply(g2); !errors.Is(err, nodepolicy.ErrGrindChanged) {
		t.Fatalf("remove past the end: %v", err)
	}
	// Version guards an edit made against an older reading.
	if g2.Version() == v {
		t.Fatal("version did not change")
	}
	if _, err := (nodepolicy.GrindEdit{RemoveAll: true, Version: v}).Apply(g2); !errors.Is(err, nodepolicy.ErrGrindChanged) {
		t.Fatalf("stale version: %v", err)
	}
	// Set changes only the fields named; auto clears the device.
	dev, floor := "auto", 12
	if _, err := (nodepolicy.GrindEdit{Set: &nodepolicy.GrindSet{Device: &dev}, Version: g2.Version()}).Apply(nodepolicy.Grind{Device: "cpu", Threads: 2}); err == nil {
		t.Fatal("version of another section accepted")
	}
	g3, err := nodepolicy.GrindEdit{Set: &nodepolicy.GrindSet{Device: &dev, LayerFloor: &floor}}.Apply(nodepolicy.Grind{Device: "cpu", Threads: 2})
	if err != nil || g3.Device != "" || g3.Threads != 2 || g3.LayerFloor != 12 {
		t.Fatalf("set %+v %v", g3, err)
	}
	if (nodepolicy.GrindEdit{}).IsZero() != true || (nodepolicy.GrindEdit{RemoveAll: true}).IsZero() {
		t.Fatal("IsZero")
	}
}
