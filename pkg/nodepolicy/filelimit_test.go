package nodepolicy

import (
	"runtime"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/series"
)

func TestCheckFileLimit(t *testing.T) {
	f := CheckFileLimit(0)
	if runtime.GOOS == "windows" || runtime.GOOS == "plan9" || runtime.GOOS == "js" {
		if f.Known {
			t.Fatalf("%+v on %s", f, runtime.GOOS)
		}
		return
	}
	if !f.Known || f.Need != FileRoom || f.Low != (f.Soft < FileRoom) {
		t.Fatalf("%+v", f)
	}
	if g := CheckFileLimit(f.Soft); !g.Low || g.String() == "" {
		t.Fatalf("a limit below the need is not low: %+v", g)
	}
}

func TestFollowedPools(t *testing.T) {
	s, err := series.New(series.Reserve())
	if err != nil {
		t.Fatal(err)
	}
	n := s.PoolCount()
	id := s.ID()
	acc := []Accepted{{Series: s}}
	other := hash.Hash{1}
	for _, c := range []struct {
		name   string
		follow []Follow
		want   uint64
	}{
		{"whole", []Follow{{Series: &id}}, n},
		{"except", []Follow{{Series: &id, Except: []uint64{1, 2}}}, n - 2},
		{"pools and mints", []Follow{{Series: &id, Pools: []uint64{1, 2}, Mints: []uint64{7}}}, 3},
		{"whole and pools", []Follow{{Series: &id, Pools: []uint64{1}}, {Series: &id}}, n},
		{"not accepted", []Follow{{Series: &other}}, 0},
	} {
		p := Policy{Follow: c.follow}
		if got := p.FollowedPools(acc); got != c.want {
			t.Errorf("%s: %d, want %d", c.name, got, c.want)
		}
	}
}
