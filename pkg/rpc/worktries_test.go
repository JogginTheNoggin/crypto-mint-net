package rpc_test

import (
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/rpc"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/testutil"
)

// poo work's limit and default follow the series' minimum layer strength,
// so a high floor still finds a layer that counts (D168).
func TestWorkTriesFollowTheFloor(t *testing.T) {
	for _, c := range []struct {
		floor       uint16
		most, deflt uint64
	}{
		{0, 1 << 24, 1 << 16},
		{14, 1 << 24, 1 << 16},
		{20, 1 << 24, 1 << 22},
		{24, 1 << 28, 1 << 26},
		{30, 1 << 34, 1 << 32},
		{40, 1 << 40, 1 << 40},
		{60, 1 << 40, 1 << 40},
	} {
		p := testutil.ExampleParams()
		p.MinLayerStrength = c.floor
		s := testutil.MustSeries(p)
		if got := rpc.WorkTriesFor(s); got != c.most {
			t.Errorf("floor %d: limit %d, want %d", c.floor, got, c.most)
		}
		if got := rpc.DefaultWorkTries(s); got != c.deflt {
			t.Errorf("floor %d: default %d, want %d", c.floor, got, c.deflt)
		}
	}
	if rpc.WorkTriesFor(nil) != rpc.MaxWorkTries || rpc.DefaultWorkTries(nil) != 1<<16 {
		t.Fatal("no series: not the plain limits")
	}
}
