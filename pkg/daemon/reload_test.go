package daemon_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/daemon"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/nodepolicy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/rpc"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/testutil"
)

// A node told to reload onto a policy with a mistake keeps running on the
// one it has, whether the reload comes from a signal or the interface,
// and reloads once the file is right (GREEN-011).
func TestBadPolicyReloadKeepsRunning(t *testing.T) {
	s := testutil.ExampleSeries()
	id := s.ID()
	p := nodepolicy.Default()
	p.Series = []nodepolicy.SeriesInfo{{Params: s.Params()}}
	p.Follow = []nodepolicy.Follow{{Series: &id, Pools: []uint64{1}}}
	p.RPC.Listen = "127.0.0.1:0"
	p.P2P.Listen = "off"
	dir := t.TempDir()
	path := filepath.Join(dir, nodepolicy.FileName)
	must(t, nodepolicy.Save(path, p))
	good, err := os.ReadFile(path)
	must(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ready := make(chan string, 4)
	hup := make(chan struct{})
	done := make(chan error, 1)
	var log logBuf
	go func() {
		done <- daemon.Run(ctx, dir, daemon.Options{Log: &log, Reload: hup, Ready: func(a string) { ready <- a }})
	}()
	wait := func() {
		t.Helper()
		select {
		case <-ready:
		case err := <-done:
			t.Fatalf("node stopped: %v\n%s", err, log.String())
		case <-time.After(10 * time.Second):
			t.Fatalf("node not up\n%s", log.String())
		}
	}
	wait()

	bad := strings.Replace(string(good), `"format": 1`, `"format": 1, "log": {"level": "loud"}`, 1)
	if bad == string(good) {
		t.Fatalf("could not spoil the policy:\n%s", good)
	}
	must(t, os.WriteFile(path, []byte(bad), 0o600))

	c, err := rpc.Dial(dir)
	must(t, err)
	if err := c.Reload(); err == nil || !strings.Contains(err.Error(), "keeps its current policy") {
		t.Fatalf("interface reload of a bad policy: %v", err)
	}
	hup <- struct{}{}
	deadline := time.Now().Add(5 * time.Second)
	for !strings.Contains(log.String(), "reload refused") {
		if time.Now().After(deadline) {
			t.Fatalf("no refusal logged:\n%s", log.String())
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := c.Status(); err != nil {
		t.Fatalf("node not serving after refused reloads: %v\n%s", err, log.String())
	}

	must(t, os.WriteFile(path, good, 0o600))
	hup <- struct{}{}
	wait() // up again on the corrected policy
	cancel()
	must(t, <-done)
}
