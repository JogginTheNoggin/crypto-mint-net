package watch

import (
	"reflect"
	"testing"
)

func TestTruncateHistoryKeepsWholeEntries(t *testing.T) {
	it := func(pool uint64, h int) HistoryItem { return HistoryItem{Pool: pool, Height: h} }
	// Pool 0 at heights 9, 9, 8; pool 1 at 5, 4; newest first by weight.
	hist := []HistoryItem{it(0, 9), it(0, 9), it(1, 5), it(0, 8), it(1, 4)}
	next := []PoolNext{{Pool: 0, Below: 7}, {Pool: 2, Below: 3}}
	got, gotNext := truncateHistory(hist, next, 3)
	if !reflect.DeepEqual(got, hist[:3]) {
		t.Fatalf("kept %+v", got)
	}
	want := []PoolNext{{Pool: 0, Below: 8}, {Pool: 1, Below: 4}, {Pool: 2, Below: 3}}
	if !reflect.DeepEqual(gotNext, want) {
		t.Fatalf("next %+v, want %+v", gotNext, want)
	}
	// An entry larger than limit is kept whole rather than split, so a
	// page always makes progress.
	got, gotNext = truncateHistory(hist, nil, 1)
	if len(got) != 2 || !reflect.DeepEqual(gotNext, []PoolNext{{Pool: 0, Below: 8}, {Pool: 1, Below: 5}}) {
		t.Fatalf("kept %+v next %+v", got, gotNext)
	}
}
