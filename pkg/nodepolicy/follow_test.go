package nodepolicy

import (
	"errors"
	"reflect"
	"strings"
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

// GUI-018 (D139): ReplaceFollow puts the given pools and mints in place
// of the series' rule in one change, and leaves other rules alone.
func TestReplaceFollow(t *testing.T) {
	s := testutil.ExampleSeries()
	id := s.ID()
	iss := hash.Hash{7}
	p := Default()
	p.Series = []SeriesInfo{{Params: s.Params()}}
	p.Follow = []Follow{{Series: &id}, {Issuer: &iss}}
	if err := p.ReplaceFollow(Follow{Series: &id, Pools: []uint64{1, 1}}); err != nil {
		t.Fatal(err)
	}
	if want := []Follow{{Issuer: &iss}, {Series: &id, Pools: []uint64{1}}}; !reflect.DeepEqual(p.Follow, want) {
		t.Fatalf("follow %+v, want %+v", p.Follow, want)
	}
	if err := p.Validate(); err != nil {
		t.Fatal(err)
	}
	other := hash.Hash{9}
	for _, bad := range []Follow{{}, {Issuer: &iss}, {Series: &other}, {Series: &id, Pools: []uint64{2}}} {
		if err := p.ReplaceFollow(bad); !errors.Is(err, ErrInvalid) {
			t.Errorf("%+v: %v", bad, err)
		}
	}
	if want := []Follow{{Issuer: &iss}, {Series: &id, Pools: []uint64{1}}}; !reflect.DeepEqual(p.Follow, want) {
		t.Fatalf("a refused replace changed the list: %+v", p.Follow)
	}
}

// RED-044, D150: addFollows, the linear merge an import uses, follows
// exactly what AddFollow one by one does.
func TestAddFollowsMatchesAddFollow(t *testing.T) {
	var p Policy
	var ids []hash.Hash
	for i := 0; i < 3; i++ {
		prm := testutil.ExampleParams()
		prm.Mints = uint64(8 + 4*i)
		s := testutil.MustSeries(prm)
		p.Series = append(p.Series, SeriesInfo{Params: prm})
		ids = append(ids, s.ID())
	}
	iss := hash.Hash{7}
	cases := [][]Follow{
		{{Series: &ids[0], Pools: []uint64{0}}, {Series: &ids[0], Pools: []uint64{1}, Mints: []uint64{7}}},
		{{Series: &ids[1], Pools: []uint64{0}}, {Series: &ids[1]}, {Series: &ids[1], Pools: []uint64{1}}},
		{{Issuer: &iss}, {Issuer: &iss}, {Series: &ids[2], Mints: []uint64{3}}},
		{{Series: &ids[0]}, {Series: &ids[2], Pools: []uint64{1}}, {Series: &ids[0], Mints: []uint64{1}}},
	}
	starts := [][]Follow{nil, {{Series: &ids[0], Pools: []uint64{2}}}, {{Series: &ids[1]}, {Issuer: &iss}}}
	for _, start := range starts {
		for i, fs := range cases {
			a, b := p, p
			a.Follow = append([]Follow(nil), start...)
			b.Follow = append([]Follow(nil), start...)
			for _, f := range fs {
				if err := a.AddFollow(f); err != nil {
					t.Fatal(err)
				}
			}
			if err := b.addFollows(fs, p.series); err != nil {
				t.Fatal(err)
			}
			if x, y := followSet(a), followSet(b); x != y {
				t.Errorf("case %d from %v: AddFollow %s, addFollows %s", i, start, x, y)
			}
		}
	}
	bad := hash.Hash{9}
	b := p
	if err := b.addFollows([]Follow{{Series: &ids[0]}, {Series: &bad}}, p.series); err == nil || b.Follow != nil {
		t.Fatalf("an unlisted series: %v, follow %v", err, b.Follow)
	}
}

func followSet(p Policy) string {
	flat := flatAll(p)
	var out []string
	for _, k := range sortedKeys(flat) {
		if strings.HasPrefix(k, "follow") {
			out = append(out, show(k)+"="+flat[k])
		}
	}
	return strings.Join(out, " ")
}
