package daemon_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/daemon"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/nodepolicy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/testutil"
)

// runLog starts a node with the policy's log section set to pl and the
// options' overrides, stops it once up, and returns its log.
func runLog(t *testing.T, pl *nodepolicy.Log, o daemon.Options) string {
	t.Helper()
	s := testutil.ExampleSeries()
	id := s.ID()
	p := nodepolicy.Default()
	p.Series = []nodepolicy.SeriesInfo{{Params: s.Params()}}
	p.Follow = []nodepolicy.Follow{{Series: &id, Pools: []uint64{1}}}
	p.RPC.Listen = "127.0.0.1:0"
	p.P2P.Listen = "127.0.0.1:0"
	p.Log = pl
	dir := t.TempDir()
	must(t, nodepolicy.Save(filepath.Join(dir, nodepolicy.FileName), p))
	ctx, cancel := context.WithCancel(context.Background())
	var log logBuf
	ready := make(chan string, 1)
	o.Log, o.Ready = &log, func(a string) { ready <- a }
	done := make(chan error, 1)
	go func() { done <- daemon.Run(ctx, dir, o) }()
	select {
	case <-ready:
	case err := <-done:
		t.Fatalf("node stopped: %v\n%s", err, log.String())
	case <-time.After(10 * time.Second):
		t.Fatalf("node not up\n%s", log.String())
	}
	cancel()
	must(t, <-done)
	return log.String()
}

func TestLogPlainByDefault(t *testing.T) {
	out := runLog(t, nil, daemon.Options{})
	for _, want := range []string{"node: started dir=", "p2p: listening addr=127.0.0.1:", "rpc: listening addr=127.0.0.1:", "node: stopped\n"} {
		if !strings.Contains(out, want) {
			t.Errorf("log lacks %q:\n%s", want, out)
		}
	}
}

func TestLogJSONFromPolicy(t *testing.T) {
	out := runLog(t, &nodepolicy.Log{Format: "json"}, daemon.Options{})
	comps := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("not JSON: %q", line)
		}
		comps[m["component"].(string)] = true
	}
	if !comps["node"] || !comps["p2p"] || !comps["rpc"] {
		t.Fatalf("components %v:\n%s", comps, out)
	}
}

func TestLogLevelOverride(t *testing.T) {
	// The policy says debug; the run says warn, so the info lines go.
	out := runLog(t, &nodepolicy.Log{Level: "debug"}, daemon.Options{LogLevel: "warn"})
	if strings.Contains(out, "listening") {
		t.Fatalf("info lines at warn:\n%s", out)
	}
}
