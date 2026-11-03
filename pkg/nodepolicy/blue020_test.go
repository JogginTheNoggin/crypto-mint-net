package nodepolicy_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/nodepolicy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/testutil"
)

// RED-050, D154: taking a pool off a series followed whole puts it on the
// follow's except list, so all but a few pools of a large series stays one
// entry; a pool followed again comes off the list, and a follow list that
// would need more than MaxSections sections is refused when changed.
func TestUnfollowPoolsOfAWholeSeries(t *testing.T) {
	prm := testutil.ExampleParams()
	prm.PoolSize, prm.Mints = 1, 5000
	s := testutil.MustSeries(prm)
	id := s.ID()
	p := nodepolicy.Default()
	p.Series = []nodepolicy.SeriesInfo{{Params: prm}}
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(p.AddFollow(nodepolicy.Follow{Series: &id}))
	must(p.RemoveFollow(nodepolicy.Follow{Series: &id, Pools: []uint64{7}}))
	must(p.RemoveFollow(nodepolicy.Follow{Series: &id, Pools: []uint64{9, 7}}))
	if want := []nodepolicy.Follow{{Series: &id, Except: []uint64{7, 9}}}; !reflect.DeepEqual(p.Follow, want) {
		t.Fatalf("follow %+v", p.Follow)
	}
	must(p.Validate())
	// A mint of pool 9 puts pool 9 back; pool 7 by its index too.
	must(p.AddFollow(nodepolicy.Follow{Series: &id, Mints: []uint64{9}}))
	must(p.AddFollow(nodepolicy.Follow{Series: &id, Pools: []uint64{7}}))
	if want := []nodepolicy.Follow{{Series: &id}}; !reflect.DeepEqual(p.Follow, want) {
		t.Fatalf("followed back: %+v", p.Follow)
	}
	// A mint followed through its pool cannot come off alone.
	if err := p.RemoveFollow(nodepolicy.Follow{Series: &id, Mints: []uint64{3}}); !errors.Is(err, nodepolicy.ErrInvalid) {
		t.Fatalf("mint off a whole series: %v", err)
	}
	// Listing more than MaxSections pools is refused, and nothing changes.
	p.Follow = nil
	var many []uint64
	for k := uint64(0); k <= nodepolicy.MaxSections; k++ {
		many = append(many, k)
	}
	if err := p.AddFollow(nodepolicy.Follow{Series: &id, Pools: many}); !errors.Is(err, nodepolicy.ErrInvalid) || p.Follow != nil {
		t.Fatalf("%d pools listed: %v, %+v", len(many), err, p.Follow)
	}
	// An except list on an issuer or with pools is invalid.
	bad := nodepolicy.Default()
	bad.Series = p.Series
	bad.Follow = []nodepolicy.Follow{{Series: &id, Pools: []uint64{1}, Except: []uint64{2}}}
	if err := bad.Validate(); !errors.Is(err, nodepolicy.ErrInvalid) {
		t.Fatalf("except with pools: %v", err)
	}
}

// When fewer pools are left than are taken off, the pools left are listed.
func TestUnfollowMostPoolsListsTheRest(t *testing.T) {
	prm := testutil.ExampleParams() // two pools
	id := testutil.MustSeries(prm).ID()
	p := nodepolicy.Default()
	p.Series = []nodepolicy.SeriesInfo{{Params: prm}}
	p.Follow = []nodepolicy.Follow{{Series: &id}}
	if err := p.RemoveFollow(nodepolicy.Follow{Series: &id, Pools: []uint64{0}}); err != nil {
		t.Fatal(err)
	}
	if want := []nodepolicy.Follow{{Series: &id, Pools: []uint64{1}}}; !reflect.DeepEqual(p.Follow, want) {
		t.Fatalf("follow %+v", p.Follow)
	}
}
