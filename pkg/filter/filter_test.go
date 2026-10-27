package filter

import (
	"flag"
	"io"
	"reflect"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/policy"
)

func TestMatch(t *testing.T) {
	h, _ := hash.Lookup(hash.SHA256)
	pol := policy.SingleKey(policy.AlgEd25519, make([]byte, 32))
	addr := policy.Address(h, pol)
	s1, s2 := hash.Hash{1}, hash.Hash{2}
	sub := Subject{Series: s1, Hash: h, Pool: 3, HasPool: true, Serials: []uint64{12}, Addresses: []hash.Hash{addr}, TIDs: []hash.Hash{{9}}, Kind: KindTx}
	cases := []struct {
		f    Filter
		want bool
	}{
		{Filter{}, true},
		{Filter{Series: []hash.Hash{s2}}, false},
		{Filter{Series: []hash.Hash{s2, s1}}, true},
		{Filter{Pools: []uint64{3}, Serials: []uint64{11, 12}}, true},
		{Filter{Pools: []uint64{3}, Serials: []uint64{11}}, false},
		{Filter{Addresses: []hash.Hash{{7}}}, false},
		{Filter{Policies: []policy.Policy{pol}}, true},
		{Filter{Addresses: []hash.Hash{{7}}, Policies: []policy.Policy{pol}}, true},
		{Filter{TIDs: []hash.Hash{{9}}, Kinds: []Kind{KindSelected}}, false},
		{Filter{TIDs: []hash.Hash{{9}}, Kinds: []Kind{KindSelected, KindTx}}, true},
	}
	for i, c := range cases {
		if got := c.f.Match(sub); got != c.want {
			t.Errorf("case %d: %v", i, got)
		}
	}
	// No hash: policies cannot match. No pool: pool filters cannot.
	if (Filter{Policies: []policy.Policy{pol}}).Match(Subject{Addresses: []hash.Hash{addr}}) {
		t.Error("policy matched without a hash")
	}
	if (Filter{Pools: []uint64{0}}).Match(Subject{}) {
		t.Error("pool matched without a pool")
	}
	if !(Filter{Serials: []uint64{5}}).MatchScope(s1, 1, 4, 7) || (Filter{Serials: []uint64{8}}).MatchScope(s1, 1, 4, 7) {
		t.Error("scope")
	}
}

func TestValuesAndFlags(t *testing.T) {
	pol := policy.SingleKey(policy.AlgEd25519, make([]byte, 32))
	f := Filter{
		Series: []hash.Hash{{1}}, Pools: []uint64{2, 1}, Serials: []uint64{9},
		Addresses: []hash.Hash{{3}}, Policies: []policy.Policy{pol}, TIDs: []hash.Hash{{4}}, Kinds: []Kind{KindTx, KindSwitch},
	}
	got, err := FromValues(f.Values())
	if err != nil {
		t.Fatal(err)
	}
	f.Pools = []uint64{1, 2}
	if !reflect.DeepEqual(got, f) {
		t.Fatalf("round trip\n got %+v\nwant %+v", got, f)
	}
	fs := flag.NewFlagSet("x", flag.ContinueOnError)
	read := Flags(fs)
	if err := fs.Parse([]string{"-pool", "2,1", "-pool", "1", "-kind", "tx", "-series", hash.Hash{1}.String()}); err != nil {
		t.Fatal(err)
	}
	ff, err := read()
	if err != nil || len(ff.Pools) != 2 || ff.Pools[0] != 1 || len(ff.Kinds) != 1 || len(ff.Series) != 1 {
		t.Fatalf("%v %+v", err, ff)
	}
	for _, bad := range [][]string{{"-kind", "nope"}, {"-pool", "x"}, {"-series", "zz"}, {"-policy", "00"}} {
		fs := flag.NewFlagSet("x", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		Flags(fs)
		if err := fs.Parse(bad); err == nil {
			t.Errorf("%v accepted", bad)
		}
	}
	if !(Filter{}).Empty() || f.Empty() {
		t.Error("Empty")
	}
}
