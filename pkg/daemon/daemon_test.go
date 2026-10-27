package daemon_test

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/daemon"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/filter"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/nodepolicy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/rpc"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/testutil"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/watch"
)

type logBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *logBuf) Write(p []byte) (int, error) { l.mu.Lock(); defer l.mu.Unlock(); return l.b.Write(p) }
func (l *logBuf) String() string              { l.mu.Lock(); defer l.mu.Unlock(); return l.b.String() }

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

// A node follows pool 1, is told to follow pool 0 too, and applies it
// without stopping; a watcher sees a transaction, then the stream ends for
// the reload.
func TestFollowAppliesLive(t *testing.T) {
	s := testutil.ExampleSeries()
	id := s.ID()
	p := nodepolicy.Default()
	p.Series = []nodepolicy.SeriesInfo{{Params: s.Params()}}
	p.Follow = []nodepolicy.Follow{{Series: &id, Pools: []uint64{1}}}
	p.RPC.Listen = "127.0.0.1:0"
	p.P2P.Listen = "127.0.0.1:0"
	dir := t.TempDir()
	must(t, nodepolicy.Save(filepath.Join(dir, nodepolicy.FileName), p))

	ctx, cancel := context.WithCancel(context.Background())
	ready := make(chan string, 4)
	done := make(chan error, 1)
	var log logBuf
	go func() { done <- daemon.Run(ctx, dir, daemon.Options{Log: &log, Ready: func(a string) { ready <- a }}) }()
	up := func() *rpc.Client {
		t.Helper()
		select {
		case <-ready:
		case err := <-done:
			t.Fatalf("node stopped: %v\n%s", err, log.String())
		case <-time.After(10 * time.Second):
			t.Fatalf("node not up\n%s", log.String())
		}
		c, err := rpc.Dial(dir)
		must(t, err)
		return c
	}
	c := up()
	fr, err := c.Follows()
	must(t, err)
	if len(fr.Pools) != 1 || fr.Pools[0].Pool != 1 {
		t.Fatalf("follows %+v", fr)
	}

	// Watch transactions of mint 5.
	var b filter.Builder
	must(t, b.Add("serial", "5"))
	events := make(chan watch.Event, 16)
	werr := make(chan error, 1)
	go func() {
		werr <- c.Watch(ctx, b.Filter(), func(ev watch.Event) error { events <- ev; return nil })
	}()
	if ev := <-events; ev.Kind != "hello" {
		t.Fatalf("first line %+v", ev)
	}
	k := testutil.NewKeys(s)
	_, err = c.SubmitTx(k.Claim(4, testutil.P("a", 100)))
	must(t, err)
	c5 := k.Claim(5, testutil.P("b", 100))
	_, err = c.SubmitTx(c5)
	must(t, err)
	if ev := <-events; ev.Kind != filter.KindTx || *ev.TID != c5.TID(s.Hash()) {
		t.Fatalf("event %+v", ev)
	}

	// Follow pool 0 as well; the node applies it and the stream ends.
	fr, err = c.Follow(rpc.FollowRequest{Series: &id, Pools: []uint64{0}})
	must(t, err)
	if !fr.Reloading || len(fr.Follow) != 1 || len(fr.Follow[0].Pools) != 2 {
		t.Fatalf("follow reply %+v", fr)
	}
	for ev := range events {
		if ev.Kind == "end" {
			break
		}
	}
	if err := <-werr; err != nil {
		t.Fatalf("watch: %v", err)
	}
	c = up()
	pools, err := c.Pools(filter.Filter{})
	must(t, err)
	if len(pools) != 2 {
		t.Fatalf("pools after reload %+v", pools)
	}
	// What was pending in pool 1 came back from its store.
	pend, err := c.Pending(filter.Filter{})
	must(t, err)
	if len(pend) != 2 {
		t.Fatalf("pending after reload %+v", pend)
	}
	// A bad change is refused and changes nothing.
	_, err = c.Follow(rpc.FollowRequest{Series: &id, Pools: []uint64{9}})
	var re *rpc.Error
	if !errors.As(err, &re) || re.Code != rpc.CodeInvalid {
		t.Fatalf("bad follow: %v", err)
	}
	must(t, c.Reload())
	c = up()
	if _, err := c.Follow(rpc.FollowRequest{Remove: true, Series: &id, Pools: []uint64{0}}); err != nil {
		t.Fatal(err)
	}
	c = up()
	if pools, _ := c.Pools(filter.Filter{}); len(pools) != 1 || pools[0].Pool != 1 {
		t.Fatalf("pools after unfollow %+v", pools)
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("run: %v\n%s", err, log.String())
	}
}
