package nodepolicy

import (
	"errors"
	"reflect"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/testutil"
)

func TestAddAndRemoveFollow(t *testing.T) {
	s := testutil.ExampleSeries() // 8 mints, pools of 4: pools 0 and 1
	id := s.ID()
	p := Default()
	p.Series = []SeriesInfo{{Params: s.Params()}}
	step := func(add bool, f Follow, want []Follow) {
		t.Helper()
		var err error
		if add {
			err = p.AddFollow(f)
		} else {
			err = p.RemoveFollow(f)
		}
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(p.Follow, want) {
			t.Fatalf("follow %+v, want %+v", p.Follow, want)
		}
		if err := p.Validate(); err != nil {
			t.Fatal(err)
		}
	}
	iss := hash.Hash{7}
	step(true, Follow{Series: &id, Mints: []uint64{6}}, []Follow{{Series: &id, Mints: []uint64{6}}})
	step(true, Follow{Series: &id, Pools: []uint64{0}, Mints: []uint64{5, 6}}, []Follow{{Series: &id, Pools: []uint64{0}, Mints: []uint64{5, 6}}})
	step(true, Follow{Issuer: &iss}, []Follow{{Series: &id, Pools: []uint64{0}, Mints: []uint64{5, 6}}, {Issuer: &iss}})
	step(true, Follow{Issuer: &iss}, []Follow{{Series: &id, Pools: []uint64{0}, Mints: []uint64{5, 6}}, {Issuer: &iss}})
	step(false, Follow{Series: &id, Mints: []uint64{5}}, []Follow{{Issuer: &iss}, {Series: &id, Pools: []uint64{0}, Mints: []uint64{6}}})
	step(true, Follow{Series: &id}, []Follow{{Issuer: &iss}, {Series: &id}})
	step(true, Follow{Series: &id, Pools: []uint64{1}}, []Follow{{Issuer: &iss}, {Series: &id}})
	// Taking a pool off a whole series writes the rest out.
	step(false, Follow{Series: &id, Pools: []uint64{0}}, []Follow{{Issuer: &iss}, {Series: &id, Pools: []uint64{1}}})
	if err := p.RemoveFollow(Follow{Series: &id, Mints: []uint64{5}}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("mint inside a followed pool: %v", err)
	}
	step(false, Follow{Issuer: &iss}, []Follow{{Series: &id, Pools: []uint64{1}}})
	step(false, Follow{Series: &id, Pools: []uint64{1}}, nil)

	other := hash.Hash{9}
	for _, bad := range []Follow{{}, {Series: &other}, {Series: &id, Pools: []uint64{2}}, {Series: &id, Mints: []uint64{8}}, {Issuer: &iss, Pools: []uint64{0}}, {Issuer: &iss, Series: &id}} {
		if err := p.AddFollow(bad); !errors.Is(err, ErrInvalid) {
			t.Errorf("%+v: %v", bad, err)
		}
	}
}
