package rpc_test

import (
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/node"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/nodepolicy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/rpc"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/testutil"
)

// RED-081, D169: a work request with more tries than MaxWorkTries and no
// for stops at the time bound, and says so.
func TestLargeWorkStopsOnTime(t *testing.T) {
	defer rpc.SetWorkTimeBound(300 * time.Millisecond)()
	dir := t.TempDir()
	params := testutil.ExampleParams()
	params.MinLayerStrength = 40
	s := testutil.MustSeries(params)
	id := s.ID()
	p := nodepolicy.Default()
	p.Series = []nodepolicy.SeriesInfo{{Params: params}}
	p.Follow = []nodepolicy.Follow{{Series: &id, Pools: []uint64{1}}}
	p.RPC.Listen = "off"
	if err := nodepolicy.Save(filepath.Join(dir, nodepolicy.FileName), p); err != nil {
		t.Fatal(err)
	}
	n, err := node.Open(dir, node.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	tok, err := rpc.LoadOrCreateToken(dir)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(rpc.NewHandler(n, tok))
	defer srv.Close()
	c := rpc.NewClient(strings.TrimPrefix(srv.URL, "http://"), tok)
	f := &fixture{t: t, dir: dir, s: s, k: testutil.NewKeys(s), n: n, pool: rpc.PoolRef{Series: id, Pool: 1}}
	if _, err := c.SubmitTx(f.k.Claim(4, P("alice", 100))); err != nil {
		t.Fatal(err)
	}
	b, err := c.Build(f.pool, 0)
	if err != nil {
		t.Fatal(err)
	}
	if rpc.WorkTriesFor(s) <= rpc.MaxWorkTries {
		t.Fatal("the floor allows no more tries")
	}
	t0 := time.Now()
	w, err := c.Work(f.pool, b.Hash, 0, 1<<34)
	if took := time.Since(t0); took > 10*time.Second {
		t.Fatalf("took %v", took)
	}
	switch {
	case err != nil && strings.Contains(err.Error(), "stopped after"):
	case err == nil && w.StoppedOnTime:
	default:
		t.Fatalf("not stopped on time: %+v, %v", w, err)
	}
}
