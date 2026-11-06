package daemon_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/daemon"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/nodepolicy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/rpc"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/testutil"
)

// BLUE-042 (RED-126): a reload keeps the node id, so peers' held offers,
// leases and records by id still name this node.
func TestBlue042ReloadKeepsNodeID(t *testing.T) {
	s := testutil.ExampleSeries()
	p := nodepolicy.Default()
	p.Series = []nodepolicy.SeriesInfo{{Params: s.Params()}}
	p.RPC.Listen = "127.0.0.1:0"
	p.P2P.Listen = "127.0.0.1:0"
	dir := t.TempDir()
	must(t, nodepolicy.Save(filepath.Join(dir, nodepolicy.FileName), p))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ready := make(chan string, 4)
	reload := make(chan struct{}, 1)
	done := make(chan error, 1)
	var log logBuf
	go func() {
		done <- daemon.Run(ctx, dir, daemon.Options{Log: &log, Ready: func(a string) { ready <- a }, Reload: reload})
	}()
	id := func(what string) string {
		t.Helper()
		select {
		case <-ready:
		case err := <-done:
			t.Fatalf("%s: node stopped: %v\n%s", what, err, log.String())
		case <-time.After(15 * time.Second):
			t.Fatalf("%s: node not up\n%s", what, log.String())
		}
		c, err := rpc.Dial(dir)
		if err != nil {
			t.Fatal(err)
		}
		r, err := c.Peers()
		if err != nil {
			t.Fatal(err)
		}
		return r.NodeID
	}
	before := id("start")
	reload <- struct{}{}
	if after := id("after the reload"); after != before || before == "" {
		t.Fatalf("node id %q before the reload, %q after", before, after)
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("run: %v\n%s", err, log.String())
	}
}
