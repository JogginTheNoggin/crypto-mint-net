package cli

import (
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/rpc"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/series"
)

// GUI ORANGE-021: poon status -counts adds the pool rows up per series, in
// the order the series first appear, and keeps the notes.
func TestStatusBySeries(t *testing.T) {
	a, b := hash.Hash{1}, hash.Hash{2}
	st := rpc.StatusReply{PolicyChanged: true, Pools: []rpc.PoolStatus{
		{Series: a, Pool: 0, Entries: 3, Pending: 1, Stored: 5},
		{Series: b, Pool: 0, Entries: 1, Waiting: 2},
		{Series: a, Pool: 1, Entries: 4, Branches: 1, Stored: 7},
	}}
	st.Bodies.Held = 9
	got := statusBySeries(st)
	want := []seriesStatus{
		{Series: a, Pools: 2, Entries: 7, Branches: 1, Pending: 1, Stored: 12},
		{Series: b, Pools: 1, Entries: 1, Waiting: 2},
	}
	if len(got.Series) != 2 || got.Series[0] != want[0] || got.Series[1] != want[1] {
		t.Fatalf("series %+v, want %+v", got.Series, want)
	}
	if !got.PolicyChanged || got.Bodies.Held != 9 {
		t.Fatalf("notes not kept: %+v", got)
	}
	if empty := statusBySeries(rpc.StatusReply{}); empty.Series == nil {
		t.Fatal("no series prints null, want []")
	}
}

// GUI-GREEN-056 (D179): a row of the default series names it, as poon
// series followed does; another series' row has no builtin.
func TestStatusBySeriesBuiltin(t *testing.T) {
	reserve, err := hash.Parse(series.ReserveID)
	if err != nil {
		t.Fatal(err)
	}
	st := rpc.StatusReply{Pools: []rpc.PoolStatus{{Series: reserve}, {Series: hash.Hash{2}}}}
	got := statusBySeries(st).Series
	if len(got) != 2 || got[0].Builtin != "default" || got[1].Builtin != "" {
		t.Fatalf("rows %+v", got)
	}
}
