package rpc_test

import (
	"context"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/nodepolicy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/p2p"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/peers"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/rpc"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tracking"
)

// poo policy limits -set applies the peer book's limits, the tracker's
// retention and the seen list's size to the running node with no reload,
// and leaves every part as a reload would (D168): the book and the
// tracker hold the new bounds, the seen list its new size, and the
// selected tips and followed pools are those of before. Any other limit,
// or anything else changed in node.json since the node started, still
// takes the reload.
func TestLimitsSetInPlace(t *testing.T) {
	f := newFixture(t)
	book, err := peers.Open(filepath.Join(f.dir, peers.FileName), f.n.Policy().BookLimits())
	if err != nil {
		t.Fatal(err)
	}
	tr, err := tracking.New(f.n.Policy().TrackingConfig(), "")
	if err != nil {
		t.Fatal(err)
	}
	host, err := p2p.New(f.n, p2p.Config{Listen: "127.0.0.1:0", CheckEvery: -1, Book: book, Tracker: tr})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	ready, done := make(chan struct{}), make(chan error, 1)
	go func() { done <- host.Run(ctx, func() { close(ready) }) }()
	<-ready
	defer func() { cancel(); <-done }()
	tok, _ := rpc.LoadOrCreateToken(f.dir)
	h := rpc.NewHandler(f.n, tok)
	h.SetHost(host)
	var reloads atomic.Int32
	h.SetReload(func() { reloads.Add(1) })
	srv := httptest.NewServer(h)
	defer srv.Close()
	c := rpc.NewClient(strings.TrimPrefix(srv.URL, "http://"), tok)

	before, err := c.Status()
	if err != nil {
		t.Fatal(err)
	}
	followed := f.n.Engine().FollowedCount()
	for _, set := range []map[string]int{
		{"p2p.max_book": 256, "p2p.book_per_range": 1}, // the floors
		{"p2p.max_book": 65536, "p2p.book_per_range": 4096, "p2p.retry_seconds": 86400, "p2p.candidate_seconds": 365 * 86400}, // the ceilings
		{"tracking.max_addresses": 1024, "tracking.flag_history_seconds": 3600, "tracking.log_seconds": 60, "tracking.log_size": 16},
		{"tracking.max_addresses": 131072, "tracking.log_size": 4096},
		{"seen_size": 1024},
		{"seen_size": 1 << 22},
		{"p2p.max_book": 0, "p2p.book_per_range": 0, "seen_size": 0, "tracking.log_size": 0}, // back to the defaults
	} {
		rep, err := c.SetLimits(rpc.LimitsRequest{Set: set})
		if err != nil {
			t.Fatal(err)
		}
		if !rep.Saved || !rep.InPlace || rep.Reloading || reloads.Load() != 0 {
			t.Fatalf("%v: %+v, %d reloads", set, rep, reloads.Load())
		}
		p, err := nodepolicy.Load(filepath.Join(f.dir, nodepolicy.FileName))
		if err != nil {
			t.Fatal(err)
		}
		if book.Limits() != p.BookLimits() || tr.Retention() != p.Retention() ||
			f.n.Engine().SeenSize() != nodepolicy.BoundSeenSize.Or(p.SeenSize) {
			t.Fatalf("%v: book %+v (want %+v), tracker %+v (want %+v), seen %d", set,
				book.Limits(), p.BookLimits(), tr.Retention(), p.Retention(), f.n.Engine().SeenSize())
		}
		after, err := c.Status()
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(after.Pools, before.Pools) || f.n.Engine().FollowedCount() != followed || after.PolicyChanged {
			t.Fatalf("%v: the node changed: %+v, %d followed", set, after, f.n.Engine().FollowedCount())
		}
	}

	// ties, max_pending, verified_per_range and registry_seconds stay on
	// the reload.
	for i, set := range []map[string]int{{"ties": 8}, {"max_pending": 64}, {"p2p.verified_per_range": 2}, {"p2p.registry_seconds": 120}} {
		rep, err := c.SetLimits(rpc.LimitsRequest{Set: set})
		if err != nil {
			t.Fatal(err)
		}
		if rep.InPlace || !rep.Reloading || reloads.Load() != int32(i+1) {
			t.Fatalf("%v: %+v, %d reloads", set, rep, reloads.Load())
		}
	}
	// Those wait for the reload (here never made), so even an in-place
	// limit now takes the reload too.
	rep, err := c.SetLimits(rpc.LimitsRequest{Set: map[string]int{"p2p.max_book": 512}})
	if err != nil {
		t.Fatal(err)
	}
	if rep.InPlace || !rep.Reloading || book.Limits().MaxRecords == 512 {
		t.Fatalf("with a change waiting for the reload: %+v, book %+v", rep, book.Limits())
	}
}
