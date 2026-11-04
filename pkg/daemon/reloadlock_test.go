package daemon_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/daemon"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/nodepolicy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/testutil"
)

// A command that takes the data directory's lock in the gap of a reload,
// as an offline poo policy limits -set can, no longer stops the node: it
// waits for the lock and comes back up (GREEN-050).
func TestReloadWaitsForTheLock(t *testing.T) {
	s := testutil.ExampleSeries()
	p := nodepolicy.Default()
	p.Series = []nodepolicy.SeriesInfo{{Params: s.Params()}}
	p.RPC.Listen = "127.0.0.1:0"
	p.P2P.Listen = "off"
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
	up := func(what string) {
		t.Helper()
		select {
		case <-ready:
		case err := <-done:
			t.Fatalf("%s: node stopped: %v\n%s", what, err, log.String())
		case <-time.After(15 * time.Second):
			t.Fatalf("%s: node not up\n%s", what, log.String())
		}
	}
	up("start")

	// In the reload's gap, a command takes the lock for 500 ms.
	took := make(chan error, 1)
	defer daemon.SetBeforeReopen(func(dir string) {
		unlock, err := nodepolicy.Lock(dir)
		took <- err
		if err == nil {
			time.AfterFunc(500*time.Millisecond, unlock)
		}
	})()
	reload <- struct{}{}
	select {
	case err := <-took:
		if err != nil {
			t.Fatalf("the reload kept its lock: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("no reload")
	}
	up("after the reload")
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("run: %v\n%s", err, log.String())
	}
}
